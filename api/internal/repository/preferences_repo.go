package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/spidey/notification-service/internal/domain"
)

// UserPreferencesRepository persists user notification preferences in PostgreSQL.
// Preferences are stored as JSONB for schema flexibility, with a primary key on user_id.
type UserPreferencesRepository struct {
	db *DB
}

func NewUserPreferencesRepository(db *DB) *UserPreferencesRepository {
	return &UserPreferencesRepository{db: db}
}

// Get retrieves preferences for a user. Returns nil if no preferences are set.
func (r *UserPreferencesRepository) Get(ctx context.Context, userID string) (*domain.UserPreferences, error) {
	const q = `SELECT preferences, updated_at FROM user_preferences WHERE user_id = $1`

	var rawJSON []byte
	var updatedAt time.Time
	err := r.db.Pool.QueryRow(ctx, q, userID).Scan(&rawJSON, &updatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("querying user_preferences: %w", err)
	}

	var prefs domain.UserPreferences
	if err := json.Unmarshal(rawJSON, &prefs); err != nil {
		return nil, fmt.Errorf("unmarshalling user_preferences: %w", err)
	}
	prefs.UserID = userID
	prefs.UpdatedAt = updatedAt
	return &prefs, nil
}

// Upsert creates or replaces preferences for a user.
// Existing preferences are fully replaced with the provided value.
func (r *UserPreferencesRepository) Upsert(ctx context.Context, userID string, prefs *domain.UserPreferences) error {
	if prefs == nil {
		return fmt.Errorf("preferences must not be nil")
	}

	// Normalize: ensure Channels map is non-nil for JSON serialization
	if prefs.Channels == nil {
		prefs.Channels = make(map[domain.Channel]bool)
	}
	if prefs.FrequencyCaps == nil {
		prefs.FrequencyCaps = make(map[string]int)
	}
	if prefs.UnsubscribedTypes == nil {
		prefs.UnsubscribedTypes = []string{}
	}
	prefs.UserID = userID

	data, err := json.Marshal(prefs)
	if err != nil {
		return fmt.Errorf("marshalling preferences: %w", err)
	}

	const q = `
		INSERT INTO user_preferences (user_id, preferences, created_at, updated_at)
		VALUES ($1, $2::jsonb, NOW(), NOW())
		ON CONFLICT (user_id)
		DO UPDATE SET preferences = $2::jsonb, updated_at = NOW()`

	_, err = r.db.Pool.Exec(ctx, q, userID, data)
	if err != nil {
		return fmt.Errorf("upserting user_preferences: %w", err)
	}
	return nil
}

// Delete removes preferences for a user.
func (r *UserPreferencesRepository) Delete(ctx context.Context, userID string) error {
	const q = `DELETE FROM user_preferences WHERE user_id = $1`
	_, err := r.db.Pool.Exec(ctx, q, userID)
	if err != nil {
		return fmt.Errorf("deleting user_preferences: %w", err)
	}
	return nil
}

// GetChannelEnabled returns whether a specific channel is enabled for a user.
// Returns true (enabled) if no preferences are set (permissive default).
func (r *UserPreferencesRepository) GetChannelEnabled(ctx context.Context, userID string, channel domain.Channel) (bool, error) {
	prefs, err := r.Get(ctx, userID)
	if err != nil {
		return false, err
	}
	if prefs == nil {
		return true, nil // permissive default
	}
	return prefs.IsChannelEnabled(channel), nil
}

// GetUserIDByNotificationID fetches the user_id from a notification record.
// Used by the workflow to look up preferences for a notification's user.
func (r *UserPreferencesRepository) GetUserIDByNotificationID(ctx context.Context, notifID uuid.UUID) (string, error) {
	const q = `SELECT user_id FROM notifications WHERE id = $1`
	var userID *string
	err := r.db.Pool.QueryRow(ctx, q, notifID).Scan(&userID)
	if err != nil {
		return "", fmt.Errorf("querying notification user_id: %w", err)
	}
	if userID == nil {
		return "", nil
	}
	return *userID, nil
}
