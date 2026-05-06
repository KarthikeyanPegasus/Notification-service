package workflow

import (
	"context"
	"fmt"
	"time"

	"github.com/spidey/notification-service/internal/domain"
	"github.com/spidey/notification-service/internal/repository"
	"go.uber.org/zap"
)

// GoRoutinesWorkflowHandler wraps the activities and executes them directly
// without Temporal/Cadence orchestration.
type GoRoutinesWorkflowHandler struct {
	activities          *Activities
	retryConfigRepo     repository.VendorRetryConfigRepository
	log                 *zap.Logger
}

// NewGoRoutinesWorkflowHandler creates a new workflow handler for Go Routines mode.
func NewGoRoutinesWorkflowHandler(activities *Activities, retryConfigRepo repository.VendorRetryConfigRepository, log *zap.Logger) *GoRoutinesWorkflowHandler {
	return &GoRoutinesWorkflowHandler{
		activities:      activities,
		retryConfigRepo: retryConfigRepo,
		log:             log,
	}
}

// ExecuteNotificationWorkflow executes the notification workflow using direct function calls
// instead of Temporal's activity execution model.
func (h *GoRoutinesWorkflowHandler) ExecuteNotificationWorkflow(ctx context.Context, req *WorkflowRequest) error {
	h.log.Info("executing go_routines notification workflow",
		zap.String("notification_id", req.ID.String()),
		zap.String("channel", string(req.Channel)),
		zap.String("priority", string(req.Priority)),
	)

	// Get retry config for SLA and retry settings
	var apiKeyID *string
	if req.ClientID != "" {
		apiKeyID = &req.ClientID
	}
	retryCfg := RetryConfigFrom(ctx, h.retryConfigRepo, string(req.Channel), apiKeyID)
	sla := SLADeadline(retryCfg)
	maxRetries := MaxRetries(retryCfg)

	// Create context with SLA timeout
	slaCtx, cancel := context.WithTimeout(ctx, sla)
	defer cancel()

	logFailure := func(step string, err error) error {
		h.log.Error("workflow step failed",
			zap.String("notification_id", req.ID.String()),
			zap.String("step", step),
			zap.Error(err),
		)
		_ = h.activities.LogDeliveryActivity(ctx, LogEntry{
			NotificationID: req.ID,
			Channel:        string(req.Channel),
			Status:         domain.StatusFailed,
			Layer:          "go_routines_workflow",
			ErrorMessage:   fmt.Sprintf("%s: %v", step, err),
		})
		return err
	}

	// Step 1: Check user preferences & DND & Governance
	prefs, err := h.activities.CheckPreferencesActivity(slaCtx, req)
	if err != nil {
		return logFailure("check_preferences", err)
	}

	if prefs.IsSuppressed {
		h.log.Info("notification suppressed by preferences",
			zap.String("notification_id", req.ID.String()),
		)
		return h.activities.LogDeliveryActivity(ctx, LogEntry{
			NotificationID: req.ID,
			Channel:        string(req.Channel),
			Status:         domain.StatusSuppressed,
			Layer:          "go_routines_workflow",
		})
	}

	// Step 2: Render template
	rendered, err := h.activities.RenderTemplateActivity(slaCtx, req)
	if err != nil {
		return logFailure("render_template", err)
	}

	// Step 2.5: Content Security & Spam check
	if err := h.activities.ContentSecurityCheckActivity(slaCtx, rendered); err != nil {
		return logFailure("content_security_check", err)
	}

	// Step 3: Direct Delivery with retry logic
	var result domain.DeliveryResult
	var lastErr error
	for attempt := 1; attempt <= maxRetries; attempt++ {
		result, lastErr = h.activities.DeliverNotificationActivity(slaCtx, rendered)
		if lastErr == nil && result.Success {
			break
		}

		// Check if we've exceeded SLA
		select {
		case <-slaCtx.Done():
			return logFailure("deliver_notification", fmt.Errorf("SLA timeout after %d attempts: %w", attempt, slaCtx.Err()))
		default:
		}

		// If not last attempt, apply backoff delay
		if attempt < maxRetries {
			delay := RetryDelay(attempt, retryCfg)
			h.log.Warn("delivery attempt failed, retrying",
				zap.String("notification_id", req.ID.String()),
				zap.Int("attempt", attempt),
				zap.Int("max_attempts", maxRetries),
				zap.Duration("backoff", delay),
				zap.Error(lastErr),
			)
			select {
			case <-time.After(delay):
			case <-slaCtx.Done():
				return logFailure("deliver_notification", fmt.Errorf("SLA timeout during backoff: %w", slaCtx.Err()))
			}
		}
	}

	if lastErr != nil || !result.Success {
		return logFailure("deliver_notification", lastErr)
	}

	// Step 4: Log success
	h.log.Info("notification delivered successfully",
		zap.String("notification_id", req.ID.String()),
		zap.String("provider", result.Provider),
		zap.String("provider_msg_id", result.ProviderMsgID),
	)

	return h.activities.LogDeliveryActivity(ctx, LogEntry{
		NotificationID: req.ID,
		MsgID:          result.ProviderMsgID,
		Provider:       result.Provider,
		Channel:        string(req.Channel),
		Status:         domain.StatusSent,
		Layer:          "go_routines_workflow",
	})
}

// ExecuteOtpNotificationWorkflow executes the OTP notification workflow.
func (h *GoRoutinesWorkflowHandler) ExecuteOtpNotificationWorkflow(ctx context.Context, req *WorkflowRequest) error {
	h.log.Info("executing go_routines OTP notification workflow",
		zap.String("notification_id", req.ID.String()),
	)

	// OTP workflows have stricter SLA (5 seconds)
	slaCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	logFailure := func(step string, err error) error {
		h.log.Error("OTP workflow step failed",
			zap.String("notification_id", req.ID.String()),
			zap.String("step", step),
			zap.Error(err),
		)
		_ = h.activities.LogDeliveryActivity(ctx, LogEntry{
			NotificationID: req.ID,
			Channel:        string(domain.ChannelSMS),
			Status:         domain.StatusFailed,
			Layer:          "go_routines_workflow",
			ErrorMessage:   fmt.Sprintf("%s: %v", step, err),
		})
		return err
	}

	// Check preferences
	prefs, err := h.activities.CheckPreferencesActivity(slaCtx, req)
	if err != nil {
		return logFailure("check_preferences", err)
	}

	if prefs.IsSuppressed {
		h.log.Info("OTP notification suppressed by preferences",
			zap.String("notification_id", req.ID.String()),
		)
		return h.activities.LogDeliveryActivity(ctx, LogEntry{
			NotificationID: req.ID,
			Channel:        string(domain.ChannelSMS),
			Status:         domain.StatusSuppressed,
			Layer:          "go_routines_workflow",
		})
	}

	// Generate OTP
	otp, err := h.activities.GenerateOtpActivity(slaCtx, req)
	if err != nil {
		return logFailure("generate_otp", err)
	}

	rendered := &RenderedNotification{
		ID:        req.ID,
		Channel:   domain.ChannelSMS,
		Recipient: req.Recipient,
		Payload:   []byte("Your OTP is " + otp + ". Valid for 5 minutes."),
	}

	// Content Security & Spam check
	if err := h.activities.ContentSecurityCheckActivity(slaCtx, rendered); err != nil {
		return logFailure("content_security_check", err)
	}

	// Direct Delivery (OTP uses fast-fail, max 2 attempts)
	var result domain.DeliveryResult
	var lastErr error
	for attempt := 1; attempt <= 2; attempt++ {
		result, lastErr = h.activities.DeliverNotificationActivity(slaCtx, rendered)
		if lastErr == nil && result.Success {
			break
		}

		if attempt < 2 {
			h.log.Warn("OTP delivery attempt failed, retrying",
				zap.String("notification_id", req.ID.String()),
				zap.Int("attempt", attempt),
				zap.Error(lastErr),
			)
			select {
			case <-time.After(500 * time.Millisecond):
			case <-slaCtx.Done():
				return logFailure("deliver_notification", slaCtx.Err())
			}
		}
	}

	if lastErr != nil || !result.Success {
		return logFailure("deliver_notification", lastErr)
	}

	h.log.Info("OTP notification delivered successfully",
		zap.String("notification_id", req.ID.String()),
		zap.String("provider", result.Provider),
	)

	return h.activities.LogDeliveryActivity(ctx, LogEntry{
		NotificationID: req.ID,
		MsgID:          result.ProviderMsgID,
		Provider:       result.Provider,
		Channel:        string(domain.ChannelSMS),
		Status:         domain.StatusSent,
		Layer:          "go_routines_workflow",
	})
}
