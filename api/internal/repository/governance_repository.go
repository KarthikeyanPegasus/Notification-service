package repository

import (
	"context"

	"github.com/google/uuid"
	"github.com/spidey/notification-service/internal/domain"
)

type GovernanceRepository struct {
	db *DB
}

func NewGovernanceRepository(db *DB) *GovernanceRepository {
	return &GovernanceRepository{db: db}
}

// Suppressions

func (r *GovernanceRepository) AddSuppression(ctx context.Context, s *domain.Suppression) error {
	query := `
		INSERT INTO suppressions (id, type, value, reason, metadata, created_by, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (type, value) DO UPDATE SET
			reason = EXCLUDED.reason,
			metadata = EXCLUDED.metadata,
			created_by = EXCLUDED.created_by,
			created_at = EXCLUDED.created_at
	`
	_, err := r.db.Pool.Exec(ctx, query, s.ID, s.Type, s.Value, s.Reason, s.Metadata, s.CreatedBy, s.CreatedAt)
	return err
}

func (r *GovernanceRepository) ListSuppressions(ctx context.Context, createdBy ...string) ([]*domain.Suppression, error) {
	var query string
	var args []any
	if len(createdBy) > 0 && createdBy[0] != "" {
		query = `SELECT id, type, value, reason, metadata, created_by, created_at FROM suppressions WHERE created_by = $1 ORDER BY created_at DESC`
		args = append(args, createdBy[0])
	} else {
		query = `SELECT id, type, value, reason, metadata, created_by, created_at FROM suppressions ORDER BY created_at DESC`
	}
	rows, err := r.db.Pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	list := make([]*domain.Suppression, 0)
	for rows.Next() {
		var s domain.Suppression
		if err := rows.Scan(&s.ID, &s.Type, &s.Value, &s.Reason, &s.Metadata, &s.CreatedBy, &s.CreatedAt); err != nil {
			return nil, err
		}
		list = append(list, &s)
	}
	return list, nil
}

func (r *GovernanceRepository) DeleteSuppression(ctx context.Context, id uuid.UUID, createdBy ...string) error {
	var query string
	var args []any
	if len(createdBy) > 0 && createdBy[0] != "" {
		query = "DELETE FROM suppressions WHERE id = $1 AND created_by = $2"
		args = append(args, id, createdBy[0])
	} else {
		query = "DELETE FROM suppressions WHERE id = $1"
		args = append(args, id)
	}
	_, err := r.db.Pool.Exec(ctx, query, args...)
	return err
}

func (r *GovernanceRepository) IsSuppressed(ctx context.Context, stype domain.SuppressionType, value string) (bool, error) {
	var exists bool
	// Support wildcard matching for domains (email) and prefixes (sms)
	const query = `
		SELECT EXISTS(
			SELECT 1 FROM suppressions 
			WHERE type = $1 AND (
				value = $2 OR 
				($1 = 'email' AND $2 LIKE '%@' || value) OR
				($1 = 'sms' AND $2 LIKE value || '%')
			)
		)`
	err := r.db.Pool.QueryRow(ctx, query, stype, value).Scan(&exists)
	return exists, err
}

// Opt-outs

func (r *GovernanceRepository) AddOptOut(ctx context.Context, o *domain.OptOut) error {
	query := `
		INSERT INTO opt_outs (id, user_id, channel, reason, source, created_by, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (user_id, channel) DO UPDATE SET
			reason = EXCLUDED.reason,
			source = EXCLUDED.source,
			created_by = EXCLUDED.created_by,
			created_at = EXCLUDED.created_at
	`
	_, err := r.db.Pool.Exec(ctx, query, o.ID, o.UserID, o.Channel, o.Reason, o.Source, o.CreatedBy, o.CreatedAt)
	return err
}

func (r *GovernanceRepository) ListOptOuts(ctx context.Context, createdBy ...string) ([]*domain.OptOut, error) {
	var query string
	var args []any
	if len(createdBy) > 0 && createdBy[0] != "" {
		query = `SELECT id, user_id, channel, reason, source, created_by, created_at FROM opt_outs WHERE created_by = $1 ORDER BY created_at DESC`
		args = append(args, createdBy[0])
	} else {
		query = `SELECT id, user_id, channel, reason, source, created_by, created_at FROM opt_outs ORDER BY created_at DESC`
	}
	rows, err := r.db.Pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	list := make([]*domain.OptOut, 0)
	for rows.Next() {
		var o domain.OptOut
		if err := rows.Scan(&o.ID, &o.UserID, &o.Channel, &o.Reason, &o.Source, &o.CreatedBy, &o.CreatedAt); err != nil {
			return nil, err
		}
		list = append(list, &o)
	}
	return list, nil
}

func (r *GovernanceRepository) DeleteOptOut(ctx context.Context, id uuid.UUID, createdBy ...string) error {
	var query string
	var args []any
	if len(createdBy) > 0 && createdBy[0] != "" {
		query = "DELETE FROM opt_outs WHERE id = $1 AND created_by = $2"
		args = append(args, id, createdBy[0])
	} else {
		query = "DELETE FROM opt_outs WHERE id = $1"
		args = append(args, id)
	}
	_, err := r.db.Pool.Exec(ctx, query, args...)
	return err
}

func (r *GovernanceRepository) IsOptedOut(ctx context.Context, userID uuid.UUID, channel domain.Channel) (bool, error) {
	var exists bool
	query := `SELECT EXISTS(SELECT 1 FROM opt_outs WHERE user_id = $1 AND channel = $2)`
	err := r.db.Pool.QueryRow(ctx, query, userID, channel).Scan(&exists)
	return exists, err
}
