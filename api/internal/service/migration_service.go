package service

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/spidey/notification-service/internal/domain"
	"github.com/spidey/notification-service/internal/repository"
	"github.com/spidey/notification-service/internal/workflow"
	"go.uber.org/zap"
)

// MigrationManager orchestrates the smooth migration of a client's workflow
// orchestration from one Temporal/Cadence provider to another.
//
// When a user updates their workflow_orchestration config, the config service
// calls TriggerMigration which:
//  1. Creates a migration record in the DB
//  2. Transfers scheduled notifications (5+ mins from now) to the new engine
//  3. Coordinates with WorkerManager to keep old workers alive
//  4. Monitors old workflow completion
//  5. Completes the migration and notifies the admin
type MigrationManager struct {
	migrationRepo   *repository.MigrationRepository
	schedRepo       *repository.ScheduledRepository
	notifRepo       *repository.NotificationRepository
	eventRepo       *repository.EventRepository
	apiKeyRepo      *repository.APIKeyRepository
	wfClients       *WorkflowClientProvider
	vendorConfigRepo repository.VendorConfigRepository
	log             *zap.Logger

	mu          sync.RWMutex
	activeMigrations map[uuid.UUID]context.CancelFunc // migration ID -> cancel

	// WorkerManager hook: set by WorkerManager on startup so MigrationManager
	// can signal when old workers should be kept alive.
	onStartMigration func(ctx context.Context, migrationID uuid.UUID, apiKeyID *uuid.UUID, oldEngine, newEngine workflow.WorkflowEngine)
	onCompleteMigration func(ctx context.Context, migrationID uuid.UUID, apiKeyID *uuid.UUID)
}

func NewMigrationManager(
	migrationRepo *repository.MigrationRepository,
	schedRepo *repository.ScheduledRepository,
	notifRepo *repository.NotificationRepository,
	eventRepo *repository.EventRepository,
	apiKeyRepo *repository.APIKeyRepository,
	wfClients *WorkflowClientProvider,
	vendorConfigRepo repository.VendorConfigRepository,
	log *zap.Logger,
) *MigrationManager {
	return &MigrationManager{
		migrationRepo:     migrationRepo,
		schedRepo:         schedRepo,
		notifRepo:         notifRepo,
		eventRepo:         eventRepo,
		apiKeyRepo:        apiKeyRepo,
		wfClients:         wfClients,
		vendorConfigRepo:  vendorConfigRepo,
		log:               log,
		activeMigrations:  make(map[uuid.UUID]context.CancelFunc),
	}
}

// SetWorkerManagerHooks allows the WorkerManager to register callbacks so that
// it knows when to keep old workers alive during a migration.
func (m *MigrationManager) SetWorkerManagerHooks(
	onStart func(ctx context.Context, migrationID uuid.UUID, apiKeyID *uuid.UUID, oldEngine, newEngine workflow.WorkflowEngine),
	onComplete func(ctx context.Context, migrationID uuid.UUID, apiKeyID *uuid.UUID),
) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onStartMigration = onStart
	m.onCompleteMigration = onComplete
}

// TriggerMigration initiates a migration when a workflow_orchestration config changes.
// It is called synchronously by the config service, then spawns a background migration.
func (m *MigrationManager) TriggerMigration(ctx context.Context, apiKeyID *string, oldConfigJSON, newConfigJSON json.RawMessage) error {
	// Parse old and new configs to detect what changed.
	var oldCfg, newCfg workflowOrchestrationConfig
	if err := json.Unmarshal(oldConfigJSON, &oldCfg); err != nil {
		return fmt.Errorf("parsing old orchestration config: %w", err)
	}
	if err := json.Unmarshal(newConfigJSON, &newCfg); err != nil {
		return fmt.Errorf("parsing new orchestration config: %w", err)
	}

	// Determine old and new engines.
	oldEngine, _ := m.wfClients.EngineFromConfig(ctx, oldConfigJSON)
	newEngine, _ := m.wfClients.EngineFromConfig(ctx, newConfigJSON)

	// Resolve client name.
	clientName := ""
	if apiKeyID != nil && *apiKeyID != "" {
		keys, err := m.apiKeyRepo.ListByIDs(ctx, []string{*apiKeyID})
		if err == nil && len(keys) > 0 {
			clientName = keys[0].Name
		}
	}

	// Create migration record.
	var apiKeyUUID *uuid.UUID
	if apiKeyID != nil && *apiKeyID != "" {
		parsed, err := uuid.Parse(*apiKeyID)
		if err == nil {
			apiKeyUUID = &parsed
		}
	}

	migration := &domain.OrchestrationMigration{
		ID:            uuid.New(),
		APIKeyID:      apiKeyUUID,
		ClientName:    clientName,
		OldProvider:   m.resolveProvider(oldCfg),
		NewProvider:   m.resolveProvider(newCfg),
		OldConfigJSON: oldConfigJSON,
		NewConfigJSON: newConfigJSON,
		Status:        domain.MigrationInProgress,
		StartedAt:     time.Now(),
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}

	if err := m.migrationRepo.Create(ctx, migration); err != nil {
		return fmt.Errorf("creating migration record: %w", err)
	}

	m.log.Info("orchestration migration triggered",
		zap.String("migration_id", migration.ID.String()),
		zap.String("api_key_id", safeString(apiKeyID)),
		zap.String("old_provider", migration.OldProvider),
		zap.String("new_provider", migration.NewProvider),
	)

	// Start background migration process.
	mCtx, cancel := context.WithCancel(context.Background())
	m.mu.Lock()
	m.activeMigrations[migration.ID] = cancel
	m.mu.Unlock()

	// Signal WorkerManager to keep old workers alive alongside new ones.
	if m.onStartMigration != nil {
		m.onStartMigration(mCtx, migration.ID, apiKeyUUID, oldEngine, newEngine)
	}

	go m.runMigration(mCtx, migration)

	return nil
}

func (m *MigrationManager) resolveProvider(cfg workflowOrchestrationConfig) string {
	provider := cfg.Provider
	if provider == "" {
		provider = "temporal"
	}
	return provider
}

// runMigration executes the full migration lifecycle in a background goroutine.
func (m *MigrationManager) runMigration(ctx context.Context, migration *domain.OrchestrationMigration) {
	m.log.Info("migration started",
		zap.String("migration_id", migration.ID.String()),
		zap.String("old", migration.OldProvider),
		zap.String("new", migration.NewProvider),
	)

	// ── Phase 1: Transfer scheduled notifications ──────────────────────────
	m.transferScheduledNotifications(ctx, migration)

	// ── Phase 2: Wait for old workflows to complete ────────────────────────
	m.waitForOldWorkflows(ctx, migration)

	// ── Phase 3: Complete migration ───────────────────────────────────────
	m.finalizeMigration(ctx, migration)
}

// transferScheduledNotifications moves scheduled notifications (5+ mins from now)
// from the old orchestration to the new one.
func (m *MigrationManager) transferScheduledNotifications(ctx context.Context, migration *domain.OrchestrationMigration) {
	m.log.Info("phase: transferring scheduled notifications",
		zap.String("migration_id", migration.ID.String()),
	)

	// Update status to transferring.
	_ = m.migrationRepo.UpdateStatus(ctx, migration.ID, domain.MigrationTransferringSched, 0, 0)

	var apiKeyID string
	if migration.APIKeyID != nil {
		apiKeyID = migration.APIKeyID.String()
	}

	// List pending scheduled notifications for this client.
	scheduledList, total, err := m.schedRepo.List(ctx, []domain.NotificationStatus{domain.StatusPending}, 1, 10000, nil)
	if err != nil || total == 0 {
		m.log.Warn("no scheduled notifications found for transfer",
			zap.String("migration_id", migration.ID.String()),
			zap.Error(err),
		)
		_ = m.migrationRepo.UpdateCounts(ctx, migration.ID, 0, 0, 0, 0)
		return
	}

	totalInt := int(total)

	cutoff := time.Now().Add(5 * time.Minute)
	transferable := 0
	tooSoon := 0

	for _, sched := range scheduledList {
		if ctx.Err() != nil {
			return
		}

		// Only consider schedules belonging to this client scope.
		if !migrationBelongsToClient(sched.APIKeyID, migration.APIKeyID) {
			continue
		}

		// Only transfer those scheduled 5+ minutes from now.
		if sched.ScheduledAt.Before(cutoff) {
			tooSoon++
			continue
		}

		// Transfer this notification: terminate old workflow, start on new engine.
		if err := m.transferSingleScheduled(ctx, migration, sched, apiKeyID); err != nil {
			m.log.Warn("failed to transfer scheduled notification",
				zap.String("migration_id", migration.ID.String()),
				zap.String("notification_id", sched.NotificationID.String()),
				zap.Error(err),
			)
			continue
		}
		transferable++
	}

	_ = m.migrationRepo.UpdateCounts(ctx, migration.ID, tooSoon, 0, totalInt, transferable)

	m.log.Info("scheduled notification transfer complete",
		zap.String("migration_id", migration.ID.String()),
		zap.Int("transferred", transferable),
		zap.Int("too_soon", tooSoon),
	)
}

func migrationBelongsToClient(schedAPIKeyID *uuid.UUID, migrationAPIKeyID *uuid.UUID) bool {
	if migrationAPIKeyID == nil {
		// Global scope: only match global schedules.
		return schedAPIKeyID == nil
	}
	if schedAPIKeyID == nil {
		return false
	}
	return *schedAPIKeyID == *migrationAPIKeyID
}

// transferSingleScheduled terminates the old workflow and starts it on the new engine.
func (m *MigrationManager) transferSingleScheduled(ctx context.Context, migration *domain.OrchestrationMigration, sched *domain.ScheduledNotification, apiKeyID string) error {
	// Get the new engine for this scope.
	newEngine, err := m.wfClients.ClientForScope(ctx, &apiKeyID)
	if err != nil || newEngine == nil {
		return fmt.Errorf("new engine unavailable: %w", err)
	}

	// Terminate the old workflow (best-effort; workflow may already be done).
	_ = newEngine.TerminateWorkflow(ctx, sched.WorkflowID, sched.RunID, "migration-transfer")

	// Load the full notification for the transfer.
	n, err := m.notifRepo.GetByID(ctx, sched.NotificationID)
	if err != nil {
		return fmt.Errorf("loading notification: %w", err)
	}

	// Build workflow request.
	req := &workflow.WorkflowRequest{
		ID:             n.ID,
		Channel:        n.Channel,
		Priority:       n.Priority,
		Recipient:      n.Recipient,
		Type:           n.Type,
		IdempotencyKey: n.IdempotencyKey,
		ForcedVendor:   n.ForcedVendor,
		ClientID:       apiKeyID,
	}
	if n.TemplateID != nil {
		tid := n.TemplateID.String()
		req.TemplateID = &tid
		if len(sched.TemplateVars) > 0 {
			req.TemplateVariables = sched.TemplateVars
		}
	}

	delaySeconds := int(time.Until(sched.ScheduledAt).Seconds())
	taskQueue := workflow.TaskQueueFor(n.Channel, n.Priority, apiKeyID)
	options := workflow.StartOptions{
		ID:                    sched.WorkflowID,
		TaskQueue:             taskQueue,
		WorkflowIDReusePolicy: workflow.IDReusePolicyAllowDuplicate,
		StartDelay:            time.Duration(delaySeconds) * time.Second,
	}

	var workflowFunc any = workflow.NotificationWorkflow
	if newEngine.ProviderName() == "cadence" {
		workflowFunc = workflow.NotificationWorkflowCadence
	}

	run, err := newEngine.ExecuteWorkflow(ctx, options, workflowFunc, req)
	if err != nil {
		return fmt.Errorf("re-starting workflow on new engine: %w", err)
	}

	newRunID := run.GetRunID()

	// Update the scheduled record with the new run ID.
	if err := m.schedRepo.UpdateSchedule(ctx, sched.NotificationID, sched.ScheduledAt, newRunID); err != nil {
		return fmt.Errorf("updating schedule: %w", err)
	}

	// Append event to notification timeline.
	_ = m.eventRepo.Append(ctx, &domain.NotificationEvent{
		ID:             uuid.New(),
		NotificationID: sched.NotificationID,
		EventType:      domain.EventQueued,
		Metadata: map[string]any{
			"migration_id":  migration.ID.String(),
			"new_engine":    newEngine.ProviderName(),
			"new_run_id":    newRunID,
			"reason":        "orchestration_migration",
		},
		CreatedAt: time.Now(),
	})

	m.log.Debug("transferred scheduled notification",
		zap.String("migration_id", migration.ID.String()),
		zap.String("notification_id", sched.NotificationID.String()),
		zap.String("new_engine", newEngine.ProviderName()),
		zap.String("new_run_id", newRunID),
	)

	return nil
}

// waitForOldWorkflows polls until all old workflows are completed or timeout.
func (m *MigrationManager) waitForOldWorkflows(ctx context.Context, migration *domain.OrchestrationMigration) {
	m.log.Info("phase: waiting for old workflows to complete",
		zap.String("migration_id", migration.ID.String()),
	)

	_ = m.migrationRepo.UpdateStatus(ctx, migration.ID, domain.MigrationWaitingOldWorkers, 0, 0)

	// Poll every 10 seconds for up to 30 minutes.
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	timeout := time.After(30 * time.Minute)
	maxWait := 30 * time.Minute

	startTime := time.Now()

	for {
		select {
		case <-ctx.Done():
			return
		case <-timeout:
			m.log.Warn("migration wait timeout — completing anyway",
				zap.String("migration_id", migration.ID.String()),
			)
			return
		case <-ticker.C:
		}

		elapsed := time.Since(startTime)
		if elapsed > maxWait {
			return
		}

		// Count remaining old workflows:
		// 1. Pending scheduled notifications for this client that are due <5 mins
		// 2. Notifications created in the last 2 minutes (in-flight immediate delivery)
		remaining, err := m.countRemainingOldWorkflows(ctx, migration)
		if err != nil {
			m.log.Warn("failed to count remaining old workflows",
				zap.String("migration_id", migration.ID.String()),
				zap.Error(err),
			)
			continue
		}

		_ = m.migrationRepo.UpdateCounts(ctx, migration.ID, migration.OldWorkflowCount,
			migration.OldWorkflowCount-remaining, migration.TotalScheduledCount,
			migration.MigratedScheduledCount,
		)

		m.log.Debug("old workflow status",
			zap.String("migration_id", migration.ID.String()),
			zap.Int("remaining", remaining),
		)

		if remaining <= 0 {
			m.log.Info("all old workflows completed",
				zap.String("migration_id", migration.ID.String()),
			)
			return
		}
	}
}

// countRemainingOldWorkflows counts workflows still running on the old orchestration.
func (m *MigrationManager) countRemainingOldWorkflows(ctx context.Context, migration *domain.OrchestrationMigration) (int, error) {
	if migration.APIKeyID != nil {
		// Count pending scheduled notifications for this client that were NOT transferred
		// (those scheduled <5 mins from now at migration start).
		// Also count notifications created/admitted within the last 2 SLA windows.
		slaWindow := time.Now().Add(-2 * time.Minute)
		count, err := m.notifRepo.CountClientNotificationsSince(ctx, *migration.APIKeyID, slaWindow)
		if err != nil {
			return 0, err
		}
		return count, nil
	}
	return 0, nil
}

// finalizeMigration completes the migration and signals the WorkerManager.
func (m *MigrationManager) finalizeMigration(ctx context.Context, migration *domain.OrchestrationMigration) {
	m.log.Info("phase: finalizing migration",
		zap.String("migration_id", migration.ID.String()),
	)

	if err := m.migrationRepo.Complete(ctx, migration.ID); err != nil {
		m.log.Error("failed to complete migration",
			zap.String("migration_id", migration.ID.String()),
			zap.Error(err),
		)
	}

	// Signal WorkerManager to stop old workers for this client.
	if m.onCompleteMigration != nil {
		m.onCompleteMigration(ctx, migration.ID, migration.APIKeyID)
	}

	// Mark as notified (in a real system this would send an email/in-app notification).
	_ = m.migrationRepo.MarkNotified(ctx, migration.ID)

	m.log.Info("migration completed successfully",
		zap.String("migration_id", migration.ID.String()),
		zap.String("old_provider", migration.OldProvider),
		zap.String("new_provider", migration.NewProvider),
		zap.Int("transferred_scheduled", migration.MigratedScheduledCount),
	)

	// Clean up from active map.
	m.mu.Lock()
	if cancel, ok := m.activeMigrations[migration.ID]; ok {
		cancel()
		delete(m.activeMigrations, migration.ID)
	}
	m.mu.Unlock()
}

// GetActiveMigration returns the active migration for a client, if any.
func (m *MigrationManager) GetActiveMigration(ctx context.Context, apiKeyID *uuid.UUID) (*domain.OrchestrationMigration, error) {
	return m.migrationRepo.GetActiveByAPIKeyID(ctx, apiKeyID)
}

// ListMigrations returns all migrations, or active ones.
func (m *MigrationManager) ListMigrations(ctx context.Context, activeOnly bool) ([]*domain.OrchestrationMigration, error) {
	if activeOnly {
		return m.migrationRepo.ListActive(ctx)
	}
	return m.migrationRepo.List(ctx)
}

// CancelMigration cancels an in-progress migration.
func (m *MigrationManager) CancelMigration(ctx context.Context, migrationID uuid.UUID) error {
	m.mu.Lock()
	if cancel, ok := m.activeMigrations[migrationID]; ok {
		cancel()
		delete(m.activeMigrations, migrationID)
	}
	m.mu.Unlock()

	// Signal WorkerManager to stop old workers.
	if m.onCompleteMigration != nil {
		m.onCompleteMigration(ctx, migrationID, nil)
	}
	return m.migrationRepo.Fail(ctx, migrationID, "cancelled by admin")
}

// GetActiveMigrations returns all currently active migration IDs.
func (m *MigrationManager) GetActiveMigrations() []uuid.UUID {
	m.mu.RLock()
	defer m.mu.RUnlock()
	ids := make([]uuid.UUID, 0, len(m.activeMigrations))
	for id := range m.activeMigrations {
		ids = append(ids, id)
	}
	return ids
}

// DryRunMigration performs a dry run of the migration without actually starting it.
// Returns what would be transferred (scheduled notifications, old workflow estimates).
func (m *MigrationManager) DryRunMigration(ctx context.Context, apiKeyID *string) (*domain.MigrationDryRunResult, error) {
	var apiKeyUUID *uuid.UUID
	if apiKeyID != nil && *apiKeyID != "" {
		parsed, err := uuid.Parse(*apiKeyID)
		if err == nil {
			apiKeyUUID = &parsed
		}
	}

	// Fetch the current orchestration config for this scope.
	vc, err := m.vendorConfigRepo.GetByType(ctx, workflowOrchestrationVendorType, apiKeyID)
	if err != nil || vc == nil || len(vc.ConfigJSON) == 0 {
		return nil, fmt.Errorf("no orchestration config found for this scope")
	}

	var cfg workflowOrchestrationConfig
	if err := json.Unmarshal(vc.ConfigJSON, &cfg); err != nil {
		return nil, fmt.Errorf("parsing orchestration config: %w", err)
	}

	provider := m.resolveProvider(cfg)
	var hostPort, namespace string
	switch provider {
	case "cadence":
		hostPort = normalizeHostPort(cfg.Cadence.HostPort)
		namespace = cfg.Cadence.Domain
	case "standalone":
		hostPort = ""
		namespace = ""
	default:
		hostPort = normalizeHostPort(cfg.Temporal.HostPort)
		namespace = cfg.Temporal.Namespace
	}

	// Estimate old workflows: notifications created in the last 2 minutes.
	slaWindow := time.Now().Add(-2 * time.Minute)
	oldWorkflowCount, err := m.notifRepo.CountClientNotificationsSince(ctx, *apiKeyUUID, slaWindow)
	if err != nil {
		m.log.Warn("failed to count notifications for dry-run", zap.Error(err))
		oldWorkflowCount = 0
	}

	// Estimate scheduled notifications to transfer: pending schedules for this client
	// that are scheduled 5+ minutes from now.
	scheduledList, total, err := m.schedRepo.List(ctx, []domain.NotificationStatus{domain.StatusPending}, 1, 10000, nil)
	if err != nil {
		m.log.Warn("failed to list scheduled notifications for dry-run", zap.Error(err))
	} else {
		cutoff := time.Now().Add(5 * time.Minute)
		transferable := 0
		for _, sched := range scheduledList {
			if !migrationBelongsToClient(sched.APIKeyID, apiKeyUUID) {
				continue
			}
			if sched.ScheduledAt.After(cutoff) {
				transferable++
			}
		}
		total = int64(transferable)
	}

	totalInt := int(total)
	duration := time.Duration(oldWorkflowCount*10 + totalInt*5) * time.Second

	return &domain.MigrationDryRunResult{
		OldProvider:          provider,
		NewProvider:          provider,
		OldHostPort:          hostPort,
		NewHostPort:          hostPort,
		OldNamespace:         namespace,
		NewNamespace:         namespace,
		OldWorkflowCount:     oldWorkflowCount,
		TotalScheduledCount:  totalInt,
		MigratedScheduledCount: totalInt,
		EstimatedDuration:    duration.String(),
	}, nil
}

// ResumeMigrations resumes any in-progress migrations on worker startup.
func (m *MigrationManager) ResumeMigrations(ctx context.Context) {
	active, err := m.migrationRepo.ListActive(ctx)
	if err != nil {
		m.log.Warn("failed to list active migrations for resume", zap.Error(err))
		return
	}
	for _, mig := range active {
		m.log.Info("resuming migration",
			zap.String("migration_id", mig.ID.String()),
		)
		mCtx, cancel := context.WithCancel(context.Background())
		m.mu.Lock()
		m.activeMigrations[mig.ID] = cancel
		m.mu.Unlock()

		go m.runMigration(mCtx, mig)
	}
}

func safeString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
