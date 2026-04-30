package domain

import (
	"time"

	"github.com/google/uuid"
)

// VendorRateLimit defines the allowed throughput for a specific vendor,
// optionally scoped to an API key (client). All window limits are optional;
// only the ones set (> 0) are enforced. Multiple windows can be configured
// simultaneously and ALL must pass for a request to proceed.
type VendorRateLimit struct {
	ID         uuid.UUID  `json:"id" db:"id"`
	VendorName string     `json:"vendor_name" db:"vendor_name"`
	APIKeyID   *uuid.UUID `json:"api_key_id,omitempty" db:"api_key_id"`
	// RPS is the maximum requests per second (supports fractional values, e.g. 0.5 = 1 per 2s).
	RPS       *float64 `json:"rps,omitempty" db:"rps"`
	PerMinute *int     `json:"per_minute,omitempty" db:"per_minute"`
	Per10Min  *int     `json:"per_10_min,omitempty" db:"per_10_min"`
	PerHour   *int     `json:"per_hour,omitempty" db:"per_hour"`
	PerDay    *int     `json:"per_day,omitempty" db:"per_day"`
	IsActive  bool     `json:"is_active" db:"is_active"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
	UpdatedAt time.Time `json:"updated_at" db:"updated_at"`
}

// UpsertVendorRateLimitRequest is the API input for creating or updating a vendor rate limit.
type UpsertVendorRateLimitRequest struct {
	RPS       *float64 `json:"rps,omitempty"`
	PerMinute *int     `json:"per_minute,omitempty"`
	Per10Min  *int     `json:"per_10_min,omitempty"`
	PerHour   *int     `json:"per_hour,omitempty"`
	PerDay    *int     `json:"per_day,omitempty"`
}
