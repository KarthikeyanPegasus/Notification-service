package workflow

import (
	"context"
	"errors"
	"sync"

	"github.com/spidey/notification-service/internal/config"
	"go.temporal.io/api/enums/v1"
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/worker"
	"go.uber.org/zap"
)

type TemporalEngine struct {
	mu        sync.Mutex
	client    client.Client
	hostPort  string
	namespace string
	log       *zap.Logger
}

func NewTemporalEngine(cfg *config.Config, log *zap.Logger) (*TemporalEngine, error) {
	return NewTemporalEngineWith(cfg.Cadence.HostPort, cfg.Cadence.Domain, log)
}

func NewTemporalEngineWith(hostPort string, namespace string, log *zap.Logger) (*TemporalEngine, error) {
	e := &TemporalEngine{
		hostPort:  hostPort,
		namespace: namespace,
		log:       log,
	}
	// Best-effort connection on startup; lazy reconnect handles failures later.
	if err := e.connect(); err != nil {
		log.Warn("failed to connect to temporal on startup; will retry on first use", zap.Error(err))
	}
	return e, nil
}

func NewTemporalEngineFromClient(c client.Client, log *zap.Logger) *TemporalEngine {
	return &TemporalEngine{client: c, log: log}
}

// connect (re)dials the Temporal frontend. Caller must hold e.mu or be in single-threaded init.
func (e *TemporalEngine) connect() error {
	if e.client != nil {
		e.client.Close()
		e.client = nil
	}
	c, err := client.Dial(client.Options{
		HostPort:  e.hostPort,
		Namespace: e.namespace,
		Logger:    newTemporalLogger(e.log),
	})
	if err != nil {
		return err
	}
	e.client = c
	return nil
}

// ensureConnected lazily reconnects if the client was never initialized or lost.
func (e *TemporalEngine) ensureConnected() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.client != nil {
		return nil
	}
	return e.connect()
}

func (e *TemporalEngine) ExecuteWorkflow(ctx context.Context, options StartOptions, workflowFunc any, args ...any) (WorkflowRun, error) {
	if err := e.ensureConnected(); err != nil {
		return nil, errors.New("temporal client not initialized: " + err.Error())
	}
	e.mu.Lock()
	cli := e.client
	e.mu.Unlock()

	opts := client.StartWorkflowOptions{
		ID:         options.ID,
		TaskQueue:  options.TaskQueue,
		StartDelay: options.StartDelay,
		WorkflowExecutionErrorWhenAlreadyStarted: options.WorkflowExecutionErrorWhenAlreadyStarted,
	}

	switch options.WorkflowIDReusePolicy {
	case IDReusePolicyAllowDuplicate:
		opts.WorkflowIDReusePolicy = enums.WORKFLOW_ID_REUSE_POLICY_ALLOW_DUPLICATE
	case IDReusePolicyAllowDuplicateFailedOnly:
		opts.WorkflowIDReusePolicy = enums.WORKFLOW_ID_REUSE_POLICY_ALLOW_DUPLICATE_FAILED_ONLY
	case IDReusePolicyRejectDuplicate:
		opts.WorkflowIDReusePolicy = enums.WORKFLOW_ID_REUSE_POLICY_REJECT_DUPLICATE
	case IDReusePolicyTerminateIfRunning:
		opts.WorkflowIDReusePolicy = enums.WORKFLOW_ID_REUSE_POLICY_TERMINATE_IF_RUNNING
	}

	run, err := cli.ExecuteWorkflow(ctx, opts, workflowFunc, args...)
	if err != nil {
		return nil, err
	}

	return &baseWorkflowRun{
		id:    run.GetID(),
		runID: run.GetRunID(),
	}, nil
}

func (e *TemporalEngine) TerminateWorkflow(ctx context.Context, workflowID string, runID string, reason string) error {
	if err := e.ensureConnected(); err != nil {
		return err
	}
	e.mu.Lock()
	cli := e.client
	e.mu.Unlock()
	return cli.TerminateWorkflow(ctx, workflowID, runID, reason)
}

func (e *TemporalEngine) NewWorker(taskQueue string) WorkflowWorker {
	if err := e.ensureConnected(); err != nil {
		e.log.Error("temporal connection failed; worker will be a no-op", zap.Error(err))
		return &temporalWorker{worker: nil}
	}
	e.mu.Lock()
	cli := e.client
	e.mu.Unlock()

	w := worker.New(cli, taskQueue, worker.Options{})
	return &temporalWorker{worker: w}
}

func (e *TemporalEngine) ProviderName() string {
	return "temporal"
}

func (e *TemporalEngine) Identity() string {
	return "temporal|" + e.hostPort + "|" + e.namespace
}

func (e *TemporalEngine) Close() {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.client != nil {
		e.client.Close()
		e.client = nil
	}
}

type temporalWorker struct {
	worker worker.Worker
}

func (t *temporalWorker) RegisterWorkflow(f any) {
	if t.worker == nil {
		return
	}
	t.worker.RegisterWorkflow(f)
}

func (t *temporalWorker) RegisterActivity(f any) {
	if t.worker == nil {
		return
	}
	t.worker.RegisterActivity(f)
}

func (t *temporalWorker) Run(stopCh <-chan interface{}) error {
	if t.worker == nil {
		return nil
	}
	return t.worker.Run(stopCh)
}

// Logger wrapper
type temporalLogger struct {
	logger *zap.Logger
}

func newTemporalLogger(l *zap.Logger) *temporalLogger {
	return &temporalLogger{logger: l}
}

func (l *temporalLogger) Debug(msg string, keyvals ...interface{}) {
	l.logger.Sugar().Debugw(msg, keyvals...)
}
func (l *temporalLogger) Info(msg string, keyvals ...interface{}) {
	l.logger.Sugar().Infow(msg, keyvals...)
}
func (l *temporalLogger) Warn(msg string, keyvals ...interface{}) {
	l.logger.Sugar().Warnw(msg, keyvals...)
}
func (l *temporalLogger) Error(msg string, keyvals ...interface{}) {
	l.logger.Sugar().Errorw(msg, keyvals...)
}
