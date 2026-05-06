package workflow

import (
	"fmt"
	"time"

	"github.com/spidey/notification-service/internal/domain"
	"go.uber.org/cadence/workflow"
)

func NotificationWorkflowCadence(ctx workflow.Context, req *WorkflowRequest) error {
	ao := workflow.ActivityOptions{
		// Cadence will fail the workflow if an activity exceeds ScheduleToCloseTimeout.
		// Local dev (or slow Redis/DB) can easily exceed 30s, so use safer defaults.
		ScheduleToCloseTimeout: 5 * time.Minute,
		ScheduleToStartTimeout: 30 * time.Second,
		StartToCloseTimeout:    2 * time.Minute,
	}
	ctx = workflow.WithActivityOptions(ctx, ao)

	fail := func(step string, err error) error {
		_ = workflow.ExecuteActivity(ctx, "LogDeliveryActivity", LogEntry{
			NotificationID: req.ID,
			Channel:        string(req.Channel),
			Status:         domain.StatusFailed,
			Layer:          "cadence_workflow",
			ErrorMessage:   fmt.Sprintf("%s: %v", step, err),
		}).Get(ctx, nil)
		return err
	}

	// Step 1: Check user preferences & DND & Governance
	var prefs domain.UserPreferences
	if err := workflow.ExecuteActivity(ctx, "CheckPreferencesActivity", req).Get(ctx, &prefs); err != nil {
		return fail("check_preferences", err)
	}
	
	if prefs.IsSuppressed {
		return workflow.ExecuteActivity(ctx, "LogDeliveryActivity", LogEntry{
			NotificationID: req.ID,
			Channel:        string(req.Channel),
			Status:         domain.StatusSuppressed,
			Layer:          "cadence_workflow",
		}).Get(ctx, nil)
	}
	
	// Step 2: Render template
	var rendered RenderedNotification
	if err := workflow.ExecuteActivity(ctx, "RenderTemplateActivity", req).Get(ctx, &rendered); err != nil {
		return fail("render_template", err)
	}

	// Step 2.5: Content Security & Spam check
	if err := workflow.ExecuteActivity(ctx, "ContentSecurityCheckActivity", &rendered).Get(ctx, nil); err != nil {
		return fail("content_security_check", err)
	}

	// Step 3: Direct Delivery
	var result domain.DeliveryResult
	if err := workflow.ExecuteActivity(ctx, "DeliverNotificationActivity", &rendered).Get(ctx, &result); err != nil {
		return fail("deliver_notification", err)
	}

	// Step 4: Log result
	status := domain.StatusSent
	if !result.Success {
		status = domain.StatusFailed
	}
	return workflow.ExecuteActivity(ctx, "LogDeliveryActivity", LogEntry{
		NotificationID: req.ID,
		MsgID:          result.ProviderMsgID,
		Provider:       result.Provider,
		Channel:        string(req.Channel),
		Status:         status,
		Layer:          "cadence_workflow",
	}).Get(ctx, nil)
}
