package worker

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/spidey/notification-service/internal/circuit"
	nsconfig "github.com/spidey/notification-service/internal/config"
	"github.com/spidey/notification-service/internal/domain"
	"github.com/spidey/notification-service/internal/provider"
	"github.com/spidey/notification-service/internal/pubsub"
	"github.com/spidey/notification-service/internal/repository"
	"go.uber.org/zap"
)

// BaseWorker contains the shared logic for all channel workers.
type BaseWorker struct {
	channel     domain.Channel
	subscription string
	subscriber  pubsub.Subscriber
	notifRepo   *repository.NotificationRepository
	attemptRepo *repository.AttemptRepository
	eventRepo   *repository.EventRepository
	registry    *circuit.Registry
	log         *zap.Logger
}

// Worker is the interface all channel workers implement.
type Worker interface {
	Start(ctx context.Context) error
	Channel() domain.Channel
	Reload(ctx context.Context, cfg nsconfig.ProviderConfig)
}

func newBaseWorker(
	channel domain.Channel,
	subscription string,
	subscriber pubsub.Subscriber,
	notifRepo *repository.NotificationRepository,
	attemptRepo *repository.AttemptRepository,
	eventRepo *repository.EventRepository,
	registry *circuit.Registry,
	log *zap.Logger,
) *BaseWorker {
	return &BaseWorker{
		channel:      channel,
		subscription: subscription,
		subscriber:   subscriber,
		notifRepo:    notifRepo,
		attemptRepo:  attemptRepo,
		eventRepo:    eventRepo,
		registry:     registry,
		log:          log,
	}
}

// dispatch executes the send using a provider, records the attempt, and acks/nacks.
func (w *BaseWorker) dispatch(
	ctx context.Context,
	msg *pubsub.Message,
	senderFn func(ctx context.Context, n *domain.Notification) (domain.DeliveryResult, error),
	vendorName string,
) error {
	notifID, err := uuid.Parse(msg.NotificationID)
	if err != nil {
		w.log.Error("invalid notification_id in message", zap.String("raw", msg.NotificationID))
		return nil // ack malformed messages to avoid infinite loop
	}

	n, err := w.notifRepo.GetByID(ctx, notifID)
	if err != nil {
		w.log.Error("notification not found", zap.String("id", msg.NotificationID), zap.Error(err))
		return nil
	}

	// Check circuit breaker
	cb := w.registry.GetOrDefault(vendorName)
	if cb.IsOpen() {
		w.log.Warn("circuit breaker open — skipping send",
			zap.String("vendor", vendorName),
			zap.String("notification_id", notifID.String()),
		)
		return fmt.Errorf("circuit breaker open for vendor %s", vendorName)
	}

	// Attempt the send
	attemptNum := 1
	start := time.Now()
	var result domain.DeliveryResult

	_, execErr := cb.Execute(func() (any, error) {
		r, err := senderFn(ctx, n)
		result = r
		return r, err
	})

	result.LatencyMs = int(time.Since(start).Milliseconds())

	if execErr != nil {
		result.Success = false
		result.ErrorMessage = execErr.Error()
	}

	// Record the attempt
	if err := w.attemptRepo.RecordAttemptFromResult(ctx, notifID, attemptNum, result); err != nil {
		w.log.Error("recording attempt", zap.Error(err))
	}

	// Update notification status and emit event
	eventType := domain.EventFailed
	status := domain.StatusFailed
	if result.Success {
		eventType = domain.EventSent
		status = domain.StatusSent
	}

	_ = w.notifRepo.UpdateStatus(ctx, notifID, status)
	_ = w.eventRepo.Append(ctx, &domain.NotificationEvent{
		ID:             uuid.New(),
		NotificationID: notifID,
		EventType:      eventType,
		Metadata: map[string]any{
			"provider":   vendorName,
			"latency_ms": result.LatencyMs,
		},
		CreatedAt: time.Now(),
	})

	// Prometheus metrics
	statusStr := "failed"
	if result.Success {
		statusStr = "sent"
	}
	NotificationsProcessedTotal.WithLabelValues(
		string(w.channel),
		statusStr,
		vendorName,
	).Inc()
	NotificationProcessingDurationSeconds.WithLabelValues(
		string(w.channel),
		vendorName,
	).Observe(time.Since(start).Seconds())

	w.log.Info("notification dispatched",
		zap.String("channel", string(w.channel)),
		zap.String("notification_id", notifID.String()),
		zap.String("vendor", vendorName),
		zap.Bool("success", result.Success),
		zap.Int("latency_ms", result.LatencyMs),
	)

	return execErr
}

// dispatchPublishAll sends the same notification through all configured vendors (best-effort),
// recording an attempt per vendor. If at least one vendor succeeds, the notification is marked sent.
func (w *BaseWorker) dispatchPublishAll(
	ctx context.Context,
	msg *pubsub.Message,
	senders []provider.Sender,
) error {
	notifID, err := uuid.Parse(msg.NotificationID)
	if err != nil {
		w.log.Error("invalid notification_id in message", zap.String("raw", msg.NotificationID))
		return nil
	}

	n, err := w.notifRepo.GetByID(ctx, notifID)
	if err != nil {
		w.log.Error("notification not found", zap.String("id", msg.NotificationID), zap.Error(err))
		return nil
	}

	attemptNum := 1
	anySuccess := false

	for _, s := range senders {
		vendor := s.ProviderName()
		cb := w.registry.GetOrDefault(vendor)
		if cb.IsOpen() {
			w.log.Warn("circuit breaker open — skipping", zap.String("vendor", vendor))
			continue
		}

		start := time.Now()
		var result domain.DeliveryResult
		_, execErr := cb.Execute(func() (any, error) {
			r, err := s.Send(ctx, n)
			result = r
			return r, err
		})
		result.LatencyMs = int(time.Since(start).Milliseconds())
		if result.Provider == "" {
			result.Provider = vendor
		}
		if execErr != nil {
			result.Success = false
			result.ErrorMessage = execErr.Error()
		}

		_ = w.attemptRepo.RecordAttemptFromResult(ctx, notifID, attemptNum, result)
		attemptNum++

		if result.Success {
			anySuccess = true
		}
	}

	eventType := domain.EventFailed
	status := domain.StatusFailed
	if anySuccess {
		eventType = domain.EventSent
		status = domain.StatusSent
	}

	_ = w.notifRepo.UpdateStatus(ctx, notifID, status)
	_ = w.eventRepo.Append(ctx, &domain.NotificationEvent{
		ID:             uuid.New(),
		NotificationID: notifID,
		EventType:      eventType,
		Metadata: map[string]any{
			"provider": "publish_all",
		},
		CreatedAt: time.Now(),
	})

	w.log.Info("notification dispatched (publish_all)",
		zap.String("channel", string(w.channel)),
		zap.String("notification_id", notifID.String()),
		zap.Bool("any_success", anySuccess),
	)

	if anySuccess {
		return nil
	}
	return fmt.Errorf("all providers failed")
}

// withFallback tries providers in order, skipping those with open circuit breakers.
func withFallback(
	ctx context.Context,
	senders []provider.Sender,
	n *domain.Notification,
	registry *circuit.Registry,
	log *zap.Logger,
) (domain.DeliveryResult, error) {
	var lastErr error

	for _, s := range senders {
		vendor := s.ProviderName()
		cb := registry.GetOrDefault(vendor)

		if cb.IsOpen() {
			log.Warn("circuit breaker open — skipping",
				zap.String("vendor", vendor),
			)
			continue
		}

		var result domain.DeliveryResult
		_, err := cb.Execute(func() (any, error) {
			r, err := s.Send(ctx, n)
			result = r
			return r, err
		})

		if err == nil {
			return result, nil
		}

		lastErr = err
		log.Warn("provider failed — trying next",
			zap.String("vendor", vendor),
			zap.Error(err),
		)
	}

	if lastErr != nil {
		return domain.DeliveryResult{ErrorMessage: "all providers failed"}, domain.ErrAllProvidersOpen
	}
	return domain.DeliveryResult{ErrorMessage: "no providers configured"}, domain.ErrAllProvidersOpen
}
