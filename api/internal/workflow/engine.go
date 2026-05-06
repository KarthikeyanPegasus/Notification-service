package workflow

import (
	"context"
	"strings"
	"time"

	"github.com/spidey/notification-service/internal/config"
	"github.com/spidey/notification-service/internal/repository"
	"go.uber.org/zap"
)

// WorkflowEngine defines a generic interface for starting workflows and running workers,
// abstracting between Temporal and Cadence SDKs.
type StartOptions struct {
	ID                                       string
	TaskQueue                                string
	StartDelay                               time.Duration
	WorkflowIDReusePolicy                    IDReusePolicy
	WorkflowExecutionErrorWhenAlreadyStarted bool
}

type IDReusePolicy int

const (
	IDReusePolicyAllowDuplicate IDReusePolicy = iota
	IDReusePolicyAllowDuplicateFailedOnly
	IDReusePolicyRejectDuplicate
	IDReusePolicyTerminateIfRunning
)

type WorkflowEngine interface {
	ExecuteWorkflow(ctx context.Context, options StartOptions, workflowFunc any, args ...any) (WorkflowRun, error)
	TerminateWorkflow(ctx context.Context, workflowID string, runID string, reason string) error
	NewWorker(taskQueue string) WorkflowWorker
	ProviderName() string
	Identity() string
	Close()
}

type WorkflowRun interface {
	GetID() string
	GetRunID() string
}

type WorkflowWorker interface {
	RegisterWorkflow(f any)
	RegisterActivity(f any)
	Run(stopCh <-chan interface{}) error
}

func NewEngine(cfg *config.Config, log *zap.Logger) (WorkflowEngine, error) {
	mode := normalizeMode(cfg.Cadence.Mode)
	if mode == "standalone" {
		return nil, nil
	}

	if mode == "cadence" {
		return NewCadenceEngine(cfg, log)
	}

	if mode == "go_routines" {
		return NewGoRoutinesEngine(log, nil), nil
	}

	return NewTemporalEngine(cfg, log)
}

// NewGoRoutinesEngineWith creates a GoRoutinesEngine with a retry config repo and activities.
// This is used by the WorkflowClientProvider for per-client scoped engines.
func NewGoRoutinesEngineWith(log *zap.Logger, retryConfigRepo repository.VendorRetryConfigRepository, activities *Activities) *GoRoutinesEngine {
	engine := NewGoRoutinesEngine(log, retryConfigRepo)
	if activities != nil {
		engine.workflowHandler = NewGoRoutinesWorkflowHandler(activities, retryConfigRepo, log)
	}
	return engine
}

func normalizeMode(raw string) string {
	mode := strings.ToLower(strings.TrimSpace(raw))
	if mode == "" {
		return "temporal"
	}
	return mode
}

// Basic implementation of WorkflowRun
type baseWorkflowRun struct {
	id    string
	runID string
}

func (r *baseWorkflowRun) GetID() string    { return r.id }
func (r *baseWorkflowRun) GetRunID() string { return r.runID }
