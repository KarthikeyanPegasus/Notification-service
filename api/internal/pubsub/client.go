package pubsub

import (
	"context"
	"encoding/json"
	"fmt"

	gcppubsub "cloud.google.com/go/pubsub"
	"google.golang.org/api/option"
	"github.com/spidey/notification-service/internal/config"
)

// TopicID maps channel names to Pub/Sub topic IDs.
var TopicID = map[string]string{
	"otp":       "notifications-otp",
	"email":     "notifications-email",
	"sms":       "notifications-sms",
	"push":      "notifications-push",
	"websocket": "notifications-websocket",
	"webhook":   "notifications-webhook",
	"dlq":       "notifications-dlq",
	"config":    "internal-config-reload",
}

// Message is the envelope published to a Pub/Sub topic.
type Message struct {
	NotificationID string            `json:"notification_id"`
	Channel        string            `json:"channel"`
	UserID         string            `json:"user_id"`
	Recipient      string            `json:"recipient"`
	Priority       string            `json:"priority"`
	Type           string            `json:"type"`
	TemplateID     string            `json:"template_id,omitempty"`
	Payload        map[string]string `json:"payload,omitempty"`
	IdempotencyKey string            `json:"idempotency_key"`
}

// Publisher sends messages to Pub/Sub topics.
type Publisher interface {
	Publish(ctx context.Context, channel string, msg *Message) (serverMsgID string, err error)
	Close() error
}

// Subscriber receives messages from a Pub/Sub subscription.
type Subscriber interface {
	Subscribe(ctx context.Context, subscription string, handler MessageHandler) error
	Close() error
}

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

func (s *GCPSubscriber) Close() error {
	return s.client.Close()
}
