package domain

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// MigrationStatus tracks the lifecycle of an orchestration migration.
type MigrationStatus string

const (
	MigrationInProgress        MigrationStatus = "in_progress"
	MigrationTransferringSched MigrationStatus = "transferring_scheduled"
	MigrationWaitingOldWorkers MigrationStatus = "waiting_old_workers"
	MigrationCompleted         MigrationStatus = "completed"
	MigrationFailed            MigrationStatus = "failed"
)

// MigrationDryRunResult contains the results of a dry-run migration.
type MigrationDryRunResult struct {
	OldProvider          string `json:"old_provider"`
	NewProvider          string `json:"new_provider"`
	OldHostPort          string `json:"old_host_port"`
	NewHostPort          string `json:"new_host_port"`
	OldNamespace         string `json:"old_namespace"`
	NewNamespace         string `json:"new_namespace"`
	OldWorkflowCount     int    `json:"old_workflow_count"`
	TotalScheduledCount  int    `json:"total_scheduled_count"`
	MigratedScheduledCount int   `json:"migrated_scheduled_count"`
	EstimatedDuration    string `json:"estimated_duration"`
}

// OrchestrationMigration tracks a workflow orchestration config change
// for a specific client scope, enabling a smooth migration from the old
// Temporal/Cadence provider to the new one.
type OrchestrationMigration struct {
	ID                   uuid.UUID       `json:"id" db:"id"`
	APIKeyID             *uuid.UUID      `json:"api_key_id,omitempty" db:"api_key_id"`
	ClientName           string          `json:"client_name" db:"client_name"`
	OldProvider          string          `json:"old_provider" db:"old_provider"`
	NewProvider          string          `json:"new_provider" db:"new_provider"`
	OldConfigJSON        json.RawMessage `json:"old_config_json,omitempty" db:"old_config_json"`
	NewConfigJSON        json.RawMessage `json:"new_config_json,omitempty" db:"new_config_json"`
	Status               MigrationStatus `json:"status" db:"status"`
	OldWorkflowCount     int             `json:"old_workflow_count" db:"old_workflow_count"`
	CompletedOldWorkflows int            `json:"completed_old_workflows" db:"completed_old_workflows"`
	MigratedScheduledCount int           `json:"migrated_scheduled_count" db:"migrated_scheduled_count"`
	TotalScheduledCount  int             `json:"total_scheduled_count" db:"total_scheduled_count"`
	ErrorMessage         *string         `json:"error_message,omitempty" db:"error_message"`
	StartedAt            time.Time       `json:"started_at" db:"started_at"`
	CompletedAt          *time.Time      `json:"completed_at,omitempty" db:"completed_at"`
	NotifiedAt           *time.Time      `json:"notified_at,omitempty" db:"notified_at"`
	CreatedAt            time.Time       `json:"created_at" db:"created_at"`
	UpdatedAt            time.Time       `json:"updated_at" db:"updated_at"`
}

// MigrationStatusResponse is returned by the migration status API.
type MigrationStatusResponse struct {
	Migrations []*OrchestrationMigration `json:"migrations"`
	Active     *OrchestrationMigration   `json:"active,omitempty"`
	// Live counts from the worker manager during active migration.
	OldWorkerCount int `json:"old_worker_count"`
	NewWorkerCount int `json:"new_worker_count"`
}

// IsActive returns true if the migration is still in progress.
func (m *OrchestrationMigration) IsActive() bool {
	return m.Status == MigrationInProgress ||
		m.Status == MigrationTransferringSched ||
		m.Status == MigrationWaitingOldWorkers
}

// Progress returns a float 0..1 indicating completion progress.
func (m *OrchestrationMigration) Progress() float64 {
	if m.Status == MigrationCompleted {
		return 1.0
	}
	if m.OldWorkflowCount == 0 && m.TotalScheduledCount == 0 {
		if m.IsActive() {
			return 0.5
		}
		return 0.0
	}
	workflowProgress := 0.0
	if m.OldWorkflowCount > 0 {
		workflowProgress = float64(m.CompletedOldWorkflows) / float64(m.OldWorkflowCount)
	}
	schedProgress := 0.0
	if m.TotalScheduledCount > 0 {
		schedProgress = float64(m.MigratedScheduledCount) / float64(m.TotalScheduledCount)
	}
	// Weight: 60% workflows, 40% scheduled transfers
	return 0.6*workflowProgress + 0.4*schedProgress
}
