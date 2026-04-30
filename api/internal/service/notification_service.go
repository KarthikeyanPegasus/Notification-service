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
	"github.com/spidey/notification-service/internal/pubsub"
	"github.com/spidey/notification-service/internal/repository"
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
	wfClients   *WorkflowClientProvider
	publisher   pubsub.Publisher
	cfg         *config.Config
	vendorRepo  config.Repository
	log         *zap.Logger
}

func NewNotificationService(
	notifRepo *repository.NotificationRepository,
	schedRepo *repository.ScheduledRepository,
	eventRepo *repository.EventRepository,
	attemptRepo *repository.AttemptRepository,
	templateSvc *TemplateService,
	prefsSvc *PreferencesService,
	wfClients *WorkflowClientProvider,
	publisher pubsub.Publisher,
	cfg *config.Config,
	vendorRepo config.Repository,
	log *zap.Logger,
) *NotificationService {
	return &NotificationService{
		notifRepo:   notifRepo,
		schedRepo:   schedRepo,
		eventRepo:   eventRepo,
		attemptRepo: attemptRepo,
		templateSvc: templateSvc,
		prefsSvc:    prefsSvc,
		wfClients:   wfClients,
		publisher:   publisher,
		cfg:         cfg,
		vendorRepo:  vendorRepo,
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
func (s *NotificationService) Send(ctx context.Context, req *domain.SendRequest, source string, apiKeyID *uuid.UUID) (*SendResponse, error) {
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

	// user_id is optional and treated as opaque metadata for preferences/rate-limits.
	// Notifications are accepted purely based on API key auth and request validity.
	var userID *uuid.UUID = nil

	// Check preferences for first channel (multi-channel sends dispatch per-channel)
	prefs, err := s.prefsSvc.Get(ctx, req.UserID)
	if err != nil {
		return nil, fmt.Errorf("getting user preferences: %w", err)
	}

	var results []*SendResponse
	for _, ch := range req.Channels {
		resp, err := s.sendToChannel(ctx, req, userID, ch, prefs, source, apiKeyID)
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
	userID *uuid.UUID,
	ch domain.Channel,
	prefs *domain.UserPreferences,
	source string,
	apiKeyID *uuid.UUID,
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

	rendered, err := s.templateSvc.RenderForChannel(ctx, templateID, ch, req.TemplateVariables, fallbackBody, req.Subject, req.HTML)
	if err != nil {
		return nil, fmt.Errorf("rendering template: %w", err)
	}

	priority := domain.PriorityFor(ch, req.Type)
	now := time.Now()
	notifID := uuid.New()

	// Idempotency key is per-channel to allow the same request across channels
	idemKey := fmt.Sprintf("%s:%s", req.IdempotencyKey, ch)

	recipient := req.Recipient
	if ch == domain.ChannelSlack && strings.TrimSpace(req.SlackChannel) != "" {
		// For Slack, allow addressing a configured channel name which will be resolved
		// to a webhook URL by the Slack provider based on vendor config.
		recipient = strings.TrimSpace(req.SlackChannel)
	}

	n := &domain.Notification{
		ID:              notifID,
		IdempotencyKey:  idemKey,
		UserID:          userID,
		Channel:         ch,
		Priority:        priority,
		Type:            req.Type,
		TemplateID:      templateID,
		RenderedContent: rendered,
		Recipient:       recipient,
		Status:          domain.StatusPending,
		ScheduledAt:     req.ScheduledAt,
		APIKeyID:        apiKeyID,
		Source:          source,
		ForcedVendor:    req.ForcedVendor,
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
	var apiKeyID string
	if n.APIKeyID != nil {
		apiKeyID = n.APIKeyID.String()
	}
	cli, err := s.wfClients.ClientForScope(ctx, &apiKeyID)
	if err != nil {
		s.log.Warn("workflow orchestration unavailable; falling back to direct delivery", zap.Error(err))
		cli = nil
	}
	// Execute Workflow if available
	if cli != nil {
		clientID := ""
		if n.APIKeyID != nil {
			clientID = n.APIKeyID.String()
		}
		taskQueue := workflow.TaskQueueFor(n.Channel, n.Priority, clientID)
		options := workflow.StartOptions{
			ID:        fmt.Sprintf("notif-%s", n.ID.String()),
			TaskQueue: taskQueue,
		}

		var workflowFunc any = workflow.NotificationWorkflow
		if cli.ProviderName() == "cadence" {
			workflowFunc = workflow.NotificationWorkflowCadence
		}

		req := &workflow.WorkflowRequest{
			ID:             n.ID,
			Channel:        n.Channel,
			Priority:       n.Priority,
			Recipient:      n.Recipient,
			Type:           n.Type,
			IdempotencyKey: n.IdempotencyKey,
			ForcedVendor:   n.ForcedVendor,
			ClientID:       clientID,
		}
		if n.TemplateID != nil {
			tid := n.TemplateID.String()
			req.TemplateID = &tid
			if n.RenderedContent != nil {
				req.TemplateVariables = n.RenderedContent.Data
			}
		} else if n.RenderedContent != nil {
			req.DirectContent = &workflow.DirectContent{
				Subject: n.RenderedContent.Subject,
				Body:    n.RenderedContent.Body,
				HTML:    n.RenderedContent.HTML,
			}
		}

		// Use a shorter timeout for starting the workflow to avoid hanging the UI
		startCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		run, err := cli.ExecuteWorkflow(startCtx, options, workflowFunc, req)
		cancel()
		if err != nil {
			s.log.Error("failed to start workflow; falling back to direct delivery", zap.Error(err))
			// Continue to direct delivery below
		} else {
			_ = s.notifRepo.UpdateStatus(ctx, n.ID, domain.StatusQueued)

			s.log.Info("workflow running",
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
	}

	// Publish to the priority-specific Kafka topic so the Dispatcher can route
	// the message to the correct client-scoped Temporal task queue.
	var clientIDStr string
	if n.APIKeyID != nil {
		clientIDStr = n.APIKeyID.String()
	}
	msg := &pubsub.Message{
		NotificationID: n.ID.String(),
		Channel:        string(n.Channel),
		ClientID:       clientIDStr,
		Recipient:      n.Recipient,
		Priority:       string(n.Priority),
		Type:           n.Type,
		IdempotencyKey: n.IdempotencyKey,
		ForcedVendor:   n.ForcedVendor,
	}
	if n.TemplateID != nil {
		msg.TemplateID = n.TemplateID.String()
	}
	if n.RenderedContent != nil {
		msg.Payload = n.RenderedContent.Data
	}

	// Route to the priority-specific topic: notifications-{channel}-{priority}
	topicKey := pubsub.PriorityTopicKey(string(n.Channel), string(n.Priority))
	_, err = s.publisher.Publish(ctx, topicKey, msg)
	if err != nil {
		_ = s.notifRepo.UpdateStatus(ctx, n.ID, domain.StatusFailed)
		return nil, fmt.Errorf("direct publishing failed: %w", err)
	}

	_ = s.notifRepo.UpdateStatus(ctx, n.ID, domain.StatusQueued)
	s.log.Info("notification published to priority topic (standalone mode)",
		zap.String("notification_id", n.ID.String()),
		zap.String("channel", string(n.Channel)),
		zap.String("priority", string(n.Priority)),
		zap.String("topic_key", topicKey),
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
		ID:             n.ID,
		Channel:        n.Channel,
		Priority:       n.Priority,
		Recipient:      n.Recipient,
		Type:           n.Type,
		IdempotencyKey: n.IdempotencyKey,
		ForcedVendor:   n.ForcedVendor,
	}
	if n.TemplateID != nil {
		tid := n.TemplateID.String()
		req.TemplateID = &tid
		req.TemplateVariables = initialReq.TemplateVariables
	} else if n.RenderedContent != nil {
		req.DirectContent = &workflow.DirectContent{
			Subject: n.RenderedContent.Subject,
			Body:    n.RenderedContent.Body,
			HTML:    n.RenderedContent.HTML,
		}
	}

	// Resolve the workflow engine: prefer explicit client_id from the request,
	// then fall back to the API key that authenticated this call.
	scopeID := ""
	var schedAPIKeyID *uuid.UUID
	if initialReq.ClientID != nil && *initialReq.ClientID != "" {
		scopeID = *initialReq.ClientID
		if parsed, err := uuid.Parse(scopeID); err == nil {
			schedAPIKeyID = &parsed
		}
	} else if n.APIKeyID != nil {
		scopeID = n.APIKeyID.String()
		schedAPIKeyID = n.APIKeyID
	}
	req.ClientID = scopeID

	options := workflow.StartOptions{
		ID:                                       workflowID,
		TaskQueue:                                workflow.TaskQueueFor(n.Channel, n.Priority, scopeID),
		WorkflowIDReusePolicy:                    workflow.IDReusePolicyAllowDuplicate,
		WorkflowExecutionErrorWhenAlreadyStarted: true,
		StartDelay:                               time.Duration(delaySeconds) * time.Second,
	}

	cli, err := s.wfClients.ClientForScope(ctx, &scopeID)
	if err != nil {
		s.log.Warn("workflow client lookup failed; using standalone scheduler fallback", zap.Error(err))
		cli = nil
	}

	newSchedRecord := func(runID string) *domain.ScheduledNotification {
		return &domain.ScheduledNotification{
			ID:              uuid.New(),
			NotificationID:  n.ID,
			Channel:         n.Channel,
			TemplateID:      templateID,
			TemplateVars:    initialReq.TemplateVariables,
			ScheduledAt:     *initialReq.ScheduledAt,
			OriginalAt:      *initialReq.ScheduledAt,
			WorkflowID:      workflowID,
			RunID:           runID,
			APIKeyID:        schedAPIKeyID,
			Status:          domain.StatusPending,
			RescheduleCount: 0,
			CreatedAt:       now,
			UpdatedAt:       now,
		}
	}

	standaloneSchedule := func(logMsg string) (*SendResponse, error) {
		sched := newSchedRecord("")
		if err := s.schedRepo.Create(ctx, sched); err != nil {
			return nil, fmt.Errorf("persisting scheduled notification: %w", err)
		}
		delay := time.Until(*initialReq.ScheduledAt)
		nCopy := *n
		go func() {
			time.Sleep(delay)
			if _, pubErr := s.publishImmediate(context.Background(), &nCopy); pubErr != nil {
				s.log.Error("standalone scheduled delivery failed",
					zap.String("notification_id", nCopy.ID.String()),
					zap.Error(pubErr),
				)
				_ = s.schedRepo.UpdateStatus(context.Background(), nCopy.ID, domain.StatusFailed)
				return
			}
			_ = s.schedRepo.UpdateStatus(context.Background(), nCopy.ID, domain.StatusSent)
		}()
		s.log.Info(logMsg,
			zap.String("notification_id", n.ID.String()),
			zap.Duration("delay", delay),
		)
		return &SendResponse{
			NotificationID: n.ID.String(),
			Status:         string(domain.StatusPending),
			WorkflowID:     workflowID,
			ScheduledAt:    initialReq.ScheduledAt,
		}, nil
	}

	// Standalone fallback: no Temporal/Cadence configured for this scope.
	if cli == nil {
		return standaloneSchedule("notification scheduled (standalone mode)")
	}

	var workflowFunc any = workflow.NotificationWorkflow
	if cli.ProviderName() == "cadence" {
		workflowFunc = workflow.NotificationWorkflowCadence
	}
	run, err := cli.ExecuteWorkflow(ctx, options, workflowFunc, req)
	if err != nil {
		// Temporal/Cadence is configured but unreachable — fall back to standalone scheduling.
		s.log.Warn("workflow scheduling failed; falling back to standalone scheduler",
			zap.String("notification_id", n.ID.String()),
			zap.Error(err),
		)
		return standaloneSchedule("notification scheduled (standalone fallback after workflow error)")
	}

	sched := newSchedRecord(run.GetRunID())
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

// GetByID returns a notification with its attempts, events, and an optional live provider status.
// For non-final statuses, it polls the provider (with a 5 s timeout) and updates the DB if the
// status changed. The live result is returned as the fourth value; it is nil when polling is
// skipped (already final or no providerMsgID available).
func (s *NotificationService) GetByID(ctx context.Context, id uuid.UUID) (*domain.Notification, []*domain.NotificationAttempt, []*domain.NotificationEvent, *domain.DeliveryResult, error) {
	n, err := s.notifRepo.GetByID(ctx, id)
	if err != nil {
		return nil, nil, nil, nil, err
	}

	attempts, err := s.attemptRepo.ListByNotificationID(ctx, id)
	if err != nil {
		return nil, nil, nil, nil, err
	}

	events, err := s.eventRepo.ListByNotificationID(ctx, id)
	if err != nil {
		return nil, nil, nil, nil, err
	}

	// Auto-poll provider for non-final statuses that have a provider message ID.
	var liveResult *domain.DeliveryResult
	if !isFinalStatus(n.Status) {
		if result, updated := s.pollAndUpdate(ctx, n, attempts); result != nil {
			liveResult = result
			if updated {
				// Refresh notification and events after DB update
				if fresh, err2 := s.notifRepo.GetByID(ctx, id); err2 == nil {
					n = fresh
				}
				if freshEvts, err2 := s.eventRepo.ListByNotificationID(ctx, id); err2 == nil {
					events = freshEvts
				}
			}
		}
	}

	return n, attempts, events, liveResult, nil
}

// isFinalStatus returns true for statuses that will never change again.
func isFinalStatus(s domain.NotificationStatus) bool {
	switch s {
	case domain.StatusDelivered, domain.StatusBounced, domain.StatusFailed,
		domain.StatusCancelled, domain.StatusSuppressed:
		return true
	}
	return false
}

// pollAndUpdate calls GetStatus() on the provider and updates the DB if the status changed.
// Returns the live DeliveryResult and whether the DB was updated.
func (s *NotificationService) pollAndUpdate(ctx context.Context, n *domain.Notification, attempts []*domain.NotificationAttempt) (*domain.DeliveryResult, bool) {
	providerMsgID, providerName := s.resolveProviderMsgID(n, attempts)
	if providerMsgID == "" {
		return nil, false
	}

	sender, err := s.getSenderForProviderScoped(ctx, n, providerName, n.Channel)
	if err != nil {
		return &domain.DeliveryResult{
			Success:       false,
			Provider:      providerName,
			ProviderMsgID: providerMsgID,
			ErrorMessage:  err.Error(),
		}, false
	}

	pollCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	result, err := sender.GetStatus(pollCtx, providerMsgID)
	if err != nil {
		// Still return a live result so the UI can show "why" (timeouts, auth, etc.).
		return &domain.DeliveryResult{
			Success:       false,
			Provider:      providerName,
			ProviderMsgID: providerMsgID,
			ErrorMessage:  err.Error(),
		}, false
	}

	// If the provider doesn't support polling (or no definitive status yet), still surface it to the UI.
	if result.Provider == "" {
		result.Provider = providerName
	}

	newStatus := vendorStatusToDomain(result.VendorStatus, result.Success)
	updated := false
	if newStatus != "" && newStatus != n.Status {
		_ = s.notifRepo.UpdateStatus(ctx, n.ID, newStatus)
		_ = s.eventRepo.Append(ctx, &domain.NotificationEvent{
			ID:             uuid.New(),
			NotificationID: n.ID,
			EventType: statusToEventType(newStatus),
			Metadata: map[string]any{
				"provider":      providerName,
				"vendor_status": result.VendorStatus,
				"new_status":    string(newStatus),
				"source":        "provider_sync",
			},
			CreatedAt: time.Now(),
		})
		updated = true
	}

	return &result, updated
}

// resolveProviderMsgID returns the provider message ID and provider name for status polling.
// It first checks attempts (populated for pubsub-path notifications), then falls back to
// event metadata where Temporal workflow activities store msg_id.
func (s *NotificationService) resolveProviderMsgID(n *domain.Notification, attempts []*domain.NotificationAttempt) (msgID string, providerName string) {
	// Prefer the most recent attempt's provider, even if msg id is missing,
	// to avoid falling back to an older vendor incorrectly.
	for i := len(attempts) - 1; i >= 0; i-- {
		a := attempts[i]
		if providerName == "" && strings.TrimSpace(a.Provider) != "" {
			providerName = a.Provider
		}
		if a.ProviderMsgID != nil && strings.TrimSpace(*a.ProviderMsgID) != "" {
			if strings.TrimSpace(a.Provider) != "" {
				providerName = a.Provider
			}
			return strings.TrimSpace(*a.ProviderMsgID), providerName
		}
	}
	if providerName != "" {
		return "", providerName
	}

	// Fallback: extract msg_id from event metadata (Temporal workflow path).
	evts, err := s.eventRepo.ListByNotificationID(context.Background(), n.ID)
	if err != nil {
		return "", ""
	}
	for i := len(evts) - 1; i >= 0; i-- {
		ev := evts[i]
		if ev.EventType != domain.EventSent && ev.EventType != domain.EventType("provider_sync") {
			continue
		}
		mid, _ := ev.Metadata["msg_id"].(string)
		if mid == "" {
			continue
		}
		// Provider name: prefer explicit metadata key, then fall back to channel default.
		pname, _ := ev.Metadata["provider"].(string)
		if pname == "" {
			pname = s.defaultProviderForChannel(n.Channel)
		}
		return mid, pname
	}

	return "", ""
}

// defaultProviderForChannel returns the first configured provider for the given channel.
func (s *NotificationService) defaultProviderForChannel(ch domain.Channel) string {
	switch ch {
	case domain.ChannelSMS:
		senders := provider.InitializeSMSSenders(s.cfg.Providers.SMS)
		if len(senders) > 0 {
			return senders[0].ProviderName()
		}
	case domain.ChannelEmail:
		senders := provider.InitializeEmailSenders(context.Background(), s.cfg.Providers.Email)
		if len(senders) > 0 {
			return senders[0].ProviderName()
		}
	}
	return ""
}

// vendorStatusToDomain maps a provider's raw status string to a domain NotificationStatus.
func vendorStatusToDomain(vs string, success bool) domain.NotificationStatus {
	switch strings.ToLower(vs) {
	case "delivered":
		return domain.StatusDelivered
	case "bounced", "bounce", "hard_bounce", "permanentfailure":
		return domain.StatusBounced
	case "failed", "undelivered", "failure", "errored", "delivery_failed":
		return domain.StatusFailed
	case "sent", "queued", "sending", "accepted", "buffered":
		return domain.StatusSent
	default:
		if success {
			return domain.StatusDelivered
		}
		return ""
	}
}

func statusToEventType(st domain.NotificationStatus) domain.EventType {
	switch st {
	case domain.StatusDelivered:
		return domain.EventDelivered
	case domain.StatusBounced:
		return domain.EventBounced
	case domain.StatusFailed:
		return domain.EventFailed
	case domain.StatusSent:
		return domain.EventSent
	default:
		return domain.EventSent
	}
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

	providerMsgID, providerName := s.resolveProviderMsgID(n, attempts)
	if providerMsgID == "" {
		return nil, domain.NewAppError(
			"NO_PROVIDER_MESSAGE_ID",
			"no provider message id found (notification has not been sent yet)",
			400,
			nil,
		)
	}

	sender, err := s.getSenderForProviderScoped(ctx, n, providerName, n.Channel)
	if err != nil {
		return &domain.DeliveryResult{
			Success:       false,
			Provider:      providerName,
			ProviderMsgID: providerMsgID,
			ErrorMessage:  err.Error(),
		}, nil
	}

	result, err := sender.GetStatus(ctx, providerMsgID)
	if err != nil {
		return &domain.DeliveryResult{
			Success:       false,
			Provider:      providerName,
			ProviderMsgID: providerMsgID,
			ErrorMessage:  err.Error(),
		}, nil
	}
	if result.Provider == "" {
		result.Provider = providerName
	}

	newStatus := vendorStatusToDomain(result.VendorStatus, result.Success)
	if newStatus != "" && newStatus != n.Status {
		_ = s.notifRepo.UpdateStatus(ctx, n.ID, newStatus)
	}

	_ = s.eventRepo.Append(ctx, &domain.NotificationEvent{
		ID:             uuid.New(),
		NotificationID: n.ID,
		EventType: statusToEventType(newStatus),
		Metadata: map[string]any{
			"provider":      providerName,
			"vendor_status": result.VendorStatus,
			"success":       result.Success,
			"error_code":    result.ErrorCode,
			"source":        "provider_sync",
		},
		CreatedAt: time.Now(),
	})

	return &result, nil
}

func (s *NotificationService) getSenderForProvider(name string, channel domain.Channel) (provider.Sender, error) {
	return s.getSenderForProviderFromConfig(context.Background(), s.cfg, name, channel)
}

func (s *NotificationService) getSenderForProviderScoped(
	ctx context.Context,
	n *domain.Notification,
	name string,
	channel domain.Channel,
) (provider.Sender, error) {
	// Apply api_key scoped vendor overrides so polling uses the same credentials used for send.
	cfg := s.cfg
	if n != nil && n.APIKeyID != nil && s.vendorRepo != nil && s.cfg != nil {
		cfgCopy := *s.cfg
		scope := n.APIKeyID.String()
		_ = cfgCopy.LoadDynamicOverridesScoped(ctx, s.vendorRepo, &scope)
		cfg = &cfgCopy
	}
	return s.getSenderForProviderFromConfig(ctx, cfg, name, channel)
}

func (s *NotificationService) getSenderForProviderFromConfig(
	ctx context.Context,
	cfg *config.Config,
	name string,
	channel domain.Channel,
) (provider.Sender, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config not available")
	}
	// This is a simplified dynamic lookup. In a real app, use a proper factory/registry.
	switch channel {
	case domain.ChannelSMS:
		senders := provider.InitializeSMSSenders(cfg.Providers.SMS)
		for _, snd := range senders {
			if snd.ProviderName() == name {
				return snd, nil
			}
		}
	case domain.ChannelEmail:
		senders := provider.InitializeEmailSenders(ctx, cfg.Providers.Email)
		for _, snd := range senders {
			if snd.ProviderName() == name {
				return snd, nil
			}
		}
	case domain.ChannelSlack:
		if name == "slack" {
			return provider.InitializeSlackSender(cfg.Providers.Slack), nil
		}
	}
	return nil, fmt.Errorf("provider %s not found for channel %s", name, channel)
}

// Retrigger re-enqueues an existing notification for delivery.
// This is intended for failed notifications or notifications stuck in queued/pending.
func (s *NotificationService) Retrigger(ctx context.Context, id uuid.UUID) error {
	n, err := s.notifRepo.GetByID(ctx, id)
	if err != nil {
		return err
	}

	// Don't allow retrigger for final states that should never be resent.
	switch n.Status {
	case domain.StatusDelivered, domain.StatusBounced, domain.StatusCancelled, domain.StatusSuppressed:
		return fmt.Errorf("cannot retrigger notification in status %s", n.Status)
	}

	now := time.Now()
	if n.ScheduledAt != nil && n.ScheduledAt.After(now) {
		return fmt.Errorf("cannot retrigger a scheduled notification before scheduled_at")
	}

	// Prefer restarting delivery via workflow engine when available.
	// In many dev setups (mock pubsub), publishing won't actually enqueue work.
	var apiKeyID string
	if n.APIKeyID != nil {
		apiKeyID = n.APIKeyID.String()
	}
	cli, wfErr := s.wfClients.ClientForScope(ctx, &apiKeyID)
	if wfErr != nil {
		s.log.Warn("retrigger: workflow orchestration lookup failed; falling back to pubsub", zap.Error(wfErr))
		cli = nil
	}
	if cli != nil {
		clientID := ""
		if n.APIKeyID != nil {
			clientID = n.APIKeyID.String()
		}
		taskQueue := workflow.TaskQueueFor(n.Channel, n.Priority, clientID)
		options := workflow.StartOptions{
			ID:                    fmt.Sprintf("notif-%s-retry-%d", n.ID.String(), time.Now().UnixNano()),
			TaskQueue:             taskQueue,
			WorkflowIDReusePolicy: workflow.IDReusePolicyAllowDuplicate,
		}

		req := &workflow.WorkflowRequest{
			ID:             n.ID,
			Channel:        n.Channel,
			Priority:       n.Priority,
			Recipient:      n.Recipient,
			Type:           n.Type,
			IdempotencyKey: n.IdempotencyKey,
			ForcedVendor:   n.ForcedVendor,
			ClientID:       clientID,
		}
		if n.TemplateID != nil {
			tid := n.TemplateID.String()
			req.TemplateID = &tid
			if n.RenderedContent != nil {
				req.TemplateVariables = n.RenderedContent.Data
			}
		} else if n.RenderedContent != nil {
			req.DirectContent = &workflow.DirectContent{
				Subject: n.RenderedContent.Subject,
				Body:    n.RenderedContent.Body,
				HTML:    n.RenderedContent.HTML,
			}
		}

		var workflowFunc any = workflow.NotificationWorkflow
		if cli.ProviderName() == "cadence" {
			workflowFunc = workflow.NotificationWorkflowCadence
		}

		startCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		_, err := cli.ExecuteWorkflow(startCtx, options, workflowFunc, req)
		cancel()
		if err == nil {
			// Record a queued event (allowed by DB constraint) and bump status back to queued.
			if err := s.eventRepo.Append(ctx, &domain.NotificationEvent{
				ID:             uuid.New(),
				NotificationID: n.ID,
				EventType:      domain.EventQueued,
				Metadata: map[string]any{
					"previous_status": string(n.Status),
					"retrigger":       true,
					"mode":            "workflow",
				},
				CreatedAt: now,
			}); err != nil {
				return fmt.Errorf("retrigger: failed to append event: %w", err)
			}
			if err := s.notifRepo.UpdateStatus(ctx, n.ID, domain.StatusQueued); err != nil {
				return fmt.Errorf("retrigger: failed to update status: %w", err)
			}
			return nil
		}

		s.log.Warn("retrigger: failed to start workflow; falling back to pubsub", zap.Error(err))
	}

	// Publish directly to Pub/Sub as a fallback (standalone mode).
	msg := &pubsub.Message{
		NotificationID: n.ID.String(),
		Channel:        string(n.Channel),
		Recipient:      n.Recipient,
		Priority:       string(n.Priority),
		Type:           n.Type,
		IdempotencyKey: n.IdempotencyKey,
		ForcedVendor:   n.ForcedVendor,
	}
	if n.TemplateID != nil {
		msg.TemplateID = n.TemplateID.String()
	}
	if n.RenderedContent != nil && len(n.RenderedContent.Data) > 0 {
		msg.Payload = n.RenderedContent.Data
	}

	if _, err := s.publisher.Publish(ctx, string(n.Channel), msg); err != nil {
		return fmt.Errorf("retrigger publish failed: %w", err)
	}

	if err := s.eventRepo.Append(ctx, &domain.NotificationEvent{
		ID:             uuid.New(),
		NotificationID: n.ID,
		// Re-use an allowed event type (DB constraint) and annotate via metadata.
		EventType: domain.EventQueued,
		Metadata: map[string]any{
			"previous_status": string(n.Status),
			"retrigger":       true,
			"mode":            "pubsub",
		},
		CreatedAt: now,
	}); err != nil {
		return fmt.Errorf("retrigger: failed to append event: %w", err)
	}

	if err := s.notifRepo.UpdateStatus(ctx, n.ID, domain.StatusQueued); err != nil {
		return fmt.Errorf("retrigger: failed to update status: %w", err)
	}
	return nil
}
