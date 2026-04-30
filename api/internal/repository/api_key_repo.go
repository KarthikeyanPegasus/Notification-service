package repository

import (
	"context"
	"crypto/subtle"
	"database/sql"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/spidey/notification-service/internal/domain"
)

type APIKeyRepository struct {
	db *DB
}

func NewAPIKeyRepository(db *DB) *APIKeyRepository {
	return &APIKeyRepository{db: db}
}

func (r *APIKeyRepository) Create(ctx context.Context, key *domain.APIKey, prefix string, hash []byte) error {
	_, err := r.db.Pool.Exec(ctx, `
		INSERT INTO api_keys (id, name, prefix, key_hash, created_at, revoked_at)
		VALUES ($1, $2, $3, $4, $5, NULL)
	`, key.ID, key.Name, prefix, hash, key.CreatedAt)
	if err != nil {
		return fmt.Errorf("insert api key: %w", err)
	}
	return nil
}

func (r *APIKeyRepository) List(ctx context.Context) ([]*domain.APIKey, error) {
	rows, err := r.db.Pool.Query(ctx, `
		SELECT id, name, prefix, created_at, revoked_at
		FROM api_keys
		ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("list api keys: %w", err)
	}
	defer rows.Close()

	var out []*domain.APIKey
	for rows.Next() {
		var k domain.APIKey
		var revoked sql.NullTime
		if err := rows.Scan(&k.ID, &k.Name, &k.Prefix, &k.CreatedAt, &revoked); err != nil {
			return nil, fmt.Errorf("scan api key: %w", err)
		}
		if revoked.Valid {
			t := revoked.Time
			k.RevokedAt = &t
		}
		out = append(out, &k)
	}
	return out, nil
}

// ListByIDs returns API keys for the provided IDs (order not guaranteed).
func (r *APIKeyRepository) ListByIDs(ctx context.Context, ids []string) ([]*domain.APIKey, error) {
	if len(ids) == 0 {
		return []*domain.APIKey{}, nil
	}
	rows, err := r.db.Pool.Query(ctx, `
		SELECT id, name, prefix, created_at, revoked_at
		FROM api_keys
		WHERE id = ANY($1::uuid[])
		ORDER BY created_at DESC
	`, ids)
	if err != nil {
		return nil, fmt.Errorf("list api keys by ids: %w", err)
	}
	defer rows.Close()

	var out []*domain.APIKey
	for rows.Next() {
		var k domain.APIKey
		var revoked sql.NullTime
		if err := rows.Scan(&k.ID, &k.Name, &k.Prefix, &k.CreatedAt, &revoked); err != nil {
			return nil, fmt.Errorf("scan api key: %w", err)
		}
		if revoked.Valid {
			t := revoked.Time
			k.RevokedAt = &t
		}
		out = append(out, &k)
	}
	if out == nil {
		out = []*domain.APIKey{}
	}
	return out, nil
}

func (r *APIKeyRepository) Revoke(ctx context.Context, id string) error {
	_, err := r.db.Pool.Exec(ctx, `UPDATE api_keys SET revoked_at = NOW() WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("revoke api key: %w", err)
	}
	return nil
}

// Verify checks whether apiKey matches a non-revoked key row.
// It returns the stored key metadata (without hash) and ok=true on match.
func (r *APIKeyRepository) Verify(ctx context.Context, prefix string, hash []byte) (*domain.APIKey, bool, error) {
	var k domain.APIKey
	var storedHash []byte
	var revoked sql.NullTime

	err := r.db.Pool.QueryRow(ctx, `
		SELECT id, name, prefix, key_hash, created_at, revoked_at
		FROM api_keys
		WHERE prefix = $1
		LIMIT 1
	`, prefix).Scan(&k.ID, &k.Name, &k.Prefix, &storedHash, &k.CreatedAt, &revoked)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("lookup api key: %w", err)
	}
	if revoked.Valid {
		return nil, false, nil
	}
	if len(storedHash) != len(hash) {
		return nil, false, nil
	}
	if subtle.ConstantTimeCompare(storedHash, hash) != 1 {
		return nil, false, nil
	}
	return &k, true, nil
}
