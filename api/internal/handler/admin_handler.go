package handler

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/spidey/notification-service/internal/domain"
	"github.com/spidey/notification-service/internal/repository"
	"github.com/spidey/notification-service/internal/service"
	"go.uber.org/zap"
)

type AdminHandler struct {
	configSvc     service.ConfigService
	rateLimitRepo repository.VendorRateLimitRepository
	notifRepo     *repository.NotificationRepository
	engineName    string // temporal | cadence | standalone
	log           *zap.Logger
}

func managerAllowedVendorType(vendorType string) bool {
	switch vendorType {
	// "delivery preference" / routing configs are non-secret and safe for managers to edit.
	case "sms_routing", "email_routing", "push_routing", "webhook_routing", "worker_pool":
		return true
	default:
		return false
	}
}

func NewAdminHandler(configSvc service.ConfigService, rateLimitRepo repository.VendorRateLimitRepository, notifRepo *repository.NotificationRepository, engineName string, log *zap.Logger) *AdminHandler {
	return &AdminHandler{
		configSvc:     configSvc,
		rateLimitRepo: rateLimitRepo,
		notifRepo:     notifRepo,
		engineName:    engineName,
		log:           log,
	}
}

// ── Admin Overview ────────────────────────────────────────────────────────────

// AdminOverviewResponse is the shape returned by GET /v1/admin/overview.
type AdminOverviewResponse struct {
	Delivery     AdminDeliverySection        `json:"delivery"`
	TopClients   []repository.AdminClientRow `json:"top_clients"`
	MTTDPriority []repository.AdminMTTDRow   `json:"mttd_by_priority"`
	MTTDVendor   []repository.AdminMTTDRow   `json:"mttd_by_vendor"`
	Workers      AdminWorkersSection         `json:"workers"`
	Kafka        AdminKafkaSection           `json:"kafka"`
	Cadence      AdminCadenceSection         `json:"cadence"`
}

type AdminDeliverySection struct {
	Window1h  repository.AdminDeliveryStats `json:"window_1h"`
	Window24h repository.AdminDeliveryStats `json:"window_24h"`
}

// AdminWorkersSection now carries both aggregate and per-client worker counts.
type AdminWorkersSection struct {
	Total      int                        `json:"total"`
	ByPriority map[string]int             `json:"by_priority"`
	ByChannel  map[string]int             `json:"by_channel"`
	ByClient   []AdminWorkerClientSummary `json:"by_client"`
}

// AdminWorkerClientSummary mirrors worker.WorkerClientSummary for the API response.
type AdminWorkerClientSummary struct {
	ClientID   string         `json:"client_id"`
	ClientName string         `json:"client_name"`
	Total      int            `json:"total"`
	ByPriority map[string]int `json:"by_priority"`
	ByChannel  map[string]int `json:"by_channel"`
}

type AdminKafkaSection struct {
	TotalLag int64           `json:"total_lag"`
	Topics   []KafkaTopicLag `json:"topics"`
}

type KafkaTopicLag struct {
	Topic    string `json:"topic"`
	Channel  string `json:"channel"`
	Priority string `json:"priority"`
	Lag      int64  `json:"lag"`
}

// AdminCadenceSection holds workflow orchestration cadence metrics.
type AdminCadenceSection struct {
	Engine          string                           `json:"engine"` // temporal | cadence | standalone
	Throughput      []repository.CadenceThroughputRow `json:"throughput"`
	Retries         []repository.CadenceRetryRow      `json:"retries"`
	ScheduleToStart []repository.CadenceScheduleRow   `json:"schedule_to_start"`
}

// GetAdminOverview returns a comprehensive system overview for the admin dashboard.
func (h *AdminHandler) GetAdminOverview(c *gin.Context) {
	ctx := c.Request.Context()
	now := time.Now()

	resp := AdminOverviewResponse{
		TopClients:   []repository.AdminClientRow{},
		MTTDPriority: []repository.AdminMTTDRow{},
		MTTDVendor:   []repository.AdminMTTDRow{},
		Workers: AdminWorkersSection{
			ByPriority: map[string]int{},
			ByChannel:  map[string]int{},
			ByClient:   []AdminWorkerClientSummary{},
		},
		Kafka: AdminKafkaSection{Topics: []KafkaTopicLag{}},
		Cadence: AdminCadenceSection{
			Engine:          h.engineName,
			Throughput:      []repository.CadenceThroughputRow{},
			Retries:         []repository.CadenceRetryRow{},
			ScheduleToStart: []repository.CadenceScheduleRow{},
		},
	}

	// DB: delivery stats
	if s, err := h.notifRepo.GetDeliveryStats(ctx, now.Add(-1*time.Hour)); err == nil {
		resp.Delivery.Window1h = s
	}
	if s, err := h.notifRepo.GetDeliveryStats(ctx, now.Add(-24*time.Hour)); err == nil {
		resp.Delivery.Window24h = s
	}

	// DB: top clients (last 24h, top 10)
	if rows, err := h.notifRepo.GetTopClients(ctx, now.Add(-24*time.Hour), 10); err == nil {
		resp.TopClients = rows
	} else {
		h.log.Warn("admin overview: top clients query failed", zap.Error(err))
	}

	// DB: MTTD by priority and vendor (last 1h)
	if rows, err := h.notifRepo.GetMTTDByPriority(ctx, now.Add(-1*time.Hour)); err == nil {
		resp.MTTDPriority = rows
	}
	if rows, err := h.notifRepo.GetMTTDByVendor(ctx, now.Add(-1*time.Hour)); err == nil {
		resp.MTTDVendor = rows
	}

	// DB: Cadence / workflow orchestration metrics (last 1h)
	if rows, err := h.notifRepo.GetWorkflowThroughput(ctx, now.Add(-1*time.Hour)); err == nil {
		resp.Cadence.Throughput = rows
	} else {
		h.log.Debug("admin overview: workflow throughput query failed", zap.Error(err))
	}
	if rows, err := h.notifRepo.GetRetryStats(ctx, now.Add(-1*time.Hour)); err == nil {
		resp.Cadence.Retries = rows
	} else {
		h.log.Debug("admin overview: retry stats query failed", zap.Error(err))
	}
	if rows, err := h.notifRepo.GetScheduleToStartLatency(ctx, now.Add(-1*time.Hour)); err == nil {
		resp.Cadence.ScheduleToStart = rows
	} else {
		h.log.Debug("admin overview: schedule-to-start query failed", zap.Error(err))
	}

	// Worker service: fetch live worker counts from the worker process HTTP server.
	if ws, err := fetchWorkerState(workerServiceBase()); err == nil {
		resp.Workers = adminWorkerSectionFrom(ws)
	} else {
		h.log.Debug("admin overview: worker state fetch skipped", zap.Error(err))
	}

	c.JSON(http.StatusOK, resp)
}

// workerServiceBase returns the worker internal HTTP URL from env or a sensible default.
func workerServiceBase() string {
	if v := os.Getenv("NS_WORKER_INTERNAL_URL"); v != "" {
		return v
	}
	return "http://localhost:8081"
}

// workerStateResponse mirrors worker.WorkerState for JSON unmarshalling.
type workerStateResponse struct {
	Total      int `json:"total"`
	ByPriority map[string]int `json:"by_priority"`
	ByChannel  map[string]int `json:"by_channel"`
	ByClient   []struct {
		ClientID   string         `json:"client_id"`
		ClientName string         `json:"client_name"`
		Total      int            `json:"total"`
		ByPriority map[string]int `json:"by_priority"`
		ByChannel  map[string]int `json:"by_channel"`
	} `json:"by_client"`
}

func fetchWorkerState(base string) (workerStateResponse, error) {
	var ws workerStateResponse
	resp, err := http.Get(base + "/workers") //nolint:noctx
	if err != nil {
		return ws, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return ws, err
	}
	if err := json.Unmarshal(body, &ws); err != nil {
		return ws, err
	}
	return ws, nil
}

func adminWorkerSectionFrom(ws workerStateResponse) AdminWorkersSection {
	byPriority := ws.ByPriority
	if byPriority == nil {
		byPriority = map[string]int{}
	}
	byChannel := ws.ByChannel
	if byChannel == nil {
		byChannel = map[string]int{}
	}
	clients := make([]AdminWorkerClientSummary, 0, len(ws.ByClient))
	for _, c := range ws.ByClient {
		bp := c.ByPriority
		if bp == nil {
			bp = map[string]int{}
		}
		bc := c.ByChannel
		if bc == nil {
			bc = map[string]int{}
		}
		clients = append(clients, AdminWorkerClientSummary{
			ClientID:   c.ClientID,
			ClientName: c.ClientName,
			Total:      c.Total,
			ByPriority: bp,
			ByChannel:  bc,
		})
	}
	return AdminWorkersSection{
		Total:      ws.Total,
		ByPriority: byPriority,
		ByChannel:  byChannel,
		ByClient:   clients,
	}
}

// GetVendorRateLimits returns all active vendor rate limit configurations.
func (h *AdminHandler) GetVendorRateLimits(c *gin.Context) {
	var apiKeyID *string
	if v := c.GetString("scoped_api_key_id"); v != "" {
		apiKeyID = &v
	} else if v := c.Query("api_key_id"); v != "" {
		apiKeyID = &v
	}
	limits, err := h.rateLimitRepo.ListActive(c.Request.Context(), apiKeyID)
	if err != nil {
		h.log.Error("failed to list vendor rate limits", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to retrieve rate limits"})
		return
	}
	if limits == nil {
		limits = []*domain.VendorRateLimit{}
	}
	c.JSON(http.StatusOK, limits)
}

// UpsertVendorRateLimit creates or updates the rate limit for a specific vendor.
// Supports RPS, per_minute, per_10_min, per_hour, per_day — all optional, all enforced simultaneously.
func (h *AdminHandler) UpsertVendorRateLimit(c *gin.Context) {
	vendorName := c.Param("vendor_name")
	if vendorName == "" {
		respondError(c, http.StatusBadRequest, "BAD_REQUEST", "vendor_name path param required")
		return
	}

	var req domain.UpsertVendorRateLimitRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var apiKeyID *string
	if v := c.GetString("scoped_api_key_id"); v != "" {
		apiKeyID = &v
	} else if v := c.Query("api_key_id"); v != "" {
		apiKeyID = &v
	}

	rl := &domain.VendorRateLimit{
		ID:         uuid.New(),
		VendorName: vendorName,
		RPS:        req.RPS,
		PerMinute:  req.PerMinute,
		Per10Min:   req.Per10Min,
		PerHour:    req.PerHour,
		PerDay:     req.PerDay,
		IsActive:   true,
	}

	if err := h.rateLimitRepo.Upsert(c.Request.Context(), rl, apiKeyID); err != nil {
		h.log.Error("failed to upsert vendor rate limit", zap.String("vendor", vendorName), zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save rate limit"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "rate limit updated", "vendor": vendorName})
}

// DeleteVendorRateLimit removes the rate limit for a specific vendor.
func (h *AdminHandler) DeleteVendorRateLimit(c *gin.Context) {
	vendorName := c.Param("vendor_name")
	if vendorName == "" {
		respondError(c, http.StatusBadRequest, "BAD_REQUEST", "vendor_name path param required")
		return
	}

	var apiKeyID *string
	if v := c.GetString("scoped_api_key_id"); v != "" {
		apiKeyID = &v
	} else if v := c.Query("api_key_id"); v != "" {
		apiKeyID = &v
	}

	if err := h.rateLimitRepo.Delete(c.Request.Context(), vendorName, apiKeyID); err != nil {
		h.log.Error("failed to delete vendor rate limit", zap.String("vendor", vendorName), zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete rate limit"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "rate limit removed", "vendor": vendorName})
}

// GetVendorConfigs returns all dynamic vendor configurations.
func (h *AdminHandler) GetVendorConfigs(c *gin.Context) {
	role, _ := getRoleAndSubject(c)
	var apiKeyID *string
	if v := c.GetString("scoped_api_key_id"); v != "" {
		apiKeyID = &v
	} else if v := c.Query("api_key_id"); v != "" {
		apiKeyID = &v
	}
	configs, err := h.configSvc.GetVendorConfigs(c.Request.Context(), apiKeyID)
	if err != nil {
		h.log.Error("failed to get vendor configs", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to retrieve configurations"})
		return
	}
	// Managers can only view routing/delivery preference configs.
	if role == "manager" {
		filtered := make([]*domain.VendorConfig, 0, len(configs))
		for _, cfg := range configs {
			if cfg != nil && managerAllowedVendorType(cfg.VendorType) {
				filtered = append(filtered, cfg)
			}
		}
		c.JSON(http.StatusOK, filtered)
		return
	}
	c.JSON(http.StatusOK, configs)
}

// UpdateVendorConfig updates or creates a dynamic vendor configuration.
func (h *AdminHandler) UpdateVendorConfig(c *gin.Context) {
	vendorType := c.Param("vendor_type")
	role, _ := getRoleAndSubject(c)
	if role == "manager" && !managerAllowedVendorType(vendorType) {
		respondError(c, http.StatusForbidden, "FORBIDDEN", "managers can only edit delivery preference routing configs")
		return
	}
	var apiKeyID *string
	if v := c.GetString("scoped_api_key_id"); v != "" {
		apiKeyID = &v
	} else if v := c.Query("api_key_id"); v != "" {
		apiKeyID = &v
	}
	var req struct {
		Config json.RawMessage `json:"config" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	err := h.configSvc.UpdateVendorConfig(c.Request.Context(), vendorType, req.Config, apiKeyID)
	if err != nil {
		h.log.Error("failed to update vendor config", zap.Error(err), zap.String("vendor", vendorType))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update configuration"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "configuration updated successfully"})
}

// DeleteVendorConfig deactivates a dynamic vendor configuration (soft delete).
// Only admin/manager routes can reach this handler (middleware enforced in router).
func (h *AdminHandler) DeleteVendorConfig(c *gin.Context) {
	vendorType := c.Param("vendor_type")
	role, _ := getRoleAndSubject(c)
	if role == "manager" {
		respondError(c, http.StatusForbidden, "FORBIDDEN", "managers cannot delete vendor configs")
		return
	}
	var apiKeyID *string
	if v := c.GetString("scoped_api_key_id"); v != "" {
		apiKeyID = &v
	} else if v := c.Query("api_key_id"); v != "" {
		apiKeyID = &v
	}

	err := h.configSvc.DeleteVendorConfig(c.Request.Context(), vendorType, apiKeyID)
	if err != nil {
		h.log.Error("failed to delete vendor config", zap.Error(err), zap.String("vendor", vendorType))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete configuration"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "configuration removed successfully"})
}
