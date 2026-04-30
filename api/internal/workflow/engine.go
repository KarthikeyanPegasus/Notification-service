package workflow

import (
	"context"
	"time"

	"github.com/spidey/notification-service/internal/config"
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
	// If mode is "cadence", we would eventually use the Cadence implementation.
	// For now, we'll start with the Temporal implementation and provide a structure for switching.
	if cfg.Cadence.Mode == "standalone" {
		return nil, nil
	}

	if cfg.Cadence.Mode == "cadence" {
		return NewCadenceEngine(cfg, log)
	}

	return NewTemporalEngine(cfg, log)
}

// Basic implementation of WorkflowRun
type baseWorkflowRun struct {
	id    string
	runID string
}

func (r *baseWorkflowRun) GetID() string    { return r.id }
func (r *baseWorkflowRun) GetRunID() string { return r.runID }
