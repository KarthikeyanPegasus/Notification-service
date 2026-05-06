package workflow

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/spidey/notification-service/internal/config"
	"go.uber.org/cadence/activity"
	"go.uber.org/cadence/.gen/go/cadence/workflowserviceclient"
	"go.uber.org/cadence/client"
	"go.uber.org/cadence/worker"
	"go.uber.org/yarpc"
	"go.uber.org/yarpc/transport/tchannel"
	"go.uber.org/zap"
)

type CadenceEngine struct {
	mu         sync.Mutex
	client     client.Client
	service    workflowserviceclient.Interface
	domain     string
	hostPort   string
	dispatcher *yarpc.Dispatcher
	log        *zap.Logger
}

func NewCadenceEngine(cfg *config.Config, log *zap.Logger) (*CadenceEngine, error) {
	return NewCadenceEngineWith(cfg.Cadence.HostPort, cfg.Cadence.Domain, log)
}

func NewCadenceEngineWith(hostPort string, domain string, log *zap.Logger) (*CadenceEngine, error) {
	e := &CadenceEngine{
		domain:   domain,
		hostPort: hostPort,
		log:      log,
	}
	// Best-effort connection on startup; lazy reconnect handles failures later.
	if err := e.connect(); err != nil {
		log.Warn("failed to connect to cadence on startup; will retry on first use", zap.Error(err))
	}
	return e, nil
}

// connect (re)builds the YARPC dispatcher, service client, and workflow client.
// Caller must hold e.mu or call during single-threaded init.
func (e *CadenceEngine) connect() error {
	// Tear down existing dispatcher if present.
	if e.dispatcher != nil {
		_ = e.dispatcher.Stop()
		e.dispatcher = nil
		e.client = nil
		e.service = nil
	}

	ch, err := tchannel.NewChannelTransport(tchannel.ServiceName("notification-service"))
	if err != nil {
		return err
	}

	dispatcher := yarpc.NewDispatcher(yarpc.Config{
		Name: "notification-service",
		Outbounds: yarpc.Outbounds{
			"cadence-frontend": {Unary: ch.NewSingleOutbound(e.hostPort)},
		},
	})

	if err := dispatcher.Start(); err != nil {
		return err
	}

	svc := workflowserviceclient.New(dispatcher.ClientConfig("cadence-frontend"))
	cli := client.NewClient(svc, e.domain, &client.Options{})

	e.dispatcher = dispatcher
	e.service = svc
	e.client = cli
	return nil
}

// ensureConnected lazily reconnects if the client was never initialized or lost.
func (e *CadenceEngine) ensureConnected() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.client != nil && e.service != nil {
		return nil
	}
	return e.connect()
}

func (e *CadenceEngine) ExecuteWorkflow(ctx context.Context, options StartOptions, workflowFunc any, args ...any) (WorkflowRun, error) {
	if err := e.ensureConnected(); err != nil {
		return nil, errors.New("cadence client not initialized: " + err.Error())
	}
	e.mu.Lock()
	cli := e.client
	e.mu.Unlock()

	opts := client.StartWorkflowOptions{
		ID:                              options.ID,
		TaskList:                        options.TaskQueue,
		ExecutionStartToCloseTimeout:    10 * time.Minute,
		DecisionTaskStartToCloseTimeout: 10 * time.Second,
	}

	switch options.WorkflowIDReusePolicy {
	case IDReusePolicyAllowDuplicate:
		opts.WorkflowIDReusePolicy = client.WorkflowIDReusePolicyAllowDuplicate
	case IDReusePolicyRejectDuplicate:
		opts.WorkflowIDReusePolicy = client.WorkflowIDReusePolicyRejectDuplicate
	case IDReusePolicyTerminateIfRunning:
		opts.WorkflowIDReusePolicy = client.WorkflowIDReusePolicyTerminateIfRunning
	}

	run, err := cli.StartWorkflow(ctx, opts, workflowFunc, args...)
	if err != nil {
		return nil, err
	}

	return &baseWorkflowRun{
		id:    run.ID,
		runID: run.RunID,
	}, nil
}

func (e *CadenceEngine) TerminateWorkflow(ctx context.Context, workflowID string, runID string, reason string) error {
	if err := e.ensureConnected(); err != nil {
		return err
	}
	e.mu.Lock()
	cli := e.client
	e.mu.Unlock()
	return cli.TerminateWorkflow(ctx, workflowID, runID, reason, nil)
}

func (e *CadenceEngine) NewWorker(taskQueue string) WorkflowWorker {
	if err := e.ensureConnected(); err != nil {
		e.log.Error("cadence connection failed; worker will be a no-op", zap.Error(err))
		return &cadenceWorker{worker: nil}
	}
	e.mu.Lock()
	svc := e.service
	domain := e.domain
	e.mu.Unlock()

	w, err := worker.NewV2(svc, domain, taskQueue, worker.Options{
		Logger: e.log,
	})
	if err != nil {
		e.log.Error("failed to create cadence worker", zap.Error(err))
		return &cadenceWorker{worker: nil}
	}
	return &cadenceWorker{worker: w}
}

func (e *CadenceEngine) ProviderName() string {
	return "cadence"
}

func (e *CadenceEngine) Identity() string {
	return "cadence|" + e.hostPort + "|" + e.domain
}

func (e *CadenceEngine) Close() {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.dispatcher != nil {
		_ = e.dispatcher.Stop()
		e.dispatcher = nil
	}
}

type cadenceWorker struct {
	worker worker.Worker
}

func (c *cadenceWorker) RegisterWorkflow(f any) {
	if c.worker == nil {
		return
	}
	c.worker.RegisterWorkflow(f)
}

func (c *cadenceWorker) RegisterActivity(f any) {
	if c.worker == nil {
		return
	}
	// Register with short name so `workflow.ExecuteActivity(ctx, "X", ...)` resolves correctly.
	c.worker.RegisterActivityWithOptions(f, activity.RegisterOptions{EnableShortName: true})
}

func (c *cadenceWorker) Run(stopCh <-chan interface{}) error {
	if c.worker == nil {
		return nil
	}
	// Cadence's worker.Run() blocks until worker.Stop() is called — it has no stopCh param.
	errCh := make(chan error, 1)
	go func() {
		errCh <- c.worker.Run()
	}()
	select {
	case <-stopCh:
		c.worker.Stop()
		return nil
	case err := <-errCh:
		return err
	}
}
