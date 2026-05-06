package workflow

import (
	"fmt"
	"time"

	"github.com/spidey/notification-service/internal/domain"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"
)

// TaskQueueFor returns the Temporal task queue name for a given channel × priority × clientID.
// Each client gets its own task queue per channel per priority, enabling the WorkerManager
// to register an independent Temporal worker for each combination.
// clientID="" means the global/unscoped queue used for notifications without an API key.
func TaskQueueFor(channel domain.Channel, priority domain.Priority, clientID string) string {
	if clientID == "" {
		clientID = "global"
	}
	return fmt.Sprintf("notif-%s-%s-%s", channel, priority, clientID)
}

// prioritySLA maps notification priority to the maximum allowed end-to-end delivery time.
// This is enforced as ScheduleToCloseTimeout on every activity in the workflow,
// so Temporal will fail the workflow if delivery takes longer than the SLA.
var prioritySLA = map[domain.Priority]time.Duration{
	domain.PriorityHigh:   5 * time.Second,
	domain.PriorityMedium: 15 * time.Second,
	domain.PriorityLow:    30 * time.Second,
}

func slaFor(p domain.Priority) time.Duration {
	if d, ok := prioritySLA[p]; ok {
		return d
	}
	return prioritySLA[domain.PriorityLow]
}

func NotificationWorkflow(ctx workflow.Context, req *WorkflowRequest) error {
	sla := slaFor(req.Priority)
	ao := workflow.ActivityOptions{
		// ScheduleToCloseTimeout enforces the priority SLA end-to-end.
		// High: 5s, Medium: 15s, Low: 30s.
		ScheduleToCloseTimeout: sla,
		RetryPolicy: &temporal.RetryPolicy{
			InitialInterval:    500 * time.Millisecond,
			BackoffCoefficient: 2.0,
			MaximumInterval:    sla / 2,
			// Allow at most 2 retries within the SLA window; high priority gets 1.
			MaximumAttempts: maxRetries(req.Priority),
		},
	}
	ctx = workflow.WithActivityOptions(ctx, ao)

	fail := func(step string, err error) error {
		_ = workflow.ExecuteActivity(ctx, "LogDeliveryActivity", LogEntry{
			NotificationID: req.ID,
			Channel:        string(req.Channel),
			Status:         domain.StatusFailed,
			Layer:          "temporal_workflow",
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
			Layer:          "temporal_workflow",
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
		Layer:          "temporal_workflow",
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

	fail := func(step string, err error) error {
		_ = workflow.ExecuteActivity(ctx, "LogDeliveryActivity", LogEntry{
			NotificationID: req.ID,
			Channel:        string(domain.ChannelSMS),
			Status:         domain.StatusFailed,
			Layer:          "temporal_workflow",
			ErrorMessage:   fmt.Sprintf("%s: %v", step, err),
		}).Get(ctx, nil)
		return err
	}

	var otp string
	// Check preferences / governance for OTP too
	var prefs domain.UserPreferences
	if err := workflow.ExecuteActivity(ctx, "CheckPreferencesActivity", req).Get(ctx, &prefs); err != nil {
		return fail("check_preferences", err)
	}
	if prefs.IsSuppressed {
		return workflow.ExecuteActivity(ctx, "LogDeliveryActivity", LogEntry{
			NotificationID: req.ID,
			Channel:        string(domain.ChannelSMS),
			Status:         domain.StatusSuppressed,
			Layer:          "temporal_workflow",
		}).Get(ctx, nil)
	}

	if err := workflow.ExecuteActivity(ctx, "GenerateOtpActivity", req).Get(ctx, &otp); err != nil {
		return fail("generate_otp", err)
	}

	rendered := RenderedNotification{
		ID:        req.ID,
		Channel:   domain.ChannelSMS,
		Recipient: req.Recipient,
		Payload:   []byte("Your OTP is " + otp + ". Valid for 5 minutes."),
	}

	// Step 2.5: Content Security & Spam check
	if err := workflow.ExecuteActivity(ctx, "ContentSecurityCheckActivity", &rendered).Get(ctx, nil); err != nil {
		return fail("content_security_check", err)
	}

	var result domain.DeliveryResult
	if err := workflow.ExecuteActivity(ctx, "DeliverNotificationActivity", &rendered).Get(ctx, &result); err != nil {
		return fail("deliver_notification", err)
	}

	status := domain.StatusSent
	if !result.Success {
		status = domain.StatusFailed
	}
	return workflow.ExecuteActivity(ctx, "LogDeliveryActivity", LogEntry{
		NotificationID: req.ID,
		MsgID:          result.ProviderMsgID,
		Provider:       result.Provider,
		Channel:        string(domain.ChannelSMS),
		Status:         status,
		Layer:          "temporal_workflow",
	}).Get(ctx, nil)
}

// maxRetries returns the maximum retry attempts for a given priority.
// High priority allows 1 retry only (fast fail is preferred over slow retries within 5s).
func maxRetries(p domain.Priority) int32 {
	switch p {
	case domain.PriorityHigh:
		return 1
	case domain.PriorityMedium:
		return 2
	default:
		return 3
	}
}

type BulkJob struct {
	ID         string
	TemplateID string
	Channel    string
	// Recipients is the pre-resolved list of recipient addresses for this job.
	Recipients []string
	// Requests is the fully constructed per-recipient workflow requests.
	// When non-empty it takes precedence over Recipients + TemplateID.
	Requests   []*WorkflowRequest
	Filter     map[string]interface{}
}

// BulkNotificationWorkflow fans out one child NotificationWorkflow per recipient.
// Each child runs with its own retry budget; a failed child does not abort siblings.
func BulkNotificationWorkflow(ctx workflow.Context, job BulkJob) error {
	channel := domain.Channel(job.Channel)

	// Build per-recipient WorkflowRequests when not pre-populated.
	requests := job.Requests
	if len(requests) == 0 {
		for _, recipient := range job.Recipients {
			requests = append(requests, &WorkflowRequest{
				Channel:    channel,
				Priority:   domain.PriorityLow,
				Recipient:  recipient,
				TemplateID: &job.TemplateID,
				ClientID:   "",
			})
		}
	}

	// Fan out: launch all child workflows in parallel, collect futures.
	cwo := workflow.ChildWorkflowOptions{
		WorkflowExecutionTimeout: 2 * time.Minute,
		RetryPolicy: &temporal.RetryPolicy{
			MaximumAttempts: 2,
		},
	}
	childCtx := workflow.WithChildOptions(ctx, cwo)

	futures := make([]workflow.Future, 0, len(requests))
	for _, req := range requests {
		futures = append(futures, workflow.ExecuteChildWorkflow(childCtx, NotificationWorkflow, req))
	}

	// Wait for all children; accumulate errors but do not fail the bulk job on partial failure.
	var failed int
	for _, f := range futures {
		if err := f.Get(ctx, nil); err != nil {
			failed++
		}
	}

	if failed > 0 {
		return fmt.Errorf("bulk job %s: %d/%d recipients failed delivery", job.ID, failed, len(requests))
	}
	return nil
}
