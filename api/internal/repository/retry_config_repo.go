package repository

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/spidey/notification-service/internal/domain"
)

// VendorRetryConfigRepository manages per-vendor retry/backoff settings.
type VendorRetryConfigRepository interface {
	Get(ctx context.Context, vendorName string, apiKeyID *string) (*domain.VendorRetryConfig, error)
	Upsert(ctx context.Context, cfg *domain.VendorRetryConfig, apiKeyID *string) error
	ListActive(ctx context.Context, apiKeyID *string) ([]*domain.VendorRetryConfig, error)
	Delete(ctx context.Context, vendorName string, apiKeyID *string) error
}

type vendorRetryConfigRepository struct {
	db *DB
}

func NewVendorRetryConfigRepository(db *DB) VendorRetryConfigRepository {
	return &vendorRetryConfigRepository{db: db}
}

func (r *vendorRetryConfigRepository) Get(ctx context.Context, vendorName string, apiKeyID *string) (*domain.VendorRetryConfig, error) {
	query := `
		SELECT id, vendor_name, api_key_id,
		       retry_initial_interval_ms, retry_max_interval_ms,
		       retry_max_attempts, retry_backoff_coefficient, sla_seconds,
		       is_active, created_at, updated_at
		FROM vendor_retry_configs
		WHERE vendor_name = $1
		  AND is_active = true
		  AND (
		    ($2::uuid IS NULL AND api_key_id IS NULL)
		    OR api_key_id = $2::uuid
		  )
	`
	var cfg domain.VendorRetryConfig
	err := r.db.Pool.QueryRow(ctx, query, vendorName, apiKeyID).Scan(
		&cfg.ID, &cfg.VendorName, &cfg.APIKeyID,
		&cfg.RetryInitialIntervalMs, &cfg.RetryMaxIntervalMs,
		&cfg.RetryMaxAttempts, &cfg.RetryBackoffCoefficient, &cfg.SLA,
		&cfg.IsActive, &cfg.CreatedAt, &cfg.UpdatedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("getting vendor retry config: %w", err)
	}
	return &cfg, nil
}

func (r *vendorRetryConfigRepository) Upsert(ctx context.Context, cfg *domain.VendorRetryConfig, apiKeyID *string) error {
	if cfg.ID == uuid.Nil {
		cfg.ID = uuid.New()
	}

	var query string
	var args []any

	if apiKeyID == nil || *apiKeyID == "" {
		query = `
			INSERT INTO vendor_retry_configs
			    (id, vendor_name, api_key_id,
			     retry_initial_interval_ms, retry_max_interval_ms,
			     retry_max_attempts, retry_backoff_coefficient, sla_seconds,
			     is_active)
			VALUES ($1, $2, NULL, $3, $4, $5, $6, $7, $8)
			ON CONFLICT (vendor_name) WHERE api_key_id IS NULL DO UPDATE SET
			    retry_initial_interval_ms  = EXCLUDED.retry_initial_interval_ms,
			    retry_max_interval_ms      = EXCLUDED.retry_max_interval_ms,
			    retry_max_attempts         = EXCLUDED.retry_max_attempts,
			    retry_backoff_coefficient  = EXCLUDED.retry_backoff_coefficient,
			    sla_seconds                = EXCLUDED.sla_seconds,
			    is_active                  = EXCLUDED.is_active,
			    updated_at                 = NOW()
		`
		args = []any{
			cfg.ID, cfg.VendorName,
			cfg.RetryInitialIntervalMs, cfg.RetryMaxIntervalMs,
			cfg.RetryMaxAttempts, cfg.RetryBackoffCoefficient, cfg.SLA,
			cfg.IsActive,
		}
	} else {
		query = `
			INSERT INTO vendor_retry_configs
			    (id, vendor_name, api_key_id,
			     retry_initial_interval_ms, retry_max_interval_ms,
			     retry_max_attempts, retry_backoff_coefficient, sla_seconds,
			     is_active)
			VALUES ($1, $2, $9::uuid, $3, $4, $5, $6, $7, $8)
			ON CONFLICT (vendor_name, api_key_id) WHERE api_key_id IS NOT NULL DO UPDATE SET
			    retry_initial_interval_ms  = EXCLUDED.retry_initial_interval_ms,
			    retry_max_interval_ms      = EXCLUDED.retry_max_interval_ms,
			    retry_max_attempts         = EXCLUDED.retry_max_attempts,
			    retry_backoff_coefficient  = EXCLUDED.retry_backoff_coefficient,
			    sla_seconds                = EXCLUDED.sla_seconds,
			    is_active                  = EXCLUDED.is_active,
			    updated_at                 = NOW()
		`
		args = []any{
			cfg.ID, cfg.VendorName,
			cfg.RetryInitialIntervalMs, cfg.RetryMaxIntervalMs,
			cfg.RetryMaxAttempts, cfg.RetryBackoffCoefficient, cfg.SLA,
			cfg.IsActive, *apiKeyID,
		}
	}

	_, err := r.db.Pool.Exec(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("upserting vendor retry config: %w", err)
	}
	return nil
}

func (r *vendorRetryConfigRepository) ListActive(ctx context.Context, apiKeyID *string) ([]*domain.VendorRetryConfig, error) {
	query := `
		SELECT id, vendor_name, api_key_id,
		       retry_initial_interval_ms, retry_max_interval_ms,
		       retry_max_attempts, retry_backoff_coefficient, sla_seconds,
		       is_active, created_at, updated_at
		FROM vendor_retry_configs
		WHERE is_active = true
		  AND (
		    ($1::uuid IS NULL AND api_key_id IS NULL)
		    OR api_key_id = $1::uuid
		  )
		ORDER BY vendor_name
	`
	rows, err := r.db.Pool.Query(ctx, query, apiKeyID)
	if err != nil {
		return nil, fmt.Errorf("listing active vendor retry configs: %w", err)
	}
	defer rows.Close()

	var configs []*domain.VendorRetryConfig
	for rows.Next() {
		var cfg domain.VendorRetryConfig
		if err := rows.Scan(
			&cfg.ID, &cfg.VendorName, &cfg.APIKeyID,
			&cfg.RetryInitialIntervalMs, &cfg.RetryMaxIntervalMs,
			&cfg.RetryMaxAttempts, &cfg.RetryBackoffCoefficient, &cfg.SLA,
			&cfg.IsActive, &cfg.CreatedAt, &cfg.UpdatedAt,
		); err != nil {
			return nil, err
		}
		configs = append(configs, &cfg)
	}
	return configs, rows.Err()
}

func (r *vendorRetryConfigRepository) Delete(ctx context.Context, vendorName string, apiKeyID *string) error {
	query := `
		DELETE FROM vendor_retry_configs
		WHERE vendor_name = $1
		  AND (
		    ($2::uuid IS NULL AND api_key_id IS NULL)
		    OR api_key_id = $2::uuid
		  )
	`
	_, err := r.db.Pool.Exec(ctx, query, vendorName, apiKeyID)
	if err != nil {
		return fmt.Errorf("deleting vendor retry config: %w", err)
	}
	return nil
}
