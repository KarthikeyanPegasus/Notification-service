package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/spidey/notification-service/internal/domain"
)

// TemplateRepository handles notification template persistence.
type TemplateRepository struct {
	db *DB
}

func NewTemplateRepository(db *DB) *TemplateRepository {
	return &TemplateRepository{db: db}
}

func (r *TemplateRepository) GetByID(ctx context.Context, id uuid.UUID) (*domain.NotificationTemplate, error) {
	const q = `
		SELECT id, name, channel, subject, body, version, is_active, created_at, updated_at
		FROM notification_templates WHERE id=$1 AND is_active=TRUE`

	t := &domain.NotificationTemplate{}
	err := r.db.Pool.QueryRow(ctx, q, id).Scan(
		&t.ID, &t.Name, &t.Channel, &t.Subject, &t.Body, &t.Version, &t.IsActive, &t.CreatedAt, &t.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, fmt.Errorf("getting template: %w", err)
	}
	return t, nil
}

func (r *TemplateRepository) GetByName(ctx context.Context, name string) (*domain.NotificationTemplate, error) {
	const q = `
		SELECT id, name, channel, subject, body, version, is_active, created_at, updated_at
		FROM notification_templates WHERE name=$1 AND is_active=TRUE`

	t := &domain.NotificationTemplate{}
	err := r.db.Pool.QueryRow(ctx, q, name).Scan(
		&t.ID, &t.Name, &t.Channel, &t.Subject, &t.Body, &t.Version, &t.IsActive, &t.CreatedAt, &t.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, fmt.Errorf("getting template by name: %w", err)
	}
	return t, nil
}

func (r *TemplateRepository) Create(ctx context.Context, t *domain.NotificationTemplate) error {
	const q = `
		INSERT INTO notification_templates (id, name, channel, subject, body, version, is_active, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`
	_, err := r.db.Pool.Exec(ctx, q,
		t.ID, t.Name, t.Channel, t.Subject, t.Body, t.Version, t.IsActive, t.CreatedAt, time.Now(),
	)
	return err
}

func (r *TemplateRepository) List(ctx context.Context, channel *domain.Channel) ([]*domain.NotificationTemplate, error) {
	q := `SELECT id, name, channel, subject, body, version, is_active, created_at, updated_at FROM notification_templates WHERE is_active=TRUE`
	args := []any{}
	if channel != nil {
		q += " AND channel=$1"
		args = append(args, *channel)
	}
	q += " ORDER BY name ASC"

	rows, err := r.db.Pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var templates []*domain.NotificationTemplate
	for rows.Next() {
		t := &domain.NotificationTemplate{}
		if err := rows.Scan(&t.ID, &t.Name, &t.Channel, &t.Subject, &t.Body, &t.Version, &t.IsActive, &t.CreatedAt, &t.CreatedAt); err != nil {
			return nil, err
		}
		templates = append(templates, t)
	}
	return templates, rows.Err()
}
