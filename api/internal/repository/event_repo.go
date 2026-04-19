package repository

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/spidey/notification-service/internal/domain"
)

// EventRepository handles the immutable notification event log.
type EventRepository struct {
	db *DB
}

func NewEventRepository(db *DB) *EventRepository {
	return &EventRepository{db: db}
}

// Append writes an immutable event to the timeline.
func (r *EventRepository) Append(ctx context.Context, e *domain.NotificationEvent) error {
	meta, err := json.Marshal(e.Metadata)
	if err != nil {
		return fmt.Errorf("marshalling event metadata: %w", err)
	}

	const q = `
		INSERT INTO notification_events (id, notification_id, event_type, metadata, created_at)
		VALUES ($1,$2,$3,$4,$5)`

	_, err = r.db.Pool.Exec(ctx, q, e.ID, e.NotificationID, e.EventType, meta, e.CreatedAt)
	if err != nil {
		return fmt.Errorf("inserting event: %w", err)
	}
	return nil
}

// ListByNotificationID returns the full timeline for a notification.
func (r *EventRepository) ListByNotificationID(ctx context.Context, notifID uuid.UUID) ([]*domain.NotificationEvent, error) {
	const q = `
		SELECT id, notification_id, event_type, metadata, created_at
		FROM notification_events
		WHERE notification_id = $1
		ORDER BY created_at ASC`

	rows, err := r.db.Pool.Query(ctx, q, notifID)
	if err != nil {
		return nil, fmt.Errorf("querying events: %w", err)
	}
	defer rows.Close()

	var events []*domain.NotificationEvent
	for rows.Next() {
		e := &domain.NotificationEvent{}
		var metaBytes []byte
		if err := rows.Scan(&e.ID, &e.NotificationID, &e.EventType, &metaBytes, &e.CreatedAt); err != nil {
			return nil, fmt.Errorf("scanning event: %w", err)
		}
		if len(metaBytes) > 0 {
			_ = json.Unmarshal(metaBytes, &e.Metadata)
		}
		events = append(events, e)
	}
	return events, rows.Err()
}
