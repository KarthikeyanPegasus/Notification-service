package domain

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// VendorConfig stores provider-specific settings in the database.
type VendorConfig struct {
	ID         uuid.UUID       `json:"id"`
	VendorType string          `json:"vendor_type"` // 'sms', 'email', etc.
	ConfigJSON json.RawMessage `json:"config_json"`
	IsActive   bool            `json:"is_active"`
	CreatedAt  time.Time       `json:"created_at"`
	UpdatedAt  time.Time       `json:"updated_at"`
}

// ConfigUpdatedEvent is published when a vendor configuration changes.
type ConfigUpdatedEvent struct {
	VendorType string `json:"vendor_type"`
	Timestamp  int64  `json:"timestamp"`
}
