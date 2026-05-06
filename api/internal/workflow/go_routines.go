package workflow

import (
	"context"
	"math"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/spidey/notification-service/internal/domain"
	"github.com/spidey/notification-service/internal/repository"
	"go.uber.org/zap"
)

// GoRoutinesEngine implements the WorkflowEngine interface using in-process
// goroutines instead of Temporal/Cadence. It executes workflow functions directly
// with configurable retry/backoff/SLA policies per vendor.
//
// Auto-scaling is handled by the WorkerManager: it calls NewWorker() N times
// where N = desired parallelism from the autoscaler.
type GoRoutinesEngine struct {
	log              *zap.Logger
	workerCount      atomic.Int32
	retryConfigRepo  repository.VendorRetryConfigRepository
	workflowHandler  *GoRoutinesWorkflowHandler
}

// NewGoRoutinesEngine creates a new GoRoutinesEngine.
func NewGoRoutinesEngine(log *zap.Logger, retryConfigRepo repository.VendorRetryConfigRepository) *GoRoutinesEngine {
	return &GoRoutinesEngine{
		log:             log,
		retryConfigRepo: retryConfigRepo,
	}
}

// ExecuteWorkflow runs the workflow function as a goroutine with retry logic.
// The options.StartDelay is respected if set. The workflowFunc is called with args.
// The returned WorkflowRun has a generated run ID.
func (e *GoRoutinesEngine) ExecuteWorkflow(ctx context.Context, options StartOptions, workflowFunc any, args ...any) (WorkflowRun, error) {
	runID := uuid.New().String()
	run := &goRoutinesRun{
		id:    options.ID,
		runID: runID,
	}

	// Respect start delay
	if options.StartDelay > 0 {
		select {
		case <-time.After(options.StartDelay):
		case <-ctx.Done():
			return run, ctx.Err()
		}
	}

	// Execute the workflow in a goroutine using the workflow handler.
	// IMPORTANT: use a context derived from the parent WITHOUT cancellation.
	// The dispatcher passes startCtx with a 5s timeout and defers cancel()
	// immediately after ExecuteWorkflow returns, so the captured ctx would
	// already be cancelled by the time the goroutine runs.
	// Each handler creates its own SLA-derived timeout internally.
	backgroundCtx := context.WithoutCancel(ctx)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				e.log.Error("go routines workflow panicked",
					zap.String("workflow_id", options.ID),
					zap.Any("panic", r),
				)
			}
		}()

		// Extract the workflow request from args
		if len(args) == 0 {
			e.log.Error("no workflow request provided")
			return
		}

		req, ok := args[0].(*WorkflowRequest)
		if !ok {
			e.log.Error("invalid workflow request type")
			return
		}

		// Route to the appropriate workflow based on the request type
		if e.workflowHandler == nil {
			e.log.Error("workflow handler not initialized",
				zap.String("notification_id", req.ID.String()),
			)
			return
		}

		var err error
		if req.Type == "otp" {
			err = e.workflowHandler.ExecuteOtpNotificationWorkflow(backgroundCtx, req)
		} else {
			err = e.workflowHandler.ExecuteNotificationWorkflow(backgroundCtx, req)
		}

		if err != nil {
			e.log.Error("workflow execution failed",
				zap.String("workflow_id", options.ID),
				zap.String("notification_id", req.ID.String()),
				zap.Error(err),
			)
		} else {
			e.log.Debug("workflow completed successfully",
				zap.String("workflow_id", options.ID),
				zap.String("notification_id", req.ID.String()),
			)
		}
	}()

	return run, nil
}

// TerminateWorkflow is a no-op for goroutine-based workflows since they run
// in-process and finish quickly.
func (e *GoRoutinesEngine) TerminateWorkflow(_ context.Context, _ string, _ string, _ string) error {
	return nil
}

// NewWorker creates a GoRoutinesWorker for the given task queue.
func (e *GoRoutinesEngine) NewWorker(taskQueue string) WorkflowWorker {
	e.workerCount.Add(1)
	return &goRoutinesWorker{
		engine:          e,
		taskQueue:       taskQueue,
		workflows:       make([]any, 0),
		activities:      make([]any, 0),
		stopCh:          make(chan struct{}),
	}
}

// ProviderName returns the provider identifier.
func (e *GoRoutinesEngine) ProviderName() string {
	return "go_routines"
}

// Identity returns a unique identifier for this engine instance.
func (e *GoRoutinesEngine) Identity() string {
	return "go_routines|inline"
}

// Close cleans up resources.
func (e *GoRoutinesEngine) Close() {
	e.log.Info("go routines engine closed")
}

// WorkerCount returns the number of active workers.
func (e *GoRoutinesEngine) WorkerCount() int32 {
	return e.workerCount.Load()
}

// goRoutinesRun implements WorkflowRun.
type goRoutinesRun struct {
	id    string
	runID string
}

func (r *goRoutinesRun) GetID() string    { return r.id }
func (r *goRoutinesRun) GetRunID() string { return r.runID }

// goRoutinesWorker implements WorkflowWorker. Each instance runs a single
// goroutine that processes delivery tasks. The WorkerManager launches multiple
// workers per task queue for parallel consumption (auto-scaling).
type goRoutinesWorker struct {
	engine      *GoRoutinesEngine
	taskQueue   string
	workflows   []any
	activities  []any
	stopCh      chan struct{}
	wg          sync.WaitGroup
}

func (w *goRoutinesWorker) RegisterWorkflow(f any) {
	w.workflows = append(w.workflows, f)
}

func (w *goRoutinesWorker) RegisterActivity(f any) {
	w.activities = append(w.activities, f)
}

// Run starts the worker goroutine. It blocks until stopCh is closed.
func (w *goRoutinesWorker) Run(stopCh <-chan interface{}) error {
	w.engine.log.Debug("go routines worker started",
		zap.String("task_queue", w.taskQueue),
	)

	// Run until stop signal
	<-stopCh

	w.engine.log.Debug("go routines worker stopped",
		zap.String("task_queue", w.taskQueue),
	)
	w.engine.workerCount.Add(-1)
	return nil
}

// ── Retry/Backoff Utilities ─────────────────────────────────────────────────

// RetryConfigFrom loads the retry config for a vendor from the repository,
// falling back to defaults if none is stored.
func RetryConfigFrom(ctx context.Context, repo repository.VendorRetryConfigRepository, vendorName string, apiKeyID *string) *domain.VendorRetryConfig {
	if repo == nil {
		return domain.DefaultRetryConfig(vendorName)
	}

	cfg, err := repo.Get(ctx, vendorName, apiKeyID)
	if err != nil || cfg == nil {
		// Fall back to global config
		cfg, err = repo.Get(ctx, vendorName, nil)
		if err != nil || cfg == nil {
			return domain.DefaultRetryConfig(vendorName)
		}
	}
	return cfg
}

// RetryDelay calculates the backoff delay for a given attempt using the
// vendor's retry configuration with jitter.
func RetryDelay(attempt int, cfg *domain.VendorRetryConfig) time.Duration {
	if cfg == nil {
		cfg = domain.DefaultRetryConfig("")
	}

	base := float64(cfg.RetryInitialIntervalMs)
	if base <= 0 {
		base = 100
	}

	coeff := cfg.RetryBackoffCoefficient
	if coeff <= 0 {
		coeff = 2.0
	}

	maxMs := float64(cfg.RetryMaxIntervalMs)
	if maxMs <= 0 {
		maxMs = 30000
	}

	// Exponential backoff: base * coeff^(attempt-1)
	delay := base * math.Pow(coeff, float64(attempt-1))
	if delay > maxMs {
		delay = maxMs
	}

	// Add ±20% jitter
	jitter := delay * 0.2 * (rand.Float64()*2 - 1)
	result := time.Duration(delay+jitter) * time.Millisecond
	if result < 0 {
		result = time.Duration(base) * time.Millisecond
	}
	return result
}

// MaxRetries returns the maximum allowed retry attempts from the config.
func MaxRetries(cfg *domain.VendorRetryConfig) int {
	if cfg == nil || cfg.RetryMaxAttempts <= 0 {
		return 5
	}
	return cfg.RetryMaxAttempts
}

// SLADeadline returns the per-vendor SLA deadline as a duration.
func SLADeadline(cfg *domain.VendorRetryConfig) time.Duration {
	if cfg == nil || cfg.SLA <= 0 {
		return 30 * time.Second
	}
	return time.Duration(cfg.SLA) * time.Second
}
