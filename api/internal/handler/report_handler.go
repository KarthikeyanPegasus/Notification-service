package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/spidey/notification-service/internal/billing"
	"github.com/spidey/notification-service/internal/config"
	"github.com/spidey/notification-service/internal/repository"
	"go.uber.org/zap"
)

// ReportHandler serves reporting and analytics endpoints.
type ReportHandler struct {
	webhookRepo *repository.WebhookEventRepository
	notifRepo   *repository.NotificationRepository
	attemptRepo *repository.AttemptRepository
	users       *repository.UserRepository
	cfg         *config.Config
	vendorRepo  config.Repository
	log         *zap.Logger
}

func NewReportHandler(
	webhookRepo *repository.WebhookEventRepository,
	notifRepo *repository.NotificationRepository,
	attemptRepo *repository.AttemptRepository,
	users *repository.UserRepository,
	cfg *config.Config,
	vendorRepo config.Repository,
	log *zap.Logger,
) *ReportHandler {
	return &ReportHandler{
		webhookRepo: webhookRepo,
		notifRepo:   notifRepo,
		attemptRepo: attemptRepo,
		users:       users,
		cfg:         cfg,
		vendorRepo:  vendorRepo,
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
	apiKeyUUID, apiKeyUUIDs, ok := enforceAPIKeyScopeOrAssigned(c, h.users)
	if !ok {
		return
	}
	apiKeyID := ""
	if apiKeyUUID != nil {
		apiKeyID = apiKeyUUID.String()
	}

	// Default to last 7 days if not provided
	if dateFrom == "" {
		dateFrom = time.Now().AddDate(0, 0, -7).UTC().Format(time.RFC3339)
	}
	if dateTo == "" {
		dateTo = time.Now().UTC().Format(time.RFC3339)
	}

	where := "WHERE n.created_at >= $1 AND n.created_at <= $2"
	args := []any{dateFrom, dateTo}
	if apiKeyID != "" {
		where += " AND n.api_key_id = $3::uuid"
		args = append(args, apiKeyID)
	} else if len(apiKeyUUIDs) > 0 {
		where += " AND n.api_key_id = ANY($3::uuid[])"
		args = append(args, apiKeyUUIDs)
	}

	q := fmt.Sprintf(`
		SELECT
			n.channel,
			DATE(n.created_at) AS date,
			COUNT(DISTINCT n.id) AS total,
			COUNT(DISTINCT n.id) FILTER (WHERE n.status IN ('sent', 'delivered')) AS sent,
			COUNT(DISTINCT n.id) FILTER (WHERE n.status = 'delivered') AS delivered,
			COUNT(DISTINCT n.id) FILTER (WHERE n.status = 'failed') AS failed,
			COUNT(DISTINCT n.id) FILTER (WHERE n.status = 'bounced') AS bounced,
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
		%s
		GROUP BY n.channel, DATE(n.created_at)
		ORDER BY date DESC, n.channel ASC
	`, where)

	rows, err := h.notifRepo.QuerySummaryArgs(c.Request.Context(), q, args...)
	if err != nil {
		h.log.Error("querying report summary", zap.Error(err))
		respondError(c, http.StatusInternalServerError, "DB_ERROR", "failed to get summary")
		return
	}

	c.JSON(http.StatusOK, rows)
}

// IngressBreakdown handles GET /v1/reports/ingress
func (h *ReportHandler) IngressBreakdown(c *gin.Context) {
	from, to, apiKeyUUID, ok := h.parseRangeAndScope(c)
	if !ok {
		return
	}

	apiKeyUUIDs := []uuid.UUID{} // Not supported for breakdown yet in this helper
	
	var (
		metrics []repository.IngressBreakdownRow
		err     error
	)
	if apiKeyUUID != nil {
		metrics, err = h.notifRepo.GetIngressBreakdown(c.Request.Context(), from, to, apiKeyUUID)
	} else {
		metrics, err = h.notifRepo.GetIngressBreakdownForKeys(c.Request.Context(), from, to, apiKeyUUIDs)
	}
	if err != nil {
		h.log.Error("getting ingress metrics", zap.Error(err))
		respondError(c, http.StatusInternalServerError, "DB_ERROR", "failed to get ingress metrics")
		return
	}

	c.JSON(http.StatusOK, metrics)
}

// SMSCountryBreakdown handles GET /v1/reports/sms-countries
func (h *ReportHandler) SMSCountryBreakdown(c *gin.Context) {
	from, to, apiKeyUUID, ok := h.parseRangeAndScope(c)
	if !ok {
		return
	}

	metrics, err := h.notifRepo.GetSMSCountryBreakdown(c.Request.Context(), from, to, apiKeyUUID)
	if err != nil {
		h.log.Error("getting sms country metrics", zap.Error(err))
		respondError(c, http.StatusInternalServerError, "DB_ERROR", "failed to get sms country metrics")
		return
	}

	c.JSON(http.StatusOK, metrics)
}

// EmailDomainBreakdown handles GET /v1/reports/email-domains
func (h *ReportHandler) EmailDomainBreakdown(c *gin.Context) {
	from, to, apiKeyUUID, ok := h.parseRangeAndScope(c)
	if !ok {
		return
	}

	metrics, err := h.notifRepo.GetEmailDomainBreakdown(c.Request.Context(), from, to, apiKeyUUID)
	if err != nil {
		h.log.Error("getting email domain metrics", zap.Error(err))
		respondError(c, http.StatusInternalServerError, "DB_ERROR", "failed to get email domain metrics")
		return
	}

	c.JSON(http.StatusOK, metrics)
}

// ScheduledStats handles GET /v1/reports/scheduled-stats
func (h *ReportHandler) ScheduledStats(c *gin.Context) {
	from, to, apiKeyUUID, ok := h.parseRangeAndScope(c)
	if !ok {
		return
	}

	stats, err := h.notifRepo.GetScheduledStats(c.Request.Context(), from, to, apiKeyUUID)
	if err != nil {
		h.log.Error("getting scheduled stats", zap.Error(err))
		respondError(c, http.StatusInternalServerError, "DB_ERROR", "failed to get scheduled stats")
		return
	}

	c.JSON(http.StatusOK, stats)
}

func (h *ReportHandler) parseRangeAndScope(c *gin.Context) (time.Time, time.Time, *uuid.UUID, bool) {
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
	apiKeyUUID, _, ok := enforceAPIKeyScopeOrAssigned(c, h.users)
	return from, to, apiKeyUUID, ok
}

// VendorMetrics handles GET /v1/reports/vendors
// Returns real-time metrics for each provider/vendor.
func (h *ReportHandler) VendorMetrics(c *gin.Context) {
	// Look back 12 hours for "real-time" connectivity feel
	metrics, err := h.attemptRepo.GetVendorMetrics(c.Request.Context(), 12*time.Hour)
	if err != nil {
		h.log.Error("failed to get vendor metrics", zap.Error(err))
		respondError(c, http.StatusInternalServerError, "DB_ERROR", "failed to retrieve vendor analytics")
		return
	}

	c.JSON(http.StatusOK, metrics)
}

// VendorBilling handles GET /v1/reports/billing
// Fetches total billing cost per vendor: real data from vendor APIs where available,
// estimated from DB records × published rate for providers with no billing API.
func (h *ReportHandler) VendorBilling(c *gin.Context) {
	ctx := c.Request.Context()

	// Fetch all-time DB send totals for estimated-cost providers.
	sendTotals, err := h.attemptRepo.GetVendorSendTotals(ctx)
	if err != nil {
		h.log.Warn("failed to load vendor send totals", zap.Error(err))
		sendTotals = map[string]int64{}
	}

	// Derive the effective api_key_id scope from the request so that client-scoped
	// vendor configs (saved with an api_key_id) are visible in billing.
	// - admin with no api_key_id query param → nil (global configs)
	// - admin with ?api_key_id=<uuid>         → that client's configs
	// - api_key callers                        → their own key
	// - manager/dev with ?api_key_id=<uuid>   → that client's configs
	var billingAPIKeyID *string
	if v := strings.TrimSpace(c.Query("api_key_id")); v != "" {
		billingAPIKeyID = &v
	} else if callerID := c.GetString("caller_id"); strings.HasPrefix(callerID, "api-key:") {
		raw := strings.TrimPrefix(callerID, "api-key:")
		billingAPIKeyID = &raw
	}

	// Build a fresh merged config: start from the base env-var config, then apply live
	// DB overrides so vendors configured via the UI are always visible.
	//
	// When a specific api_key_id scope is requested, load that client's configs.
	// When no scope is given (admin global view), use LoadPreferredDynamicOverrides so
	// that client-scoped vendors (e.g. mailgun saved under a client's api_key_id) are
	// also reflected — otherwise newly-configured vendors with zero sends stay invisible.
	liveCfg := *h.cfg
	if h.vendorRepo != nil {
		var loadErr error
		if billingAPIKeyID != nil {
			loadErr = liveCfg.LoadDynamicOverridesScoped(ctx, h.vendorRepo, billingAPIKeyID)
		} else {
			loadErr = liveCfg.LoadPreferredDynamicOverrides(ctx, h.vendorRepo)
		}
		if loadErr != nil {
			h.log.Warn("failed to load dynamic vendor config for billing", zap.Error(loadErr))
		}
	}

	// Track which vendor names have been added to avoid duplicates.
	added := map[string]bool{}

	// Build per-vendor fetchers. We call all fetchers concurrently with a shared timeout.
	type namedFetcher struct {
		name    string
		fetcher billing.Fetcher
	}

	fetchers := []namedFetcher{}
	add := func(name string, f billing.Fetcher) {
		if added[name] {
			return
		}
		added[name] = true
		fetchers = append(fetchers, namedFetcher{name, f})
	}

	// SMS vendors
	if liveCfg.Providers.SMS.Twilio.AccountSID != "" {
		add("twilio", billing.NewTwilioFetcher(liveCfg.Providers.SMS.Twilio))
	}
	if liveCfg.Providers.SMS.Plivo.AuthID != "" {
		add("plivo", billing.NewPlivoFetcher(liveCfg.Providers.SMS.Plivo))
	}
	if liveCfg.Providers.SMS.Vonage.APIKey != "" {
		add("vonage", billing.NewVonageFetcher(liveCfg.Providers.SMS.Vonage))
	}
	if liveCfg.Providers.SMS.MessageBird.AccessKey != "" {
		add("messagebird", billing.NewMessageBirdFetcher(liveCfg.Providers.SMS.MessageBird))
	}

	// Email vendors
	if liveCfg.Providers.Email.Mailgun.APIKey != "" {
		add("mailgun", billing.NewMailgunFetcher(liveCfg.Providers.Email.Mailgun, sendTotals["mailgun"]))
	}
	if liveCfg.Providers.Email.SES.AccessKeyID != "" || liveCfg.Providers.Email.SES.AccessSecret != "" {
		add("amazon-ses", billing.NewEstimatedFetcher("amazon-ses", sendTotals["amazon-ses"], 0.0001))
	}
	if liveCfg.Providers.Email.SendGrid.APIKey != "" {
		add("sendgrid", billing.NewEstimatedFetcher("sendgrid", sendTotals["sendgrid"], 0.001))
	}
	if liveCfg.Providers.Email.Postmark.ServerToken != "" {
		add("postmark", billing.NewEstimatedFetcher("postmark", sendTotals["postmark"], 0.0015))
	}

	// Push vendors
	if liveCfg.Providers.Push.FCM.ServiceAccountJSON != "" {
		add("fcm", billing.NewFreeFetcher("fcm"))
	}
	if liveCfg.Providers.Push.OneSignal.AppID != "" {
		add("onesignal", billing.NewFreeFetcher("onesignal"))
	}
	if liveCfg.Providers.Push.Pusher.InstanceID != "" {
		add("pusher", billing.NewFreeFetcher("pusher"))
	}

	// Messaging vendors
	if liveCfg.Providers.Slack.WebhookURL != "" || len(liveCfg.Providers.Slack.Channels) > 0 {
		add("slack", billing.NewFreeFetcher("slack"))
	}

	// Fallback: scan vendor_configs directly and add any active vendor that the config-struct
	// checks above missed (e.g. a client-scoped mailgun with no global counterpart).
	// This runs after LoadPreferredDynamicOverrides so it's a belt-and-suspenders guard.
	if h.vendorRepo != nil {
		if activeVCs, vcErr := h.vendorRepo.ListPreferredActive(ctx); vcErr == nil {
			for _, vc := range activeVCs {
				switch vc.VendorType {
				case "mailgun":
					var s config.MailgunConfig
					if json.Unmarshal(vc.ConfigJSON, &s) == nil && s.APIKey != "" {
						add("mailgun", billing.NewMailgunFetcher(s, sendTotals["mailgun"]))
					}
				case "ses":
					var s config.SESConfig
					if json.Unmarshal(vc.ConfigJSON, &s) == nil && (s.AccessKeyID != "" || s.AccessSecret != "" || s.SMTPUsername != "") {
						add("amazon-ses", billing.NewEstimatedFetcher("amazon-ses", sendTotals["amazon-ses"], 0.0001))
					}
				case "sendgrid":
					var s config.SendGridConfig
					if json.Unmarshal(vc.ConfigJSON, &s) == nil && s.APIKey != "" {
						add("sendgrid", billing.NewEstimatedFetcher("sendgrid", sendTotals["sendgrid"], 0.001))
					}
				case "postmark":
					var s config.PostmarkConfig
					if json.Unmarshal(vc.ConfigJSON, &s) == nil && s.ServerToken != "" {
						add("postmark", billing.NewEstimatedFetcher("postmark", sendTotals["postmark"], 0.0015))
					}
				case "twilio":
					var s config.TwilioConfig
					if json.Unmarshal(vc.ConfigJSON, &s) == nil && s.AccountSID != "" {
						add("twilio", billing.NewTwilioFetcher(s))
					}
				case "plivo":
					var s config.PlivoConfig
					if json.Unmarshal(vc.ConfigJSON, &s) == nil && s.AuthID != "" {
						add("plivo", billing.NewPlivoFetcher(s))
					}
				case "vonage":
					var s config.VonageConfig
					if json.Unmarshal(vc.ConfigJSON, &s) == nil && s.APIKey != "" {
						add("vonage", billing.NewVonageFetcher(s))
					}
				case "messagebird":
					var s config.MessageBirdConfig
					if json.Unmarshal(vc.ConfigJSON, &s) == nil && s.AccessKey != "" {
						add("messagebird", billing.NewMessageBirdFetcher(s))
					}
				case "slack":
					add("slack", billing.NewFreeFetcher("slack"))
				case "fcm", "onesignal", "pusher":
					add(vc.VendorType, billing.NewFreeFetcher(vc.VendorType))
				}
			}
		}
	}

	// Fallback: paid providers seen in DB attempts but not added via any config.
	paidFallbacks := map[string]float64{
		"mailgun":     0.0008,
		"amazon-ses":  0.0001,
		"ses":         0.0001,
		"sendgrid":    0.001,
		"postmark":    0.0015,
		"twilio":      0.0079,
		"plivo":       0.0035,
		"vonage":      0.0065,
		"messagebird": 0.009,
	}
	for name, price := range paidFallbacks {
		if sendTotals[name] > 0 {
			add(name, billing.NewEstimatedFetcher(name, sendTotals[name], price))
		}
	}

	// Free providers seen in DB attempts but not yet added above
	for _, p := range []string{"webhook", "webhooks"} {
		if sendTotals[p] > 0 {
			add("webhook", billing.NewFreeFetcher("webhook"))
		}
	}
	for _, p := range []string{"fcm", "onesignal", "pusher", "slack"} {
		if sendTotals[p] > 0 {
			add(p, billing.NewFreeFetcher(p))
		}
	}

	// Call all fetchers concurrently with a 12-second total timeout.
	fetchCtx, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()

	results := make([]billing.BillingResult, len(fetchers))
	var wg sync.WaitGroup
	for i, f := range fetchers {
		wg.Add(1)
		go func(idx int, nf namedFetcher) {
			defer wg.Done()
			results[idx] = nf.fetcher.Fetch(fetchCtx)
		}(i, f)
	}
	wg.Wait()

	c.JSON(http.StatusOK, results)
}

