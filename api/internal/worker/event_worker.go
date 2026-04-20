package worker

import (
	"context"
	"encoding/json"
	"time"

	"github.com/spidey/notification-service/internal/config"
	"github.com/spidey/notification-service/internal/domain"
	"github.com/spidey/notification-service/internal/pubsub"
	"github.com/spidey/notification-service/internal/service"
	"go.uber.org/zap"
)

// EventWorker ingests notification requests from a Pub/Sub topic.
type EventWorker struct {
	subscriber   pubsub.Subscriber
	notifSvc     *service.NotificationService
	log          *zap.Logger
	subscription string
	source       string
}

func NewEventWorker(
	subscriber pubsub.Subscriber,
	notifSvc *service.NotificationService,
	cfg config.PubSubConfig,
	log *zap.Logger,
) *EventWorker {
	sub := cfg.EventsSubscription
	if sub == "" {
		sub = "notif-service-ingress"
	}

	source := "pubsub"
	if _, ok := subscriber.(*pubsub.RedisSubscriber); ok {
		source = "redis"
	}

	return &EventWorker{
		subscriber:   subscriber,
		notifSvc:     notifSvc,
		subscription: sub,
		source:       source,
		log:          log.With(zap.String("worker", "event-ingress")),
	}
}

func (w *EventWorker) Start(ctx context.Context) error {
	w.log.Info("starting event ingestion worker", zap.String("subscription", w.subscription))

	return w.subscriber.SubscribeRaw(ctx, w.subscription, func(ctx context.Context, data []byte) error {
		var req domain.SendRequest
		if err := json.Unmarshal(data, &req); err != nil {
			w.log.Error("failed to unmarshal incoming event", 
				zap.Error(err),
				zap.String("payload", string(data)),
			)
			return nil // Ack malformed messages
		}

		// Validation (basic)
		if req.UserID == "" || len(req.Channels) == 0 {
			w.log.Warn("received malformed event: missing required fields",
				zap.String("user_id", req.UserID),
				zap.Int("channels_count", len(req.Channels)),
			)
			return nil
		}

		w.log.Info("processing event-driven notification",
			zap.String("user_id", req.UserID),
			zap.String("type", req.Type),
			zap.String("idempotency_key", req.IdempotencyKey),
		)

		startTime := time.Now()
		resp, err := w.notifSvc.Send(ctx, &req, w.source)
		if err != nil {
			w.log.Error("failed to process event notification",
				zap.Error(err),
				zap.String("user_id", req.UserID),
			)
			// Return error to Nack if it's a transient failure? 
			// For now, let's assume if it fails here, we might want a retry depending on the error.
			// However, domain errors like OptedOut shouldn't be retried.
			return nil 
		}

		w.log.Info("event notification accepted",
			zap.String("notification_id", resp.NotificationID),
			zap.String("status", resp.Status),
			zap.Duration("latency", time.Since(startTime)),
		)

		return nil
	})
}

func (w *EventWorker) Channel() domain.Channel {
	return "ingress" // Special channel for the worker loop
}

func (w *EventWorker) Reload(ctx context.Context, cfg config.ProviderConfig) {
	// No provider-specific config to reload for this worker
}
