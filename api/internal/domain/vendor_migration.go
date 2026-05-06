package domain

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// VendorMigrationStatus is the lifecycle state of a vendor migration.
type VendorMigrationStatus string

const (
	VendorMigrationInProgress  VendorMigrationStatus = "in_progress"
	VendorMigrationCompleted   VendorMigrationStatus = "completed"
	VendorMigrationFailed      VendorMigrationStatus = "failed"
	VendorMigrationRolledBack  VendorMigrationStatus = "rolled_back"
)

// VendorMigration records a vendor swap (cross-vendor or same-vendor config change)
// initiated by an admin. It captures the before/after state and current status so
// that reputation metrics can be viewed relative to the cutover timestamp and
// the old config can be restored via rollback.
type VendorMigration struct {
	ID             uuid.UUID             `json:"id"              db:"id"`
	APIKeyID       *uuid.UUID            `json:"api_key_id,omitempty" db:"api_key_id"`
	Channel        string                `json:"channel"         db:"channel"`        // email | sms | push
	FromVendor     string                `json:"from_vendor"     db:"from_vendor"`
	ToVendor       string                `json:"to_vendor"       db:"to_vendor"`
	FromConfigJSON json.RawMessage       `json:"from_config_json,omitempty" db:"from_config_json"`
	ToConfigJSON   json.RawMessage       `json:"to_config_json"  db:"to_config_json"`
	Strategy       string                `json:"strategy"        db:"strategy"`       // instant | gradual
	Status         VendorMigrationStatus `json:"status"          db:"status"`
	TrafficPercent int                   `json:"traffic_percent" db:"traffic_percent"`
	ErrorMessage   *string               `json:"error_message,omitempty" db:"error_message"`
	StartedAt      time.Time             `json:"started_at"      db:"started_at"`
	CompletedAt    *time.Time            `json:"completed_at,omitempty" db:"completed_at"`
	CreatedAt      time.Time             `json:"created_at"      db:"created_at"`
	UpdatedAt      time.Time             `json:"updated_at"      db:"updated_at"`
}

// IsSameVendor reports whether this is a credential/config swap within the same vendor.
func (m *VendorMigration) IsSameVendor() bool {
	return m.FromVendor == m.ToVendor
}

// IsActive reports whether the migration is still in progress.
func (m *VendorMigration) IsActive() bool {
	return m.Status == VendorMigrationInProgress
}

// StartVendorMigrationRequest is the payload for initiating a vendor migration.
type StartVendorMigrationRequest struct {
	Channel      string          `json:"channel"       binding:"required"` // email | sms | push
	FromVendor   string          `json:"from_vendor"   binding:"required"`
	ToVendor     string          `json:"to_vendor"     binding:"required"`
	ToConfigJSON json.RawMessage `json:"to_config"     binding:"required"`
	Strategy     string          `json:"strategy"`     // instant (default) | gradual
}
