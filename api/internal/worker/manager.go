// Package worker — WorkerManager dynamically registers Temporal/Cadence workflow workers
// per client × channel × priority.
//
// Task queue naming: notif-{channel}-{priority}-{clientID}
// e.g. notif-email-high-abc123, notif-sms-low-global
//
// The WorkerManager reconciles every ReconcileInterval by reading active API keys
// from the DB and ensuring a Temporal worker is registered for every combination.
//
// During orchestration migrations (client switches temporal↔cadence or changes
// host/port/namespace), the WorkerManager supports dual-mode operation:
// old-provider workers stay alive until migration completes while new-provider
// workers start serving the same task queues.
package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/spidey/notification-service/internal/autoscaler"
	"github.com/spidey/notification-service/internal/domain"
	"github.com/spidey/notification-service/internal/repository"
	wf "github.com/spidey/notification-service/internal/workflow"
	"go.uber.org/zap"
)

// AutoScalerProvider is the subset of autoscaler.AutoScaler that the
// WorkerManager needs. Defined here to avoid a hard dependency.
type AutoScalerProvider interface {
	GetDesiredParallelism(clientID, channel, priority string) int32
}

const ReconcileInterval = 30 * time.Second

// workerEntry holds a running Temporal worker and its stop channel.
type workerEntry struct {
	w      wf.WorkflowWorker
	stopCh chan interface{}
	// providerName is the workflow engine provider used to register/run this worker.
	providerName string
	// identity is the unique connection string for the engine (e.g. host:port|namespace).
	identity string
}

// workerGroup holds one or more worker goroutines for the same task queue.
// Multiple workers on the same task queue provide parallel consumption.
type workerGroup struct {
	taskQueue    string
	providerName string
	identity     string
	workers      []*workerEntry
	desiredCount int32 // target number of workers from the last reconcile
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

// MigrationWorkerState extends WorkerState with migration-specific breakdowns.
type MigrationWorkerState struct {
	OldWorkerTotal      int                   `json:"old_worker_total"`
	OldByPriority       map[string]int        `json:"old_by_priority"`
	OldByChannel        map[string]int        `json:"old_by_channel"`
	OldByClient         []WorkerClientSummary `json:"old_by_client"`
	NewWorkerTotal      int                   `json:"new_worker_total"`
	NewByPriority       map[string]int        `json:"new_by_priority"`
	NewByChannel        map[string]int        `json:"new_by_channel"`
	NewByClient         []WorkerClientSummary `json:"new_by_client"`
	ActiveMigrationIDs  []string              `json:"active_migration_ids"`
}

// migrationState tracks a client whose orchestration layer is being migrated.
type migrationState struct {
	migrationID  uuid.UUID
	clientID     string
	oldProvider  string
	oldEngine    wf.WorkflowEngine
	newProvider  string
	startedAt    time.Time
}

// runnerKey is the internal key for the running map, combining task queue
// and provider discriminator so that during migration both old and new
// provider workers can coexist on the same task queue name.
func runnerKey(taskQueue, provider, discriminator string) string {
	return taskQueue + "|" + provider + "|" + discriminator
}

// WorkerManager dynamically creates and tears down Temporal/Cadence workflow workers,
// supporting multiple workers per (clientID × channel × priority) task queue for
// parallel consumption.
// Each worker group registers on task queue notif-{channel}-{priority}-{clientID}.
// The Dispatcher routes Kafka messages to the correct task queue.
//
// Auto-scaling:
// When an AutoScalerProvider is attached via SetAutoScaler(), the reconcile loop
// adjusts the number of workers per task queue based on MTTD and Kafka lag.
// Workers are stopped when not needed (scale-down) respecting cooldown periods.
type WorkerManager struct {
	mu      sync.RWMutex
	groups  map[string]*workerGroup // key: runnerKey (taskQueue|provider|discriminator) → group

	// clientNames caches clientID → name from the last reconcile for state reporting.
	clientNames map[string]string

	// engineProvider chooses the workflow engine per API key scope (temporal/cadence/standalone).
	engineProvider workflowEngineProvider

	// vendorConfigRepo provides access to per-client vendor configs (e.g. worker_pool).
	vendorConfigRepo repository.VendorConfigRepository
	autoscaler  AutoScalerProvider
	acts        *wf.Activities
	apiKeyRepo  *repository.APIKeyRepository
	log         *zap.Logger

	// migrations tracks clients currently being migrated.
	// Key: clientID (or "global" for nil scope).
	migrations map[string]*migrationState
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
		groups:     make(map[string]*workerGroup),
		clientNames: make(map[string]string),
		engineProvider: engineProvider,
		vendorConfigRepo: vendorConfigRepo,
		acts:        acts,
		apiKeyRepo:  apiKeyRepo,
		log:         log,
		migrations:  make(map[string]*migrationState),
	}
}

// SetAutoScaler attaches an autoscaler to this WorkerManager.
// The autoscaler's desired parallelism is consulted during each reconcile.
func (m *WorkerManager) SetAutoScaler(as AutoScalerProvider) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.autoscaler = as
	m.log.Info("autoscaler attached to worker manager")
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

// StartMigration tells the WorkerManager to keep old-provider workers alive
// for the given client scope during an orchestration migration.
func (m *WorkerManager) StartMigration(ctx context.Context, migrationID uuid.UUID, apiKeyID *uuid.UUID, oldEngine, newEngine wf.WorkflowEngine) {
	m.mu.Lock()
	defer m.mu.Unlock()

	clientID := clientIDFromPtr(apiKeyID)

	if _, exists := m.migrations[clientID]; exists {
		m.log.Warn("migration already in progress for client, replacing",
			zap.String("client_id", clientID),
			zap.String("migration_id", migrationID.String()),
		)
	}

	oldProvider := ""
	if oldEngine != nil {
		oldProvider = oldEngine.ProviderName()
	}
	newProvider := ""
	if newEngine != nil {
		newProvider = newEngine.ProviderName()
	}

	m.migrations[clientID] = &migrationState{
		migrationID: migrationID,
		clientID:    clientID,
		oldProvider: oldProvider,
		oldEngine:   oldEngine,
		newProvider: newProvider,
		startedAt:   time.Now(),
	}

	m.log.Info("migration started — old workers retained",
		zap.String("client_id", clientID),
		zap.String("migration_id", migrationID.String()),
		zap.String("old_provider", oldProvider),
		zap.String("new_provider", newProvider),
	)
}

// CompleteMigration tells the WorkerManager to stop old-provider workers
// for the given client scope after migration completes.
func (m *WorkerManager) CompleteMigration(ctx context.Context, migrationID uuid.UUID, apiKeyID *uuid.UUID) {
	m.mu.Lock()
	defer m.mu.Unlock()

	clientID := clientIDFromPtr(apiKeyID)
	mig, exists := m.migrations[clientID]
	if !exists {
		return
	}
	if mig.migrationID != migrationID {
		m.log.Warn("migration ID mismatch on completion",
			zap.String("expected", mig.migrationID.String()),
			zap.String("got", migrationID.String()),
		)
		return
	}

	// Stop all old-provider workers for this client.
	discriminator := oldDiscriminator(clientID, migrationID)
	for key, group := range m.groups {
		if group.providerName == mig.oldProvider && containsDiscriminator(key, discriminator) {
			for _, entry := range group.workers {
				close(entry.stopCh)
			}
			delete(m.groups, key)
			m.log.Info("stopped old-provider worker group after migration",
				zap.String("key", key),
				zap.String("provider", mig.oldProvider),
				zap.Int("worker_count", len(group.workers)),
			)
		}
	}

	delete(m.migrations, clientID)
	m.updateWorkerGauge()

	m.log.Info("migration completed — old workers stopped",
		zap.String("client_id", clientID),
		zap.String("migration_id", migrationID.String()),
		zap.String("old_provider", mig.oldProvider),
	)
}

// reconcile re-reads API keys and ensures the right Temporal workers are running.
// With autoscaler support, it also adjusts the number of workers per task queue
// for parallel consumption, stopping excess workers when not needed.
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

	// ── Determine desired parallelism (base + autoscaler) ────────────────
	// tqParallelism maps taskQueue -> target worker count
	tqParallelism := make(map[string]int32)
	for tq := range desired {
		parallelism := int32(1) // base: at least 1 worker

		// Consult autoscaler for per-client/channel/priority scaling
		if m.autoscaler != nil {
			clientID, ch, pr := parseTaskQueue(tq)
			if desiredP := m.autoscaler.GetDesiredParallelism(clientID, ch, pr); desiredP > parallelism {
				parallelism = desiredP
			}
		}

		tqParallelism[tq] = parallelism
	}

	// ── Reconcile standard workers (new provider) ────────────────────────
	type taskQueueTarget struct {
		engine       wf.WorkflowEngine
		providerName string
		parallelism  int32
	}
	targets := make(map[string]*taskQueueTarget)

	for tq, dw := range desired {
		baseKey := runnerKey(tq, dw.providerName, "current")
		targets[baseKey] = &taskQueueTarget{
			engine:       dw.engine,
			providerName: dw.providerName,
			parallelism:  tqParallelism[tq],
		}
	}

	// ── Reconcile migration old-provider workers ─────────────────────────
	for clientID, mig := range m.migrations {
		if mig.oldEngine == nil {
			continue
		}
		discriminator := oldDiscriminator(clientID, mig.migrationID)

		for tq, dw := range desired {
			if !tqMatchesClient(tq, clientID) {
				continue
			}
			if dw.engine.Identity() == mig.oldEngine.Identity() {
				continue
			}

			baseKey := runnerKey(tq, mig.oldProvider, discriminator)
			// Migration workers get same parallelism as their new-provider counterparts
			targets[baseKey] = &taskQueueTarget{
				engine:       mig.oldEngine,
				providerName: mig.oldProvider,
				parallelism:  tqParallelism[tq],
			}
		}
	}

	// ── Reconcile each target group ──────────────────────────────────────
	for baseKey, target := range targets {
		group := m.groups[baseKey]
		if group == nil {
			// New group — create it
			group = &workerGroup{
				taskQueue:    extractTaskQueue(baseKey),
				providerName: target.providerName,
				identity:     target.engine.Identity(),
			}
			m.groups[baseKey] = group
		} else if group.providerName != target.providerName || group.identity != target.engine.Identity() {
			// Provider or identity changed — recreate all workers
			m.log.Info("restarting worker group (config changed)",
				zap.String("key", baseKey),
				zap.String("from_identity", group.identity),
				zap.String("to_identity", target.engine.Identity()),
			)
			for _, entry := range group.workers {
				close(entry.stopCh)
			}
			group.workers = nil
			group.providerName = target.providerName
			group.identity = target.engine.Identity()
		}

		group.desiredCount = target.parallelism

		// Scale up: start missing workers
		currentCount := int32(len(group.workers))
		for i := currentCount; i < target.parallelism; i++ {
			entryKey := fmt.Sprintf("%s|worker-%d", baseKey, i)
			entry, err := m.startSingleWorker(group.taskQueue, target.engine, entryKey)
			if err != nil {
				m.log.Error("failed to start worker",
					zap.String("key", entryKey),
					zap.Error(err),
				)
				continue
			}
			group.workers = append(group.workers, entry)
		}

		// Scale down: stop excess workers
		if int32(len(group.workers)) > target.parallelism {
			excess := int32(len(group.workers)) - target.parallelism
			m.log.Info("scaling down workers",
				zap.String("key", baseKey),
				zap.Int32("from", int32(len(group.workers))),
				zap.Int32("to", target.parallelism),
				zap.Int32("excess", excess),
			)
			// Stop from the end
			stop := group.workers[target.parallelism:]
			for _, entry := range stop {
				close(entry.stopCh)
			}
			group.workers = group.workers[:target.parallelism]
		}
	}

	// ── Remove groups that are no longer desired ─────────────────────────
	for k, group := range m.groups {
		if _, ok := targets[k]; !ok {
			m.log.Info("stopping worker group (no longer desired)", zap.String("key", k))
			for _, entry := range group.workers {
				close(entry.stopCh)
			}
			delete(m.groups, k)
		}
	}

	// ── Export actual parallelism metrics ────────────────────────────────
	m.exportParallelismMetrics()
}

// desiredWorker describes a worker that should be active.
type desiredWorker struct {
	engine       wf.WorkflowEngine
	providerName string
	parallelism  int32 // how many worker goroutines for this task queue
}

// desiredWorkers returns the workflow-worker task queues that must be active.
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
		if minW <= 0 && maxW <= 0 {
			return lenPriorities
		}
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
		if minW <= 0 {
			return lenPriorities
		}
		return minW
	}

	globalPool := getPool(nil)

	// Global workers.
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
		} else if m.engineProvider != nil {
			// Even without a global engine, still register desired workers
			// for any per-client go_routines engines.
		}
	}

	// Per-client workers.
	for _, k := range keys {
		if k == nil || k.RevokedAt != nil {
			continue
		}
		if m.engineProvider == nil {
			continue
		}
		id := k.ID
		engine, err := m.engineProvider.ClientForScope(ctx, &id)
		if err != nil {
			m.log.Warn("failed to get workflow engine for client; skipping worker creation",
				zap.String("client_id", id),
				zap.String("client_name", k.Name),
				zap.Error(err),
			)
			continue
		}
		if engine == nil {
			m.log.Debug("no workflow engine configured for client; skipping worker creation",
				zap.String("client_id", id),
				zap.String("client_name", k.Name),
			)
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

// startSingleWorker creates and starts a single workflow worker on the given
// task queue. Returns the workerEntry. Does NOT hold the lock — the caller must.
func (m *WorkerManager) startSingleWorker(taskQueue string, engine wf.WorkflowEngine, key string) (*workerEntry, error) {
	if engine == nil {
		return nil, nil
	}

	w := engine.NewWorker(taskQueue)

	if engine.ProviderName() == "cadence" {
		w.RegisterWorkflow(wf.NotificationWorkflowCadence)
	} else {
		w.RegisterWorkflow(wf.NotificationWorkflow)
		w.RegisterWorkflow(wf.OtpNotificationWorkflow)
		w.RegisterWorkflow(wf.BulkNotificationWorkflow)
	}

	// Register all activities in the Activities struct.
	w.RegisterActivity(m.acts)

	stopCh := make(chan interface{})
	go func() {
		if err := w.Run(stopCh); err != nil {
			m.log.Error("workflow worker exited with error",
				zap.String("key", key), zap.Error(err))
		}
	}()

	m.log.Info("started workflow worker",
		zap.String("key", key),
		zap.String("task_queue", taskQueue),
		zap.String("provider", engine.ProviderName()),
	)
	m.updateWorkerGauge()

	return &workerEntry{
		w:             w,
		stopCh:        stopCh,
		providerName: engine.ProviderName(),
		identity:      engine.Identity(),
	}, nil
}

// stopAll gracefully stops all running workers.
func (m *WorkerManager) stopAll() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for k, group := range m.groups {
		for _, entry := range group.workers {
			close(entry.stopCh)
		}
		delete(m.groups, k)
		m.log.Info("stopped worker group", zap.String("key", k), zap.Int("count", len(group.workers)))
	}
	m.updateWorkerGauge()
}

// updateWorkerGauge no-op.
func (m *WorkerManager) updateWorkerGauge() {}

// GetState returns a point-in-time snapshot of the worker fleet.
func (m *WorkerManager) GetState() WorkerState {
	m.mu.RLock()
	defer m.mu.RUnlock()

	state := WorkerState{
		ByPriority: make(map[string]int),
		ByChannel:  make(map[string]int),
		UpdatedAt:  time.Now(),
	}

	clientData := make(map[string]*WorkerClientSummary)

	for _, group := range m.groups {
		tq := group.taskQueue
		for _, ch := range AllChannels() {
			for _, p := range AllPriorities() {
				prefix := fmt.Sprintf("notif-%s-%s-", ch, p)
				if len(tq) < len(prefix) || tq[:len(prefix)] != prefix {
					continue
				}
				clientID := tq[len(prefix):]
				chStr := string(ch)
				prStr := string(p)
				count := len(group.workers)

				state.Total += count
				state.ByPriority[prStr] += count
				state.ByChannel[chStr] += count

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
				clientData[clientID].Total += count
				clientData[clientID].ByPriority[prStr] += count
				clientData[clientID].ByChannel[chStr] += count
			}
		}
	}

	for _, s := range clientData {
		state.ByClient = append(state.ByClient, *s)
	}
	return state
}

// GetMigrationWorkersState returns a detailed breakdown of old vs new workers
// during active migrations.
func (m *WorkerManager) GetMigrationWorkersState() MigrationWorkerState {
	m.mu.RLock()
	defer m.mu.RUnlock()

	state := MigrationWorkerState{
		OldByPriority: make(map[string]int),
		OldByChannel:  make(map[string]int),
		NewByPriority: make(map[string]int),
		NewByChannel:  make(map[string]int),
	}

	oldClientData := make(map[string]*WorkerClientSummary)
	newClientData := make(map[string]*WorkerClientSummary)

	for key, group := range m.groups {
		isOld := false
		for _, mig := range m.migrations {
			if group.providerName == mig.oldProvider && containsDiscriminator(key, oldDiscriminator(mig.clientID, mig.migrationID)) {
				isOld = true
				break
			}
		}

		tq := group.taskQueue
		count := len(group.workers)

		for _, ch := range AllChannels() {
			for _, p := range AllPriorities() {
				prefix := fmt.Sprintf("notif-%s-%s-", ch, p)
				if len(tq) < len(prefix) || tq[:len(prefix)] != prefix {
					continue
				}
				clientID := tq[len(prefix):]
				chStr := string(ch)
				prStr := string(p)

				if isOld {
					state.OldWorkerTotal += count
					state.OldByPriority[prStr] += count
					state.OldByChannel[chStr] += count
					updateClientSummary(oldClientData, clientID, chStr, prStr, m.clientNames)
				} else {
					state.NewWorkerTotal += count
					state.NewByPriority[prStr] += count
					state.NewByChannel[chStr] += count
					updateClientSummary(newClientData, clientID, chStr, prStr, m.clientNames)
				}
			}
		}
	}

	for _, s := range oldClientData {
		state.OldByClient = append(state.OldByClient, *s)
	}
	for _, s := range newClientData {
		state.NewByClient = append(state.NewByClient, *s)
	}

	for _, mig := range m.migrations {
		state.ActiveMigrationIDs = append(state.ActiveMigrationIDs, mig.migrationID.String())
	}

	return state
}

// Reload triggers an immediate reconcile.
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

// ── Internal helpers ──────────────────────────────────────────────────────────

func clientIDFromPtr(apiKeyID *uuid.UUID) string {
	if apiKeyID == nil {
		return "global"
	}
	return apiKeyID.String()
}

func oldDiscriminator(clientID string, migrationID uuid.UUID) string {
	return "migration:" + migrationID.String() + ":" + clientID
}

func containsDiscriminator(key, discriminator string) bool {
	// Key format: taskQueue|provider|discriminator
	return len(key) > len(discriminator) && key[len(key)-len(discriminator):] == discriminator
}

func extractTaskQueue(key string) string {
	// Key format: taskQueue|provider|discriminator
	for i := 0; i < len(key); i++ {
		if key[i] == '|' {
			return key[:i]
		}
	}
	return key
}

func tqMatchesClient(tq, clientID string) bool {
	// tq format: notif-{channel}-{priority}-{clientID}
	if clientID == "global" {
		return len(tq) > 6 && tq[len(tq)-6:] == "-global"
	}
	return len(tq) > len(clientID) && tq[len(tq)-len(clientID):] == clientID
}

func updateClientSummary(data map[string]*WorkerClientSummary, clientID, channel, priority string, names map[string]string) {
	if _, ok := data[clientID]; !ok {
		name := names[clientID]
		if name == "" && clientID == "" {
			name = "global"
		}
		data[clientID] = &WorkerClientSummary{
			ClientID:   clientID,
			ClientName: name,
			ByPriority: make(map[string]int),
			ByChannel:  make(map[string]int),
		}
	}
	data[clientID].Total++
	data[clientID].ByPriority[priority]++
	data[clientID].ByChannel[channel]++
}

// parseTaskQueue extracts clientID, channel, and priority from a task queue name.
// Task queue format: notif-{channel}-{priority}-{clientID}
// Returns (clientID, channelStr, priorityStr).
func parseTaskQueue(tq string) (string, string, string) {
	// notif-email-high-abc123
	for _, ch := range AllChannels() {
		for _, p := range AllPriorities() {
			prefix := fmt.Sprintf("notif-%s-%s-", ch, p)
			if len(tq) >= len(prefix) && tq[:len(prefix)] == prefix {
				return tq[len(prefix):], string(ch), string(p)
			}
		}
	}
	return "", "", ""
}

// exportParallelismMetrics exports the actual running worker count per group
// to the autoscaler's Prometheus metric. Must hold m.mu.
func (m *WorkerManager) exportParallelismMetrics() {
	// Reset and repopulate from current worker groups.
	autoscaler.WorkerParallelismActual.Reset()

	counts := make(map[string]int)
	for _, group := range m.groups {
		clientID, ch, pr := parseTaskQueue(group.taskQueue)
		if ch == "" || pr == "" {
			continue
		}
		key := clientID + "|" + ch + "|" + pr
		counts[key] += len(group.workers)
	}

	for key, count := range counts {
		parts := splitThree(key)
		if len(parts) != 3 {
			continue
		}
		autoscaler.WorkerParallelismActual.WithLabelValues(parts[0], parts[1], parts[2]).Set(float64(count))
	}
}

func splitThree(s string) []string {
	out := make([]string, 0, 3)
	start := 0
	for i := 0; i < len(s) && len(out) < 2; i++ {
		if s[i] == '|' {
			out = append(out, s[start:i])
			start = i + 1
		}
	}
	if start <= len(s) {
		out = append(out, s[start:])
	}
	return out
}
