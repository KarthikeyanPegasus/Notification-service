package pubsub

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// RedisPublisher implements Publisher using Redis PUB/SUB.
type RedisPublisher struct {
	client *redis.Client
	log    *zap.Logger
}

func NewRedisPublisher(client *redis.Client, log *zap.Logger) *RedisPublisher {
	return &RedisPublisher{
		client: client,
		log:    log,
	}
}

func (p *RedisPublisher) Publish(ctx context.Context, channel string, msg *Message) (string, error) {
	data, err := json.Marshal(msg)
	if err != nil {
		return "", fmt.Errorf("marshalling redis message: %w", err)
	}

	topic := TopicID[channel]
	if topic == "" {
		topic = "notifications-" + channel
	}

	err = p.client.Publish(ctx, topic, data).Err()
	if err != nil {
		return "", fmt.Errorf("redis publish to %s: %w", topic, err)
	}

	return fmt.Sprintf("redis:%s", topic), nil
}

func (p *RedisPublisher) Close() error {
	return nil // connection managed externally
}

// RedisSubscriber implements Subscriber using Redis PUB/SUB.
type RedisSubscriber struct {
	client *redis.Client
	log    *zap.Logger
}

func NewRedisSubscriber(client *redis.Client, log *zap.Logger) *RedisSubscriber {
	return &RedisSubscriber{
		client: client,
		log:    log,
	}
}

func (s *RedisSubscriber) Subscribe(ctx context.Context, subscription string, handler MessageHandler) error {
	// For Redis, the "subscription" string is treated as the channel name.
	// Map provided name if it's a known channel
	topic := TopicID[subscription]
	if topic == "" {
		topic = subscription
	}

	pubsub := s.client.Subscribe(ctx, topic)
	defer pubsub.Close()

	ch := pubsub.Channel()
	s.log.Info("redis subscriber listening", zap.String("topic", topic))

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case msg, ok := <-ch:
			if !ok {
				return nil
			}

			var m Message
			if err := json.Unmarshal([]byte(msg.Payload), &m); err != nil {
				s.log.Error("failed to unmarshal redis message", zap.Error(err))
				continue
			}

			if err := handler(ctx, &m); err != nil {
				s.log.Error("handler error in redis subscriber", zap.Error(err))
				// No built-in retry/nack in Redis PubSub; error is logged and we move on.
			}
		}
	}
}

func (s *RedisSubscriber) SubscribeRaw(ctx context.Context, subscription string, handler RawMessageHandler) error {
	topic := TopicID[subscription]
	if topic == "" {
		topic = subscription
	}

	pubsub := s.client.Subscribe(ctx, topic)
	defer pubsub.Close()

	ch := pubsub.Channel()
	s.log.Info("redis raw subscriber listening", zap.String("topic", topic))

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case msg, ok := <-ch:
			if !ok {
				return nil
			}

			if err := handler(ctx, []byte(msg.Payload)); err != nil {
				s.log.Error("handler error in redis raw subscriber", zap.Error(err))
			}
		}
	}
}

func (s *RedisSubscriber) Close() error {
	return nil // connection managed externally
}
