package repository

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/spidey/notification-service/internal/domain"
)

type VendorConfigRepository interface {
	GetByType(ctx context.Context, vendorType string) (*domain.VendorConfig, error)
	Upsert(ctx context.Context, config *domain.VendorConfig) error
	ListActive(ctx context.Context) ([]*domain.VendorConfig, error)
}

type vendorConfigRepository struct {
	db *DB
}

func NewVendorConfigRepository(db *DB) VendorConfigRepository {
	return &vendorConfigRepository{db: db}
}

func (r *vendorConfigRepository) GetByType(ctx context.Context, vendorType string) (*domain.VendorConfig, error) {
	query := `
		SELECT id, vendor_type, config_json, is_active, created_at, updated_at
		FROM vendor_configs
		WHERE vendor_type = $1
	`
	var cfg domain.VendorConfig
	err := r.db.Pool.QueryRow(ctx, query, vendorType).Scan(
		&cfg.ID, &cfg.VendorType, &cfg.ConfigJSON, &cfg.IsActive, &cfg.CreatedAt, &cfg.UpdatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil // Not found
		}
		return nil, fmt.Errorf("getting vendor config: %w", err)
	}
	return &cfg, nil
}

func (r *vendorConfigRepository) Upsert(ctx context.Context, config *domain.VendorConfig) error {
	query := `
		INSERT INTO vendor_configs (vendor_type, config_json, is_active)
		VALUES ($1, $2, $3)
		ON CONFLICT (vendor_type) DO UPDATE SET
			config_json = EXCLUDED.config_json,
			is_active = EXCLUDED.is_active,
			updated_at = CURRENT_TIMESTAMP
	`
	_, err := r.db.Pool.Exec(ctx, query, config.VendorType, config.ConfigJSON, config.IsActive)
	if err != nil {
		return fmt.Errorf("upserting vendor config: %w", err)
	}
	return nil
}

func (r *vendorConfigRepository) ListActive(ctx context.Context) ([]*domain.VendorConfig, error) {
	query := `
		SELECT id, vendor_type, config_json, is_active, created_at, updated_at
		FROM vendor_configs
		WHERE is_active = true
	`
	rows, err := r.db.Pool.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("listing active vendor configs: %w", err)
	}
	defer rows.Close()

	configs := make([]*domain.VendorConfig, 0)
	for rows.Next() {
		var cfg domain.VendorConfig
		if err := rows.Scan(&cfg.ID, &cfg.VendorType, &cfg.ConfigJSON, &cfg.IsActive, &cfg.CreatedAt, &cfg.UpdatedAt); err != nil {
			return nil, err
		}
		configs = append(configs, &cfg)
	}
	return configs, nil
}
