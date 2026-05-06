package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/spidey/notification-service/internal/domain"
)

// WebhookEventRepository stores raw inbound provider webhook payloads.
type WebhookEventRepository struct {
	db *DB
}

func NewWebhookEventRepository(db *DB) *WebhookEventRepository {
	return &WebhookEventRepository{db: db}
}

func (r *WebhookEventRepository) Create(ctx context.Context, e *domain.ProviderWebhookEvent) error {
	payload, err := json.Marshal(e.RawPayload)
	if err != nil {
		return fmt.Errorf("marshalling webhook payload: %w", err)
	}

	const q = `
		INSERT INTO provider_webhook_events
			(id, provider, channel, notification_id, event_type, raw_payload, received_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7)`

	_, err = r.db.Pool.Exec(ctx, q,
		e.ID, e.Provider, e.Channel, e.NotificationID, e.EventType, payload, e.ReceivedAt,
	)
	return err
}

func (r *WebhookEventRepository) ListByNotificationID(ctx context.Context, notifID uuid.UUID) ([]*domain.ProviderWebhookEvent, error) {
	const q = `
		SELECT id, provider, channel, notification_id, event_type, raw_payload, received_at
		FROM provider_webhook_events
		WHERE notification_id=$1
		ORDER BY received_at DESC`

	rows, err := r.db.Pool.Query(ctx, q, notifID)
	if err != nil {
		return nil, fmt.Errorf("querying webhook events: %w", err)
	}
	defer rows.Close()

	var events []*domain.ProviderWebhookEvent
	for rows.Next() {
		e := &domain.ProviderWebhookEvent{}
		var payloadBytes []byte
		if err := rows.Scan(&e.ID, &e.Provider, &e.Channel, &e.NotificationID, &e.EventType, &payloadBytes, &e.ReceivedAt); err != nil {
			return nil, err
		}
		if len(payloadBytes) > 0 {
			_ = json.Unmarshal(payloadBytes, &e.RawPayload)
		}
		events = append(events, e)
	}
	return events, rows.Err()
}

// DailyChannelMetrics returns aggregated metrics for reporting.
func (r *WebhookEventRepository) GetDailyMetrics(ctx context.Context, days int) ([]map[string]any, error) {
	return r.GetDailyMetricsWithScope(ctx, days, nil, nil)
}

// GetDailyMetricsWithScope returns daily channel metrics, optionally scoped to specific API keys.
// When scope is specified, it queries the notifications table directly (slower but tenant-safe).
// When scope is nil/empty, it uses the pre-aggregated reporting table (fast, global view).
func (r *WebhookEventRepository) GetDailyMetricsWithScope(ctx context.Context, days int, apiKeyID *uuid.UUID, apiKeyIDs []uuid.UUID) ([]map[string]any, error) {
	// If scoped, query notifications table directly for tenant isolation
	if apiKeyID != nil || (apiKeyIDs != nil && len(apiKeyIDs) > 0) {
		return r.getDailyMetricsFromNotifications(ctx, days, apiKeyID, apiKeyIDs)
	}
	// Global view: use pre-aggregated table
	return r.getDailyMetricsAggregated(ctx, days)
}

func (r *WebhookEventRepository) getDailyMetricsAggregated(ctx context.Context, days int) ([]map[string]any, error) {
	const q = `
		SELECT metric_date, channel, provider,
		       total_sent, total_delivered, total_failed, total_bounced,
		       avg_latency_ms, p50_latency_ms, p95_latency_ms
		FROM reporting_daily_channel_metrics
		WHERE metric_date >= CURRENT_DATE - INTERVAL '1 day' * $1
		ORDER BY metric_date DESC, channel ASC`

	rows, err := r.db.Pool.Query(ctx, q, days)
	if err != nil {
		return nil, fmt.Errorf("querying daily metrics: %w", err)
	}
	defer rows.Close()

	var results []map[string]any
	for rows.Next() {
		var (
			metricDate                      time.Time
			channel, provider               string
			totalSent, totalDelivered       int64
			totalFailed, totalBounced       int64
			avgLatency                      *float64
			p50Latency, p95Latency          *int
		)
		if err := rows.Scan(
			&metricDate, &channel, &provider,
			&totalSent, &totalDelivered, &totalFailed, &totalBounced,
			&avgLatency, &p50Latency, &p95Latency,
		); err != nil {
			return nil, err
		}
		results = append(results, map[string]any{
			"date":            metricDate.Format("2006-01-02"),
			"channel":         channel,
			"provider":        provider,
			"total_sent":      totalSent,
			"total_delivered": totalDelivered,
			"total_failed":    totalFailed,
			"total_bounced":   totalBounced,
			"avg_latency_ms":  avgLatency,
			"p50_latency_ms":  p50Latency,
			"p95_latency_ms":  p95Latency,
		})
	}
	return results, rows.Err()
}

func (r *WebhookEventRepository) getDailyMetricsFromNotifications(ctx context.Context, days int, apiKeyID *uuid.UUID, apiKeyIDs []uuid.UUID) ([]map[string]any, error) {
	// Build WHERE clause for scope
	scopeWhere := ""
	var args []any
	argIdx := 2
	if apiKeyID != nil {
		scopeWhere = fmt.Sprintf("AND n.api_key_id = $%d::uuid", argIdx)
		args = append(args, apiKeyID)
		argIdx++
	} else if apiKeyIDs != nil {
		// Non-nil: scoped. Empty slice → ANY({}) returns 0 rows. nil → no filter (admin).
		scopeWhere = fmt.Sprintf("AND n.api_key_id = ANY($%d::uuid[])", argIdx)
		args = append(args, apiKeyIDs)
		argIdx++
	}

	var q = `
		SELECT
			DATE(n.created_at) AS metric_date,
			n.channel,
			COALESCE(l.provider, 'unknown') AS provider,
			COUNT(*) AS total_sent,
			COUNT(*) FILTER (WHERE n.status IN ('sent','delivered')) AS total_delivered,
			COUNT(*) FILTER (WHERE n.status = 'failed') AS total_failed,
			COUNT(*) FILTER (WHERE n.status = 'bounced') AS total_bounced,
			COALESCE(AVG(EXTRACT(EPOCH FROM (COALESCE(n.delivered_at, n.sent_at) - n.created_at)) * 1000), 0) AS avg_latency_ms,
			COALESCE(PERCENTILE_CONT(0.50) WITHIN GROUP (ORDER BY EXTRACT(EPOCH FROM (COALESCE(n.delivered_at, n.sent_at) - n.created_at)) * 1000), 0) AS p50_latency_ms,
			COALESCE(PERCENTILE_CONT(0.95) WITHIN GROUP (ORDER BY EXTRACT(EPOCH FROM (COALESCE(n.delivered_at, n.sent_at) - n.created_at)) * 1000), 0) AS p95_latency_ms
		FROM notifications n
		LEFT JOIN LATERAL (
			SELECT a.provider FROM notification_attempts a
			WHERE a.notification_id = n.id ORDER BY a.created_at DESC LIMIT 1
		) l ON TRUE
		WHERE n.created_at >= CURRENT_DATE - INTERVAL '1 day' * $1
		  ` + scopeWhere + `
		GROUP BY metric_date, n.channel, l.provider
		ORDER BY metric_date DESC, n.channel ASC`

	args = append([]any{days}, args...)

	rows, err := r.db.Pool.Query(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("querying daily metrics from notifications: %w", err)
	}
	defer rows.Close()

	var results []map[string]any
	for rows.Next() {
		var (
			metricDate                      time.Time
			channel, provider               string
			totalSent, totalDelivered       int64
			totalFailed, totalBounced       int64
			avgLatency                      float64
			p50Latency, p95Latency          float64
		)
		if err := rows.Scan(
			&metricDate, &channel, &provider,
			&totalSent, &totalDelivered, &totalFailed, &totalBounced,
			&avgLatency, &p50Latency, &p95Latency,
		); err != nil {
			return nil, err
		}
		results = append(results, map[string]any{
			"date":            metricDate.Format("2006-01-02"),
			"channel":         channel,
			"provider":        provider,
			"total_sent":      totalSent,
			"total_delivered": totalDelivered,
			"total_failed":    totalFailed,
			"total_bounced":   totalBounced,
			"avg_latency_ms":  avgLatency,
			"p50_latency_ms":  p50Latency,
			"p95_latency_ms":  p95Latency,
		})
	}
	return results, rows.Err()
}
