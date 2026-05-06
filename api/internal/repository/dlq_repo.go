package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// DLQEntry represents a dead-letter queue record for a notification that
// exceeded its maximum retry attempts.
type DLQEntry struct {
	ID             uuid.UUID              `json:"id"`
	NotificationID *uuid.UUID             `json:"notification_id,omitempty"`
	APIKeyID       *uuid.UUID             `json:"api_key_id,omitempty"`
	Channel        string                 `json:"channel"`
	Recipient      string                 `json:"recipient,omitempty"`
	Reason         string                 `json:"reason"`
	Payload        map[string]interface{} `json:"payload,omitempty"`
	AttemptCount   int                    `json:"attempt_count"`
	Replayed       bool                   `json:"replayed"`
	ReplayedAt     *time.Time             `json:"replayed_at,omitempty"`
	CreatedAt      time.Time              `json:"created_at"`
}

// DLQRepository provides CRUD operations on the dlq_entries table.
type DLQRepository struct {
	db *DB
}

// NewDLQRepository creates a new DLQ repository.
func NewDLQRepository(db *DB) *DLQRepository {
	return &DLQRepository{db: db}
}

// Insert adds a new entry to the dead-letter queue.
func (r *DLQRepository) Insert(ctx context.Context, entry *DLQEntry) error {
	payloadJSON, err := json.Marshal(entry.Payload)
	if err != nil {
		return fmt.Errorf("marshalling dlq payload: %w", err)
	}

	const q = `
		INSERT INTO dlq_entries (id, notification_id, api_key_id, channel, recipient, reason, payload, attempt_count)
		VALUES ($1, $2, $3, $4, $5, $6, $7::jsonb, $8)
		ON CONFLICT DO NOTHING`

	_, err = r.db.Pool.Exec(ctx, q,
		entry.ID,
		entry.NotificationID,
		entry.APIKeyID,
		entry.Channel,
		entry.Recipient,
		entry.Reason,
		payloadJSON,
		entry.AttemptCount,
	)
	if err != nil {
		return fmt.Errorf("inserting dlq entry: %w", err)
	}
	return nil
}

// List returns DLQ entries with pagination and optional filter for un-replayed entries.
func (r *DLQRepository) List(ctx context.Context, page, pageSize int, unplayedOnly bool, apiKeyID ...string) ([]DLQEntry, int, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 50
	}
	offset := (page - 1) * pageSize

	// Determine if we have a client scope filter
	scoped := len(apiKeyID) > 0 && apiKeyID[0] != ""

	// Build count query
	countQ := `SELECT COUNT(*) FROM dlq_entries`
	var countArgs []interface{}
	argIdx := 1

	var conditions []string
	if unplayedOnly {
		conditions = append(conditions, "replayed = FALSE")
	}
	if scoped {
		conditions = append(conditions, fmt.Sprintf("api_key_id = $%d", argIdx))
		countArgs = append(countArgs, apiKeyID[0])
		argIdx++
	}
	if len(conditions) > 0 {
		countQ += " WHERE " + strings.Join(conditions, " AND ")
	}

	var total int
	if err := r.db.Pool.QueryRow(ctx, countQ, countArgs...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("counting dlq entries: %w", err)
	}

	// Build data query
	dataQ := `
		SELECT id, notification_id, api_key_id, channel, recipient, reason, payload, attempt_count,
		       replayed, replayed_at, created_at
		FROM dlq_entries`
	var dataArgs []interface{}
	argIdx = 1

	var dataConditions []string
	if unplayedOnly {
		dataConditions = append(dataConditions, "replayed = FALSE")
	}
	if scoped {
		dataConditions = append(dataConditions, fmt.Sprintf("api_key_id = $%d", argIdx))
		dataArgs = append(dataArgs, apiKeyID[0])
		argIdx++
	}
	if len(dataConditions) > 0 {
		dataQ += " WHERE " + strings.Join(dataConditions, " AND ")
	}

	dataQ += fmt.Sprintf(" ORDER BY created_at DESC LIMIT $%d OFFSET $%d", argIdx, argIdx+1)
	dataArgs = append(dataArgs, pageSize, offset)

	rows, err := r.db.Pool.Query(ctx, dataQ, dataArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("querying dlq entries: %w", err)
	}
	defer rows.Close()

	var entries []DLQEntry
	for rows.Next() {
		var e DLQEntry
		var payloadJSON []byte
		if err := rows.Scan(
			&e.ID, &e.NotificationID, &e.APIKeyID, &e.Channel, &e.Recipient, &e.Reason,
			&payloadJSON, &e.AttemptCount, &e.Replayed, &e.ReplayedAt, &e.CreatedAt,
		); err != nil {
			return nil, 0, fmt.Errorf("scanning dlq entry: %w", err)
		}
		if len(payloadJSON) > 0 {
			_ = json.Unmarshal(payloadJSON, &e.Payload)
		}
		entries = append(entries, e)
	}

	return entries, total, nil
}

// GetByID retrieves a single DLQ entry by ID.
func (r *DLQRepository) GetByID(ctx context.Context, id uuid.UUID) (*DLQEntry, error) {
	const q = `
		SELECT id, notification_id, channel, recipient, reason, payload, attempt_count,
		       replayed, replayed_at, created_at
		FROM dlq_entries
		WHERE id = $1`

	var e DLQEntry
	var payloadJSON []byte
	err := r.db.Pool.QueryRow(ctx, q, id).Scan(
		&e.ID, &e.NotificationID, &e.Channel, &e.Recipient, &e.Reason,
		&payloadJSON, &e.AttemptCount, &e.Replayed, &e.ReplayedAt, &e.CreatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("querying dlq entry: %w", err)
	}
	if len(payloadJSON) > 0 {
		_ = json.Unmarshal(payloadJSON, &e.Payload)
	}
	return &e, nil
}

// MarkReplayed marks a DLQ entry as replayed.
func (r *DLQRepository) MarkReplayed(ctx context.Context, id uuid.UUID) error {
	const q = `UPDATE dlq_entries SET replayed = TRUE, replayed_at = NOW() WHERE id = $1`
	_, err := r.db.Pool.Exec(ctx, q, id)
	if err != nil {
		return fmt.Errorf("marking dlq entry replayed: %w", err)
	}
	return nil
}

// MarkAllReplayed marks all un-replayed entries as replayed.
func (r *DLQRepository) MarkAllReplayed(ctx context.Context) error {
	const q = `UPDATE dlq_entries SET replayed = TRUE, replayed_at = NOW() WHERE replayed = FALSE`
	_, err := r.db.Pool.Exec(ctx, q)
	if err != nil {
		return fmt.Errorf("marking all dlq entries replayed: %w", err)
	}
	return nil
}

// DeleteOlderThan deletes DLQ entries older than the specified duration.
func (r *DLQRepository) DeleteOlderThan(ctx context.Context, cutoff time.Time) (int, error) {
	const q = `DELETE FROM dlq_entries WHERE created_at < $1`
	result, err := r.db.Pool.Exec(ctx, q, cutoff)
	if err != nil {
		return 0, fmt.Errorf("deleting old dlq entries: %w", err)
	}
	return int(result.RowsAffected()), nil
}
