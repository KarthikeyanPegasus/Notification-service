package pubsub

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

// MockPublisher implements Publisher using in-memory channels for local dev.
type MockPublisher struct {
	mu       sync.RWMutex
	channels map[string]chan *Message
	log      *zap.Logger
}

func NewMockPublisher(log *zap.Logger) *MockPublisher {
	channels := make(map[string]chan *Message)
	for ch := range TopicID {
		channels[ch] = make(chan *Message, 1000)
	}
	return &MockPublisher{channels: channels, log: log}
}

func (p *MockPublisher) Publish(_ context.Context, channel string, msg *Message) (string, error) {
	p.mu.RLock()
	ch, ok := p.channels[channel]
	p.mu.RUnlock()

	if !ok {
		return "", fmt.Errorf("mock publisher: unknown channel %s", channel)
	}

	select {
	case ch <- msg:
		msgID := uuid.New().String()
		if p.log != nil {
			data, _ := json.Marshal(msg)
			p.log.Debug("mock pubsub published",
				zap.String("channel", channel),
				zap.String("msg_id", msgID),
				zap.String("payload", string(data)),
			)
		}
		return msgID, nil
	default:
		return "", fmt.Errorf("mock publisher: channel %s buffer full", channel)
	}
}

func (p *MockPublisher) Chan(channel string) <-chan *Message {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.channels[channel]
}

func (p *MockPublisher) Close() error { return nil }

// MockSubscriber consumes from MockPublisher channels.
type MockSubscriber struct {
	publisher *MockPublisher
	log       *zap.Logger
}

func NewMockSubscriber(publisher *MockPublisher, log *zap.Logger) *MockSubscriber {
	return &MockSubscriber{publisher: publisher, log: log}
}

func (s *MockSubscriber) Subscribe(ctx context.Context, channel string, handler MessageHandler) error {
	ch := s.publisher.Chan(channel)
	if ch == nil {
		return fmt.Errorf("mock subscriber: unknown channel %s", channel)
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case msg, ok := <-ch:
			if !ok {
				return nil
			}
			if err := handler(ctx, msg); err != nil {
				if s.log != nil {
					s.log.Warn("mock subscriber: handler error",
						zap.String("channel", channel),
						zap.Error(err),
					)
				}
			}
		}
	}
}

func (s *MockSubscriber) SubscribeRaw(ctx context.Context, channel string, handler RawMessageHandler) error {
	ch := s.publisher.Chan(channel)
	if ch == nil {
		return fmt.Errorf("mock subscriber: unknown channel %s", channel)
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case msg, ok := <-ch:
			if !ok {
				return nil
			}
			data, _ := json.Marshal(msg)
			if err := handler(ctx, data); err != nil {
				if s.log != nil {
					s.log.Warn("mock subscriber raw: handler error",
						zap.String("channel", channel),
						zap.Error(err),
					)
				}
			}
		}
	}
}

func (s *MockSubscriber) Close() error { return nil }
