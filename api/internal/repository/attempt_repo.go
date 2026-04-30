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
		ORDER BY created_at ASC, attempt_number ASC`

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

// FindByProviderMsgID returns the most recent attempt with the given provider message ID.
func (r *AttemptRepository) FindByProviderMsgID(ctx context.Context, providerMsgID string) (*domain.NotificationAttempt, error) {
	const q = `
		SELECT id, notification_id, attempt_number, status, provider, provider_msg_id,
		       error_code, error_message, latency_ms, created_at
		FROM notification_attempts
		WHERE provider_msg_id = $1
		ORDER BY created_at DESC
		LIMIT 1`
	a := &domain.NotificationAttempt{}
	err := r.db.Pool.QueryRow(ctx, q, providerMsgID).Scan(
		&a.ID, &a.NotificationID, &a.AttemptNumber, &a.Status, &a.Provider,
		&a.ProviderMsgID, &a.ErrorCode, &a.ErrorMessage, &a.LatencyMs, &a.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	return a, nil
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

// VendorMetricRow holds aggregated stats per provider/vendor.
type VendorMetricRow struct {
	Provider      string  `json:"provider"`
	Total         int64   `json:"total"`
	SuccessRate   float64 `json:"success_rate"`
	AvgLatencyMs  float64 `json:"avg_latency_ms"`
	LatestError   string  `json:"latest_error"`
	LastStatus    string  `json:"last_status"`
	// CostPerMsg is the published per-message price in USD.
	CostPerMsg    float64 `json:"cost_per_msg"`
	// EstimatedCost is CostPerMsg * Total (USD, current window).
	EstimatedCost  float64 `json:"estimated_cost"`
	BounceCount    int64   `json:"bounce_count"`
	BounceRate     float64 `json:"bounce_rate"`
	ComplaintCount int64   `json:"complaint_count"`
	ComplaintRate  float64 `json:"complaint_rate"`
	// AllTimeTotal is the denominator used for bounce/complaint rate calculations.
	// It is all-time, not window-scoped, so rates are meaningful even when
	// the live 12h window shows 0 requests.
	AllTimeTotal   int64   `json:"all_time_total"`
}

// vendorPricing maps provider names to their published per-message cost in USD.
// Sources: official pricing pages as of 2025.
var vendorPricing = map[string]float64{
	// Email providers
	"mailgun":    0.0008,   // $0.80 / 1,000 emails (Flex plan)
	"sendgrid":   0.001,    // $0.001 / email (Essentials plan)
	"amazon-ses": 0.0001,   // $0.10 / 1,000 emails
	"ses":        0.0001,
	"postmark":   0.0015,   // $1.50 / 1,000 emails (transactional)
	"smtp-relay": 0.0,      // self-hosted SMTP — no per-msg cost
	// SMS providers
	"twilio":      0.0079,  // $0.0079 / SMS outbound (US)
	"plivo":       0.0035,  // $0.0035 / SMS (US)
	"vonage":      0.0065,  // $0.0065 / SMS (US)
	"messagebird": 0.009,   // $0.009 / SMS (US)
	// Push providers (no per-message cost at API level)
	"fcm":       0.0,
	"onesignal": 0.0,
	"pusher":    0.0,
	// Slack / webhook
	"slack":   0.0,
	"webhook": 0.0,
}

// GetVendorMetrics returns real-time metrics for each provider/vendor for a given duration.
func (r *AttemptRepository) GetVendorMetrics(ctx context.Context, since time.Duration) ([]VendorMetricRow, error) {
	threshold := time.Now().Add(-since)
	// Provider universe = active vendor_configs (specific keys only) UNION providers
	// that have recent attempt records. This ensures env-var-configured providers
	// (no DB entry) still show up as long as they have sent at least one notification.
	const q = `
		WITH providers AS (
			SELECT DISTINCT vendor_type
			FROM vendor_configs
			WHERE is_active = true
			  AND vendor_type NOT IN (
			    'email', 'sms', 'push', 'webhook', 'webhooks',
			    'email_routing', 'sms_routing', 'push_routing', 'webhook_routing',
			    'workflow_orchestration'
			  )
			UNION
			SELECT DISTINCT provider AS vendor_type
			FROM notification_attempts
			WHERE created_at >= $1
			  AND provider IS NOT NULL AND provider <> ''
		),
		windowed AS (
			SELECT *
			FROM notification_attempts
			WHERE created_at >= $1
		),
		latest AS (
			SELECT DISTINCT ON (provider)
				provider,
				status        AS last_status,
				error_message AS last_error_message
			FROM windowed
			ORDER BY provider, created_at DESC
		),
		-- All-time send totals per provider — used as the denominator for
		-- bounce/complaint rates so that vendors with 0 recent sends still
		-- show a meaningful historical rate.
		all_time_sends AS (
			SELECT provider, COUNT(*) AS all_time_total
			FROM notification_attempts
			WHERE provider IS NOT NULL AND provider <> ''
			GROUP BY provider
		),
		-- All-time bounced notifications attributed to each provider (not
		-- limited to the live window so the rate reflects historical reality).
		bounces AS (
			SELECT na.provider, COUNT(*) AS bounce_count
			FROM notification_attempts na
			JOIN notifications n ON n.id = na.notification_id
			WHERE n.status = 'bounced'
			  AND na.provider IS NOT NULL AND na.provider <> ''
			GROUP BY na.provider
		),
		-- All-time spam complaints attributed to each provider.
		complaints AS (
			SELECT (metadata->>'provider') AS provider, COUNT(*) AS complaint_count
			FROM notification_events
			WHERE event_type = 'complained'
			  AND metadata ? 'provider'
			GROUP BY metadata->>'provider'
		)
		SELECT
			p.vendor_type AS provider,
			COUNT(w.id)   AS total,
			COALESCE(COUNT(w.id) FILTER (WHERE w.status IN ('sent', 'delivered'))::float
			         / NULLIF(COUNT(w.id), 0)::float, 1.0) AS success_rate,
			COALESCE(AVG(w.latency_ms), 0)                 AS avg_latency,
			COALESCE(CASE WHEN l.last_status = 'failed' THEN l.last_error_message END, '') AS latest_error,
			COALESCE(l.last_status, '')                    AS last_status,
			COALESCE(b.bounce_count, 0)                    AS bounce_count,
			COALESCE(c.complaint_count, 0)                 AS complaint_count,
			COALESCE(ats.all_time_total, 0)                AS all_time_total
		FROM providers p
		LEFT JOIN windowed     w   ON w.provider   = p.vendor_type
		LEFT JOIN latest       l   ON l.provider   = p.vendor_type
		LEFT JOIN all_time_sends ats ON ats.provider = p.vendor_type
		LEFT JOIN bounces      b   ON b.provider   = p.vendor_type
		LEFT JOIN complaints   c   ON c.provider   = p.vendor_type
		GROUP BY p.vendor_type, l.last_status, l.last_error_message,
		         b.bounce_count, c.complaint_count, ats.all_time_total
		ORDER BY p.vendor_type ASC`

	rows, err := r.db.Pool.Query(ctx, q, threshold)
	if err != nil {
		return nil, fmt.Errorf("querying vendor metrics: %w", err)
	}
	defer rows.Close()

	var results []VendorMetricRow
	for rows.Next() {
		var row VendorMetricRow
		var avgLatency *float64
		if err := rows.Scan(
			&row.Provider, &row.Total, &row.SuccessRate, &avgLatency,
			&row.LatestError, &row.LastStatus,
			&row.BounceCount, &row.ComplaintCount, &row.AllTimeTotal,
		); err != nil {
			return nil, fmt.Errorf("scanning vendor metric: %w", err)
		}
		if avgLatency != nil {
			row.AvgLatencyMs = *avgLatency
		}
		if price, ok := vendorPricing[row.Provider]; ok {
			row.CostPerMsg = price
			row.EstimatedCost = price * float64(row.Total)
		}
		// Use all-time total as denominator so rates remain meaningful even
		// when the live 12h window has zero requests.
		if row.AllTimeTotal > 0 {
			row.BounceRate = float64(row.BounceCount) / float64(row.AllTimeTotal)
			row.ComplaintRate = float64(row.ComplaintCount) / float64(row.AllTimeTotal)
		}
		results = append(results, row)
	}
	if results == nil {
		results = []VendorMetricRow{}
	}
	return results, rows.Err()
}

// GetVendorSendTotals returns all-time successful send counts per provider from our attempt records.
// Used by the billing service to compute estimated cost for providers without a billing API.
func (r *AttemptRepository) GetVendorSendTotals(ctx context.Context) (map[string]int64, error) {
	const q = `
		SELECT provider, COUNT(*) AS total
		FROM notification_attempts
		WHERE provider IS NOT NULL AND provider <> ''
		  AND status IN ('sent', 'delivered')
		GROUP BY provider`

	rows, err := r.db.Pool.Query(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("querying vendor send totals: %w", err)
	}
	defer rows.Close()

	totals := make(map[string]int64)
	for rows.Next() {
		var provider string
		var count int64
		if err := rows.Scan(&provider, &count); err != nil {
			return nil, err
		}
		totals[provider] = count
	}
	return totals, rows.Err()
}

