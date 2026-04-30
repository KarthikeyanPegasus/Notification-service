package repository

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/spidey/notification-service/internal/domain"
)

// VendorRateLimitRepository manages per-vendor throughput limits.
type VendorRateLimitRepository interface {
	Get(ctx context.Context, vendorName string, apiKeyID *string) (*domain.VendorRateLimit, error)
	Upsert(ctx context.Context, limit *domain.VendorRateLimit, apiKeyID *string) error
	ListActive(ctx context.Context, apiKeyID *string) ([]*domain.VendorRateLimit, error)
	Delete(ctx context.Context, vendorName string, apiKeyID *string) error
}

type vendorRateLimitRepository struct {
	db *DB
}

func NewVendorRateLimitRepository(db *DB) VendorRateLimitRepository {
	return &vendorRateLimitRepository{db: db}
}

func (r *vendorRateLimitRepository) Get(ctx context.Context, vendorName string, apiKeyID *string) (*domain.VendorRateLimit, error) {
	query := `
		SELECT id, vendor_name, api_key_id, rps, per_minute, per_10_min, per_hour, per_day,
		       is_active, created_at, updated_at
		FROM vendor_rate_limits
		WHERE vendor_name = $1
		  AND (
		    ($2::uuid IS NULL AND api_key_id IS NULL)
		    OR api_key_id = $2::uuid
		  )
	`
	var rl domain.VendorRateLimit
	err := r.db.Pool.QueryRow(ctx, query, vendorName, apiKeyID).Scan(
		&rl.ID, &rl.VendorName, &rl.APIKeyID,
		&rl.RPS, &rl.PerMinute, &rl.Per10Min, &rl.PerHour, &rl.PerDay,
		&rl.IsActive, &rl.CreatedAt, &rl.UpdatedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("getting vendor rate limit: %w", err)
	}
	return &rl, nil
}

func (r *vendorRateLimitRepository) Upsert(ctx context.Context, limit *domain.VendorRateLimit, apiKeyID *string) error {
	if limit.ID == uuid.Nil {
		limit.ID = uuid.New()
	}

	var query string
	var args []any

	if apiKeyID == nil || *apiKeyID == "" {
		query = `
			INSERT INTO vendor_rate_limits
			    (id, vendor_name, api_key_id, rps, per_minute, per_10_min, per_hour, per_day, is_active)
			VALUES ($1, $2, NULL, $3, $4, $5, $6, $7, $8)
			ON CONFLICT (vendor_name) WHERE api_key_id IS NULL DO UPDATE SET
			    rps        = EXCLUDED.rps,
			    per_minute = EXCLUDED.per_minute,
			    per_10_min = EXCLUDED.per_10_min,
			    per_hour   = EXCLUDED.per_hour,
			    per_day    = EXCLUDED.per_day,
			    is_active  = EXCLUDED.is_active,
			    updated_at = NOW()
		`
		args = []any{limit.ID, limit.VendorName, limit.RPS, limit.PerMinute, limit.Per10Min, limit.PerHour, limit.PerDay, limit.IsActive}
	} else {
		query = `
			INSERT INTO vendor_rate_limits
			    (id, vendor_name, api_key_id, rps, per_minute, per_10_min, per_hour, per_day, is_active)
			VALUES ($1, $2, $9::uuid, $3, $4, $5, $6, $7, $8)
			ON CONFLICT (vendor_name, api_key_id) WHERE api_key_id IS NOT NULL DO UPDATE SET
			    rps        = EXCLUDED.rps,
			    per_minute = EXCLUDED.per_minute,
			    per_10_min = EXCLUDED.per_10_min,
			    per_hour   = EXCLUDED.per_hour,
			    per_day    = EXCLUDED.per_day,
			    is_active  = EXCLUDED.is_active,
			    updated_at = NOW()
		`
		args = []any{limit.ID, limit.VendorName, limit.RPS, limit.PerMinute, limit.Per10Min, limit.PerHour, limit.PerDay, limit.IsActive, *apiKeyID}
	}

	_, err := r.db.Pool.Exec(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("upserting vendor rate limit: %w", err)
	}
	return nil
}

func (r *vendorRateLimitRepository) ListActive(ctx context.Context, apiKeyID *string) ([]*domain.VendorRateLimit, error) {
	query := `
		SELECT id, vendor_name, api_key_id, rps, per_minute, per_10_min, per_hour, per_day,
		       is_active, created_at, updated_at
		FROM vendor_rate_limits
		WHERE is_active = true
		  AND (
		    ($1::uuid IS NULL AND api_key_id IS NULL)
		    OR api_key_id = $1::uuid
		  )
		ORDER BY vendor_name
	`
	rows, err := r.db.Pool.Query(ctx, query, apiKeyID)
	if err != nil {
		return nil, fmt.Errorf("listing active vendor rate limits: %w", err)
	}
	defer rows.Close()

	var limits []*domain.VendorRateLimit
	for rows.Next() {
		var rl domain.VendorRateLimit
		if err := rows.Scan(
			&rl.ID, &rl.VendorName, &rl.APIKeyID,
			&rl.RPS, &rl.PerMinute, &rl.Per10Min, &rl.PerHour, &rl.PerDay,
			&rl.IsActive, &rl.CreatedAt, &rl.UpdatedAt,
		); err != nil {
			return nil, err
		}
		limits = append(limits, &rl)
	}
	return limits, rows.Err()
}

func (r *vendorRateLimitRepository) Delete(ctx context.Context, vendorName string, apiKeyID *string) error {
	query := `
		DELETE FROM vendor_rate_limits
		WHERE vendor_name = $1
		  AND (
		    ($2::uuid IS NULL AND api_key_id IS NULL)
		    OR api_key_id = $2::uuid
		  )
	`
	_, err := r.db.Pool.Exec(ctx, query, vendorName, apiKeyID)
	if err != nil {
		return fmt.Errorf("deleting vendor rate limit: %w", err)
	}
	return nil
}
