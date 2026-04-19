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
