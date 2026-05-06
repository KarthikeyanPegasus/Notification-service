package workflow

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/spidey/notification-service/internal/cache"
	"github.com/spidey/notification-service/internal/config"
	"github.com/spidey/notification-service/internal/domain"
	"github.com/spidey/notification-service/internal/pubsub"
	"github.com/spidey/notification-service/internal/ratelimit"
	"github.com/spidey/notification-service/internal/repository"
	"github.com/spidey/notification-service/internal/security"
	"go.temporal.io/sdk/activity"
	"go.uber.org/zap"
)

type TemplateRenderer interface {
	RenderString(tmpl string, vars map[string]string) string
}

type DeliveryProvider interface {
	Deliver(ctx context.Context, n *domain.Notification) (domain.DeliveryResult, error)
	DeliverScoped(ctx context.Context, n *domain.Notification, cfg config.ProviderConfig) (domain.DeliveryResult, error)
}

type Activities struct {
	cacheClient      *cache.Client
	templateRepo     *repository.TemplateRepository
	notifRepo        *repository.NotificationRepository
	schedRepo        *repository.ScheduledRepository
	eventRepo        *repository.EventRepository
	attemptRepo      *repository.AttemptRepository
	prefsRepo        *repository.UserPreferencesRepository
	templateRenderer TemplateRenderer
	deliverySvc      DeliveryProvider
	pubsub           pubsub.Publisher
	govRepo          *repository.GovernanceRepository
	cfg              *config.Config
	vendorRepo       config.Repository
	rateLimitRepo    repository.VendorRateLimitRepository
	rateLimiter      *ratelimit.VendorLimiter
	contentFilter    security.ContentFilter
	log              *zap.Logger
}

func NewActivities(
	cacheClient *cache.Client,
	templateRepo *repository.TemplateRepository,
	notifRepo *repository.NotificationRepository,
	schedRepo *repository.ScheduledRepository,
	eventRepo *repository.EventRepository,
	attemptRepo *repository.AttemptRepository,
	prefsRepo *repository.UserPreferencesRepository,
	templateRenderer TemplateRenderer,
	deliverySvc DeliveryProvider,
	pubsub pubsub.Publisher,
	govRepo *repository.GovernanceRepository,
	cfg *config.Config,
	vendorRepo config.Repository,
	rateLimitRepo repository.VendorRateLimitRepository,
	rateLimiter *ratelimit.VendorLimiter,
	contentFilter security.ContentFilter,
	log *zap.Logger,
) *Activities {
	return &Activities{
		cacheClient:      cacheClient,
		templateRepo:     templateRepo,
		notifRepo:        notifRepo,
		schedRepo:        schedRepo,
		eventRepo:        eventRepo,
		attemptRepo:      attemptRepo,
		prefsRepo:        prefsRepo,
		templateRenderer: templateRenderer,
		deliverySvc:      deliverySvc,
		pubsub:           pubsub,
		govRepo:          govRepo,
		cfg:              cfg,
		vendorRepo:       vendorRepo,
		rateLimitRepo:    rateLimitRepo,
		rateLimiter:      rateLimiter,
		contentFilter:    contentFilter,
		log:              log,
	}
}

// RenderedNotification represents the final message payload.
type RenderedNotification struct {
	ID           uuid.UUID
	Channel      domain.Channel
	Recipient    string
	Payload      []byte // plain text body
	Subject      string
	HTML         string
	ForcedVendor string
	Priority     domain.Priority
	ClientID     string
}

func (a *Activities) CheckPreferencesActivity(ctx context.Context, req *WorkflowRequest) (*domain.UserPreferences, error) {
	start := time.Now()
	var deadline string
	if dl, ok := ctx.Deadline(); ok {
		deadline = dl.UTC().Format(time.RFC3339Nano)
	}
	a.log.Info("CheckPreferencesActivity start",
		zap.String("notif_id", req.ID.String()),
		zap.String("type", req.Type),
		zap.String("channel", string(req.Channel)),
		zap.String("recipient", security.RedactRecipient(req.Recipient, req.Channel)),
		zap.String("trace_id", req.TraceID),
		zap.String("deadline", deadline),
	)
	defer func() {
		a.log.Info("CheckPreferencesActivity done",
			zap.String("notif_id", req.ID.String()),
			zap.String("trace_id", req.TraceID),
			zap.Duration("took", time.Since(start)),
		)
	}()

	if req.Type == "test" {
		return &domain.UserPreferences{
			Channels: map[domain.Channel]bool{req.Channel: true},
		}, nil
	}

	// Governance Check: Identifier-level Suppression
	if stype := domain.SuppressionTypeForChannel(req.Channel); stype != "" {
		govCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		suppressed, err := a.govRepo.IsSuppressed(govCtx, stype, req.Recipient)
		cancel()
		if err != nil {
			a.log.Warn("CheckPreferencesActivity suppression check error",
				zap.String("notif_id", req.ID.String()),
				zap.String("trace_id", req.TraceID),
				zap.Error(err),
			)
			// Fail-closed: if governance check errors, reject the notification
			return nil, fmt.Errorf("checking suppression: %w", err)
		}
		if suppressed {
			a.log.Info("CheckPreferencesActivity recipient suppressed",
				zap.String("notif_id", req.ID.String()),
				zap.String("channel", string(req.Channel)),
			)
			return &domain.UserPreferences{IsSuppressed: true}, nil
		}
	}

	// DB-backed Preference Lookup: fetch actual user preferences from PostgreSQL
	if a.prefsRepo != nil && req.UserID != "" {
		prefCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		prefs, err := a.prefsRepo.Get(prefCtx, req.UserID)
		cancel()
		if err != nil {
			a.log.Warn("CheckPreferencesActivity preferences lookup error, using permissive defaults",
				zap.String("notif_id", req.ID.String()),
				zap.String("user_id", req.UserID),
				zap.String("trace_id", req.TraceID),
				zap.Error(err),
			)
			// Fail-open for preference lookup errors (preferences outage shouldn't block all delivery)
			return &domain.UserPreferences{
				Channels:  map[domain.Channel]bool{req.Channel: true},
				UpdatedAt: time.Now(),
			}, nil
		}
		if prefs != nil {
			a.log.Info("CheckPreferencesActivity loaded user preferences from DB",
				zap.String("notif_id", req.ID.String()),
				zap.String("user_id", req.UserID),
				zap.Bool("channel_enabled", prefs.IsChannelEnabled(req.Channel)),
				zap.Bool("suppressed", prefs.IsSuppressed),
			)
			// Check channel enablement
			if !prefs.IsChannelEnabled(req.Channel) {
				a.log.Info("CheckPreferencesActivity channel disabled by user",
					zap.String("notif_id", req.ID.String()),
					zap.String("channel", string(req.Channel)),
				)
				return &domain.UserPreferences{IsSuppressed: true}, nil
			}
			return prefs, nil
		}
	}

	// Fallback: no preferences set for this user — return permissive defaults
	return &domain.UserPreferences{
		Channels:  map[domain.Channel]bool{req.Channel: true},
		UpdatedAt: time.Now(),
	}, nil
}

func (a *Activities) RenderTemplateActivity(ctx context.Context, req *WorkflowRequest) (*RenderedNotification, error) {
	if req.TemplateID == nil {
		// Prefer inline direct content passed in the request.
		if req.DirectContent != nil && strings.TrimSpace(req.DirectContent.Body) != "" {
			return &RenderedNotification{
				ID:           req.ID,
				Channel:      req.Channel,
				Recipient:    req.Recipient,
				Payload:      []byte(req.DirectContent.Body),
				Subject:      req.DirectContent.Subject,
				HTML:         req.DirectContent.HTML,
				ForcedVendor: req.ForcedVendor,
				Priority:     req.Priority,
				ClientID:     req.ClientID,
			}, nil
		}

		// Fall back to rendered content stored on the notification record.
		n, err := a.notifRepo.GetByID(ctx, req.ID)
		if err == nil && n != nil && n.RenderedContent != nil {
			return &RenderedNotification{
				ID:           req.ID,
				Channel:      req.Channel,
				Recipient:    req.Recipient,
				Payload:      []byte(n.RenderedContent.Body),
				Subject:      n.RenderedContent.Subject,
				HTML:         n.RenderedContent.HTML,
				ForcedVendor: req.ForcedVendor,
				Priority:     req.Priority,
				ClientID:     req.ClientID,
			}, nil
		} else if err != nil && !errors.Is(err, domain.ErrNotFound) {
			return nil, fmt.Errorf("loading notification for render: %w", err)
		}
		return &RenderedNotification{
			ID:           req.ID,
			Channel:      req.Channel,
			Recipient:    req.Recipient,
			Payload:      []byte(req.Type), // last-resort fallback
			ForcedVendor: req.ForcedVendor,
			Priority:     req.Priority,
			ClientID:     req.ClientID,
		}, nil
	}

	templateUUID, err := uuid.Parse(*req.TemplateID)
	if err != nil {
		return nil, fmt.Errorf("invalid template_id %q: %w", *req.TemplateID, err)
	}

	tmpl, err := a.templateRepo.GetByID(ctx, templateUUID)
	if err != nil {
		return nil, fmt.Errorf("template not found: %s: %w", templateUUID, err)
	}

	body := a.templateRenderer.RenderString(tmpl.Body, req.TemplateVariables)
	var subject string
	if tmpl.Subject != nil {
		subject = a.templateRenderer.RenderString(*tmpl.Subject, req.TemplateVariables)
	}

	return &RenderedNotification{
		ID:           req.ID,
		Channel:      req.Channel,
		Recipient:    req.Recipient,
		Payload:      []byte(body),
		Subject:      subject,
		ForcedVendor: req.ForcedVendor,
		Priority:     req.Priority,
		ClientID:     req.ClientID,
	}, nil
}

func (a *Activities) ContentSecurityCheckActivity(ctx context.Context, rendered *RenderedNotification) error {
	if a.contentFilter == nil {
		return nil
	}

	content := &domain.RenderedContent{
		Body:    string(rendered.Payload),
		Subject: rendered.Subject,
		HTML:    rendered.HTML,
	}

	if err := a.contentFilter.CheckContent(ctx, content); err != nil {
		a.log.Warn("ContentSecurityCheckActivity flagged content",
			zap.String("notif_id", rendered.ID.String()),
			zap.Error(err),
		)
		return fmt.Errorf("content security check failed: %w", err)
	}

	return nil
}

func (a *Activities) PublishToPubSubActivity(ctx context.Context, rendered *RenderedNotification) (string, error) {
	payload := map[string]string{
		"body": string(rendered.Payload),
	}
	if rendered.Subject != "" {
		payload["subject"] = rendered.Subject
	}
	if rendered.HTML != "" {
		payload["html"] = rendered.HTML
	}

	msg := &pubsub.Message{
		NotificationID: rendered.ID.String(),
		Channel:        string(rendered.Channel),
		Recipient:      rendered.Recipient,
		Payload:        payload,
		IdempotencyKey: fmt.Sprintf("msg:%s", rendered.ID.String()),
		ForcedVendor:   rendered.ForcedVendor,
	}
	serverMsgID, err := a.pubsub.Publish(ctx, string(rendered.Channel), msg)
	if err != nil {
		return "", err
	}
	return serverMsgID, nil
}

func (a *Activities) DeliverNotificationActivity(ctx context.Context, rendered *RenderedNotification) (domain.DeliveryResult, error) {
	// 1. Reconstruct Notification object from RenderedNotification
	n := &domain.Notification{
		ID:        rendered.ID,
		Channel:   rendered.Channel,
		Recipient: rendered.Recipient,
		RenderedContent: &domain.RenderedContent{
			Body:    string(rendered.Payload),
			Subject: rendered.Subject,
			HTML:    rendered.HTML,
		},
		ForcedVendor: rendered.ForcedVendor,
	}

	// 2. Resolve client-scoped configuration if available
	cfg := a.cfg
	dbNotif, err := a.notifRepo.GetByID(ctx, n.ID)
	if err == nil && dbNotif != nil && dbNotif.APIKeyID != nil && a.vendorRepo != nil && a.cfg != nil {
		scope := dbNotif.APIKeyID.String()
		cfgCopy := *a.cfg
		_ = cfgCopy.LoadDynamicOverridesScoped(ctx, a.vendorRepo, &scope)
		cfg = &cfgCopy
		n.APIKeyID = dbNotif.APIKeyID
	}

	// 3. Enforce vendor rate limit before calling the provider.
	if a.rateLimiter != nil && a.rateLimitRepo != nil {
		vendorName := n.ForcedVendor
		if vendorName == "" {
			vendorName = string(n.Channel)
		}
		clientIDStr := ""
		if n.APIKeyID != nil {
			clientIDStr = n.APIKeyID.String()
		}
		rl, _ := a.rateLimitRepo.Get(ctx, vendorName, &clientIDStr)
		if rl == nil {
			rl, _ = a.rateLimitRepo.Get(ctx, vendorName, nil)
		}
		if rl != nil {
			allowed, err := a.rateLimiter.Allow(ctx, vendorName, clientIDStr, rl)
			if err != nil {
				a.log.Warn("DeliverNotificationActivity rate-limit check error — proceeding",
					zap.String("vendor", vendorName),
					zap.Error(err),
				)
			} else if !allowed {
				return domain.DeliveryResult{}, fmt.Errorf("vendor %q rate limited — Temporal will retry within SLA window", vendorName)
			}
		}
	}

	// 4. Perform actual delivery
	var result domain.DeliveryResult
	var sendErr error

	if cfg != a.cfg {
		result, sendErr = a.deliverySvc.DeliverScoped(ctx, n, cfg.Providers)
	} else {
		result, sendErr = a.deliverySvc.Deliver(ctx, n)
	}

	a.log.Info("DeliverNotificationActivity result",
		zap.String("notif_id", n.ID.String()),
		zap.String("provider", result.Provider),
		zap.Bool("success", result.Success),
		zap.String("forced_vendor", n.ForcedVendor),
		zap.Error(sendErr),
	)

	// 5. Record attempt using the actual Temporal attempt number.
	attemptNum := int32(1)
	if info, ok := activityInfo(ctx); ok {
		attemptNum = info.Attempt
	}
	if a.attemptRepo != nil {
		_ = a.attemptRepo.RecordAttemptFromResult(ctx, n.ID, int(attemptNum), result)
	}

	_ = a.notifRepo.UpdateStatus(ctx, n.ID, domain.StatusSent)
	if !result.Success {
		_ = a.notifRepo.UpdateStatus(ctx, n.ID, domain.StatusFailed)
	}

	return result, sendErr
}

// activityInfo safely retrieves Temporal activity info from context.
// Returns false when running outside a Temporal worker (e.g. in tests).
func activityInfo(ctx context.Context) (info activity.Info, ok bool) {
	defer func() {
		if r := recover(); r != nil {
			ok = false
		}
	}()
	info = activity.GetInfo(ctx)
	ok = true
	return
}

func normalizeForcedVendorEmail(v string) string {
	switch strings.TrimSpace(strings.ToLower(v)) {
	case "ses", "amazon-ses", "amazon_ses":
		return "amazon-ses"
	case "smtp", "smtp-relay", "smtp_relay":
		return "smtp-relay"
	default:
		return v
	}
}

func normalizeForcedVendorSMS(v string) string {
	switch strings.TrimSpace(strings.ToLower(v)) {
	case "twilio":
		return "twilio"
	case "plivo":
		return "plivo"
	case "vonage":
		return "vonage"
	default:
		return v
	}
}

type LogEntry struct {
	NotificationID uuid.UUID
	MsgID          string
	Provider       string
	Channel        string
	Status         domain.NotificationStatus
	Layer          string
	ErrorMessage   string
}

func (a *Activities) LogDeliveryActivity(ctx context.Context, entry LogEntry) error {
	err := a.notifRepo.UpdateStatus(ctx, entry.NotificationID, entry.Status)
	if err != nil {
		return err
	}

	if a.schedRepo != nil {
		_ = a.schedRepo.UpdateStatus(ctx, entry.NotificationID, entry.Status)
	}

	eventType := domain.EventSent
	switch entry.Status {
	case domain.StatusDelivered:
		eventType = domain.EventDelivered
	case domain.StatusFailed:
		eventType = domain.EventFailed
	case domain.StatusCancelled:
		eventType = domain.EventCancelled
	case domain.StatusBounced:
		eventType = domain.EventBounced
	case domain.StatusSent:
		eventType = domain.EventSent
	case domain.StatusSuppressed:
		eventType = domain.EventSuppressed
	}

	_ = a.eventRepo.Append(ctx, &domain.NotificationEvent{
		ID:             uuid.New(),
		NotificationID: entry.NotificationID,
		EventType:      eventType,
		Metadata: map[string]any{
			"msg_id":   entry.MsgID,
			"provider": entry.Provider,
			"layer":    firstNonEmpty(entry.Layer, "temporal_workflow"),
			"error":    strings.TrimSpace(entry.ErrorMessage),
		},
		CreatedAt: time.Now(),
	})
	return nil
}

func firstNonEmpty(v string, fallback string) string {
	if strings.TrimSpace(v) != "" {
		return v
	}
	return fallback
}

const (
	otpLength = 6
	otpTTL    = 10 * time.Minute
)

// GenerateOtpActivity generates a cryptographically random 6-digit OTP and
// stores it in Redis keyed by notification ID with a 10-minute TTL.
func (a *Activities) GenerateOtpActivity(ctx context.Context, req *WorkflowRequest) (string, error) {
	max := big.NewInt(1_000_000)
	n, err := rand.Int(rand.Reader, max)
	if err != nil {
		return "", fmt.Errorf("generating otp: %w", err)
	}
	otp := fmt.Sprintf("%06d", n.Int64())

	if a.cacheClient != nil {
		key := fmt.Sprintf("otp:%s", req.ID.String())
		if err := a.cacheClient.Set(ctx, key, otp, otpTTL); err != nil {
			a.log.Warn("GenerateOtpActivity failed to store OTP in cache",
				zap.String("notif_id", req.ID.String()),
				zap.Error(err),
			)
		}
	}

	return otp, nil
}
