package worker

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/spidey/notification-service/internal/domain"
	"github.com/spidey/notification-service/internal/pubsub"
	wf "github.com/spidey/notification-service/internal/workflow"
	"go.uber.org/zap"
)

// Dispatcher reads notification messages from a single Kafka priority topic
// and starts a Temporal/Cadence workflow on the task queue for the client that owns that message.
//
// One Dispatcher goroutine is created per channel × priority combination (18 total).
// Task queue: notif-{channel}-{priority}-{clientID} (or -global when clientID is absent).
//
// The Dispatcher does NOT execute delivery — it only routes to the right Temporal worker.
// Delivery, rate limiting, and SLA enforcement happen inside the workflow.
type Dispatcher struct {
	channel    domain.Channel
	priority   domain.Priority
	subscriber pubsub.Subscriber  // Kafka subscriber, one consumer group per dispatcher
	engine     wf.WorkflowEngine
	log        *zap.Logger
}

// NewDispatcher creates a Dispatcher for a specific channel × priority pair.
// subscriber must be a dedicated Kafka subscriber whose consumer group ID is unique
// for this channel × priority (so partitions are not shared with other dispatchers).
func NewDispatcher(
	channel domain.Channel,
	priority domain.Priority,
	subscriber pubsub.Subscriber,
	engine wf.WorkflowEngine,
	log *zap.Logger,
) *Dispatcher {
	return &Dispatcher{
		channel:    channel,
		priority:   priority,
		subscriber: subscriber,
		engine:     engine,
		log:        log.With(zap.String("channel", string(channel)), zap.String("priority", string(priority))),
	}
}

// Run blocks and dispatches messages until ctx is cancelled.
// topicKey is the pubsub subscription key (e.g. "email-high").
func (d *Dispatcher) Run(ctx context.Context) error {
	topicKey := pubsub.PriorityTopicKey(string(d.channel), string(d.priority))
	d.log.Info("dispatcher started", zap.String("topic_key", topicKey))

	return d.subscriber.Subscribe(ctx, topicKey, func(ctx context.Context, msg *pubsub.Message) error {
		return d.dispatch(ctx, msg)
	})
}

func (d *Dispatcher) dispatch(ctx context.Context, msg *pubsub.Message) error {
	notifID, err := uuid.Parse(msg.NotificationID)
	if err != nil {
		d.log.Error("dispatcher: invalid notification_id — acking to avoid loop",
			zap.String("raw", msg.NotificationID))
		return nil // ack malformed
	}

	// Determine the client-scoped task queue.
	clientID := msg.ClientID
	taskQueue := wf.TaskQueueFor(d.channel, d.priority, clientID)

	// Build the workflow request.
	req := &wf.WorkflowRequest{
		ID:             notifID,
		Channel:        d.channel,
		Priority:       d.priority,
		Recipient:      msg.Recipient,
		Type:           msg.Type,
		IdempotencyKey: msg.IdempotencyKey,
		ForcedVendor:   msg.ForcedVendor,
		ClientID:       clientID,
	}
	if msg.TemplateID != "" {
		req.TemplateID = &msg.TemplateID
	}
	if len(msg.Payload) > 0 {
		req.TemplateVariables = msg.Payload
	}

	// Start workflow on the client-specific task queue.
	startCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	opts := wf.StartOptions{
		ID:        fmt.Sprintf("notif-%s", notifID.String()),
		TaskQueue: taskQueue,
	}

	var workflowFn interface{} = wf.NotificationWorkflow
	if d.engine.ProviderName() == "cadence" {
		workflowFn = wf.NotificationWorkflowCadence
	}

	if _, err := d.engine.ExecuteWorkflow(startCtx, opts, workflowFn, req); err != nil {
		d.log.Error("dispatcher: failed to start workflow — nacking for retry",
			zap.String("notification_id", notifID.String()),
			zap.String("task_queue", taskQueue),
			zap.Error(err),
		)
		return err // nack → Kafka will redeliver
	}

	d.log.Debug("dispatcher: workflow started",
		zap.String("notification_id", notifID.String()),
		zap.String("task_queue", taskQueue),
		zap.String("client_id", clientID),
	)
	return nil
}
