package pubsub

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/segmentio/kafka-go"
	"github.com/spidey/notification-service/internal/config"
	"go.uber.org/zap"
)

// MaxRetryAttempts defines the maximum number of delivery attempts before sending to DLQ.
const MaxRetryAttempts = 5

// DLQTopic is the topic where messages are sent after max retries.
const DLQTopic = "notifications-dlq"

// KafkaPublisher publishes notifications to Kafka priority topics.
// Clients publish only to notifications-ingress; internal services publish to priority topics.
type KafkaPublisher struct {
	brokers []string
	writers map[string]*kafka.Writer // topic → writer
	log     *zap.Logger
}

func NewKafkaPublisher(cfg config.KafkaConfig, log *zap.Logger) *KafkaPublisher {
	return &KafkaPublisher{
		brokers: cfg.Brokers,
		writers: make(map[string]*kafka.Writer),
		log:     log,
	}
}

func (p *KafkaPublisher) writerFor(topic string) *kafka.Writer {
	if w, ok := p.writers[topic]; ok {
		return w
	}
	w := &kafka.Writer{
		Addr:         kafka.TCP(p.brokers...),
		Topic:        topic,
		Balancer:     &kafka.LeastBytes{},
		WriteTimeout: 10 * time.Second,
		RequiredAcks: kafka.RequireOne,
		Compression:  kafka.Snappy,
	}
	p.writers[topic] = w
	return w
}

func (p *KafkaPublisher) Publish(ctx context.Context, channel string, msg *Message) (string, error) {
	topicKey := channel
	// For channel-only keys (e.g. "email"), keep as-is for backwards compat with config topic.
	// For priority keys (e.g. "email-high"), resolve the full topic name.
	topic, ok := TopicID[topicKey]
	if !ok {
		// Fallback: notifications-{channel}
		topic = "notifications-" + strings.TrimPrefix(channel, "notifications-")
	}

	data, err := json.Marshal(msg)
	if err != nil {
		return "", fmt.Errorf("marshalling kafka message: %w", err)
	}

	km := kafka.Message{
		Key:   []byte(msg.UserID), // per-user ordering within a partition
		Value: data,
		Headers: []kafka.Header{
			{Key: "channel", Value: []byte(channel)},
			{Key: "priority", Value: []byte(msg.Priority)},
			{Key: "notif_id", Value: []byte(msg.NotificationID)},
		},
		Time: time.Now(),
	}

	w := p.writerFor(topic)
	if err := w.WriteMessages(ctx, km); err != nil {
		return "", fmt.Errorf("writing to kafka topic %s: %w", topic, err)
	}

	p.log.Debug("published to kafka",
		zap.String("topic", topic),
		zap.String("channel", channel),
		zap.String("notification_id", msg.NotificationID),
	)

	return fmt.Sprintf("%s:%d", topic, time.Now().UnixNano()), nil
}

func (p *KafkaPublisher) Close() error {
	var errs []string
	for topic, w := range p.writers {
		if err := w.Close(); err != nil {
			errs = append(errs, fmt.Sprintf("closing writer for %s: %v", topic, err))
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("closing kafka writers: %s", strings.Join(errs, "; "))
	}
	return nil
}

// PublishToDLQ sends a message to the dead-letter queue topic.
func (p *KafkaPublisher) PublishToDLQ(ctx context.Context, msg *Message, reason string) (string, error) {
	data, err := json.Marshal(msg)
	if err != nil {
		return "", fmt.Errorf("marshalling dlq message: %w", err)
	}

	km := kafka.Message{
		Key:   []byte(msg.NotificationID),
		Value: data,
		Headers: []kafka.Header{
			{Key: "channel", Value: []byte(msg.Channel)},
			{Key: "priority", Value: []byte(msg.Priority)},
			{Key: "notif_id", Value: []byte(msg.NotificationID)},
			{Key: "dlq_reason", Value: []byte(reason)},
		},
		Time: time.Now(),
	}

	w := p.writerFor(DLQTopic)
	if err := w.WriteMessages(ctx, km); err != nil {
		return "", fmt.Errorf("writing to DLQ topic %s: %w", DLQTopic, err)
	}

	p.log.Info("message sent to DLQ",
		zap.String("topic", DLQTopic),
		zap.String("notification_id", msg.NotificationID),
		zap.String("reason", reason),
		zap.Int("attempt_count", msg.AttemptCount),
	)

	return fmt.Sprintf("%s:%d", DLQTopic, time.Now().UnixNano()), nil
}

// KafkaSubscriber consumes messages from Kafka priority topics.
// Each worker instance uses a unique consumer group ID encoding channel × priority × vendor × clientID,
// so workers for different priorities/clients don't share partitions.
type KafkaSubscriber struct {
	brokers         []string
	consumerGroupID string
	log             *zap.Logger
}

func NewKafkaSubscriber(cfg config.KafkaConfig, log *zap.Logger) *KafkaSubscriber {
	groupID := cfg.ConsumerGroupID
	if groupID == "" {
		groupID = "notification-service"
	}
	return &KafkaSubscriber{
		brokers:         cfg.Brokers,
		consumerGroupID: groupID,
		log:             log,
	}
}

// groupIDFor builds a consumer group ID scoped to the subscription name so that
// different channel × priority × worker combinations don't share partitions.
func (s *KafkaSubscriber) groupIDFor(subscription string) string {
	return s.consumerGroupID + "-" + subscription
}

func (s *KafkaSubscriber) Subscribe(ctx context.Context, subscription string, handler MessageHandler) error {
	topic, ok := TopicID[subscription]
	if !ok {
		topic = "notifications-" + subscription
	}

	r := kafka.NewReader(kafka.ReaderConfig{
		Brokers:        s.brokers,
		Topic:          topic,
		GroupID:        s.groupIDFor(subscription),
		MinBytes:       1,
		MaxBytes:       10e6,
		CommitInterval: time.Second,
		StartOffset:    kafka.FirstOffset,
	})
	defer r.Close()

	s.log.Info("kafka subscriber started",
		zap.String("topic", topic),
		zap.String("group_id", s.groupIDFor(subscription)),
	)

	for {
		km, err := r.FetchMessage(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return nil // clean shutdown
			}
			s.log.Error("kafka fetch error", zap.String("topic", topic), zap.Error(err))
			continue
		}

		var msg Message
		if err := json.Unmarshal(km.Value, &msg); err != nil {
			s.log.Error("kafka unmarshal error", zap.Error(err))
			_ = r.CommitMessages(ctx, km) // ack malformed messages
			continue
		}

		if err := handler(ctx, &msg); err != nil {
			s.log.Warn("message handler returned error — will not commit (nack)",
				zap.String("topic", topic),
				zap.String("offset", fmt.Sprintf("%d", km.Offset)),
				zap.Error(err),
			)
			// In Kafka, not committing effectively "nacks" — the message will be redelivered.
			// To avoid infinite loops on poison pills, implement a DLQ or max-retry counter
			// at the handler level.
			continue
		}

		if err := r.CommitMessages(ctx, km); err != nil {
			s.log.Warn("kafka commit error", zap.Error(err))
		}
	}
}

func (s *KafkaSubscriber) SubscribeRaw(ctx context.Context, subscription string, handler RawMessageHandler) error {
	topic, ok := TopicID[subscription]
	if !ok {
		topic = "notifications-" + subscription
	}

	r := kafka.NewReader(kafka.ReaderConfig{
		Brokers:        s.brokers,
		Topic:          topic,
		GroupID:        s.groupIDFor(subscription),
		MinBytes:       1,
		MaxBytes:       10e6,
		CommitInterval: time.Second,
		StartOffset:    kafka.FirstOffset,
	})
	defer r.Close()

	s.log.Info("kafka raw subscriber started",
		zap.String("topic", topic),
		zap.String("group_id", s.groupIDFor(subscription)),
	)

	for {
		km, err := r.FetchMessage(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			s.log.Error("kafka raw fetch error", zap.String("topic", topic), zap.Error(err))
			continue
		}

		if err := handler(ctx, km.Value); err != nil {
			s.log.Warn("raw message handler error — skipping commit",
				zap.String("topic", topic),
				zap.Error(err),
			)
			continue
		}

		if err := r.CommitMessages(ctx, km); err != nil {
			s.log.Warn("kafka commit error", zap.Error(err))
		}
	}
}

func (s *KafkaSubscriber) Close() error { return nil }

// KafkaSubscriberFactory creates KafkaSubscriber instances with explicit consumer group IDs.
// Used by the Dispatcher to create one subscriber per channel × priority topic,
// where the consumer group ID is: notif-dispatcher-{channel}-{priority}.
// This isolates Kafka partition assignments so each dispatcher reads its own topic independently.
type KafkaSubscriberFactory struct {
	brokers []string
	log     *zap.Logger
}

func NewKafkaSubscriberFactory(cfg config.KafkaConfig, log *zap.Logger) *KafkaSubscriberFactory {
	return &KafkaSubscriberFactory{
		brokers: cfg.Brokers,
		log:     log,
	}
}

// NewForTopic creates a dedicated KafkaSubscriber for a specific channel+priority topic.
// groupID should uniquely identify this subscriber (e.g. "notif-dispatcher-email-high").
func (f *KafkaSubscriberFactory) NewForTopic(groupID string) *KafkaSubscriber {
	return &KafkaSubscriber{
		brokers:         f.brokers,
		consumerGroupID: groupID,
		log:             f.log,
	}
}
