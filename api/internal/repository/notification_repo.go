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
			 rendered_content, recipient, status, scheduled_at, sent_at, delivered_at,
			 api_key_id, source, orchestration, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18)
		ON CONFLICT (idempotency_key) DO NOTHING`

	tag, err := r.db.Pool.Exec(ctx, q,
		n.ID, n.IdempotencyKey, n.UserID, n.Channel, n.Priority, n.Type,
		n.TemplateID, content, n.Recipient, n.Status, n.ScheduledAt,
		n.SentAt, n.DeliveredAt, n.APIKeyID, n.Source, n.Orchestration, n.CreatedAt, n.UpdatedAt,
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
		SELECT n.id, n.idempotency_key, n.user_id, n.channel, n.priority, n.type, n.template_id,
		       n.rendered_content, n.recipient, n.status, n.scheduled_at, n.sent_at, n.delivered_at,
		       n.api_key_id, n.source, n.orchestration, n.created_at, n.updated_at,
		       COALESCE(k.name, '') as client_name
		FROM notifications n
		LEFT JOIN api_keys k ON n.api_key_id = k.id
		WHERE n.id = $1`

	row := r.db.Pool.QueryRow(ctx, q, id)
	return scanNotification(row)
}

// GetByIdempotencyKey fetches a notification by its idempotency key.
func (r *NotificationRepository) GetByIdempotencyKey(ctx context.Context, key string) (*domain.Notification, error) {
	const q = `
		SELECT n.id, n.idempotency_key, n.user_id, n.channel, n.priority, n.type, n.template_id,
		       n.rendered_content, n.recipient, n.status, n.scheduled_at, n.sent_at, n.delivered_at,
		       n.api_key_id, n.source, n.orchestration, n.created_at, n.updated_at,
		       COALESCE(k.name, '') as client_name
		FROM notifications n
		LEFT JOIN api_keys k ON n.api_key_id = k.id
		WHERE n.idempotency_key = $1`

	row := r.db.Pool.QueryRow(ctx, q, key)
	return scanNotification(row)
}

// SetOrchestration updates the orchestration provider on an existing notification.
func (r *NotificationRepository) SetOrchestration(ctx context.Context, id uuid.UUID, orchestration string) error {
	const q = `UPDATE notifications SET orchestration=$1, updated_at=NOW() WHERE id=$2`
	_, err := r.db.Pool.Exec(ctx, q, orchestration, id)
	return err
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
	APIKeyID *uuid.UUID
	APIKeyIDs []uuid.UUID
	From     *time.Time
	To        *time.Time
	Recipient *string
	Search    *string
	Page      int
	PageSize  int
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
		where += fmt.Sprintf(" AND n.user_id=$%d", idx)
		args = append(args, *f.UserID)
		idx++
	}
	if f.Channel != nil {
		where += fmt.Sprintf(" AND n.channel=$%d", idx)
		args = append(args, *f.Channel)
		idx++
	}
	if f.Status != nil {
		where += fmt.Sprintf(" AND n.status=$%d", idx)
		args = append(args, *f.Status)
		idx++
	}
	if f.Type != nil {
		where += fmt.Sprintf(" AND n.type=$%d", idx)
		args = append(args, *f.Type)
		idx++
	}
	if f.APIKeyID != nil {
		where += fmt.Sprintf(" AND n.api_key_id=$%d", idx)
		args = append(args, *f.APIKeyID)
		idx++
	}
	if f.APIKeyID == nil && f.APIKeyIDs != nil {
		// Non-nil slice: scoped. Empty → ANY({}) matches nothing (returns 0 rows).
		// nil means admin/no-filter.
		where += fmt.Sprintf(" AND n.api_key_id = ANY($%d::uuid[])", idx)
		args = append(args, f.APIKeyIDs)
		idx++
	}
	if f.From != nil {
		where += fmt.Sprintf(" AND n.created_at >= $%d", idx)
		args = append(args, *f.From)
		idx++
	}
	if f.To != nil {
		where += fmt.Sprintf(" AND n.created_at <= $%d", idx)
		args = append(args, *f.To)
		idx++
	}
	if f.Recipient != nil && *f.Recipient != "" {
		where += fmt.Sprintf(" AND n.recipient ILIKE $%d", idx)
		args = append(args, "%"+*f.Recipient+"%")
		idx++
	}
	if f.Search != nil && *f.Search != "" {
		where += fmt.Sprintf(" AND (n.recipient ILIKE $%d OR n.id::text ILIKE $%d OR n.idempotency_key ILIKE $%d)", idx, idx, idx)
		args = append(args, "%"+*f.Search+"%")
		idx++
	}

	countQ := "SELECT COUNT(*) FROM notifications n " + where
	var total int64
	if err := r.db.Pool.QueryRow(ctx, countQ, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("counting notifications: %w", err)
	}

	offset := (f.Page - 1) * f.PageSize
	dataQ := fmt.Sprintf(`
		SELECT n.id, n.idempotency_key, n.user_id, n.channel, n.priority, n.type, n.template_id,
		       n.rendered_content, n.recipient, n.status, n.scheduled_at, n.sent_at, n.delivered_at,
		       n.api_key_id, n.source, n.orchestration, n.created_at, n.updated_at,
		       COALESCE(k.name, '') as client_name,
		       COALESCE(
		           (SELECT provider FROM notification_attempts WHERE notification_id = n.id ORDER BY created_at DESC LIMIT 1),
		           (SELECT (metadata->>'provider') FROM notification_events WHERE notification_id = n.id AND metadata ? 'provider' ORDER BY created_at DESC LIMIT 1),
		           ''
		       ) as provider
		FROM notifications n
		LEFT JOIN api_keys k ON n.api_key_id = k.id
		%s
		ORDER BY n.created_at DESC
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
		&n.ScheduledAt, &n.SentAt, &n.DeliveredAt, &n.APIKeyID, &n.Source, &n.Orchestration, &n.CreatedAt, &n.UpdatedAt,
		&n.ClientName,
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
		&n.ScheduledAt, &n.SentAt, &n.DeliveredAt, &n.APIKeyID, &n.Source, &n.Orchestration, &n.CreatedAt, &n.UpdatedAt, &n.ClientName, &n.Provider,
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
		SELECT n.id, n.idempotency_key, n.user_id, n.channel, n.priority, n.type, n.template_id,
		       n.rendered_content, n.recipient, n.status, n.scheduled_at, n.sent_at, n.delivered_at,
		       n.api_key_id, n.source, n.orchestration, n.created_at, n.updated_at,
		       COALESCE(k.name, '') as client_name,
		       COALESCE(
		           (SELECT provider FROM notification_attempts WHERE notification_id = n.id ORDER BY created_at DESC LIMIT 1),
		           (SELECT (metadata->>'provider') FROM notification_events WHERE notification_id = n.id AND metadata ? 'provider' ORDER BY created_at DESC LIMIT 1),
		           ''
		       ) as provider
		FROM notifications n
		LEFT JOIN api_keys k ON n.api_key_id = k.id
		WHERE n.status IN ($1, $2) AND n.updated_at < $3
		ORDER BY n.updated_at ASC
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
	Channel   string `json:"channel"`
	Date      string `json:"date"`
	Total     int64  `json:"total"`
	Sent      int64  `json:"sent"`
	Delivered int64  `json:"delivered"`
	Failed    int64  `json:"failed"`
	Bounced   int64  `json:"bounced"`
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
			&row.Channel, &date, &row.Total, &row.Sent, &row.Delivered, &row.Failed, &row.Bounced,
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

// QuerySummaryArgs runs the provided aggregation SQL with arbitrary args.
func (r *NotificationRepository) QuerySummaryArgs(ctx context.Context, query string, args ...any) ([]ReportSummaryRow, error) {
	rows, err := r.db.Pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("querying summary: %w", err)
	}
	defer rows.Close()

	var results []ReportSummaryRow
	for rows.Next() {
		var row ReportSummaryRow
		var date time.Time
		if err := rows.Scan(
			&row.Channel, &date, &row.Total, &row.Sent, &row.Delivered, &row.Failed, &row.Bounced,
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

// BreakdownRow holds generic key-count pairs for analytics.
type BreakdownRow struct {
	Key   string `json:"key"`
	Count int64  `json:"count"`
}

// GetIngressBreakdown calculates ingestion counts per source for a time range.
func (r *NotificationRepository) GetIngressBreakdown(ctx context.Context, from, to time.Time, apiKeyID *uuid.UUID) ([]IngressBreakdownRow, error) {
	const q = `
		SELECT source, COUNT(*)
		FROM notifications
		WHERE created_at >= $1 AND created_at <= $2
		  AND ($3::uuid IS NULL OR api_key_id = $3::uuid)
		GROUP BY source
		ORDER BY count DESC`

	rows, err := r.db.Pool.Query(ctx, q, from, to, apiKeyID)
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

// GetIngressBreakdownForKeys calculates ingestion counts per source for a time range for multiple api keys.
// If apiKeyIDs is nil, it returns results across all keys (admin). If empty, returns nothing.
func (r *NotificationRepository) GetIngressBreakdownForKeys(ctx context.Context, from, to time.Time, apiKeyIDs []uuid.UUID) ([]IngressBreakdownRow, error) {
	const q = `
		SELECT source, COUNT(*)
		FROM notifications
		WHERE created_at >= $1 AND created_at <= $2
		  AND ($3::uuid[] IS NULL OR api_key_id = ANY($3::uuid[]))
		GROUP BY source
		ORDER BY count DESC`

	rows, err := r.db.Pool.Query(ctx, q, from, to, apiKeyIDs)
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

// GetSMSCountryBreakdown calculates SMS counts per country prefix for a time range.
func (r *NotificationRepository) GetSMSCountryBreakdown(ctx context.Context, from, to time.Time, apiKeyID *uuid.UUID) ([]BreakdownRow, error) {
	return r.GetSMSCountryBreakdownWithKeys(ctx, from, to, apiKeyID, nil)
}

// GetSMSCountryBreakdownWithKeys calculates SMS counts per country prefix for a time range,
// supporting both single and multi-key scope enforcement.
func (r *NotificationRepository) GetSMSCountryBreakdownWithKeys(ctx context.Context, from, to time.Time, apiKeyID *uuid.UUID, apiKeyIDs []uuid.UUID) ([]BreakdownRow, error) {
	const q = `
		SELECT 
			CASE 
				WHEN recipient LIKE '+1%' THEN '+1 (USA/Canada)'
				WHEN recipient LIKE '+91%' THEN '+91 (India)'
				WHEN recipient LIKE '+44%' THEN '+44 (UK)'
				WHEN recipient LIKE '+61%' THEN '+61 (Australia)'
				WHEN recipient LIKE '+49%' THEN '+49 (Germany)'
				WHEN recipient LIKE '+33%' THEN '+33 (France)'
				ELSE LEFT(recipient, 3) 
			END as country_prefix,
			COUNT(*) as count
		FROM notifications
		WHERE channel = 'sms' AND created_at >= $1 AND created_at <= $2
		  AND (
		    ($3::uuid IS NULL AND $4::uuid[] IS NULL)
		    OR ($3::uuid IS NOT NULL AND api_key_id = $3::uuid)
		    OR ($4::uuid[] IS NOT NULL AND api_key_id = ANY($4::uuid[]))
		  )
		GROUP BY country_prefix
		ORDER BY count DESC
		LIMIT 10`

	var args []any
	args = append(args, from, to)
	if apiKeyID != nil {
		args = append(args, *apiKeyID, nil)
	} else {
		args = append(args, nil, apiKeyIDs)
	}

	rows, err := r.db.Pool.Query(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("querying sms country breakdown: %w", err)
	}
	defer rows.Close()

	var results []BreakdownRow
	for rows.Next() {
		var row BreakdownRow
		if err := rows.Scan(&row.Key, &row.Count); err != nil {
			return nil, err
		}
		results = append(results, row)
	}
	return results, rows.Err()
}

// GetEmailDomainBreakdown calculates Email counts per domain for a time range.
func (r *NotificationRepository) GetEmailDomainBreakdown(ctx context.Context, from, to time.Time, apiKeyID *uuid.UUID) ([]BreakdownRow, error) {
	return r.GetEmailDomainBreakdownWithKeys(ctx, from, to, apiKeyID, nil)
}

// GetEmailDomainBreakdownWithKeys calculates Email counts per domain for a time range,
// supporting both single and multi-key scope enforcement.
func (r *NotificationRepository) GetEmailDomainBreakdownWithKeys(ctx context.Context, from, to time.Time, apiKeyID *uuid.UUID, apiKeyIDs []uuid.UUID) ([]BreakdownRow, error) {
	const q = `
		SELECT split_part(recipient, '@', 2) as domain, COUNT(*) as count
		FROM notifications
		WHERE channel = 'email' AND created_at >= $1 AND created_at <= $2
		  AND (
		    ($3::uuid IS NULL AND $4::uuid[] IS NULL)
		    OR ($3::uuid IS NOT NULL AND api_key_id = $3::uuid)
		    OR ($4::uuid[] IS NOT NULL AND api_key_id = ANY($4::uuid[]))
		  )
		GROUP BY domain
		ORDER BY count DESC
		LIMIT 10`

	var args []any
	args = append(args, from, to)
	if apiKeyID != nil {
		args = append(args, *apiKeyID, nil)
	} else {
		args = append(args, nil, apiKeyIDs)
	}

	rows, err := r.db.Pool.Query(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("querying email domain breakdown: %w", err)
	}
	defer rows.Close()

	var results []BreakdownRow
	for rows.Next() {
		var row BreakdownRow
		if err := rows.Scan(&row.Key, &row.Count); err != nil {
			return nil, err
		}
		results = append(results, row)
	}
	return results, rows.Err()
}

// ScheduledStatsRow holds aggregate metrics for scheduled notifications.
type ScheduledStatsRow struct {
	TotalScheduled       int64    `json:"total_scheduled"`
	Pending              int64    `json:"pending"`
	Delivered            int64    `json:"delivered"`
	AvgDeliveryLatencyMs *float64 `json:"avg_delivery_latency_ms"`
}

// GetScheduledStats returns aggregate stats for scheduled notifications in the given time range.
// ── Admin overview queries ────────────────────────────────────────────────────

// AdminClientRow holds per-client notification volume for the admin overview.
type AdminClientRow struct {
	ClientID    string  `json:"client_id"`
	ClientName  string  `json:"client_name"`
	Total       int64   `json:"total"`
	Sent        int64   `json:"sent"`
	Failed      int64   `json:"failed"`
	FailureRate float64 `json:"failure_rate"`
}

// GetTopClients returns the top N clients by notification volume in the given window.
func (r *NotificationRepository) GetTopClients(ctx context.Context, since time.Time, limit int) ([]AdminClientRow, error) {
	const q = `
		SELECT
			n.api_key_id::text                                                    AS client_id,
			COALESCE(k.name, n.api_key_id::text)                                  AS client_name,
			COUNT(*)                                                              AS total,
			COUNT(*) FILTER (WHERE n.status IN ('sent','delivered'))              AS sent,
			COUNT(*) FILTER (WHERE n.status = 'failed')                           AS failed
		FROM notifications n
		LEFT JOIN api_keys k ON k.id = n.api_key_id
		WHERE n.created_at >= $1
		  AND n.api_key_id IS NOT NULL
		GROUP BY n.api_key_id, k.name
		ORDER BY total DESC
		LIMIT $2`

	rows, err := r.db.Pool.Query(ctx, q, since, limit)
	if err != nil {
		return nil, fmt.Errorf("querying top clients: %w", err)
	}
	defer rows.Close()

	var result []AdminClientRow
	for rows.Next() {
		var row AdminClientRow
		if err := rows.Scan(&row.ClientID, &row.ClientName, &row.Total, &row.Sent, &row.Failed); err != nil {
			return nil, fmt.Errorf("scanning top client row: %w", err)
		}
		if row.Total > 0 {
			row.FailureRate = float64(row.Failed) / float64(row.Total)
		}
		result = append(result, row)
	}
	if result == nil {
		result = []AdminClientRow{}
	}
	return result, rows.Err()
}

// AdminMTTDRow holds mean time-to-deliver aggregated by a grouping key.
type AdminMTTDRow struct {
	Group     string  `json:"group"`
	AvgMs     float64 `json:"avg_ms"`
	P95Ms     float64 `json:"p95_ms"`
	Count     int64   `json:"count"`
}

// GetMTTDByPriority returns MTTD (mean delivery latency) grouped by notification priority.
func (r *NotificationRepository) GetMTTDByPriority(ctx context.Context, since time.Time) ([]AdminMTTDRow, error) {
	const q = `
		SELECT
			n.priority AS grp,
			COALESCE(AVG(EXTRACT(EPOCH FROM (COALESCE(n.delivered_at, n.sent_at) - n.created_at)) * 1000), 0) AS avg_ms,
			COALESCE(
				PERCENTILE_CONT(0.95) WITHIN GROUP (
					ORDER BY (EXTRACT(EPOCH FROM (COALESCE(n.delivered_at, n.sent_at) - n.created_at)) * 1000)
				),
				0
			) AS p95_ms,
			COUNT(*) AS cnt
		FROM notifications n
		WHERE n.created_at >= $1
		  AND n.status IN ('sent', 'delivered')
		  AND (n.delivered_at IS NOT NULL OR n.sent_at IS NOT NULL)
		  AND COALESCE(n.delivered_at, n.sent_at) >= n.created_at
		GROUP BY n.priority
		ORDER BY
			CASE n.priority WHEN 'high' THEN 1 WHEN 'medium' THEN 2 ELSE 3 END`

	return r.queryMTTD(ctx, q, since)
}

// GetMTTDByVendor returns MTTD grouped by delivery provider/vendor.
func (r *NotificationRepository) GetMTTDByVendor(ctx context.Context, since time.Time) ([]AdminMTTDRow, error) {
	const q = `
		SELECT
			COALESCE(la.provider, 'unknown') AS grp,
			COALESCE(AVG(EXTRACT(EPOCH FROM (COALESCE(n.delivered_at, n.sent_at) - n.created_at)) * 1000), 0) AS avg_ms,
			COALESCE(
				PERCENTILE_CONT(0.95) WITHIN GROUP (
					ORDER BY (EXTRACT(EPOCH FROM (COALESCE(n.delivered_at, n.sent_at) - n.created_at)) * 1000)
				),
				0
			) AS p95_ms,
			COUNT(*) AS cnt
		FROM notifications n
		LEFT JOIN LATERAL (
			SELECT a.provider
			FROM notification_attempts a
			WHERE a.notification_id = n.id
			ORDER BY a.created_at DESC
			LIMIT 1
		) la ON TRUE
		WHERE n.created_at >= $1
		  AND n.status IN ('sent', 'delivered')
		  AND (n.delivered_at IS NOT NULL OR n.sent_at IS NOT NULL)
		  AND COALESCE(n.delivered_at, n.sent_at) >= n.created_at
		GROUP BY la.provider
		ORDER BY avg_ms DESC
		LIMIT 20`

	return r.queryMTTD(ctx, q, since)
}

// MTTDClientRow holds MTTD grouped by clientID + priority.
// Used by the AutoScaler to make per-client scaling decisions.
type MTTDClientRow struct {
	ClientID string  `json:"client_id"`
	Priority string  `json:"priority"`
	AvgMs    float64 `json:"avg_ms"`
	Count    int64   `json:"count"`
}

// GetMTTDByClientAndPriority returns MTTD grouped by clientID + priority
// for the given lookback window. Used by the AutoScaler.
func (r *NotificationRepository) GetMTTDByClientAndPriority(ctx context.Context, since time.Time) ([]MTTDClientRow, error) {
	const q = `
		SELECT
			COALESCE(n.api_key_id::text, 'global') AS client_id,
			n.priority,
			COALESCE(AVG(EXTRACT(EPOCH FROM (COALESCE(n.delivered_at, n.sent_at) - n.created_at)) * 1000), 0) AS avg_ms,
			COUNT(*) AS cnt
		FROM notifications n
		WHERE n.created_at >= $1
		  AND n.status IN ('sent', 'delivered')
		  AND (n.delivered_at IS NOT NULL OR n.sent_at IS NOT NULL)
		  AND COALESCE(n.delivered_at, n.sent_at) >= n.created_at
		GROUP BY n.api_key_id, n.priority
		ORDER BY n.priority, n.api_key_id`

	rows, err := r.db.Pool.Query(ctx, q, since)
	if err != nil {
		return nil, fmt.Errorf("querying mttd by client: %w", err)
	}
	defer rows.Close()

	var result []MTTDClientRow
	for rows.Next() {
		var row MTTDClientRow
		if err := rows.Scan(&row.ClientID, &row.Priority, &row.AvgMs, &row.Count); err != nil {
			return nil, fmt.Errorf("scanning mttd client row: %w", err)
		}
		result = append(result, row)
	}
	if result == nil {
		result = []MTTDClientRow{}
	}
	return result, rows.Err()
}

func (r *NotificationRepository) queryMTTD(ctx context.Context, q string, since time.Time) ([]AdminMTTDRow, error) {
	rows, err := r.db.Pool.Query(ctx, q, since)
	if err != nil {
		return nil, fmt.Errorf("querying mttd: %w", err)
	}
	defer rows.Close()

	var result []AdminMTTDRow
	for rows.Next() {
		var row AdminMTTDRow
		if err := rows.Scan(&row.Group, &row.AvgMs, &row.P95Ms, &row.Count); err != nil {
			return nil, fmt.Errorf("scanning mttd row: %w", err)
		}
		result = append(result, row)
	}
	if result == nil {
		result = []AdminMTTDRow{}
	}
	return result, rows.Err()
}

// AdminDeliveryStats holds overall delivery stats for a time window.
type AdminDeliveryStats struct {
	Total       int64   `json:"total"`
	Sent        int64   `json:"sent"`
	Failed      int64   `json:"failed"`
	FailureRate float64 `json:"failure_rate"`
}

// GetDeliveryStats returns total/sent/failed counts for the given window.
func (r *NotificationRepository) GetDeliveryStats(ctx context.Context, since time.Time) (AdminDeliveryStats, error) {
	const q = `
		SELECT
			COUNT(*)                                             AS total,
			COUNT(*) FILTER (WHERE status IN ('sent','delivered')) AS sent,
			COUNT(*) FILTER (WHERE status = 'failed')            AS failed
		FROM notifications
		WHERE created_at >= $1`

	var s AdminDeliveryStats
	err := r.db.Pool.QueryRow(ctx, q, since).Scan(&s.Total, &s.Sent, &s.Failed)
	if err != nil {
		return s, fmt.Errorf("querying delivery stats: %w", err)
	}
	if s.Total > 0 {
		s.FailureRate = float64(s.Failed) / float64(s.Total)
	}
	return s, nil
}

func (r *NotificationRepository) GetScheduledStats(ctx context.Context, from, to time.Time, apiKeyID *uuid.UUID) (ScheduledStatsRow, error) {
	return r.GetScheduledStatsWithKeys(ctx, from, to, apiKeyID, nil)
}

// GetScheduledStatsWithKeys returns aggregate stats for scheduled notifications in the given time range,
// supporting both single and multi-key scope enforcement.
func (r *NotificationRepository) GetScheduledStatsWithKeys(ctx context.Context, from, to time.Time, apiKeyID *uuid.UUID, apiKeyIDs []uuid.UUID) (ScheduledStatsRow, error) {
	const q = `
		SELECT
			COUNT(*) FILTER (WHERE scheduled_at IS NOT NULL)                                   AS total_scheduled,
			COUNT(*) FILTER (WHERE scheduled_at IS NOT NULL AND status IN ('pending','queued')) AS pending,
			COUNT(*) FILTER (WHERE scheduled_at IS NOT NULL AND status IN ('sent','delivered')) AS delivered,
			AVG(
				EXTRACT(EPOCH FROM (COALESCE(delivered_at, sent_at) - scheduled_at)) * 1000
			) FILTER (
				WHERE scheduled_at IS NOT NULL
				  AND (delivered_at IS NOT NULL OR sent_at IS NOT NULL)
				  AND COALESCE(delivered_at, sent_at) > scheduled_at
			) AS avg_delivery_latency_ms
		FROM notifications
		WHERE created_at >= $1 AND created_at <= $2
		  AND (
		    ($3::uuid IS NULL AND $4::uuid[] IS NULL)
		    OR ($3::uuid IS NOT NULL AND api_key_id = $3::uuid)
		    OR ($4::uuid[] IS NOT NULL AND api_key_id = ANY($4::uuid[]))
		  )`

	var args []any
	args = append(args, from, to)
	if apiKeyID != nil {
		args = append(args, *apiKeyID, nil)
	} else {
		args = append(args, nil, apiKeyIDs)
	}

	var row ScheduledStatsRow
	err := r.db.Pool.QueryRow(ctx, q, args...).Scan(
		&row.TotalScheduled, &row.Pending, &row.Delivered, &row.AvgDeliveryLatencyMs,
	)
	if err != nil {
		return row, fmt.Errorf("querying scheduled stats: %w", err)
	}
	return row, nil
}

// ── Cadence / workflow orchestration metrics ──────────────────────────────────

// CadenceThroughputRow holds notification throughput per channel × priority.
type CadenceThroughputRow struct {
	Channel  string `json:"channel"`
	Priority string `json:"priority"`
	Count    int64  `json:"count"`
}

// CadenceRetryRow holds retry statistics per channel.
type CadenceRetryRow struct {
	Channel            string  `json:"channel"`
	TotalNotifications int64   `json:"total_notifications"`
	TotalAttempts      int64   `json:"total_attempts"`
	RetriedCount       int64   `json:"retried_count"`
	RetryRate          float64 `json:"retry_rate"`
	AvgAttempts        float64 `json:"avg_attempts"`
}

// CadenceScheduleRow holds schedule-to-start latency per channel × priority.
type CadenceScheduleRow struct {
	Channel  string  `json:"channel"`
	Priority string  `json:"priority"`
	AvgMs    float64 `json:"avg_ms"`
	P95Ms    float64 `json:"p95_ms"`
	Count    int64   `json:"count"`
}

// GetWorkflowThroughput returns notification counts per channel × priority for the given window.
func (r *NotificationRepository) GetWorkflowThroughput(ctx context.Context, since time.Time) ([]CadenceThroughputRow, error) {
	const q = `
		SELECT channel, priority, COUNT(*) AS count
		FROM notifications
		WHERE created_at >= $1
		GROUP BY channel, priority
		ORDER BY count DESC`

	rows, err := r.db.Pool.Query(ctx, q, since)
	if err != nil {
		return nil, fmt.Errorf("querying workflow throughput: %w", err)
	}
	defer rows.Close()

	var result []CadenceThroughputRow
	for rows.Next() {
		var row CadenceThroughputRow
		if err := rows.Scan(&row.Channel, &row.Priority, &row.Count); err != nil {
			return nil, fmt.Errorf("scanning throughput row: %w", err)
		}
		result = append(result, row)
	}
	if result == nil {
		result = []CadenceThroughputRow{}
	}
	return result, rows.Err()
}

// GetRetryStats returns attempt and retry statistics per channel for the given window.
func (r *NotificationRepository) GetRetryStats(ctx context.Context, since time.Time) ([]CadenceRetryRow, error) {
	const q = `
		WITH attempt_counts AS (
			SELECT notification_id, COUNT(*) AS attempts
			FROM notification_attempts
			WHERE created_at >= $1
			GROUP BY notification_id
		)
		SELECT
			n.channel,
			COUNT(DISTINCT n.id)                                       AS total_notifications,
			COALESCE(SUM(ac.attempts), 0)                              AS total_attempts,
			COUNT(DISTINCT n.id) FILTER (WHERE ac.attempts > 1)        AS retried_count,
			COALESCE(AVG(ac.attempts::float), 1.0)                     AS avg_attempts
		FROM notifications n
		JOIN attempt_counts ac ON ac.notification_id = n.id
		WHERE n.created_at >= $1
		GROUP BY n.channel
		ORDER BY total_notifications DESC`

	rows, err := r.db.Pool.Query(ctx, q, since)
	if err != nil {
		return nil, fmt.Errorf("querying retry stats: %w", err)
	}
	defer rows.Close()

	var result []CadenceRetryRow
	for rows.Next() {
		var row CadenceRetryRow
		if err := rows.Scan(&row.Channel, &row.TotalNotifications, &row.TotalAttempts, &row.RetriedCount, &row.AvgAttempts); err != nil {
			return nil, fmt.Errorf("scanning retry row: %w", err)
		}
		if row.TotalNotifications > 0 {
			row.RetryRate = float64(row.RetriedCount) / float64(row.TotalNotifications)
		}
		result = append(result, row)
	}
	if result == nil {
		result = []CadenceRetryRow{}
	}
	return result, rows.Err()
}

// GetScheduleToStartLatency returns the time from notification created_at to first attempt
// per channel × priority, representing Kafka→Temporal dispatch latency.
func (r *NotificationRepository) GetScheduleToStartLatency(ctx context.Context, since time.Time) ([]CadenceScheduleRow, error) {
	const q = `
		SELECT
			n.channel,
			n.priority,
			AVG(
				EXTRACT(EPOCH FROM (fa.first_attempt - n.created_at)) * 1000
			)                                                             AS avg_ms,
			PERCENTILE_CONT(0.95) WITHIN GROUP (
				ORDER BY EXTRACT(EPOCH FROM (fa.first_attempt - n.created_at)) * 1000
			)                                                             AS p95_ms,
			COUNT(*)                                                      AS cnt
		FROM notifications n
		JOIN (
			SELECT notification_id, MIN(created_at) AS first_attempt
			FROM notification_attempts
			WHERE created_at >= $1
			GROUP BY notification_id
		) fa ON fa.notification_id = n.id
		WHERE n.created_at >= $1
		  AND fa.first_attempt > n.created_at
		GROUP BY n.channel, n.priority
		ORDER BY
			CASE n.priority WHEN 'high' THEN 1 WHEN 'medium' THEN 2 ELSE 3 END,
			n.channel`

	rows, err := r.db.Pool.Query(ctx, q, since)
	if err != nil {
		return nil, fmt.Errorf("querying schedule-to-start latency: %w", err)
	}
	defer rows.Close()

	var result []CadenceScheduleRow
	for rows.Next() {
		var row CadenceScheduleRow
		if err := rows.Scan(&row.Channel, &row.Priority, &row.AvgMs, &row.P95Ms, &row.Count); err != nil {
			return nil, fmt.Errorf("scanning schedule row: %w", err)
		}
		result = append(result, row)
	}
	if result == nil {
		result = []CadenceScheduleRow{}
	}
	return result, rows.Err()
}

// CountClientNotificationsSince counts notifications for a client created since a given time.
// Used by the migration manager to estimate in-flight workflows on the old orchestration.
func (r *NotificationRepository) CountClientNotificationsSince(ctx context.Context, apiKeyID uuid.UUID, since time.Time) (int, error) {
	const q = `SELECT COUNT(*) FROM notifications WHERE api_key_id=$1 AND created_at>=$2`
	var count int
	err := r.db.Pool.QueryRow(ctx, q, apiKeyID, since).Scan(&count)
	return count, err
}
