// Package worker — WorkerManager dynamically registers Temporal/Cadence workflow workers
// per client × channel × priority.
//
// Task queue naming: notif-{channel}-{priority}-{clientID}
// e.g. notif-email-high-abc123, notif-sms-low-global
//
// The WorkerManager reconciles every ReconcileInterval by reading active API keys
// from the DB and ensuring a Temporal worker is registered for every combination.
package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/spidey/notification-service/internal/domain"
	"github.com/spidey/notification-service/internal/repository"
	wf "github.com/spidey/notification-service/internal/workflow"
	"go.uber.org/zap"
)

const ReconcileInterval = 30 * time.Second

// workerEntry holds a running Temporal worker and its stop channel.
type workerEntry struct {
	w      wf.WorkflowWorker
	stopCh chan interface{}
	// providerName is the workflow engine provider used to register/run this worker.
	// Used to restart the worker when a client switches temporal/cadence.
	providerName string
}

// WorkerClientSummary aggregates worker counts for a single client (or global).
type WorkerClientSummary struct {
	ClientID   string         `json:"client_id"`
	ClientName string         `json:"client_name"`
	Total      int            `json:"total"`
	ByPriority map[string]int `json:"by_priority"`
	ByChannel  map[string]int `json:"by_channel"`
}

// WorkerState is a snapshot of the current WorkerManager fleet, safe to serialize.
type WorkerState struct {
	Total      int                   `json:"total"`
	ByPriority map[string]int        `json:"by_priority"`
	ByChannel  map[string]int        `json:"by_channel"`
	ByClient   []WorkerClientSummary `json:"by_client"`
	UpdatedAt  time.Time             `json:"updated_at"`
}

// WorkerManager dynamically creates and tears down Temporal/Cadence workflow workers,
// one per (clientID × channel × priority) combination.
// Each worker registers on task queue notif-{channel}-{priority}-{clientID}.
// The Dispatcher routes Kafka messages to the correct task queue, so each client's
// notifications are handled by its own dedicated Temporal worker independently.
type WorkerManager struct {
	mu      sync.RWMutex
	running map[string]*workerEntry // taskQueue → entry

	// clientNames caches clientID → name from the last reconcile for state reporting.
	clientNames map[string]string

	// engineProvider chooses the workflow engine per API key scope (temporal/cadence/standalone).
	engineProvider workflowEngineProvider

	// vendorConfigRepo provides access to per-client vendor configs (e.g. worker_pool).
	vendorConfigRepo repository.VendorConfigRepository
	acts       *wf.Activities
	apiKeyRepo *repository.APIKeyRepository
	log        *zap.Logger
}

// workflowEngineProvider abstracts service.WorkflowClientProvider so the worker package
// can be wired without depending on internal/service.
type workflowEngineProvider interface {
	ClientForScope(ctx context.Context, apiKeyID *string) (wf.WorkflowEngine, error)
}

func NewWorkerManager(
	engineProvider workflowEngineProvider,
	acts *wf.Activities,
	apiKeyRepo *repository.APIKeyRepository,
	vendorConfigRepo repository.VendorConfigRepository,
	log *zap.Logger,
) *WorkerManager {
	return &WorkerManager{
		running:     make(map[string]*workerEntry),
		clientNames: make(map[string]string),
		engineProvider: engineProvider,
		vendorConfigRepo: vendorConfigRepo,
		acts:        acts,
		apiKeyRepo:  apiKeyRepo,
		log:         log,
	}
}

// Run starts the reconciliation loop. Blocks until ctx is cancelled.
func (m *WorkerManager) Run(ctx context.Context) {
	m.log.Info("worker manager started")
	m.reconcile(ctx) // initial reconcile on startup

	t := time.NewTicker(ReconcileInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			m.stopAll()
			m.log.Info("worker manager stopped")
			return
		case <-t.C:
			m.reconcile(ctx)
		}
	}
}

// reconcile re-reads API keys and ensures the right Temporal workers are running.
func (m *WorkerManager) reconcile(ctx context.Context) {
	m.log.Debug("reconciling workflow workers")

	keys, err := m.apiKeyRepo.List(ctx)
	if err != nil {
		m.log.Error("worker manager: failed to list api keys", zap.Error(err))
		return
	}

	desired := m.desiredWorkers(ctx, keys)

	m.mu.Lock()
	defer m.mu.Unlock()

	// Update client name cache from the freshly fetched keys.
	for _, k := range keys {
		m.clientNames[k.ID] = k.Name
	}

	// Start/refresh workers for desired combinations.
	for tq, dw := range desired {
		if existing, ok := m.running[tq]; ok {
			if existing.providerName != dw.providerName {
				m.log.Info("restarting workflow worker (provider changed)",
					zap.String("task_queue", tq),
					zap.String("from_provider", existing.providerName),
					zap.String("to_provider", dw.providerName),
				)
				close(existing.stopCh)
				delete(m.running, tq)
			} else {
				continue
			}
		}

		if err := m.startWorkerLocked(ctx, tq, dw.engine); err != nil {
			m.log.Error("failed to start workflow worker",
				zap.String("task_queue", tq),
				zap.String("provider", dw.providerName),
				zap.Error(err),
			)
		}
	}

	// Stop workers for removed combinations.
	for tq, entry := range m.running {
		if _, ok := desired[tq]; !ok {
			m.log.Info("stopping workflow worker (key removed)", zap.String("task_queue", tq))
			close(entry.stopCh)
			delete(m.running, tq)
		}
	}
}

type desiredWorker struct {
	engine       wf.WorkflowEngine
	providerName string
}

// desiredWorkers returns the workflow-worker task queues that must be active.
// It filters per-client on the engine returned by WorkflowClientProvider (temporal/cadence/standalone).
func (m *WorkerManager) desiredWorkers(ctx context.Context, keys []*domain.APIKey) map[string]*desiredWorker {
	set := make(map[string]*desiredWorker)

	priorities := AllPriorities()
	lenPriorities := len(priorities)

	type channelWorkerPool struct {
		MinWorkers int `json:"min_workers"`
		MaxWorkers int `json:"max_workers"`
	}
	type workerPoolConfig struct {
		Email     channelWorkerPool `json:"email"`
		SMS       channelWorkerPool `json:"sms"`
		Push      channelWorkerPool `json:"push"`
		Webhook   channelWorkerPool `json:"webhook"`
		WebSocket channelWorkerPool `json:"websocket"`
		Slack     channelWorkerPool `json:"slack"`
	}

	getPool := func(apiKeyID *string) *workerPoolConfig {
		if m.vendorConfigRepo == nil {
			return nil
		}
		vc, err := m.vendorConfigRepo.GetByType(ctx, "worker_pool", apiKeyID)
		if err != nil || vc == nil || len(vc.ConfigJSON) == 0 {
			return nil
		}
		var out workerPoolConfig
		if err := json.Unmarshal(vc.ConfigJSON, &out); err != nil {
			return nil
		}
		return &out
	}

	enabledPriorityCount := func(ch domain.Channel, pool *workerPoolConfig) int {
		// Channels not covered by the pool config keep the default: all priority tiers.
		if pool == nil {
			return lenPriorities
		}

		var minW, maxW int
		switch ch {
		case domain.ChannelEmail:
			minW, maxW = pool.Email.MinWorkers, pool.Email.MaxWorkers
		case domain.ChannelSMS:
			minW, maxW = pool.SMS.MinWorkers, pool.SMS.MaxWorkers
		case domain.ChannelPush:
			minW, maxW = pool.Push.MinWorkers, pool.Push.MaxWorkers
		case domain.ChannelWebhook:
			minW, maxW = pool.Webhook.MinWorkers, pool.Webhook.MaxWorkers
		case domain.ChannelWebSocket:
			minW, maxW = pool.WebSocket.MinWorkers, pool.WebSocket.MaxWorkers
		case domain.ChannelSlack:
			minW, maxW = pool.Slack.MinWorkers, pool.Slack.MaxWorkers
		default:
			return lenPriorities
		}

		// If both are zero/unset, default to all tiers.
		if minW <= 0 && maxW <= 0 {
			return lenPriorities
		}

		// Clamp to valid bounds.
		if minW < 0 {
			minW = 0
		}
		if maxW < 0 {
			maxW = 0
		}
		if minW > lenPriorities {
			minW = lenPriorities
		}
		if maxW > lenPriorities {
			maxW = lenPriorities
		}

		if maxW > 0 {
			if maxW < minW {
				return maxW
			}
			return maxW
		}
		// No max specified => use min.
		if minW <= 0 {
			return lenPriorities
		}
		return minW
	}

	// Pre-load global worker pool config once per reconcile.
	globalPool := getPool(nil)

	// Global workers: handle notifications without a client scope.
	// If the engine provider returns nil for global, we skip global workers.
	if m.engineProvider != nil {
		if globalEngine, _ := m.engineProvider.ClientForScope(ctx, nil); globalEngine != nil {
			for _, ch := range AllChannels() {
				enabled := enabledPriorityCount(ch, globalPool)
				if enabled > lenPriorities {
					enabled = lenPriorities
				}
				for i := 0; i < enabled; i++ {
					p := priorities[i]
					set[wf.TaskQueueFor(ch, p, "")] = &desiredWorker{engine: globalEngine, providerName: globalEngine.ProviderName()}
				}
			}
		}
	}

	// Per-client workers: one task queue per key × channel × priority.
	for _, k := range keys {
		// Revoked clients should not have any workers.
		if k == nil || k.RevokedAt != nil {
			continue
		}
		if m.engineProvider == nil {
			continue
		}
		id := k.ID
		engine, _ := m.engineProvider.ClientForScope(ctx, &id)
		if engine == nil {
			// Provider "standalone" (or missing config + defaultEngine nil) => no workers for this client.
			continue
		}

		pool := getPool(&id)

		for _, ch := range AllChannels() {
			enabled := enabledPriorityCount(ch, pool)
			if enabled > lenPriorities {
				enabled = lenPriorities
			}
			for i := 0; i < enabled; i++ {
				p := priorities[i]
				set[wf.TaskQueueFor(ch, p, k.ID)] = &desiredWorker{
					engine:       engine,
					providerName: engine.ProviderName(),
				}
			}
		}
	}

	return set
}

// startWorkerLocked registers a new workflow worker on taskQueue. Must hold m.mu.
func (m *WorkerManager) startWorkerLocked(ctx context.Context, taskQueue string, engine wf.WorkflowEngine) error {
	if engine == nil {
		return nil
	}

	w := engine.NewWorker(taskQueue)

	if engine.ProviderName() == "cadence" {
		w.RegisterWorkflow(wf.NotificationWorkflowCadence)
	} else {
		w.RegisterWorkflow(wf.NotificationWorkflow)
		w.RegisterWorkflow(wf.OtpNotificationWorkflow)
		w.RegisterWorkflow(wf.BulkNotificationWorkflow)
	}

	w.RegisterActivity(m.acts.CheckPreferencesActivity)
	w.RegisterActivity(m.acts.RenderTemplateActivity)
	w.RegisterActivity(m.acts.DeliverNotificationActivity)
	w.RegisterActivity(m.acts.LogDeliveryActivity)
	w.RegisterActivity(m.acts.GenerateOtpActivity)

	stopCh := make(chan interface{})
	go func() {
		if err := w.Run(stopCh); err != nil {
			m.log.Error("workflow worker exited with error",
				zap.String("task_queue", taskQueue), zap.Error(err))
		}
	}()

	m.running[taskQueue] = &workerEntry{
		w:             w,
		stopCh:        stopCh,
		providerName: engine.ProviderName(),
	}
	m.log.Info("started workflow worker",
		zap.String("task_queue", taskQueue),
		zap.String("provider", engine.ProviderName()),
	)
	m.updateWorkerGauge()
	return nil
}

// stopAll gracefully stops all running workers.
func (m *WorkerManager) stopAll() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for tq, entry := range m.running {
		close(entry.stopCh)
		delete(m.running, tq)
		m.log.Info("stopped temporal worker", zap.String("task_queue", tq))
	}
	m.updateWorkerGauge()
}

// updateWorkerGauge used to emit Prometheus metrics (TemporalWorkersActive).
// Prometheus/Grafana are intentionally removed from this service for now.
func (m *WorkerManager) updateWorkerGauge() {
	// no-op
}

// GetState returns a point-in-time snapshot of the worker fleet, safe to serve over HTTP.
func (m *WorkerManager) GetState() WorkerState {
	m.mu.RLock()
	defer m.mu.RUnlock()

	state := WorkerState{
		ByPriority: make(map[string]int),
		ByChannel:  make(map[string]int),
		UpdatedAt:  time.Now(),
	}

	// clientData: clientID → *WorkerClientSummary
	clientData := make(map[string]*WorkerClientSummary)

	for tq := range m.running {
		for _, ch := range AllChannels() {
			for _, p := range AllPriorities() {
				prefix := fmt.Sprintf("notif-%s-%s-", ch, p)
				if len(tq) < len(prefix) || tq[:len(prefix)] != prefix {
					continue
				}
				clientID := tq[len(prefix):]
				chStr := string(ch)
				prStr := string(p)

				state.Total++
				state.ByPriority[prStr]++
				state.ByChannel[chStr]++

				if _, ok := clientData[clientID]; !ok {
					name := m.clientNames[clientID]
					if name == "" && clientID == "" {
						name = "global"
					}
					clientData[clientID] = &WorkerClientSummary{
						ClientID:   clientID,
						ClientName: name,
						ByPriority: make(map[string]int),
						ByChannel:  make(map[string]int),
					}
				}
				clientData[clientID].Total++
				clientData[clientID].ByPriority[prStr]++
				clientData[clientID].ByChannel[chStr]++
			}
		}
	}

	for _, s := range clientData {
		state.ByClient = append(state.ByClient, *s)
	}

	return state
}

// Reload triggers an immediate reconcile (e.g. after a new API key is created).
func (m *WorkerManager) Reload(ctx context.Context) {
	m.reconcile(ctx)
}

// AllChannels returns every supported delivery channel.
func AllChannels() []domain.Channel {
	return []domain.Channel{
		domain.ChannelEmail,
		domain.ChannelSMS,
		domain.ChannelPush,
		domain.ChannelWebhook,
		domain.ChannelSlack,
		domain.ChannelWebSocket,
	}
}

// AllPriorities returns all priority tiers.
func AllPriorities() []domain.Priority {
	return []domain.Priority{
		domain.PriorityHigh,
		domain.PriorityMedium,
		domain.PriorityLow,
	}
}
