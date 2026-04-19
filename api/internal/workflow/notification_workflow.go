package workflow

import (
	"time"

	"github.com/google/uuid"
	"github.com/spidey/notification-service/internal/domain"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"
)

func NotificationWorkflow(ctx workflow.Context, req *WorkflowRequest) error {
	ao := workflow.ActivityOptions{
		ScheduleToCloseTimeout: 30 * time.Second,
		RetryPolicy: &temporal.RetryPolicy{
			InitialInterval:    time.Second,
			BackoffCoefficient: 2.0,
			MaximumInterval:    16 * time.Second,
			MaximumAttempts:    5,
		},
	}
	ctx = workflow.WithActivityOptions(ctx, ao)
	var a *Activities

	// Step 1: Check user preferences & DND & Governance
	var prefs domain.UserPreferences
	if err := workflow.ExecuteActivity(ctx, a.CheckPreferencesActivity, req).Get(ctx, &prefs); err != nil {
		return err
	}
	
	if prefs.IsSuppressed {
		return workflow.ExecuteActivity(ctx, a.LogDeliveryActivity, LogEntry{
			NotificationID: req.ID,
			Channel:        string(req.Channel),
			Status:         domain.StatusCancelled, // Or add StatusSuppressed
		}).Get(ctx, nil)
	}
	
	// Assume no channels disabled for simplicity if channel is missing
	// Real implementation would inspect DND window here

	// Step 2: Render template
	var rendered RenderedNotification
	if err := workflow.ExecuteActivity(ctx, a.RenderTemplateActivity, req).Get(ctx, &rendered); err != nil {
		return err
	}

	// Step 3: Publish to Pub/Sub
	var msgID string
	if err := workflow.ExecuteActivity(ctx, a.PublishToPubSubActivity, &rendered).Get(ctx, &msgID); err != nil {
		return err
	}

	// Step 4: Log result
	return workflow.ExecuteActivity(ctx, a.LogDeliveryActivity, LogEntry{
		NotificationID: req.ID,
		MsgID:          msgID,
		Channel:        string(req.Channel),
		Status:         domain.StatusDelivered,
	}).Get(ctx, nil)
}

func OtpNotificationWorkflow(ctx workflow.Context, req *WorkflowRequest) error {
	ao := workflow.ActivityOptions{
		ScheduleToCloseTimeout: 5 * time.Second,
		RetryPolicy: &temporal.RetryPolicy{
			MaximumAttempts: 2, // fail fast
		},
	}
	ctx = workflow.WithActivityOptions(ctx, ao)
	var a *Activities

	var otp string
	// Check preferences / governance for OTP too
	var prefs domain.UserPreferences
	if err := workflow.ExecuteActivity(ctx, a.CheckPreferencesActivity, req).Get(ctx, &prefs); err != nil {
		return err
	}
	if prefs.IsSuppressed {
		return workflow.ExecuteActivity(ctx, a.LogDeliveryActivity, LogEntry{
			NotificationID: req.ID,
			Channel:        string(domain.ChannelSMS),
			Status:         domain.StatusCancelled,
		}).Get(ctx, nil)
	}

	if err := workflow.ExecuteActivity(ctx, a.GenerateOtpActivity, req).Get(ctx, &otp); err != nil {
		return err
	}

	rendered := RenderedNotification{
		ID:        req.ID,
		UserID:    uuid.MustParse(req.UserID),
		Channel:   domain.ChannelSMS,
		Recipient: req.Recipient,
		Payload:   []byte("Your OTP is " + otp + ". Valid for 5 minutes."),
	}

	var msgID string
	if err := workflow.ExecuteActivity(ctx, a.PublishToPubSubActivity, &rendered).Get(ctx, &msgID); err != nil {
		return err
	}

	return workflow.ExecuteActivity(ctx, a.LogDeliveryActivity, LogEntry{
		NotificationID: req.ID,
		MsgID:          msgID,
		Channel:        string(domain.ChannelSMS),
		Status:         domain.StatusDelivered,
	}).Get(ctx, nil)
}

type BulkJob struct {
	ID         string
	TemplateID string
	Channel    string
	Filter     map[string]interface{}
}

func BulkNotificationWorkflow(ctx workflow.Context, job BulkJob) error {
	// Minimal mock placeholder for fan-out
	return nil
}
