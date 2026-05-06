package domain

import (
	"time"

	"github.com/google/uuid"
)

// VendorRetryConfig defines retry/backoff behavior for a specific vendor,
// optionally scoped to an API key (client). When no scoped config exists,
// a default configuration is used by the worker.
//
// Fields:
//   - RetryInitialIntervalMs: initial backoff delay in milliseconds (default 100)
//   - RetryMaxIntervalMs: maximum backoff delay in milliseconds (default 30000 = 30s)
//   - RetryMaxAttempts: maximum delivery attempts before sending to DLQ (default 5)
//   - RetryBackoffCoefficient: exponential factor (default 2.0)
//   - SLA: per-vendor SLA deadline in seconds (default 30; high=5, medium=15, low=30)
type VendorRetryConfig struct {
	ID                     uuid.UUID  `json:"id" db:"id"`
	VendorName             string     `json:"vendor_name" db:"vendor_name"`
	APIKeyID               *uuid.UUID `json:"api_key_id,omitempty" db:"api_key_id"`
	RetryInitialIntervalMs int        `json:"retry_initial_interval_ms" db:"retry_initial_interval_ms"`
	RetryMaxIntervalMs     int        `json:"retry_max_interval_ms" db:"retry_max_interval_ms"`
	RetryMaxAttempts       int        `json:"retry_max_attempts" db:"retry_max_attempts"`
	RetryBackoffCoefficient float64   `json:"retry_backoff_coefficient" db:"retry_backoff_coefficient"`
	SLA                    int        `json:"sla_seconds" db:"sla_seconds"`
	IsActive               bool       `json:"is_active" db:"is_active"`
	CreatedAt              time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt              time.Time  `json:"updated_at" db:"updated_at"`
}

// UpsertVendorRetryConfigRequest is the API input for creating or updating a vendor retry config.
type UpsertVendorRetryConfigRequest struct {
	RetryInitialIntervalMs  *int     `json:"retry_initial_interval_ms,omitempty"`
	RetryMaxIntervalMs      *int     `json:"retry_max_interval_ms,omitempty"`
	RetryMaxAttempts        *int     `json:"retry_max_attempts,omitempty"`
	RetryBackoffCoefficient *float64 `json:"retry_backoff_coefficient,omitempty"`
	SLA                     *int     `json:"sla_seconds,omitempty"`
}

// DefaultRetryConfig returns sensible defaults used when no config is stored.
func DefaultRetryConfig(vendorName string) *VendorRetryConfig {
	return &VendorRetryConfig{
		VendorName:             vendorName,
		RetryInitialIntervalMs: 100,
		RetryMaxIntervalMs:     30000,
		RetryMaxAttempts:       5,
		RetryBackoffCoefficient: 2.0,
		SLA:                    30,
		IsActive:               true,
	}
}
