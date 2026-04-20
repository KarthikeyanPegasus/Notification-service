package handler

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/spidey/notification-service/internal/repository"
	"go.uber.org/zap"
)

// ReportHandler serves reporting and analytics endpoints.
type ReportHandler struct {
	webhookRepo  *repository.WebhookEventRepository
	notifRepo    *repository.NotificationRepository
	log          *zap.Logger
}

func NewReportHandler(
	webhookRepo *repository.WebhookEventRepository,
	notifRepo *repository.NotificationRepository,
	log *zap.Logger,
) *ReportHandler {
	return &ReportHandler{
		webhookRepo: webhookRepo,
		notifRepo:   notifRepo,
		log:         log,
	}
}

// ChannelMetrics handles GET /v1/reports/channel-metrics
func (h *ReportHandler) ChannelMetrics(c *gin.Context) {
	days := parseInt(c.Query("days"), 7)
	if days > 90 {
		days = 90
	}

	metrics, err := h.webhookRepo.GetDailyMetrics(c.Request.Context(), days)
	if err != nil {
		h.log.Error("getting daily metrics", zap.Error(err))
		respondError(c, http.StatusInternalServerError, "DB_ERROR", "failed to get metrics")
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"days":    days,
		"metrics": metrics,
	})
}

// Summary handles GET /v1/reports/summary
// Aggregates notification stats from the notifications table grouped by channel and date.
func (h *ReportHandler) Summary(c *gin.Context) {
	dateFrom := c.Query("date_from")
	dateTo := c.Query("date_to")

	// Default to last 7 days if not provided
	if dateFrom == "" {
		dateFrom = time.Now().AddDate(0, 0, -7).UTC().Format(time.RFC3339)
	}
	if dateTo == "" {
		dateTo = time.Now().UTC().Format(time.RFC3339)
	}

	const q = `
		SELECT
			n.channel,
			DATE(n.created_at) AS date,
			COUNT(DISTINCT n.id) AS total,
			COUNT(DISTINCT n.id) FILTER (WHERE n.status IN ('sent', 'delivered')) AS sent,
			COUNT(DISTINCT n.id) FILTER (WHERE n.status = 'delivered') AS delivered,
			COUNT(DISTINCT n.id) FILTER (WHERE n.status = 'failed') AS failed,
			COALESCE(
				PERCENTILE_CONT(0.50) WITHIN GROUP (
					ORDER BY EXTRACT(EPOCH FROM (COALESCE(n.delivered_at, n.sent_at) - n.created_at)) * 1000
				) FILTER (WHERE n.delivered_at IS NOT NULL OR n.sent_at IS NOT NULL),
				0
			) AS p50_latency_ms,
			COALESCE(
				PERCENTILE_CONT(0.95) WITHIN GROUP (
					ORDER BY EXTRACT(EPOCH FROM (COALESCE(n.delivered_at, n.sent_at) - n.created_at)) * 1000
				) FILTER (WHERE n.delivered_at IS NOT NULL OR n.sent_at IS NOT NULL),
				0
			) AS p95_latency_ms
		FROM notifications n
		WHERE n.created_at >= $1 AND n.created_at <= $2
		GROUP BY n.channel, DATE(n.created_at)
		ORDER BY date DESC, n.channel ASC`

	rows, err := h.notifRepo.QuerySummary(c.Request.Context(), q, dateFrom, dateTo)
	if err != nil {
		h.log.Error("querying report summary", zap.Error(err))
		respondError(c, http.StatusInternalServerError, "DB_ERROR", "failed to get summary")
		return
	}

	c.JSON(http.StatusOK, rows)
}
// IngressBreakdown handles GET /v1/reports/ingress
func (h *ReportHandler) IngressBreakdown(c *gin.Context) {
	dateFrom := c.Query("date_from")
	dateTo := c.Query("date_to")

	from := time.Now().AddDate(0, 0, -1) // Default last 24h
	to := time.Now()

	if dateFrom != "" {
		if t, err := time.Parse(time.RFC3339, dateFrom); err == nil {
			from = t
		}
	}
	if dateTo != "" {
		if t, err := time.Parse(time.RFC3339, dateTo); err == nil {
			to = t
		}
	}

	metrics, err := h.notifRepo.GetIngressBreakdown(c.Request.Context(), from, to)
	if err != nil {
		h.log.Error("getting ingress metrics", zap.Error(err))
		respondError(c, http.StatusInternalServerError, "DB_ERROR", "failed to get ingress metrics")
		return
	}

	c.JSON(http.StatusOK, metrics)
}
