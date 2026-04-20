package workflow

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/spidey/notification-service/internal/cache"
	"github.com/spidey/notification-service/internal/domain"
	"github.com/spidey/notification-service/internal/pubsub"
	"github.com/spidey/notification-service/internal/repository"
)

type TemplateRenderer interface {
	RenderString(tmpl string, vars map[string]string) string
}

type Activities struct {
	cacheClient    *cache.Client
	templateRepo   *repository.TemplateRepository
	notifRepo      *repository.NotificationRepository
	eventRepo      *repository.EventRepository
	templateRenderer TemplateRenderer
	pubsub         pubsub.Publisher
	govRepo        *repository.GovernanceRepository
}

func NewActivities(
	cacheClient *cache.Client,
	templateRepo *repository.TemplateRepository,
	notifRepo *repository.NotificationRepository,
	eventRepo *repository.EventRepository,
	templateRenderer TemplateRenderer,
	pubsub pubsub.Publisher,
	govRepo *repository.GovernanceRepository,
) *Activities {
	return &Activities{
		cacheClient:    cacheClient,
		templateRepo:   templateRepo,
		notifRepo:      notifRepo,
		eventRepo:      eventRepo,
		templateRenderer: templateRenderer,
		pubsub:         pubsub,
		govRepo:        govRepo,
	}
}

// RenderedNotification represents the final message payload.
type RenderedNotification struct {
	ID        uuid.UUID
	UserID    uuid.UUID
	Channel   domain.Channel
	Recipient string
	Payload   []byte
}

func (a *Activities) CheckPreferencesActivity(ctx context.Context, req *WorkflowRequest) (*domain.UserPreferences, error) {
	// 1. Governance Check: Suppression List
	stype := domain.SuppressionTypeEmail
	if req.Channel == domain.ChannelSMS {
		stype = domain.SuppressionTypeSMS
	}
	suppressed, err := a.govRepo.IsSuppressed(ctx, stype, req.Recipient)
	if err != nil {
		return nil, fmt.Errorf("checking suppression: %w", err)
	}
	if suppressed {
		return &domain.UserPreferences{IsSuppressed: true}, nil
	}

	// 2. Governance Check: Hard Opt-out
	optedOut, err := a.govRepo.IsOptedOut(ctx, uuid.MustParse(req.UserID), req.Channel)
	if err != nil {
		return nil, fmt.Errorf("checking opt-out: %w", err)
	}
	if optedOut {
		return &domain.UserPreferences{IsSuppressed: true}, nil
	}

	// 3. User Preferences Check (Redis)
	var prefs domain.UserPreferences
	key := fmt.Sprintf("prefs:user:%s", req.UserID)
	if err := a.cacheClient.Get(ctx, key, &prefs); err != nil {
		// return default if not found
		return &domain.UserPreferences{
			UserID:    req.UserID,
			Channels:  map[domain.Channel]bool{},
			UpdatedAt: time.Now(),
		}, nil
	}
	return &prefs, nil
}

func (a *Activities) RenderTemplateActivity(ctx context.Context, req *WorkflowRequest) (*RenderedNotification, error) {
	if req.TemplateID == nil {
		return &RenderedNotification{
			ID:        req.ID,
			UserID:    uuid.MustParse(req.UserID),
			Channel:   req.Channel,
			Recipient: req.Recipient,
			Payload:   []byte(req.Type), // fallback
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

	rendered := a.templateRenderer.RenderString(tmpl.Body, req.TemplateVariables)

	return &RenderedNotification{
		ID:        req.ID,
		UserID:    uuid.MustParse(req.UserID),
		Channel:   req.Channel,
		Recipient: req.Recipient,
		Payload:   []byte(rendered),
	}, nil
}

func (a *Activities) PublishToPubSubActivity(ctx context.Context, rendered *RenderedNotification) (string, error) {
	msg := &pubsub.Message{
		NotificationID: rendered.ID.String(),
		Channel:        string(rendered.Channel),
		UserID:         rendered.UserID.String(),
		Recipient:      rendered.Recipient,
		Payload:        map[string]string{"body": string(rendered.Payload)},
	}
	serverMsgID, err := a.pubsub.Publish(ctx, string(rendered.Channel), msg)
	if err != nil {
		return "", err
	}
	return serverMsgID, nil
}

type LogEntry struct {
	NotificationID uuid.UUID
	MsgID          string
	Channel        string
	Status         domain.NotificationStatus
}

func (a *Activities) LogDeliveryActivity(ctx context.Context, entry LogEntry) error {
	err := a.notifRepo.UpdateStatus(ctx, entry.NotificationID, entry.Status)
	if err != nil {
		return err
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
	}

	_ = a.eventRepo.Append(ctx, &domain.NotificationEvent{
		ID:             uuid.New(),
		NotificationID: entry.NotificationID,
		EventType:      eventType,
		Metadata:       map[string]any{"msg_id": entry.MsgID, "layer": "cadence_workflow"},
		CreatedAt:      time.Now(),
	})
	return nil
}

func (a *Activities) GenerateOtpActivity(ctx context.Context, req *WorkflowRequest) (string, error) {
    // Basic Mock OTP for demonstration
    otp := "123456"
	return otp, nil
}
