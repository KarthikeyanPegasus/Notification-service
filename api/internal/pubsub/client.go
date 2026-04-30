package pubsub

import (
	"context"
	"encoding/json"
	"fmt"

	gcppubsub "cloud.google.com/go/pubsub"
	"google.golang.org/api/option"
	"github.com/spidey/notification-service/internal/config"
)

// TopicID maps channel+priority keys to Kafka topic names.
// Topics are segregated by priority: notifications-{channel}-{priority}.
// Clients use the REST API only — there is no pubsub ingress path.
// Workers consume from priority topics; the notification service publishes here.
var TopicID = map[string]string{
	"config": "internal-config-reload",
	"dlq":    "notifications-dlq",

	"email-high":   "notifications-email-high",
	"email-medium": "notifications-email-medium",
	"email-low":    "notifications-email-low",

	"sms-high":   "notifications-sms-high",
	"sms-medium": "notifications-sms-medium",
	"sms-low":    "notifications-sms-low",

	"push-high":   "notifications-push-high",
	"push-medium": "notifications-push-medium",
	"push-low":    "notifications-push-low",

	"webhook-high":   "notifications-webhook-high",
	"webhook-medium": "notifications-webhook-medium",
	"webhook-low":    "notifications-webhook-low",

	"slack-high":   "notifications-slack-high",
	"slack-medium": "notifications-slack-medium",
	"slack-low":    "notifications-slack-low",

	"websocket-high":   "notifications-websocket-high",
	"websocket-medium": "notifications-websocket-medium",
	"websocket-low":    "notifications-websocket-low",
}

// PriorityTopicKey returns the map key for publishing to a priority-specific topic.
// e.g. PriorityTopicKey("email", "high") → "email-high"
func PriorityTopicKey(channel, priority string) string {
	if priority == "" {
		priority = "low"
	}
	return channel + "-" + priority
}

// PriorityTopicName returns the Kafka/pubsub topic name for a channel+priority pair.
func PriorityTopicName(channel, priority string) string {
	key := PriorityTopicKey(channel, priority)
	if topic, ok := TopicID[key]; ok {
		return topic
	}
	return "notifications-" + channel + "-" + priority
}

// Message is the envelope published to a Kafka/pubsub topic.
type Message struct {
	NotificationID string            `json:"notification_id"`
	Channel        string            `json:"channel"`
	UserID         string            `json:"user_id"`
	// ClientID is the API key ID of the caller; used by the Dispatcher to route
	// the workflow to the correct client-scoped Temporal task queue.
	ClientID       string            `json:"client_id,omitempty"`
	Recipient      string            `json:"recipient"`
	Priority       string            `json:"priority"`
	Type           string            `json:"type"`
	TemplateID     string            `json:"template_id,omitempty"`
	Payload        map[string]string `json:"payload,omitempty"`
	IdempotencyKey string            `json:"idempotency_key"`
	ForcedVendor   string            `json:"forced_vendor,omitempty"`
}

// Publisher sends messages to Pub/Sub topics.
type Publisher interface {
	Publish(ctx context.Context, channel string, msg *Message) (serverMsgID string, err error)
	Close() error
}

// Subscriber receives messages from a Pub/Sub subscription.
type Subscriber interface {
	Subscribe(ctx context.Context, subscription string, handler MessageHandler) error
	SubscribeRaw(ctx context.Context, subscription string, handler RawMessageHandler) error
	Close() error
}

type RawMessageHandler func(ctx context.Context, data []byte) error

// MessageHandler processes an incoming Pub/Sub message.
// Returning nil acks the message; returning an error nacks it.
type MessageHandler func(ctx context.Context, msg *Message) error

func NewGCPPublisher(ctx context.Context, cfg config.PubSubConfig) (*GCPPublisher, error) {
	var opts []option.ClientOption
	if cfg.CredentialsFile != "" {
		opts = append(opts, option.WithCredentialsFile(cfg.CredentialsFile))
	}

	client, err := gcppubsub.NewClient(ctx, cfg.ProjectID, opts...)
	if err != nil {
		return nil, fmt.Errorf("creating pubsub client: %w", err)
	}
	return &GCPPublisher{client: client, projectID: cfg.ProjectID, topicOverride: cfg.TopicOverride}, nil
}

type GCPPublisher struct {
	client        *gcppubsub.Client
	projectID     string
	topicOverride string
}

func (p *GCPPublisher) Publish(ctx context.Context, channel string, msg *Message) (string, error) {
	topicID := p.topicOverride
	if topicID == "" {
		var ok bool
		topicID, ok = TopicID[channel]
		if !ok {
			return "", fmt.Errorf("unknown channel: %s", channel)
		}
	}

	data, err := json.Marshal(msg)
	if err != nil {
		return "", fmt.Errorf("marshalling message: %w", err)
	}

	topic := p.client.Topic(topicID)
	defer topic.Stop()

	result := topic.Publish(ctx, &gcppubsub.Message{
		Data: data,
		Attributes: map[string]string{
			"channel":  channel,
			"notifId":  msg.NotificationID,
			"priority": msg.Priority,
		},
		OrderingKey: msg.UserID, // per-user ordering
	})

	serverID, err := result.Get(ctx)
	if err != nil {
		return "", fmt.Errorf("publishing to topic %s: %w", topicID, err)
	}
	return serverID, nil
}

func (p *GCPPublisher) Close() error {
	return p.client.Close()
}

// GCPSubscriber wraps a Google Cloud Pub/Sub client for subscribing.
type GCPSubscriber struct {
	client               *gcppubsub.Client
	subscriptionOverride string
}

func NewGCPSubscriber(ctx context.Context, cfg config.PubSubConfig) (*GCPSubscriber, error) {
	var opts []option.ClientOption
	if cfg.CredentialsFile != "" {
		opts = append(opts, option.WithCredentialsFile(cfg.CredentialsFile))
	}

	client, err := gcppubsub.NewClient(ctx, cfg.ProjectID, opts...)
	if err != nil {
		return nil, fmt.Errorf("creating pubsub subscriber client: %w", err)
	}
	return &GCPSubscriber{client: client, subscriptionOverride: cfg.SubscriptionOverride}, nil
}

func (s *GCPSubscriber) Subscribe(ctx context.Context, subscription string, handler MessageHandler) error {
	subID := s.subscriptionOverride
	if subID == "" {
		subID = subscription
	}
	sub := s.client.Subscription(subID)
	sub.ReceiveSettings.MaxOutstandingMessages = 100
	sub.ReceiveSettings.NumGoroutines = 4

	return sub.Receive(ctx, func(ctx context.Context, m *gcppubsub.Message) {
		var msg Message
		if err := json.Unmarshal(m.Data, &msg); err != nil {
			m.Nack()
			return
		}
		if err := handler(ctx, &msg); err != nil {
			m.Nack()
			return
		}
		m.Ack()
	})
}

func (s *GCPSubscriber) SubscribeRaw(ctx context.Context, subscription string, handler RawMessageHandler) error {
	subID := s.subscriptionOverride
	if subID == "" {
		subID = subscription
	}
	sub := s.client.Subscription(subID)
	return sub.Receive(ctx, func(ctx context.Context, m *gcppubsub.Message) {
		if err := handler(ctx, m.Data); err != nil {
			m.Nack()
			return
		}
		m.Ack()
	})
}

func (s *GCPSubscriber) Close() error {
	return s.client.Close()
}
