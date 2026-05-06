package repository

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/spidey/notification-service/internal/domain"
)

// MigrationRepository persists orchestration migration records.
type MigrationRepository struct {
	db *DB
}

func NewMigrationRepository(db *DB) *MigrationRepository {
	return &MigrationRepository{db: db}
}

// Create inserts a new migration record.
func (r *MigrationRepository) Create(ctx context.Context, m *domain.OrchestrationMigration) error {
	oldCfg, _ := json.Marshal(m.OldConfigJSON)
	newCfg, _ := json.Marshal(m.NewConfigJSON)
	const q = `
		INSERT INTO orchestration_migrations
			(id, api_key_id, client_name,
			 old_provider, new_provider,
			 old_config_json, new_config_json,
			 status, old_workflow_count, completed_old_workflows,
			 migrated_scheduled_count, total_scheduled_count,
			 error_message, started_at, completed_at, notified_at,
			 created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18)`
	_, err := r.db.Pool.Exec(ctx, q,
		m.ID, m.APIKeyID, m.ClientName,
		m.OldProvider, m.NewProvider,
		oldCfg, newCfg,
		m.Status, m.OldWorkflowCount, m.CompletedOldWorkflows,
		m.MigratedScheduledCount, m.TotalScheduledCount,
		m.ErrorMessage, m.StartedAt, m.CompletedAt, m.NotifiedAt,
		m.CreatedAt, m.UpdatedAt,
	)
	return err
}

// GetByID retrieves a migration by ID.
func (r *MigrationRepository) GetByID(ctx context.Context, id uuid.UUID) (*domain.OrchestrationMigration, error) {
	const q = `
		SELECT id, api_key_id, client_name,
		       old_provider, new_provider,
		       old_config_json, new_config_json,
		       status, old_workflow_count, completed_old_workflows,
		       migrated_scheduled_count, total_scheduled_count,
		       error_message, started_at, completed_at, notified_at,
		       created_at, updated_at
		FROM orchestration_migrations WHERE id=$1`
	return r.scanOne(r.db.Pool.QueryRow(ctx, q, id))
}

// GetActiveByAPIKeyID returns the currently in-progress migration for a client scope.
func (r *MigrationRepository) GetActiveByAPIKeyID(ctx context.Context, apiKeyID *uuid.UUID) (*domain.OrchestrationMigration, error) {
	const q = `
		SELECT id, api_key_id, client_name,
		       old_provider, new_provider,
		       old_config_json, new_config_json,
		       status, old_workflow_count, completed_old_workflows,
		       migrated_scheduled_count, total_scheduled_count,
		       error_message, started_at, completed_at, notified_at,
		       created_at, updated_at
		FROM orchestration_migrations
		WHERE api_key_id IS NOT DISTINCT FROM $1
		  AND status IN ('in_progress','transferring_scheduled','waiting_old_workers')
		ORDER BY started_at DESC
		LIMIT 1`
	return r.scanOne(r.db.Pool.QueryRow(ctx, q, apiKeyID))
}

// ListActive returns all in-progress migrations.
func (r *MigrationRepository) ListActive(ctx context.Context) ([]*domain.OrchestrationMigration, error) {
	const q = `
		SELECT id, api_key_id, client_name,
		       old_provider, new_provider,
		       old_config_json, new_config_json,
		       status, old_workflow_count, completed_old_workflows,
		       migrated_scheduled_count, total_scheduled_count,
		       error_message, started_at, completed_at, notified_at,
		       created_at, updated_at
		FROM orchestration_migrations
		WHERE status IN ('in_progress','transferring_scheduled','waiting_old_workers')
		ORDER BY started_at DESC`
	return r.scanRows(r.db.Pool.Query(ctx, q))
}

// List returns all migrations, ordered by most recent first.
func (r *MigrationRepository) List(ctx context.Context) ([]*domain.OrchestrationMigration, error) {
	const q = `
		SELECT id, api_key_id, client_name,
		       old_provider, new_provider,
		       old_config_json, new_config_json,
		       status, old_workflow_count, completed_old_workflows,
		       migrated_scheduled_count, total_scheduled_count,
		       error_message, started_at, completed_at, notified_at,
		       created_at, updated_at
		FROM orchestration_migrations
		ORDER BY started_at DESC`
	return r.scanRows(r.db.Pool.Query(ctx, q))
}

// UpdateStatus updates the status and progress counters.
func (r *MigrationRepository) UpdateStatus(ctx context.Context, id uuid.UUID, status domain.MigrationStatus, completedOldWorkflows int, migratedScheduledCount int) error {
	const q = `
		UPDATE orchestration_migrations
		SET status=$1, completed_old_workflows=$2, migrated_scheduled_count=$3,
		    updated_at=$4
		WHERE id=$5`
	_, err := r.db.Pool.Exec(ctx, q, status, completedOldWorkflows, migratedScheduledCount, time.Now(), id)
	return err
}

// Complete marks the migration as completed.
func (r *MigrationRepository) Complete(ctx context.Context, id uuid.UUID) error {
	now := time.Now()
	const q = `
		UPDATE orchestration_migrations
		SET status='completed', completed_at=$1, updated_at=$1
		WHERE id=$2`
	_, err := r.db.Pool.Exec(ctx, q, now, id)
	return err
}

// MarkNotified marks that the user was notified about migration completion.
func (r *MigrationRepository) MarkNotified(ctx context.Context, id uuid.UUID) error {
	now := time.Now()
	const q = `
		UPDATE orchestration_migrations
		SET notified_at=$1, updated_at=$1
		WHERE id=$2`
	_, err := r.db.Pool.Exec(ctx, q, now, id)
	return err
}

// Fail marks the migration as failed.
func (r *MigrationRepository) Fail(ctx context.Context, id uuid.UUID, errMsg string) error {
	now := time.Now()
	const q = `
		UPDATE orchestration_migrations
		SET status='failed', error_message=$1, completed_at=$2, updated_at=$2
		WHERE id=$3`
	_, err := r.db.Pool.Exec(ctx, q, errMsg, now, id)
	return err
}

// UpdateCounts updates workflow/scheduled counts for an active migration.
func (r *MigrationRepository) UpdateCounts(ctx context.Context, id uuid.UUID, oldWorkflowCount, completedOldWorkflows, totalScheduledCount, migratedScheduledCount int) error {
	const q = `
		UPDATE orchestration_migrations
		SET old_workflow_count=$1, completed_old_workflows=$2,
		    total_scheduled_count=$3, migrated_scheduled_count=$4,
		    updated_at=$5
		WHERE id=$6`
	_, err := r.db.Pool.Exec(ctx, q, oldWorkflowCount, completedOldWorkflows, totalScheduledCount, migratedScheduledCount, time.Now(), id)
	return err
}

func (r *MigrationRepository) scanOne(row pgx.Row) (*domain.OrchestrationMigration, error) {
	m := &domain.OrchestrationMigration{}
	var oldCfg, newCfg []byte
	var apiKeyID *uuid.UUID
	err := row.Scan(
		&m.ID, &apiKeyID, &m.ClientName,
		&m.OldProvider, &m.NewProvider,
		&oldCfg, &newCfg,
		&m.Status, &m.OldWorkflowCount, &m.CompletedOldWorkflows,
		&m.MigratedScheduledCount, &m.TotalScheduledCount,
		&m.ErrorMessage, &m.StartedAt, &m.CompletedAt, &m.NotifiedAt,
		&m.CreatedAt, &m.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	m.APIKeyID = apiKeyID
	if len(oldCfg) > 0 {
		_ = json.Unmarshal(oldCfg, &m.OldConfigJSON)
	}
	if len(newCfg) > 0 {
		_ = json.Unmarshal(newCfg, &m.NewConfigJSON)
	}
	return m, nil
}

func (r *MigrationRepository) scanRows(rows pgx.Rows, err error) ([]*domain.OrchestrationMigration, error) {
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*domain.OrchestrationMigration
	for rows.Next() {
		m, err := r.scanOne(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}
