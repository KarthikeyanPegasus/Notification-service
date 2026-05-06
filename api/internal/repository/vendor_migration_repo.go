package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/spidey/notification-service/internal/domain"
)

// VendorMigrationRepository manages vendor_migrations records.
type VendorMigrationRepository interface {
	Create(ctx context.Context, m *domain.VendorMigration) error
	GetByID(ctx context.Context, id uuid.UUID) (*domain.VendorMigration, error)
	List(ctx context.Context, apiKeyID *string, channel string, status string) ([]*domain.VendorMigration, error)
	UpdateStatus(ctx context.Context, id uuid.UUID, status domain.VendorMigrationStatus, errMsg *string, completedAt *time.Time) error
}

type vendorMigrationRepository struct {
	db *DB
}

func NewVendorMigrationRepository(db *DB) VendorMigrationRepository {
	return &vendorMigrationRepository{db: db}
}

func (r *vendorMigrationRepository) Create(ctx context.Context, m *domain.VendorMigration) error {
	if m.ID == uuid.Nil {
		m.ID = uuid.New()
	}
	const q = `
		INSERT INTO vendor_migrations
			(id, api_key_id, channel, from_vendor, to_vendor,
			 from_config_json, to_config_json, strategy, status, traffic_percent, started_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, NOW())
	`
	_, err := r.db.Pool.Exec(ctx, q,
		m.ID, m.APIKeyID, m.Channel, m.FromVendor, m.ToVendor,
		m.FromConfigJSON, m.ToConfigJSON, m.Strategy, m.Status, m.TrafficPercent,
	)
	if err != nil {
		return fmt.Errorf("creating vendor migration: %w", err)
	}
	return nil
}

func (r *vendorMigrationRepository) GetByID(ctx context.Context, id uuid.UUID) (*domain.VendorMigration, error) {
	const q = `
		SELECT id, api_key_id, channel, from_vendor, to_vendor,
		       from_config_json, to_config_json, strategy, status, traffic_percent,
		       error_message, started_at, completed_at, created_at, updated_at
		FROM vendor_migrations
		WHERE id = $1
	`
	var m domain.VendorMigration
	err := r.db.Pool.QueryRow(ctx, q, id).Scan(
		&m.ID, &m.APIKeyID, &m.Channel, &m.FromVendor, &m.ToVendor,
		&m.FromConfigJSON, &m.ToConfigJSON, &m.Strategy, &m.Status, &m.TrafficPercent,
		&m.ErrorMessage, &m.StartedAt, &m.CompletedAt, &m.CreatedAt, &m.UpdatedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("getting vendor migration: %w", err)
	}
	return &m, nil
}

// List returns migrations filtered by optional apiKeyID, channel and status.
// Pass empty strings to skip a filter. Results are newest-first.
func (r *vendorMigrationRepository) List(ctx context.Context, apiKeyID *string, channel string, status string) ([]*domain.VendorMigration, error) {
	q := `
		SELECT id, api_key_id, channel, from_vendor, to_vendor,
		       from_config_json, to_config_json, strategy, status, traffic_percent,
		       error_message, started_at, completed_at, created_at, updated_at
		FROM vendor_migrations
		WHERE ($1::uuid IS NULL OR api_key_id = $1::uuid)
		  AND ($2 = '' OR channel = $2)
		  AND ($3 = '' OR status  = $3)
		ORDER BY created_at DESC
		LIMIT 100
	`
	rows, err := r.db.Pool.Query(ctx, q, apiKeyID, channel, status)
	if err != nil {
		return nil, fmt.Errorf("listing vendor migrations: %w", err)
	}
	defer rows.Close()

	var results []*domain.VendorMigration
	for rows.Next() {
		var m domain.VendorMigration
		if err := rows.Scan(
			&m.ID, &m.APIKeyID, &m.Channel, &m.FromVendor, &m.ToVendor,
			&m.FromConfigJSON, &m.ToConfigJSON, &m.Strategy, &m.Status, &m.TrafficPercent,
			&m.ErrorMessage, &m.StartedAt, &m.CompletedAt, &m.CreatedAt, &m.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scanning vendor migration: %w", err)
		}
		results = append(results, &m)
	}
	if results == nil {
		results = []*domain.VendorMigration{}
	}
	return results, rows.Err()
}

func (r *vendorMigrationRepository) UpdateStatus(
	ctx context.Context,
	id uuid.UUID,
	status domain.VendorMigrationStatus,
	errMsg *string,
	completedAt *time.Time,
) error {
	const q = `
		UPDATE vendor_migrations
		SET status = $2, error_message = $3, completed_at = $4
		WHERE id = $1
	`
	_, err := r.db.Pool.Exec(ctx, q, id, status, errMsg, completedAt)
	if err != nil {
		return fmt.Errorf("updating vendor migration status: %w", err)
	}
	return nil
}
