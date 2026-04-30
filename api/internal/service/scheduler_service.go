package service

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/spidey/notification-service/internal/domain"
	"github.com/spidey/notification-service/internal/repository"
	"github.com/spidey/notification-service/internal/workflow"
	"go.uber.org/zap"
)

// SchedulerService manages rescheduling and cancellation of pending scheduled notifications.
type SchedulerService struct {
	schedRepo *repository.ScheduledRepository
	notifRepo *repository.NotificationRepository
	eventRepo   *repository.EventRepository
	wfClients   *WorkflowClientProvider
	log         *zap.Logger
}

func NewSchedulerService(
	schedRepo *repository.ScheduledRepository,
	notifRepo *repository.NotificationRepository,
	eventRepo *repository.EventRepository,
	wfClients *WorkflowClientProvider,
	log *zap.Logger,
) *SchedulerService {
	return &SchedulerService{
		schedRepo:   schedRepo,
		notifRepo:   notifRepo,
		eventRepo:   eventRepo,
		wfClients:   wfClients,
		log:         log,
	}
}

// RescheduleResponse is returned on successful reschedule.
type RescheduleResponse struct {
	NotificationID  string    `json:"notification_id"`
	Status          string    `json:"status"`
	ScheduledAt     time.Time `json:"scheduled_at"`
	WorkflowID      string    `json:"workflow_id"`
	RescheduleCount int       `json:"reschedule_count"`
}

// Reschedule changes the delivery time for a PENDING scheduled notification.
func (s *SchedulerService) Reschedule(ctx context.Context, notifID uuid.UUID, newTime time.Time) (*RescheduleResponse, error) {
	if newTime.Before(time.Now()) {
		return nil, domain.ErrScheduleInPast
	}

	sched, err := s.schedRepo.GetByNotificationID(ctx, notifID)
	if err != nil {
		return nil, err
	}

	if sched.Status != domain.StatusPending {
		switch sched.Status {
		case domain.StatusCancelled:
			return nil, domain.ErrAlreadyCancelled
		case domain.StatusDelivered:
			return nil, domain.ErrAlreadyDelivered
		default:
			return nil, fmt.Errorf("%w: notification is in status %s", domain.ErrAlreadyRunning, sched.Status)
		}
	}

	// Re-start logic requires a full payload so we fetch it.
	n, err := s.notifRepo.GetByID(ctx, notifID)
	if err != nil {
		return nil, fmt.Errorf("loading notification for reschedule: %w", err)
	}

	// Prefer the client stored on the scheduled record (set at creation time); fall back to the notification's API key.
	apiKeyID := ""
	if sched.APIKeyID != nil {
		apiKeyID = sched.APIKeyID.String()
	} else if n != nil && n.APIKeyID != nil {
		apiKeyID = n.APIKeyID.String()
	}
	cli, err := s.wfClients.ClientForScope(ctx, &apiKeyID)
	if err != nil {
		return nil, err
	}
	if cli == nil {
		return nil, domain.NewAppError(
			"WORKFLOWS_DISABLED",
			"rescheduling requires Temporal/Cadence; enable workflow orchestration",
			400,
			nil,
		)
	}

	// Native Temporal TerminateWorkflow cancels the internal server delay
	err = cli.TerminateWorkflow(ctx, sched.WorkflowID, "", "rescheduled")
	if err != nil {
		s.log.Warn("failed to terminate temporal workflow on reschedule", zap.Error(err))
	}

	req := &workflow.WorkflowRequest{
		ID:             n.ID,
		Channel:        n.Channel,
		Priority:       n.Priority,
		Recipient:      n.Recipient,
		Type:           n.Type,
		IdempotencyKey: n.IdempotencyKey,
		ForcedVendor:   n.ForcedVendor,
		ClientID:       apiKeyID,
	}
	if n.TemplateID != nil {
		tid := n.TemplateID.String()
		req.TemplateID = &tid
		if len(sched.TemplateVars) > 0 {
			req.TemplateVariables = sched.TemplateVars
		}
	} else if n.RenderedContent != nil {
		req.DirectContent = &workflow.DirectContent{
			Subject: n.RenderedContent.Subject,
			Body:    n.RenderedContent.Body,
			HTML:    n.RenderedContent.HTML,
		}
	}

	delaySeconds := int(time.Until(newTime).Seconds())
	taskQueue := workflow.TaskQueueFor(n.Channel, n.Priority, apiKeyID)
	options := workflow.StartOptions{
		ID:                    sched.WorkflowID, // preserve ID
		TaskQueue:             taskQueue,
		WorkflowIDReusePolicy: workflow.IDReusePolicyAllowDuplicate,
		StartDelay:            time.Duration(delaySeconds) * time.Second,
	}

	var workflowFunc any = workflow.NotificationWorkflow
	if cli.ProviderName() == "cadence" {
		workflowFunc = workflow.NotificationWorkflowCadence
	}
	run, err := cli.ExecuteWorkflow(ctx, options, workflowFunc, req)
	if err != nil {
		return nil, fmt.Errorf("re-executing rescheduled workflow: %w", err)
	}

	newRunID := run.GetRunID()
	if err := s.schedRepo.UpdateSchedule(ctx, notifID, newTime, newRunID); err != nil {
		return nil, fmt.Errorf("updating schedule: %w", err)
	}

	s.log.Info("notification rescheduled",
		zap.String("notification_id", notifID.String()),
		zap.Time("new_time", newTime),
	)

	return &RescheduleResponse{
		NotificationID:  notifID.String(),
		Status:          string(domain.StatusPending),
		ScheduledAt:     newTime,
		WorkflowID:      sched.WorkflowID,
		RescheduleCount: sched.RescheduleCount + 1,
	}, nil
}

// Cancel terminates a PENDING scheduled notification.
func (s *SchedulerService) Cancel(ctx context.Context, notifID uuid.UUID) error {
	sched, err := s.schedRepo.GetByNotificationID(ctx, notifID)
	if err != nil {
		return err
	}

	if sched.Status != domain.StatusPending {
		switch sched.Status {
		case domain.StatusCancelled:
			return domain.ErrAlreadyCancelled
		case domain.StatusDelivered:
			return domain.ErrAlreadyDelivered
		default:
			return fmt.Errorf("%w: notification is in status %s", domain.ErrAlreadyRunning, sched.Status)
		}
	}

	// Prefer the client stored on the scheduled record; fall back to the notification's API key.
	apiKeyID := ""
	if sched.APIKeyID != nil {
		apiKeyID = sched.APIKeyID.String()
	} else {
		n, _ := s.notifRepo.GetByID(ctx, notifID)
		if n != nil && n.APIKeyID != nil {
			apiKeyID = n.APIKeyID.String()
		}
	}
	cli, err := s.wfClients.ClientForScope(ctx, &apiKeyID)
	if err != nil {
		return err
	}
	if cli == nil {
		return domain.NewAppError(
			"WORKFLOWS_DISABLED",
			"cancelling scheduled notifications requires Temporal/Cadence; enable workflow orchestration",
			400,
			nil,
		)
	}

	err = cli.TerminateWorkflow(ctx, sched.WorkflowID, "", "cancelled by user")
	if err != nil {
		s.log.Warn("failed to terminate temporal workflow on cancel", zap.Error(err))
	}

	if err := s.schedRepo.UpdateStatus(ctx, notifID, domain.StatusCancelled); err != nil {
		return err
	}
	if err := s.notifRepo.UpdateStatus(ctx, notifID, domain.StatusCancelled); err != nil {
		return err
	}

	_ = s.eventRepo.Append(ctx, &domain.NotificationEvent{
		ID:             uuid.New(),
		NotificationID: notifID,
		EventType:      domain.EventCancelled,
		Metadata:       map[string]any{"cancelled_by": "user"},
		CreatedAt:      time.Now(),
	})

	s.log.Info("notification cancelled", zap.String("notification_id", notifID.String()))
	return nil
}

func (s *SchedulerService) ListScheduled(
	ctx context.Context,
	statuses []domain.NotificationStatus,
	page, pageSize int,
) ([]*domain.ScheduledNotification, int64, error) {
	return s.schedRepo.List(ctx, statuses, page, pageSize)
}
