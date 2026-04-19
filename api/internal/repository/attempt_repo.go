package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/spidey/notification-service/internal/domain"
)

// AttemptRepository handles persistence for notification delivery attempts.
type AttemptRepository struct {
	db *DB
}

func NewAttemptRepository(db *DB) *AttemptRepository {
	return &AttemptRepository{db: db}
}

// Create inserts a new attempt record.
func (r *AttemptRepository) Create(ctx context.Context, a *domain.NotificationAttempt) error {
	const q = `
		INSERT INTO notification_attempts
			(id, notification_id, attempt_number, status, provider, provider_msg_id,
			 error_code, error_message, latency_ms, created_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`

	_, err := r.db.Pool.Exec(ctx, q,
		a.ID, a.NotificationID, a.AttemptNumber, a.Status, a.Provider,
		a.ProviderMsgID, a.ErrorCode, a.ErrorMessage, a.LatencyMs, a.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("inserting attempt: %w", err)
	}
	return nil
}

// ListByNotificationID returns all attempts for a notification ordered by attempt number.
func (r *AttemptRepository) ListByNotificationID(ctx context.Context, notifID uuid.UUID) ([]*domain.NotificationAttempt, error) {
	const q = `
		SELECT id, notification_id, attempt_number, status, provider, provider_msg_id,
		       error_code, error_message, latency_ms, created_at
		FROM notification_attempts
		WHERE notification_id = $1
		ORDER BY attempt_number ASC`

	rows, err := r.db.Pool.Query(ctx, q, notifID)
	if err != nil {
		return nil, fmt.Errorf("querying attempts: %w", err)
	}
	defer rows.Close()

	var attempts []*domain.NotificationAttempt
	for rows.Next() {
		a := &domain.NotificationAttempt{}
		if err := rows.Scan(
			&a.ID, &a.NotificationID, &a.AttemptNumber, &a.Status, &a.Provider,
			&a.ProviderMsgID, &a.ErrorCode, &a.ErrorMessage, &a.LatencyMs, &a.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scanning attempt: %w", err)
		}
		attempts = append(attempts, a)
	}
	return attempts, rows.Err()
}

// RecordAttemptFromResult creates an attempt record from a DeliveryResult.
func (r *AttemptRepository) RecordAttemptFromResult(
	ctx context.Context,
	notifID uuid.UUID,
	attemptNum int,
	result domain.DeliveryResult,
) error {
	a := &domain.NotificationAttempt{
		ID:             uuid.New(),
		NotificationID: notifID,
		AttemptNumber:  attemptNum,
		Provider:       result.Provider,
		LatencyMs:      &result.LatencyMs,
		CreatedAt:      time.Now(),
	}

	if result.Success {
		a.Status = domain.AttemptSent
		a.ProviderMsgID = &result.ProviderMsgID
	} else {
		a.Status = domain.AttemptFailed
		if result.ErrorCode != "" {
			a.ErrorCode = &result.ErrorCode
		}
		if result.ErrorMessage != "" {
			a.ErrorMessage = &result.ErrorMessage
		}
	}

	return r.Create(ctx, a)
}
