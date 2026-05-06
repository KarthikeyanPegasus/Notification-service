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
	"github.com/spidey/notification-service/internal/ratelimit"
	"github.com/spidey/notification-service/internal/repository"
	"github.com/spidey/notification-service/internal/security"
	"go.uber.org/zap"
)

// MaxRetryAttempts defines the maximum number of delivery attempts before sending to DLQ.
const MaxRetryAttempts = 5

// exponentialBackoff calculates delay with exponential backoff and jitter.
func exponentialBackoff(attempt int) time.Duration {
	// Base delay: 100ms, 200ms, 400ms, 800ms, 1600ms
	baseDelay := time.Duration(100) * time.Millisecond
	backoff := baseDelay * time.Duration(1<<uint(attempt-1))
	if backoff > 30*time.Second {
		backoff = 30 * time.Second
	}
	// Add jitter (±20%)
	jitter := time.Duration(float64(backoff) * 0.2 * (2*0.5 - 1))
	return backoff + jitter
}

// suppressionCheckTimeout is the max time for a governance DB lookup per message.
const suppressionCheckTimeout = 5 * time.Second

// prioritySLA defines the maximum end-to-end delivery time per priority level.
// Workers enforce this as a context deadline from the moment a message is received.
var prioritySLA = map[domain.Priority]time.Duration{
	domain.PriorityHigh:   5 * time.Second,
	domain.PriorityMedium: 15 * time.Second,
	domain.PriorityLow:    30 * time.Second,
}

// shouldPreferFallback returns true when the primary/preferred vendor has an error rate above the configured threshold.
// It uses the circuit breaker's rolling window counts (see circuit.BreakerConfig.Interval).
func shouldPreferFallback(registry *circuit.Registry, primaryVendor string, routing nsconfig.RoutingConfig) bool {
	if registry == nil {
		return false
	}
	if routing.ErrorRateThreshold <= 0 {
		return false
	}
	if primaryVendor == "" {
		return false
	}
	minReq := routing.MinRequests
	if minReq <= 0 {
		minReq = 20
	}

	cb := registry.GetOrDefault(primaryVendor)
	counts := cb.Counts()
	total := int(counts.TotalSuccesses + counts.TotalFailures)
	if total < minReq {
		return false
	}
	if counts.TotalFailures == 0 {
		return false
	}
	errRate := float64(counts.TotalFailures) / float64(total)
	return errRate >= routing.ErrorRateThreshold
}

// BaseWorker contains the shared logic for all channel workers.
type BaseWorker struct {
	channel      domain.Channel
	priority     domain.Priority // the priority this worker instance handles
	subscription string
	subscriber   pubsub.Subscriber
	publisher    pubsub.Publisher // for publishing to DLQ
	notifRepo    *repository.NotificationRepository
	attemptRepo  *repository.AttemptRepository
	eventRepo    *repository.EventRepository
	govRepo      *repository.GovernanceRepository
	vendorRepo   nsconfig.Repository
	rateLimitRepo repository.VendorRateLimitRepository
	rateLimiter  *ratelimit.VendorLimiter
	cfg          *nsconfig.Config
	registry     *circuit.Registry
	log          *zap.Logger
	// vendorFilter, when set, restricts this worker to a specific vendor.
	vendorFilter string
	// clientIDFilter, when set, restricts this worker to a specific client's scope.
	clientIDFilter string
}

// Worker is the interface all channel workers implement.
type Worker interface {
	Start(ctx context.Context) error
	Channel() domain.Channel
	Reload(ctx context.Context, cfg nsconfig.ProviderConfig)
}

// WorkerOptions carries optional configuration for a worker instance.
type WorkerOptions struct {
	// Priority restricts this worker to a single priority tier (high/medium/low).
	// When empty, the worker defaults to consuming all priorities (legacy mode).
	Priority domain.Priority
	// VendorFilter restricts this worker to a specific vendor name.
	VendorFilter string
	// ClientIDFilter scopes this worker to a specific API key / client.
	ClientIDFilter string
	// RateLimitRepo enables vendor-level rate limit enforcement.
	RateLimitRepo repository.VendorRateLimitRepository
	// RateLimiter is the Redis-backed rate limiter used by the worker.
	RateLimiter *ratelimit.VendorLimiter
}

func newBaseWorker(
	channel domain.Channel,
	subscription string,
	subscriber pubsub.Subscriber,
	publisher pubsub.Publisher,
	notifRepo *repository.NotificationRepository,
	attemptRepo *repository.AttemptRepository,
	eventRepo *repository.EventRepository,
	govRepo *repository.GovernanceRepository,
	vendorRepo nsconfig.Repository,
	cfg *nsconfig.Config,
	registry *circuit.Registry,
	log *zap.Logger,
	opts ...WorkerOptions,
) *BaseWorker {
	bw := &BaseWorker{
		channel:      channel,
		subscription: subscription,
		subscriber:   subscriber,
		publisher:    publisher,
		notifRepo:    notifRepo,
		attemptRepo:  attemptRepo,
		eventRepo:    eventRepo,
		govRepo:      govRepo,
		vendorRepo:   vendorRepo,
		cfg:          cfg,
		registry:     registry,
		log:          log,
	}
	if len(opts) > 0 {
		o := opts[0]
		bw.priority = o.Priority
		bw.vendorFilter = o.VendorFilter
		bw.clientIDFilter = o.ClientIDFilter
		bw.rateLimitRepo = o.RateLimitRepo
		bw.rateLimiter = o.RateLimiter
	}
	return bw
}

// slaContext wraps ctx with a deadline derived from the message priority.
// High-priority messages must be delivered within 5s, medium within 15s, low within 30s.
func (w *BaseWorker) slaContext(ctx context.Context, priority domain.Priority) (context.Context, context.CancelFunc) {
	sla, ok := prioritySLA[priority]
	if !ok || sla <= 0 {
		sla = prioritySLA[domain.PriorityLow]
	}
	return context.WithTimeout(ctx, sla)
}

// checkVendorRateLimit returns true if the send should proceed, false if the vendor
// is currently rate-limited. When throttled, the message should be nacked so it can
// be redelivered after the rate window resets.
func (w *BaseWorker) checkVendorRateLimit(ctx context.Context, vendorName string, apiKeyID *uuid.UUID) bool {
	if w.rateLimiter == nil || w.rateLimitRepo == nil {
		return true
	}

	var clientIDStr string
	if apiKeyID != nil {
		clientIDStr = apiKeyID.String()
	}

	// Prefer scoped limit; fall back to global.
	rl, err := w.rateLimitRepo.Get(ctx, vendorName, &clientIDStr)
	if err != nil || rl == nil {
		if err != nil {
			w.log.Warn("failed to load rate limit config — allowing send",
				zap.String("vendor", vendorName), zap.Error(err))
		}
		// No scoped config; try global
		rl, err = w.rateLimitRepo.Get(ctx, vendorName, nil)
		if err != nil || rl == nil {
			return true
		}
	}

	allowed, err := w.rateLimiter.Allow(ctx, vendorName, clientIDStr, rl)
	if err != nil {
		w.log.Warn("rate limiter error — allowing send", zap.String("vendor", vendorName), zap.Error(err))
		return true
	}
	return allowed
}

// checkGovernance returns true if the notification should be suppressed.
// It checks the identifier-level suppression list and the user-level opt-out table.
// When suppressed it updates the notification status to StatusSuppressed and appends an event.
func (w *BaseWorker) checkGovernance(ctx context.Context, n *domain.Notification) bool {
	if w.govRepo == nil {
		return false
	}

	checkCtx, cancel := context.WithTimeout(ctx, suppressionCheckTimeout)
	defer cancel()

	// Identifier-level suppression
	if stype := domain.SuppressionTypeForChannel(w.channel); stype != "" {
		suppressed, err := w.govRepo.IsSuppressed(checkCtx, stype, n.Recipient)
		if err != nil {
			w.log.Warn("suppression check error — allowing delivery",
				zap.String("notification_id", n.ID.String()),
				zap.Error(err),
			)
		} else if suppressed {
			w.log.Info("notification suppressed (identifier)",
				zap.String("notification_id", n.ID.String()),
				zap.String("channel", string(w.channel)),
				zap.String("recipient", security.RedactRecipient(n.Recipient, w.channel)),
			)
			_ = w.notifRepo.UpdateStatus(ctx, n.ID, domain.StatusSuppressed)
			_ = w.eventRepo.Append(ctx, &domain.NotificationEvent{
				ID:             uuid.New(),
				NotificationID: n.ID,
				EventType:      domain.EventSuppressed,
				Metadata:       map[string]any{"reason": "identifier_suppressed", "channel": string(w.channel)},
				CreatedAt:      time.Now(),
			})
			return true
		}
	}

	// User-level opt-out
	if n.UserID != nil {
		optedOut, err := w.govRepo.IsOptedOut(checkCtx, *n.UserID, w.channel)
		if err != nil {
			w.log.Warn("opt-out check error — allowing delivery",
				zap.String("notification_id", n.ID.String()),
				zap.Error(err),
			)
		} else if optedOut {
			w.log.Info("notification suppressed (opt-out)",
				zap.String("notification_id", n.ID.String()),
				zap.String("channel", string(w.channel)),
				zap.String("user_id", n.UserID.String()),
			)
			_ = w.notifRepo.UpdateStatus(ctx, n.ID, domain.StatusSuppressed)
			_ = w.eventRepo.Append(ctx, &domain.NotificationEvent{
				ID:             uuid.New(),
				NotificationID: n.ID,
				EventType:      domain.EventOptedOut,
				Metadata:       map[string]any{"reason": "user_opt_out", "channel": string(w.channel)},
				CreatedAt:      time.Now(),
			})
			return true
		}
	}

	return false
}

func (w *BaseWorker) getEffectiveConfig(ctx context.Context, apiKeyID *uuid.UUID) *nsconfig.Config {
	if apiKeyID == nil || w.vendorRepo == nil || w.cfg == nil {
		return w.cfg
	}
	scope := apiKeyID.String()
	cfgCopy := *w.cfg
	_ = cfgCopy.LoadDynamicOverridesScoped(ctx, w.vendorRepo, &scope)
	return &cfgCopy
}

// dispatch executes the send using a provider, records the attempt, and handles retries/DLQ.
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

	// Governance check: suppress before delivery (non-Temporal path)
	if w.checkGovernance(ctx, n) {
		return nil // ack — suppressed notifications should not be retried
	}

	// Get current attempt count from message (defaults to 0 for first attempt)
	attemptNum := msg.AttemptCount + 1
	if attemptNum > MaxRetryAttempts {
		w.log.Warn("max retry attempts exceeded, sending to DLQ",
			zap.String("notification_id", notifID.String()),
			zap.Int("attempt_count", attemptNum),
		)
		// Send to DLQ
		if w.publisher != nil {
			msg.AttemptCount = attemptNum
			_, dlqErr := w.publisher.PublishToDLQ(ctx, msg, "max_retries_exceeded")
			if dlqErr != nil {
				w.log.Error("failed to send to DLQ", zap.Error(dlqErr))
			}
		}
		return nil // ack to prevent infinite loop
	}

	// Apply exponential backoff for retries (skip for first attempt)
	if attemptNum > 1 {
		delay := exponentialBackoff(attemptNum)
		w.log.Info("applying exponential backoff before retry",
			zap.String("notification_id", notifID.String()),
			zap.Int("attempt", attemptNum),
			zap.Duration("delay", delay),
		)
		select {
		case <-time.After(delay):
			// Continue with retry
		case <-ctx.Done():
			w.log.Warn("context cancelled during backoff",
				zap.String("notification_id", notifID.String()),
			)
			return ctx.Err()
		}
	}

	// Enforce priority SLA: wrap context with a deadline based on priority.
	slaCtx, slaCancel := w.slaContext(ctx, n.Priority)
	defer slaCancel()

	// Vendor rate limit check — nack if throttled so the message is redelivered.
	if !w.checkVendorRateLimit(slaCtx, vendorName, n.APIKeyID) {
		return fmt.Errorf("vendor %s rate limited — nacking for redelivery", vendorName)
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

	ctx = slaCtx // use SLA-bounded context for the actual send

	// Attempt the send
	start := time.Now()
	var result domain.DeliveryResult

	_, execErr := cb.Execute(func() (any, error) {
		r, err := senderFn(ctx, n)
		result = r
		return r, err
	})

	result.LatencyMs = int(time.Since(start).Milliseconds())
	if result.Provider == "" {
		// Ensure attempt records carry the actual vendor used, so status polling uses the right provider.
		result.Provider = vendorName
	}

	if execErr != nil {
		result.Success = false
		result.ErrorMessage = execErr.Error()
	}

	// Record the attempt with actual attempt number
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
			"attempt":    attemptNum,
		},
		CreatedAt: time.Now(),
	})

	w.log.Info("notification dispatched",
		zap.String("channel", string(w.channel)),
		zap.String("notification_id", notifID.String()),
		zap.String("vendor", vendorName),
		zap.Bool("success", result.Success),
		zap.Int("latency_ms", result.LatencyMs),
		zap.Int("attempt", attemptNum),
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

	// Governance check: suppress before delivery (non-Temporal path)
	if w.checkGovernance(ctx, n) {
		return nil
	}

	// Get current attempt count from message
	attemptNum := msg.AttemptCount + 1
	if attemptNum > MaxRetryAttempts {
		w.log.Warn("max retry attempts exceeded, sending to DLQ (publish_all)",
			zap.String("notification_id", notifID.String()),
			zap.Int("attempt_count", attemptNum),
		)
		if w.publisher != nil {
			msg.AttemptCount = attemptNum
			_, dlqErr := w.publisher.PublishToDLQ(ctx, msg, "max_retries_exceeded")
			if dlqErr != nil {
				w.log.Error("failed to send to DLQ", zap.Error(dlqErr))
			}
		}
		return nil
	}

	// Apply exponential backoff for retries (skip for first attempt)
	if attemptNum > 1 {
		delay := exponentialBackoff(attemptNum)
		w.log.Info("applying exponential backoff before retry (publish_all)",
			zap.String("notification_id", notifID.String()),
			zap.Int("attempt", attemptNum),
			zap.Duration("delay", delay),
		)
		select {
		case <-time.After(delay):
			// Continue with retry
		case <-ctx.Done():
			w.log.Warn("context cancelled during backoff",
				zap.String("notification_id", notifID.String()),
			)
			return ctx.Err()
		}
	}

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
			"attempt":   attemptNum,
		},
		CreatedAt: time.Now(),
	})

	w.log.Info("notification dispatched (publish_all)",
		zap.String("channel", string(w.channel)),
		zap.String("notification_id", notifID.String()),
		zap.Bool("any_success", anySuccess),
		zap.Int("attempt", attemptNum),
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
