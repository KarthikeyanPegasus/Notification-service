package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/spidey/notification-service/internal/config"
	"github.com/spidey/notification-service/internal/domain"
	"github.com/spidey/notification-service/internal/provider"
	"github.com/spidey/notification-service/internal/repository"
	"go.temporal.io/api/enums/v1"
	"go.temporal.io/sdk/client"
	"github.com/spidey/notification-service/internal/pubsub"
	"github.com/spidey/notification-service/internal/workflow"
	"go.uber.org/zap"
)

// NotificationService orchestrates the notification send flow.
type NotificationService struct {
	notifRepo   *repository.NotificationRepository
	schedRepo   *repository.ScheduledRepository
	eventRepo   *repository.EventRepository
	attemptRepo *repository.AttemptRepository
	templateSvc *TemplateService
	prefsSvc    *PreferencesService
	temporalCli client.Client
	publisher   pubsub.Publisher
	cfg         *config.Config
	log         *zap.Logger
}

func NewNotificationService(
	notifRepo *repository.NotificationRepository,
	schedRepo *repository.ScheduledRepository,
	eventRepo *repository.EventRepository,
	attemptRepo *repository.AttemptRepository,
	templateSvc *TemplateService,
	prefsSvc *PreferencesService,
	temporalCli client.Client,
	publisher pubsub.Publisher,
	cfg *config.Config,
	log *zap.Logger,
) *NotificationService {
	return &NotificationService{
		notifRepo:   notifRepo,
		schedRepo:   schedRepo,
		eventRepo:   eventRepo,
		attemptRepo: attemptRepo,
		templateSvc: templateSvc,
		prefsSvc:    prefsSvc,
		temporalCli: temporalCli,
		publisher:   publisher,
		cfg:         cfg,
		log:         log,
	}
}

// SendResponse is returned after accepting a notification request.
type SendResponse struct {
	NotificationID string     `json:"notification_id"`
	Status         string     `json:"status"`
	WorkflowID     string     `json:"workflow_id,omitempty"`
	ScheduledAt    *time.Time `json:"scheduled_at,omitempty"`
}

// Send validates and enqueues a notification for delivery.
func (s *NotificationService) Send(ctx context.Context, req *domain.SendRequest, source string) (*SendResponse, error) {
	// Resolve idempotency: return existing if key already used
	existing, err := s.notifRepo.GetByIdempotencyKey(ctx, req.IdempotencyKey)
	if err != nil && !errors.Is(err, domain.ErrNotFound) {
		return nil, fmt.Errorf("checking idempotency key: %w", err)
	}
	if existing != nil {
		return &SendResponse{
			NotificationID: existing.ID.String(),
			Status:         string(existing.Status),
		}, nil
	}

	userID, err := uuid.Parse(req.UserID)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid user_id: %s", domain.ErrInvalidRecipient, req.UserID)
	}

	// Check preferences for first channel (multi-channel sends dispatch per-channel)
	prefs, err := s.prefsSvc.Get(ctx, req.UserID)
	if err != nil {
		return nil, fmt.Errorf("getting user preferences: %w", err)
	}

	var results []*SendResponse
	for _, ch := range req.Channels {
		resp, err := s.sendToChannel(ctx, req, userID, ch, prefs, source)
		if err != nil {
			s.log.Warn("failed to enqueue for channel",
				zap.String("channel", string(ch)),
				zap.String("user_id", req.UserID),
				zap.Error(err),
			)
			continue
		}
		results = append(results, resp)
	}

	if len(results) == 0 {
		return nil, fmt.Errorf("all channels failed to enqueue")
	}

	return results[0], nil
}

func (s *NotificationService) sendToChannel(
	ctx context.Context,
	req *domain.SendRequest,
	userID uuid.UUID,
	ch domain.Channel,
	prefs *domain.UserPreferences,
	source string,
) (*SendResponse, error) {
	// Check opt-in
	if !prefs.IsChannelEnabled(ch) {
		return nil, fmt.Errorf("%w: channel %s", domain.ErrOptedOut, ch)
	}

	// Check DND (skip for OTP and transactional)
	if s.prefsSvc.IsInDND(prefs) && domain.PriorityFor(ch, req.Type) == domain.PriorityLow {
		return nil, fmt.Errorf("user is in DND window; promotional deferred")
	}

	// Rate limit promotional messages
	if domain.PriorityFor(ch, req.Type) == domain.PriorityLow {
		limited, err := s.prefsSvc.IsRateLimited(ctx, req.UserID, ch, req.Type)
		if err != nil {
			s.log.Warn("rate limit check error", zap.Error(err))
		} else if limited {
			return nil, fmt.Errorf("%w: channel %s type %s", domain.ErrRateLimited, ch, req.Type)
		}
	}

	// Render template
	var templateID *uuid.UUID
	if req.TemplateID != nil {
		parsed, err := uuid.Parse(*req.TemplateID)
		if err == nil {
			templateID = &parsed
		}
	}
	fallbackBody := req.Body
	if fallbackBody == "" {
		fallbackBody = req.Recipient
	}
	if ch == domain.ChannelSlack {
		// Recipient is often the Incoming Webhook URL; never use that as template/body fallback.
		if strings.Contains(fallbackBody, "hooks.slack.com") {
			fallbackBody = ""
		}
		if fallbackBody == "" && req.TemplateVariables != nil {
			if v := strings.TrimSpace(req.TemplateVariables["message"]); v != "" {
				fallbackBody = v
			} else if v := strings.TrimSpace(req.TemplateVariables["text"]); v != "" {
				fallbackBody = v
			}
		}
	}

	rendered, err := s.templateSvc.RenderForChannel(ctx, templateID, ch, req.TemplateVariables, fallbackBody)
	if err != nil {
		return nil, fmt.Errorf("rendering template: %w", err)
	}

	priority := domain.PriorityFor(ch, req.Type)
	now := time.Now()
	notifID := uuid.New()

	// Idempotency key is per-channel to allow the same request across channels
	idemKey := fmt.Sprintf("%s:%s", req.IdempotencyKey, ch)

	n := &domain.Notification{
		ID:              notifID,
		IdempotencyKey:  idemKey,
		UserID:          userID,
		Channel:         ch,
		Priority:        priority,
		Type:            req.Type,
		TemplateID:      templateID,
		RenderedContent: rendered,
		Recipient:       req.Recipient,
		Status:          domain.StatusPending,
		ScheduledAt:     req.ScheduledAt,
		Source:          source,
		CreatedAt:       now,
		UpdatedAt:       now,
	}

	if err := s.notifRepo.Create(ctx, n); err != nil {
		if errors.Is(err, domain.ErrAlreadyExists) {
			return &SendResponse{NotificationID: notifID.String(), Status: string(domain.StatusQueued)}, nil
		}
		return nil, fmt.Errorf("persisting notification: %w", err)
	}

	// Emit queued event
	_ = s.eventRepo.Append(ctx, &domain.NotificationEvent{
		ID:             uuid.New(),
		NotificationID: notifID,
		EventType:      domain.EventQueued,
		CreatedAt:      now,
	})

	if req.ScheduledAt != nil && req.ScheduledAt.After(now) {
		return s.handleScheduled(ctx, n, req)
	}

	return s.publishImmediate(ctx, n)
}

func (s *NotificationService) publishImmediate(ctx context.Context, n *domain.Notification) (*SendResponse, error) {
	// Execute Temporal Workflow if available
	if s.temporalCli != nil {
		options := client.StartWorkflowOptions{
			ID:        fmt.Sprintf("notif-%s", n.ID.String()),
			TaskQueue: "notification-default",
		}

		workflowFunc := workflow.NotificationWorkflow

		req := &workflow.WorkflowRequest{
			ID:                n.ID,
			UserID:            n.UserID.String(),
			Channel:           n.Channel,
			Recipient:         n.Recipient,
			Type:              n.Type,
			TemplateVariables: n.RenderedContent.Data,
			IdempotencyKey:    n.IdempotencyKey,
		}
		if n.TemplateID != nil {
			tid := n.TemplateID.String()
			req.TemplateID = &tid
		}

		run, err := s.temporalCli.ExecuteWorkflow(ctx, options, workflowFunc, req)
		if err != nil {
			_ = s.notifRepo.UpdateStatus(ctx, n.ID, domain.StatusFailed)
			return nil, fmt.Errorf("starting temporal workflow execution: %w", err)
		}

		_ = s.notifRepo.UpdateStatus(ctx, n.ID, domain.StatusQueued)

		s.log.Info("temporal workflow running",
			zap.String("notification_id", n.ID.String()),
			zap.String("workflow_id", run.GetID()),
			zap.String("run_id", run.GetRunID()),
		)

		return &SendResponse{
			NotificationID: n.ID.String(),
			Status:         string(domain.StatusQueued),
			WorkflowID:     run.GetID(),
		}, nil
	}

	// Fallback to direct PubSub publishing in standalone mode
	msg := &pubsub.Message{
		NotificationID: n.ID.String(),
		Channel:        string(n.Channel),
		UserID:         n.UserID.String(),
		Recipient:      n.Recipient,
		Priority:       string(n.Priority),
		Type:           n.Type,
		IdempotencyKey: n.IdempotencyKey,
	}
	if n.TemplateID != nil {
		msg.TemplateID = n.TemplateID.String()
	}
	if n.RenderedContent != nil {
		msg.Payload = n.RenderedContent.Data
	}

	_, err := s.publisher.Publish(ctx, string(n.Channel), msg)
	if err != nil {
		_ = s.notifRepo.UpdateStatus(ctx, n.ID, domain.StatusFailed)
		return nil, fmt.Errorf("direct publishing failed: %w", err)
	}

	_ = s.notifRepo.UpdateStatus(ctx, n.ID, domain.StatusQueued)
	s.log.Info("notification published directly to pubsub (standalone mode)",
		zap.String("notification_id", n.ID.String()),
		zap.String("channel", string(n.Channel)),
	)

	return &SendResponse{
		NotificationID: n.ID.String(),
		Status:         string(domain.StatusQueued),
	}, nil
}

func (s *NotificationService) handleScheduled(ctx context.Context, n *domain.Notification, initialReq *domain.SendRequest) (*SendResponse, error) {
	now := time.Now()
	workflowID := fmt.Sprintf("sched-notif-%s", n.ID.String())

	var templateID *uuid.UUID
	if initialReq.TemplateID != nil {
		parsed, err := uuid.Parse(*initialReq.TemplateID)
		if err == nil {
			templateID = &parsed
		}
	}

	delaySeconds := int(time.Until(*initialReq.ScheduledAt).Seconds())
	if delaySeconds < 0 {
		return nil, errors.New("deliverAt is in the past")
	}

	req := &workflow.WorkflowRequest{
		ID:                n.ID,
		UserID:            n.UserID.String(),
		Channel:           n.Channel,
		Recipient:         n.Recipient,
		Type:              n.Type,
		TemplateVariables: initialReq.TemplateVariables,
		IdempotencyKey:    n.IdempotencyKey,
	}
	if n.TemplateID != nil {
		tid := n.TemplateID.String()
		req.TemplateID = &tid
	}

	options := client.StartWorkflowOptions{
		ID:                                   workflowID,
		TaskQueue:                            "notification-default",
		WorkflowIDReusePolicy:                enums.WORKFLOW_ID_REUSE_POLICY_ALLOW_DUPLICATE,
		WorkflowExecutionErrorWhenAlreadyStarted: true,
	}

	// Wait, temporal v1.x doesn't use DelayStartSeconds directly in StartWorkflowOptions if it's too old
	// Actually yes, go.temporal.io/sdk/client v1.22+ has it! 
	// We'll use workflow.Sleep in a wrapper if needed, but we'll try options.WorkflowExecutionTimeout instead,
	// wait, StartDelay was added in v1.23+ which maps to DelayStartSeconds. 
	// To be perfectly safe, Temporal Go SDK calls it "StartDelay" in `client.StartWorkflowOptions`.
	
	options.StartDelay = time.Duration(delaySeconds) * time.Second

	run, err := s.temporalCli.ExecuteWorkflow(ctx, options, workflow.NotificationWorkflow, req)
	if err != nil {
		return nil, fmt.Errorf("scheduling temporal workflow: %w", err)
	}

	sched := &domain.ScheduledNotification{
		ID:              uuid.New(),
		NotificationID:  n.ID,
		UserID:          n.UserID,
		Channel:         n.Channel,
		TemplateID:      templateID,
		TemplateVars:    initialReq.TemplateVariables,
		ScheduledAt:     *initialReq.ScheduledAt,
		OriginalAt:      *initialReq.ScheduledAt,
		WorkflowID:      workflowID,
		RunID:           run.GetRunID(),
		Status:          domain.StatusPending,
		RescheduleCount: 0,
		CreatedAt:       now,
		UpdatedAt:       now,
	}

	if err := s.schedRepo.Create(ctx, sched); err != nil {
		return nil, fmt.Errorf("persisting scheduled notification: %w", err)
	}

	return &SendResponse{
		NotificationID: n.ID.String(),
		Status:         string(domain.StatusPending),
		WorkflowID:     workflowID,
		ScheduledAt:    initialReq.ScheduledAt,
	}, nil
}

// GetByID returns a notification with its attempts and events.
func (s *NotificationService) GetByID(ctx context.Context, id uuid.UUID) (*domain.Notification, []*domain.NotificationAttempt, []*domain.NotificationEvent, error) {
	n, err := s.notifRepo.GetByID(ctx, id)
	if err != nil {
		return nil, nil, nil, err
	}

	attempts, err := s.attemptRepo.ListByNotificationID(ctx, id)
	if err != nil {
		return nil, nil, nil, err
	}

	events, err := s.eventRepo.ListByNotificationID(ctx, id)
	if err != nil {
		return nil, nil, nil, err
	}

	return n, attempts, events, nil
}

// List returns a paginated list of notifications.
func (s *NotificationService) List(ctx context.Context, f repository.ListFilters) ([]*domain.Notification, int64, error) {
	return s.notifRepo.List(ctx, f)
}

// RecordDelivery records a successful or failed delivery attempt.
func (s *NotificationService) RecordDelivery(ctx context.Context, notifID uuid.UUID, attemptNum int, result domain.DeliveryResult) error {
	if err := s.attemptRepo.RecordAttemptFromResult(ctx, notifID, attemptNum, result); err != nil {
		return err
	}

	eventType := domain.EventFailed
	status := domain.StatusFailed
	if result.Success {
		eventType = domain.EventSent
		status = domain.StatusSent
	}

	_ = s.eventRepo.Append(ctx, &domain.NotificationEvent{
		ID:             uuid.New(),
		NotificationID: notifID,
		EventType:      eventType,
		Metadata: map[string]any{
			"provider":   result.Provider,
			"latency_ms": result.LatencyMs,
		},
		CreatedAt: time.Now(),
	})

	return s.notifRepo.UpdateStatus(ctx, notifID, status)
}

// SyncStatus fetches the latest status from the provider and updates the local record.
func (s *NotificationService) SyncStatus(ctx context.Context, id uuid.UUID) (*domain.DeliveryResult, error) {
	n, err := s.notifRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	attempts, err := s.attemptRepo.ListByNotificationID(ctx, id)
	if err != nil {
		return nil, err
	}

	if len(attempts) == 0 {
		return nil, fmt.Errorf("no attempts found for notification %s", id)
	}

	// Use the latest attempt
	latest := attempts[len(attempts)-1]
	if latest.ProviderMsgID == nil || *latest.ProviderMsgID == "" {
		return nil, fmt.Errorf("latest attempt has no provider message ID")
	}

	// Resolve provider
	sender, err := s.getSenderForProvider(latest.Provider, n.Channel)
	if err != nil {
		return nil, err
	}

	result, err := sender.GetStatus(ctx, *latest.ProviderMsgID)
	if err != nil {
		return nil, fmt.Errorf("provider GetStatus error: %w", err)
	}

	// If status changed or sync performed, record it
	_ = s.eventRepo.Append(ctx, &domain.NotificationEvent{
		ID:             uuid.New(),
		NotificationID: n.ID,
		EventType:      "provider_sync",
		Metadata: map[string]any{
			"provider":      latest.Provider,
			"vendor_status": result.ErrorMessage, // We stored external status here
			"success":       result.Success,
		},
		CreatedAt: time.Now(),
	})

	// Mapping logic for StatusDelivered etc. can be added here
	if result.ErrorMessage == "delivered" {
		_ = s.notifRepo.UpdateStatus(ctx, n.ID, domain.StatusDelivered)
	}

	return &result, nil
}

func (s *NotificationService) getSenderForProvider(name string, channel domain.Channel) (provider.Sender, error) {
	// This is a simplified dynamic lookup. In a real app, use a proper factory/registry.
	switch channel {
	case domain.ChannelSMS:
		senders := provider.InitializeSMSSenders(s.cfg.Providers.SMS)
		for _, snd := range senders {
			if snd.ProviderName() == name {
				return snd, nil
			}
		}
	case domain.ChannelEmail:
		senders := provider.InitializeEmailSenders(context.Background(), s.cfg.Providers.Email)
		for _, snd := range senders {
			if snd.ProviderName() == name {
				return snd, nil
			}
		}
	case domain.ChannelSlack:
		if name == "slack" {
			return provider.InitializeSlackSender(s.cfg.Providers.Slack), nil
		}
	}
	return nil, fmt.Errorf("provider %s not found for channel %s", name, channel)
}
