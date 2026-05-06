package repository

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/spidey/notification-service/internal/domain"
)

type UserRepository struct {
	db *DB
}

func NewUserRepository(db *DB) *UserRepository {
	return &UserRepository{db: db}
}

type userRow struct {
	User         domain.User
	PasswordHash []byte
}

func (r *UserRepository) GetByID(ctx context.Context, id string) (*domain.User, error) {
	var u domain.User
	var role string
	err := r.db.Pool.QueryRow(ctx, `
		SELECT id, COALESCE(clerk_id, '') AS clerk_id, email, name, role, created_at
		FROM users
		WHERE id = $1
	`, id).Scan(&u.ID, &u.ClerkID, &u.Email, &u.Name, &role, &u.CreatedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, domain.ErrNotFound
		}
		return nil, fmt.Errorf("get user by id: %w", err)
	}
	u.Role = domain.UserRole(role)
	return &u, nil
}

// GetByClerkID retrieves a user by their Clerk ID.
func (r *UserRepository) GetByClerkID(ctx context.Context, clerkID string) (*domain.User, error) {
	var u domain.User
	var role string
	err := r.db.Pool.QueryRow(ctx, `
		SELECT id, clerk_id, email, name, role, created_at
		FROM users
		WHERE clerk_id = $1
	`, clerkID).Scan(&u.ID, &u.ClerkID, &u.Email, &u.Name, &role, &u.CreatedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, domain.ErrNotFound
		}
		return nil, fmt.Errorf("get user by clerk_id: %w", err)
	}
	u.Role = domain.UserRole(role)
	return &u, nil
}

func (r *UserRepository) Create(ctx context.Context, email, name string, passwordHash []byte, role domain.UserRole) (*domain.User, error) {
	var u domain.User
	err := r.db.Pool.QueryRow(ctx, `
		INSERT INTO users (email, name, password_hash, role)
		VALUES ($1, $2, $3, $4)
		RETURNING id, email, name, role, created_at
	`, email, name, passwordHash, string(role)).Scan(&u.ID, &u.Email, &u.Name, &u.Role, &u.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("create user: %w", err)
	}
	return &u, nil
}

// CreateFromClerk creates a new user from Clerk webhook data.
func (r *UserRepository) CreateFromClerk(ctx context.Context, clerkID, email, name string, role domain.UserRole) (*domain.User, error) {
	var u domain.User
	var rl string
	err := r.db.Pool.QueryRow(ctx, `
		INSERT INTO users (clerk_id, email, name, role)
		VALUES ($1, $2, $3, $4)
		RETURNING id, clerk_id, email, name, role, created_at
	`, clerkID, email, name, string(role)).Scan(&u.ID, &u.ClerkID, &u.Email, &u.Name, &rl, &u.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("create user from clerk: %w", err)
	}
	u.Role = domain.UserRole(rl)
	return &u, nil
}

// UpdateFromClerk updates an existing user from Clerk webhook data.
func (r *UserRepository) UpdateFromClerk(ctx context.Context, id, email, name string, role domain.UserRole) error {
	cmd, err := r.db.Pool.Exec(ctx, `
		UPDATE users SET email=$2, name=$3, role=$4 WHERE id=$1
	`, id, email, name, string(role))
	if err != nil {
		return fmt.Errorf("update user from clerk: %w", err)
	}
	if cmd.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (r *UserRepository) GetByEmailWithHash(ctx context.Context, email string) (*userRow, error) {
	var row userRow
	var role string
	err := r.db.Pool.QueryRow(ctx, `
		SELECT id, email, name, role, created_at, password_hash
		FROM users
		WHERE email = $1
	`, email).Scan(&row.User.ID, &row.User.Email, &row.User.Name, &role, &row.User.CreatedAt, &row.PasswordHash)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, domain.ErrNotFound
		}
		return nil, fmt.Errorf("get user by email: %w", err)
	}
	row.User.Role = domain.UserRole(role)
	return &row, nil
}

func (r *UserRepository) List(ctx context.Context) ([]*domain.User, error) {
	rows, err := r.db.Pool.Query(ctx, `
		SELECT id, COALESCE(clerk_id, '') AS clerk_id, email, name, role, created_at
		FROM users
		ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}
	defer rows.Close()

	var out []*domain.User
	for rows.Next() {
		var u domain.User
		var role string
		if err := rows.Scan(&u.ID, &u.ClerkID, &u.Email, &u.Name, &role, &u.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan user: %w", err)
		}
		u.Role = domain.UserRole(role)
		out = append(out, &u)
	}
	if out == nil {
		out = []*domain.User{}
	}
	return out, nil
}

func (r *UserRepository) SetRole(ctx context.Context, id string, role domain.UserRole) error {
	cmd, err := r.db.Pool.Exec(ctx, `UPDATE users SET role=$2 WHERE id=$1`, id, string(role))
	if err != nil {
		return fmt.Errorf("set role: %w", err)
	}
	if cmd.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (r *UserRepository) Delete(ctx context.Context, id string) error {
	cmd, err := r.db.Pool.Exec(ctx, `DELETE FROM users WHERE id=$1`, id)
	if err != nil {
		return fmt.Errorf("delete user: %w", err)
	}
	if cmd.RowsAffected() == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (r *UserRepository) SetUserAPIKeys(ctx context.Context, userID string, apiKeyIDs []string) error {
	tx, err := r.db.Pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	if _, err := tx.Exec(ctx, `DELETE FROM user_api_keys WHERE user_id = $1`, userID); err != nil {
		return fmt.Errorf("clear user api keys: %w", err)
	}
	for _, k := range apiKeyIDs {
		if _, err := tx.Exec(ctx, `INSERT INTO user_api_keys (user_id, api_key_id) VALUES ($1, $2)`, userID, k); err != nil {
			return fmt.Errorf("assign api key: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}

func (r *UserRepository) ListAPIKeysForUser(ctx context.Context, userID string) ([]string, error) {
	rows, err := r.db.Pool.Query(ctx, `SELECT api_key_id FROM user_api_keys WHERE user_id=$1`, userID)
	if err != nil {
		return nil, fmt.Errorf("list user api keys: %w", err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan user api key: %w", err)
		}
		out = append(out, id)
	}
	if out == nil {
		out = []string{}
	}
	return out, nil
}
