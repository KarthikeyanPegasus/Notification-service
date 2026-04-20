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

// NotificationRepository handles persistence for notifications.
type NotificationRepository struct {
	db *DB
}

func NewNotificationRepository(db *DB) *NotificationRepository {
	return &NotificationRepository{db: db}
}

// Create inserts a new notification. Returns ErrAlreadyExists on idempotency key collision.
func (r *NotificationRepository) Create(ctx context.Context, n *domain.Notification) error {
	content, err := json.Marshal(n.RenderedContent)
	if err != nil {
		return fmt.Errorf("marshalling rendered content: %w", err)
	}

	const q = `
		INSERT INTO notifications
			(id, idempotency_key, user_id, channel, priority, type, template_id,
			 rendered_content, recipient, status, scheduled_at, sent_at, delivered_at, source, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16)
		ON CONFLICT (idempotency_key) DO NOTHING`

	tag, err := r.db.Pool.Exec(ctx, q,
		n.ID, n.IdempotencyKey, n.UserID, n.Channel, n.Priority, n.Type,
		n.TemplateID, content, n.Recipient, n.Status, n.ScheduledAt,
		n.SentAt, n.DeliveredAt, n.Source, n.CreatedAt, n.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("inserting notification: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrAlreadyExists
	}
	return nil
}

// GetByID fetches a notification by its primary key.
func (r *NotificationRepository) GetByID(ctx context.Context, id uuid.UUID) (*domain.Notification, error) {
	const q = `
		SELECT id, idempotency_key, user_id, channel, priority, type, template_id,
		       rendered_content, recipient, status, scheduled_at, sent_at, delivered_at, source, created_at, updated_at
		FROM notifications WHERE id = $1`

	row := r.db.Pool.QueryRow(ctx, q, id)
	return scanNotification(row)
}

// GetByIdempotencyKey fetches a notification by its idempotency key.
func (r *NotificationRepository) GetByIdempotencyKey(ctx context.Context, key string) (*domain.Notification, error) {
	const q = `
		SELECT id, idempotency_key, user_id, channel, priority, type, template_id,
		       rendered_content, recipient, status, scheduled_at, sent_at, delivered_at, source, created_at, updated_at
		FROM notifications WHERE idempotency_key = $1`

	row := r.db.Pool.QueryRow(ctx, q, key)
	return scanNotification(row)
}

// UpdateStatus atomically updates notification status and updated_at.
// It also sets sent_at or delivered_at based on the status.
func (r *NotificationRepository) UpdateStatus(ctx context.Context, id uuid.UUID, status domain.NotificationStatus) error {
	now := time.Now()
	var q string
	switch status {
	case domain.StatusSent:
		q = `UPDATE notifications SET status=$1, updated_at=$2, sent_at=$2 WHERE id=$3`
	case domain.StatusDelivered:
		q = `UPDATE notifications SET status=$1, updated_at=$2, delivered_at=$2 WHERE id=$3`
	default:
		q = `UPDATE notifications SET status=$1, updated_at=$2 WHERE id=$3`
	}
	_, err := r.db.Pool.Exec(ctx, q, status, now, id)
	return err
}

// ListFilters controls the list query.
type ListFilters struct {
	UserID   *uuid.UUID
	Channel  *domain.Channel
	Status   *domain.NotificationStatus
	Type     *string
	From     *time.Time
	To       *time.Time
	Page     int
	PageSize int
}

// List returns a paginated list of notifications.
func (r *NotificationRepository) List(ctx context.Context, f ListFilters) ([]*domain.Notification, int64, error) {
	if f.Page < 1 {
		f.Page = 1
	}
	if f.PageSize < 1 || f.PageSize > 200 {
		f.PageSize = 50
	}

	args := []any{}
	where := "WHERE 1=1"
	idx := 1

	if f.UserID != nil {
		where += fmt.Sprintf(" AND user_id=$%d", idx)
		args = append(args, *f.UserID)
		idx++
	}
	if f.Channel != nil {
		where += fmt.Sprintf(" AND channel=$%d", idx)
		args = append(args, *f.Channel)
		idx++
	}
	if f.Status != nil {
		where += fmt.Sprintf(" AND status=$%d", idx)
		args = append(args, *f.Status)
		idx++
	}
	if f.Type != nil {
		where += fmt.Sprintf(" AND type=$%d", idx)
		args = append(args, *f.Type)
		idx++
	}
	if f.From != nil {
		where += fmt.Sprintf(" AND created_at >= $%d", idx)
		args = append(args, *f.From)
		idx++
	}
	if f.To != nil {
		where += fmt.Sprintf(" AND created_at <= $%d", idx)
		args = append(args, *f.To)
		idx++
	}

	countQ := "SELECT COUNT(*) FROM notifications " + where
	var total int64
	if err := r.db.Pool.QueryRow(ctx, countQ, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("counting notifications: %w", err)
	}

	offset := (f.Page - 1) * f.PageSize
	dataQ := fmt.Sprintf(`
		SELECT id, idempotency_key, user_id, channel, priority, type, template_id,
		       rendered_content, recipient, status, scheduled_at, sent_at, delivered_at, source, created_at, updated_at
		FROM notifications %s
		ORDER BY created_at DESC
		LIMIT $%d OFFSET $%d`, where, idx, idx+1)

	args = append(args, f.PageSize, offset)
	rows, err := r.db.Pool.Query(ctx, dataQ, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("listing notifications: %w", err)
	}
	defer rows.Close()

	results := make([]*domain.Notification, 0)
	for rows.Next() {
		n, err := scanNotificationRow(rows)
		if err != nil {
			return nil, 0, err
		}
		results = append(results, n)
	}
	return results, total, rows.Err()
}

// scanNotification scans a single QueryRow result.
func scanNotification(row pgx.Row) (*domain.Notification, error) {
	n := &domain.Notification{}
	var contentBytes []byte
	err := row.Scan(
		&n.ID, &n.IdempotencyKey, &n.UserID, &n.Channel, &n.Priority,
		&n.Type, &n.TemplateID, &contentBytes, &n.Recipient, &n.Status,
		&n.ScheduledAt, &n.SentAt, &n.DeliveredAt, &n.Source, &n.CreatedAt, &n.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, fmt.Errorf("scanning notification: %w", err)
	}
	if len(contentBytes) > 0 {
		var content domain.RenderedContent
		if err := json.Unmarshal(contentBytes, &content); err == nil {
			n.RenderedContent = &content
		}
	}
	return n, nil
}

func scanNotificationRow(rows pgx.Rows) (*domain.Notification, error) {
	n := &domain.Notification{}
	var contentBytes []byte
	err := rows.Scan(
		&n.ID, &n.IdempotencyKey, &n.UserID, &n.Channel, &n.Priority,
		&n.Type, &n.TemplateID, &contentBytes, &n.Recipient, &n.Status,
		&n.ScheduledAt, &n.SentAt, &n.DeliveredAt, &n.Source, &n.CreatedAt, &n.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("scanning notification row: %w", err)
	}
	if len(contentBytes) > 0 {
		var content domain.RenderedContent
		if err := json.Unmarshal(contentBytes, &content); err == nil {
			n.RenderedContent = &content
		}
	}
	return n, nil
}

// GetStuckNotifications fetches notifications that have been in pending or sent state for older than a duration.
func (r *NotificationRepository) GetStuckNotifications(ctx context.Context, olderThan time.Duration, limit int) ([]*domain.Notification, error) {
	threshold := time.Now().Add(-olderThan)
	const q = `
		SELECT id, idempotency_key, user_id, channel, priority, type, template_id,
		       rendered_content, recipient, status, scheduled_at, sent_at, delivered_at, source, created_at, updated_at
		FROM notifications 
		WHERE status IN ($1, $2) AND updated_at < $3
		ORDER BY updated_at ASC
		LIMIT $4`

	rows, err := r.db.Pool.Query(ctx, q, domain.StatusPending, domain.StatusSent, threshold, limit)
	if err != nil {
		return nil, fmt.Errorf("listing stuck notifications: %w", err)
	}
	defer rows.Close()

	results := make([]*domain.Notification, 0)
	for rows.Next() {
		n, err := scanNotificationRow(rows)
		if err != nil {
			return nil, err
		}
		results = append(results, n)
	}
	return results, rows.Err()
}

// ReportSummaryRow holds aggregated stats per channel per day.
type ReportSummaryRow struct {
	Channel   string  `json:"channel"`
	Date      string  `json:"date"`
	Total     int64   `json:"total"`
	Sent      int64   `json:"sent"`
	Delivered int64   `json:"delivered"`
	Failed    int64   `json:"failed"`
	// SuccessRate is calculated from the counts.
	SuccessRate  float64 `json:"success_rate"`
	P50LatencyMs float64 `json:"p50_latency_ms"`
	P95LatencyMs float64 `json:"p95_latency_ms"`
}

// QuerySummary runs the provided aggregation SQL and returns ReportSummaryRows.
func (r *NotificationRepository) QuerySummary(ctx context.Context, query, dateFrom, dateTo string) ([]ReportSummaryRow, error) {
	rows, err := r.db.Pool.Query(ctx, query, dateFrom, dateTo)
	if err != nil {
		return nil, fmt.Errorf("querying summary: %w", err)
	}
	defer rows.Close()

	var results []ReportSummaryRow
	for rows.Next() {
		var row ReportSummaryRow
		var date time.Time
		if err := rows.Scan(
			&row.Channel, &date, &row.Total, &row.Sent, &row.Delivered, &row.Failed,
			&row.P50LatencyMs, &row.P95LatencyMs,
		); err != nil {
			return nil, fmt.Errorf("scanning summary row: %w", err)
		}
		row.Date = date.Format("2006-01-02")
		if row.Total > 0 {
			row.SuccessRate = float64(row.Delivered) / float64(row.Total)
		}
		results = append(results, row)
	}
	if results == nil {
		results = []ReportSummaryRow{}
	}
	return results, rows.Err()
}
// IngressBreakdownRow holds counts per source.
type IngressBreakdownRow struct {
	Source string `json:"source"`
	Count  int64  `json:"count"`
}

// GetIngressBreakdown calculates ingestion counts per source for a time range.
func (r *NotificationRepository) GetIngressBreakdown(ctx context.Context, from, to time.Time) ([]IngressBreakdownRow, error) {
	const q = `
		SELECT source, COUNT(*) 
		FROM notifications 
		WHERE created_at >= $1 AND created_at <= $2 
		GROUP BY source 
		ORDER BY count DESC`

	rows, err := r.db.Pool.Query(ctx, q, from, to)
	if err != nil {
		return nil, fmt.Errorf("querying ingress breakdown: %w", err)
	}
	defer rows.Close()

	var results []IngressBreakdownRow
	for rows.Next() {
		var row IngressBreakdownRow
		if err := rows.Scan(&row.Source, &row.Count); err != nil {
			return nil, fmt.Errorf("scanning ingress breakdown row: %w", err)
		}
		results = append(results, row)
	}
	if results == nil {
		results = []IngressBreakdownRow{}
	}
	return results, rows.Err()
}
