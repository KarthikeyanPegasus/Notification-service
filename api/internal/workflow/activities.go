package workflow

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/spidey/notification-service/internal/cache"
	"github.com/spidey/notification-service/internal/config"
	"github.com/spidey/notification-service/internal/domain"
	"github.com/spidey/notification-service/internal/pubsub"
	"github.com/spidey/notification-service/internal/ratelimit"
	"github.com/spidey/notification-service/internal/repository"
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
	templateRenderer TemplateRenderer
	deliverySvc      DeliveryProvider
	pubsub           pubsub.Publisher
	govRepo          *repository.GovernanceRepository
	cfg              *config.Config
	vendorRepo       config.Repository
	rateLimitRepo    repository.VendorRateLimitRepository
	rateLimiter      *ratelimit.VendorLimiter
}

func NewActivities(
	cacheClient *cache.Client,
	templateRepo *repository.TemplateRepository,
	notifRepo *repository.NotificationRepository,
	schedRepo *repository.ScheduledRepository,
	eventRepo *repository.EventRepository,
	attemptRepo *repository.AttemptRepository,
	templateRenderer TemplateRenderer,
	deliverySvc DeliveryProvider,
	pubsub pubsub.Publisher,
	govRepo *repository.GovernanceRepository,
	cfg *config.Config,
	vendorRepo config.Repository,
	rateLimitRepo repository.VendorRateLimitRepository,
	rateLimiter *ratelimit.VendorLimiter,
) *Activities {
	return &Activities{
		cacheClient:      cacheClient,
		templateRepo:     templateRepo,
		notifRepo:        notifRepo,
		schedRepo:        schedRepo,
		eventRepo:        eventRepo,
		attemptRepo:      attemptRepo,
		templateRenderer: templateRenderer,
		deliverySvc:      deliverySvc,
		pubsub:           pubsub,
		govRepo:          govRepo,
		cfg:              cfg,
		vendorRepo:       vendorRepo,
		rateLimitRepo:    rateLimitRepo,
		rateLimiter:      rateLimiter,
	}
}

// RenderedNotification represents the final message payload.
type RenderedNotification struct {
	ID        uuid.UUID
	Channel   domain.Channel
	Recipient string
	Payload   []byte // plain text body
	Subject      string
	HTML         string
	ForcedVendor string
	Priority  domain.Priority
	ClientID  string
}

func (a *Activities) CheckPreferencesActivity(ctx context.Context, req *WorkflowRequest) (*domain.UserPreferences, error) {
	start := time.Now()
	var deadline string
	if dl, ok := ctx.Deadline(); ok {
		deadline = dl.UTC().Format(time.RFC3339Nano)
	}
	log.Printf("[activity] CheckPreferencesActivity start notif_id=%s type=%q channel=%s recipient=%s deadline=%s",
		req.ID.String(), req.Type, string(req.Channel), req.Recipient, deadline,
	)
	defer func() {
		log.Printf("[activity] CheckPreferencesActivity done notif_id=%s took=%s", req.ID.String(), time.Since(start))
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
			log.Printf("[activity] CheckPreferencesActivity suppression check error notif_id=%s err=%v", req.ID.String(), err)
			return nil, fmt.Errorf("checking suppression: %w", err)
		}
		if suppressed {
			log.Printf("[activity] CheckPreferencesActivity recipient suppressed notif_id=%s channel=%s", req.ID.String(), req.Channel)
			return &domain.UserPreferences{IsSuppressed: true}, nil
		}
	}

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
		// Ensure n.APIKeyID is set on the object we pass to delivery
		n.APIKeyID = dbNotif.APIKeyID
	}

	// 3. Enforce vendor rate limit before calling the provider.
	// If throttled, return a retryable error so Temporal retries within the SLA window.
	if a.rateLimiter != nil && a.rateLimitRepo != nil {
		vendorName := n.ForcedVendor
		if vendorName == "" {
			vendorName = string(n.Channel) // fallback to channel name for rate limit key
		}
		clientIDStr := ""
		if n.APIKeyID != nil {
			clientIDStr = n.APIKeyID.String()
		}
		// Try scoped rate limit first; fall back to global
		rl, _ := a.rateLimitRepo.Get(ctx, vendorName, &clientIDStr)
		if rl == nil {
			rl, _ = a.rateLimitRepo.Get(ctx, vendorName, nil)
		}
		if rl != nil {
			allowed, err := a.rateLimiter.Allow(ctx, vendorName, clientIDStr, rl)
			if err != nil {
				log.Printf("[activity] DeliverNotificationActivity rate-limit check error vendor=%s err=%v — proceeding", vendorName, err)
			} else if !allowed {
				return domain.DeliveryResult{}, fmt.Errorf("vendor %q rate limited — Temporal will retry within SLA window", vendorName)
			}
		}
	}

	// 4. Perform actual delivery
	var result domain.DeliveryResult
	var sendErr error

	if cfg != a.cfg {
		// Use scoped delivery
		result, sendErr = a.deliverySvc.DeliverScoped(ctx, n, cfg.Providers)
	} else {
		// Use global delivery
		result, sendErr = a.deliverySvc.Deliver(ctx, n)
	}

	log.Printf("[activity] DeliverNotificationActivity notif_id=%s provider=%s success=%v forced_vendor=%q err=%v",
		n.ID.String(),
		result.Provider,
		result.Success,
		n.ForcedVendor,
		sendErr,
	)

	// 4. Record attempt so the UI can display delivery data and enable status sync.
	if a.attemptRepo != nil {
		_ = a.attemptRepo.RecordAttemptFromResult(ctx, n.ID, 1, result)
	}

	_ = a.notifRepo.UpdateStatus(ctx, n.ID, domain.StatusSent)
	if !result.Success {
		_ = a.notifRepo.UpdateStatus(ctx, n.ID, domain.StatusFailed)
	}

	return result, sendErr
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

	// Sync scheduled_notifications.status so the Scheduled page reflects delivery.
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
		CreatedAt:      time.Now(),
	})
	return nil
}

func firstNonEmpty(v string, fallback string) string {
	if strings.TrimSpace(v) != "" {
		return v
	}
	return fallback
}

func (a *Activities) GenerateOtpActivity(ctx context.Context, req *WorkflowRequest) (string, error) {
    // Basic Mock OTP for demonstration
    otp := "123456"
	return otp, nil
}
