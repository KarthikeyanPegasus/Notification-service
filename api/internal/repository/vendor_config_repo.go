package repository

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/spidey/notification-service/internal/domain"
	"github.com/spidey/notification-service/internal/security"
	"go.uber.org/zap"
)

type VendorConfigRepository interface {
	GetByType(ctx context.Context, vendorType string, apiKeyID *string) (*domain.VendorConfig, error)
	Upsert(ctx context.Context, config *domain.VendorConfig, apiKeyID *string) error
	ListActive(ctx context.Context, apiKeyID *string) ([]*domain.VendorConfig, error)
	// ListPreferredActive returns one active config per vendor_type across all scopes,
	// preferring the global config (api_key_id IS NULL) when both global and client-scoped
	// configs exist for the same vendor. Used for admin-level views (billing, connectivity)
	// where all configured vendors must be visible regardless of which client owns them.
	ListPreferredActive(ctx context.Context) ([]*domain.VendorConfig, error)
	SetActive(ctx context.Context, vendorType string, isActive bool, apiKeyID *string) error
}

type vendorConfigRepository struct {
	db     *DB
	crypto *security.VendorConfigCrypto
	log    *zap.Logger
}

func NewVendorConfigRepository(db *DB, crypto *security.VendorConfigCrypto) VendorConfigRepository {
	return &vendorConfigRepository{db: db, crypto: crypto, log: zap.NewNop()}
}

// WithLogger sets a logger for non-fatal warnings (e.g. decrypt failures).
func (r *vendorConfigRepository) WithLogger(log *zap.Logger) *vendorConfigRepository {
	if log != nil {
		r.log = log
	}
	return r
}

func (r *vendorConfigRepository) GetByType(ctx context.Context, vendorType string, apiKeyID *string) (*domain.VendorConfig, error) {
	query := `
		SELECT id, vendor_type, config_json, is_active, created_at, updated_at
		FROM vendor_configs
		WHERE vendor_type = $1
		  AND (
		    ($2::uuid IS NULL AND api_key_id IS NULL)
		    OR api_key_id = $2::uuid
		  )
	`
	var cfg domain.VendorConfig
	err := r.db.Pool.QueryRow(ctx, query, vendorType, apiKeyID).Scan(
		&cfg.ID, &cfg.VendorType, &cfg.ConfigJSON, &cfg.IsActive, &cfg.CreatedAt, &cfg.UpdatedAt,
	)
	if err != nil {
		// Back-compat for older DBs that don't have api_key_id scoping.
		if strings.Contains(err.Error(), `column "api_key_id" does not exist`) {
			legacy := `
				SELECT id, vendor_type, config_json, is_active, created_at, updated_at
				FROM vendor_configs
				WHERE vendor_type = $1
			`
			err = r.db.Pool.QueryRow(ctx, legacy, vendorType).Scan(
				&cfg.ID, &cfg.VendorType, &cfg.ConfigJSON, &cfg.IsActive, &cfg.CreatedAt, &cfg.UpdatedAt,
			)
		}
		if err == pgx.ErrNoRows {
			return nil, nil // Not found
		}
		return nil, fmt.Errorf("getting vendor config: %w", err)
	}

	if r.crypto != nil && len(cfg.ConfigJSON) > 0 {
		plain, wasEncrypted, derr := r.crypto.DecryptJSON(cfg.ConfigJSON)
		if derr != nil {
			return nil, fmt.Errorf("decrypting vendor config (vendor_type=%s, encrypted=%t): %w", vendorType, wasEncrypted, derr)
		}
		cfg.ConfigJSON = plain
	}
	return &cfg, nil
}

func (r *vendorConfigRepository) Upsert(ctx context.Context, config *domain.VendorConfig, apiKeyID *string) error {
	if r.crypto == nil {
		return fmt.Errorf("vendor config encryption is not configured")
	}
	if len(config.ConfigJSON) == 0 {
		return fmt.Errorf("vendor config is empty")
	}

	toStore := config.ConfigJSON
	if !r.crypto.IsEncrypted(toStore) {
		enc, err := r.crypto.EncryptJSON(toStore)
		if err != nil {
			return fmt.Errorf("encrypting vendor config: %w", err)
		}
		toStore = enc
	}

	// Two conflict targets depending on scope:
	// - global config (api_key_id IS NULL): unique on vendor_type for NULL api_key_id rows
	// - per-client config: unique on (vendor_type, api_key_id)
	query := ""
	args := []any{config.VendorType, toStore, config.IsActive}
	if apiKeyID == nil || *apiKeyID == "" {
		query = `
			INSERT INTO vendor_configs (vendor_type, config_json, is_active, api_key_id)
			VALUES ($1, $2, $3, NULL)
			ON CONFLICT (vendor_type) WHERE api_key_id IS NULL DO UPDATE SET
				config_json = EXCLUDED.config_json,
				is_active = EXCLUDED.is_active,
				updated_at = CURRENT_TIMESTAMP
		`
	} else {
		query = `
			INSERT INTO vendor_configs (vendor_type, api_key_id, config_json, is_active)
			VALUES ($1, $4::uuid, $2, $3)
			ON CONFLICT (vendor_type, api_key_id) DO UPDATE SET
				config_json = EXCLUDED.config_json,
				is_active = EXCLUDED.is_active,
				updated_at = CURRENT_TIMESTAMP
		`
		// IMPORTANT: pgx expects the actual uuid string value, not a *string pointer.
		args = append(args, *apiKeyID)
	}

	_, err := r.db.Pool.Exec(ctx, query, args...)
	if err != nil {
		// Back-compat for older DBs:
		// - no api_key_id column
		// - or no uniq index on (vendor_type, api_key_id)
		msg := err.Error()
		if strings.Contains(msg, `column "api_key_id" does not exist`) ||
			strings.Contains(msg, "there is no unique or exclusion constraint matching the ON CONFLICT specification") {
			legacy := `
				INSERT INTO vendor_configs (vendor_type, config_json, is_active)
				VALUES ($1, $2, $3)
				ON CONFLICT (vendor_type) DO UPDATE SET
					config_json = EXCLUDED.config_json,
					is_active = EXCLUDED.is_active,
					updated_at = CURRENT_TIMESTAMP
			`
			_, err = r.db.Pool.Exec(ctx, legacy, config.VendorType, toStore, config.IsActive)
		}
		if err == nil {
			return nil
		}
		return fmt.Errorf("upserting vendor config: %w", err)
	}
	return nil
}

func (r *vendorConfigRepository) ListActive(ctx context.Context, apiKeyID *string) ([]*domain.VendorConfig, error) {
	query := `
		SELECT id, vendor_type, config_json, is_active, created_at, updated_at
		FROM vendor_configs
		WHERE is_active = true
		  AND (
		    ($1::uuid IS NULL AND api_key_id IS NULL)
		    OR api_key_id = $1::uuid
		  )
	`
	rows, err := r.db.Pool.Query(ctx, query, apiKeyID)
	if err != nil {
		// Back-compat for older DBs that don't have api_key_id scoping.
		if strings.Contains(err.Error(), `column "api_key_id" does not exist`) {
			legacy := `
				SELECT id, vendor_type, config_json, is_active, created_at, updated_at
				FROM vendor_configs
				WHERE is_active = true
			`
			rows, err = r.db.Pool.Query(ctx, legacy)
		}
		if err != nil {
			return nil, fmt.Errorf("listing active vendor configs: %w", err)
		}
	}
	defer rows.Close()

	configs := make([]*domain.VendorConfig, 0)
	for rows.Next() {
		var cfg domain.VendorConfig
		if err := rows.Scan(&cfg.ID, &cfg.VendorType, &cfg.ConfigJSON, &cfg.IsActive, &cfg.CreatedAt, &cfg.UpdatedAt); err != nil {
			return nil, err
		}
		if r.crypto != nil && len(cfg.ConfigJSON) > 0 {
			plain, _, derr := r.crypto.DecryptJSON(cfg.ConfigJSON)
			if derr != nil {
				// Don't fail the entire list endpoint if one row is corrupted or was encrypted with a different key.
				// The UI can still function with the remaining configs, and admins can re-save the broken vendor config.
				if r.log != nil {
					r.log.Warn("skipping undecryptable vendor config", zap.String("vendor_type", cfg.VendorType), zap.Error(derr))
				}
				continue
			}
			cfg.ConfigJSON = plain
		}
		configs = append(configs, &cfg)
	}
	return configs, nil
}

func (r *vendorConfigRepository) ListPreferredActive(ctx context.Context) ([]*domain.VendorConfig, error) {
	// DISTINCT ON (vendor_type) with global rows ordered first ensures that when both
	// a global config (api_key_id IS NULL) and a client-scoped config exist for the same
	// vendor_type, the global one is returned. If only a client-scoped config exists, that
	// is returned instead. This makes every configured vendor visible regardless of scope.
	query := `
		SELECT DISTINCT ON (vendor_type)
			id, vendor_type, config_json, is_active, created_at, updated_at
		FROM vendor_configs
		WHERE is_active = true
		ORDER BY vendor_type, (api_key_id IS NULL) DESC, updated_at DESC
	`
	rows, err := r.db.Pool.Query(ctx, query)
	if err != nil {
		if strings.Contains(err.Error(), `column "api_key_id" does not exist`) {
			// Legacy DB: just list all active configs (no scoping column).
			legacy := `
				SELECT id, vendor_type, config_json, is_active, created_at, updated_at
				FROM vendor_configs WHERE is_active = true
			`
			rows, err = r.db.Pool.Query(ctx, legacy)
		}
		if err != nil {
			return nil, fmt.Errorf("listing preferred vendor configs: %w", err)
		}
	}
	defer rows.Close()

	configs := make([]*domain.VendorConfig, 0)
	for rows.Next() {
		var cfg domain.VendorConfig
		if err := rows.Scan(&cfg.ID, &cfg.VendorType, &cfg.ConfigJSON, &cfg.IsActive, &cfg.CreatedAt, &cfg.UpdatedAt); err != nil {
			return nil, err
		}
		if r.crypto != nil && len(cfg.ConfigJSON) > 0 {
			plain, _, derr := r.crypto.DecryptJSON(cfg.ConfigJSON)
			if derr != nil {
				// Same rationale as ListActive: tolerate a single corrupted row.
				if r.log != nil {
					r.log.Warn("skipping undecryptable vendor config", zap.String("vendor_type", cfg.VendorType), zap.Error(derr))
				}
				continue
			}
			cfg.ConfigJSON = plain
		}
		configs = append(configs, &cfg)
	}
	return configs, rows.Err()
}

func (r *vendorConfigRepository) SetActive(ctx context.Context, vendorType string, isActive bool, apiKeyID *string) error {
	query := `
		UPDATE vendor_configs
		SET is_active = $1, updated_at = CURRENT_TIMESTAMP
		WHERE vendor_type = $2
		  AND (
		    ($3::uuid IS NULL AND api_key_id IS NULL)
		    OR api_key_id = $3::uuid
		  )
	`
	_, err := r.db.Pool.Exec(ctx, query, isActive, vendorType, apiKeyID)
	if err != nil {
		// Back-compat for older DBs that don't have api_key_id scoping.
		if strings.Contains(err.Error(), `column "api_key_id" does not exist`) {
			legacy := `
				UPDATE vendor_configs
				SET is_active = $1, updated_at = CURRENT_TIMESTAMP
				WHERE vendor_type = $2
			`
			_, err = r.db.Pool.Exec(ctx, legacy, isActive, vendorType)
		}
	}
	if err != nil {
		return fmt.Errorf("updating vendor config active flag: %w", err)
	}
	return nil
}
