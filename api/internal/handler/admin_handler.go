package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/segmentio/kafka-go"
	"github.com/spidey/notification-service/internal/config"
	"github.com/spidey/notification-service/internal/domain"
	"github.com/spidey/notification-service/internal/pubsub"
	"github.com/spidey/notification-service/internal/repository"
	"github.com/spidey/notification-service/internal/service"
	"go.uber.org/zap"
) 

type AdminHandler struct {
	configSvc          service.ConfigService
	rateLimitRepo      repository.VendorRateLimitRepository
	retryConfigRepo    repository.VendorRetryConfigRepository
	notifRepo          *repository.NotificationRepository
	dlqRepo            *repository.DLQRepository
	engineName         string // temporal | cadence | standalone | go_routines
	pubsubMode         string // kafka | redis | mock | gcp
	kafkaBrokers       []string
	log                *zap.Logger
	migrationMgr       *service.MigrationManager
	vendorMigrationSvc service.VendorMigrationService
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

func NewAdminHandler(
	configSvc service.ConfigService,
	rateLimitRepo repository.VendorRateLimitRepository,
	notifRepo *repository.NotificationRepository,
	engineName string,
	pubsubMode string,
	kafkaBrokers []string,
	log *zap.Logger,
) *AdminHandler {
	return &AdminHandler{
		configSvc:     configSvc,
		rateLimitRepo: rateLimitRepo,
		notifRepo:     notifRepo,
		engineName:    engineName,
		pubsubMode:    pubsubMode,
		kafkaBrokers:  kafkaBrokers,
		log:           log,
	}
}

// WithRetryConfigRepository wires the retry config repository for retry config endpoints.
func (h *AdminHandler) WithRetryConfigRepository(retryRepo repository.VendorRetryConfigRepository) *AdminHandler {
	h.retryConfigRepo = retryRepo
	return h
}

// WithMigrationManager wires the MigrationManager for migration status reporting and control.
func (h *AdminHandler) WithMigrationManager(mgr *service.MigrationManager) *AdminHandler {
	h.migrationMgr = mgr
	return h
}

// WithVendorMigrationService wires the VendorMigrationService for vendor-swap migrations.
func (h *AdminHandler) WithVendorMigrationService(svc service.VendorMigrationService) *AdminHandler {
	h.vendorMigrationSvc = svc
	return h
}

// WithDLQRepository wires the DLQ repository for dead-letter queue stats in the admin overview.
func (h *AdminHandler) WithDLQRepository(dlqRepo *repository.DLQRepository) *AdminHandler {
	h.dlqRepo = dlqRepo
	return h
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
	Migrations   AdminMigrationSection       `json:"migrations"`
	DLQ          AdminDLQSection             `json:"dlq"`
}

// AdminDLQSection holds dead-letter queue statistics for the admin dashboard.
type AdminDLQSection struct {
	TotalEntries  int `json:"total_entries"`
	PendingReplay int `json:"pending_replay"`
	Replayed      int `json:"replayed"`
}

type AdminDeliverySection struct {
	// Window is the user-selected window from the request (defaults to 1h).
	Window      repository.AdminDeliveryStats `json:"window"`
	WindowLabel string                       `json:"window_label"`
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
	Enabled     bool            `json:"enabled"`
	Mode        string          `json:"mode"`
	TotalLag    int64           `json:"total_lag"`
	TotalEvents int64           `json:"total_events"`
	Topics      []KafkaTopicLag `json:"topics"`
}

type KafkaTopicLag struct {
	Topic       string `json:"topic"`
	Channel     string `json:"channel"`
	Priority    string `json:"priority"`
	Lag         int64  `json:"lag"`
	TotalEvents int64  `json:"total_events"`
}

// AdminCadenceSection holds workflow orchestration cadence metrics.
type AdminCadenceSection struct {
	Engine          string                           `json:"engine"` // temporal | cadence | standalone
	Throughput      []repository.CadenceThroughputRow `json:"throughput"`
	Retries         []repository.CadenceRetryRow      `json:"retries"`
	ScheduleToStart []repository.CadenceScheduleRow   `json:"schedule_to_start"`
}

// AdminMigrationSection holds the state of active orchestration migrations.
type AdminMigrationSection struct {
	Active          int                              `json:"active"`
	Migrations      []*domain.OrchestrationMigration `json:"migrations"`
	OldWorkerTotal  int                              `json:"old_worker_total"`
	NewWorkerTotal  int                              `json:"new_worker_total"`
	OldByPriority   map[string]int                   `json:"old_by_priority"`
	OldByChannel    map[string]int                   `json:"old_by_channel"`
	OldByClient     []AdminWorkerClientSummary        `json:"old_by_client"`
	NewByPriority   map[string]int                   `json:"new_by_priority"`
	NewByChannel    map[string]int                   `json:"new_by_channel"`
	NewByClient     []AdminWorkerClientSummary        `json:"new_by_client"`
	DryRunResult    *domain.MigrationDryRunResult     `json:"dry_run_result,omitempty"`
}

// GetAdminOverview returns a comprehensive system overview for the admin dashboard.
func (h *AdminHandler) GetAdminOverview(c *gin.Context) {
	ctx := c.Request.Context()
	now := time.Now()
	windowKey := strings.TrimSpace(c.Query("window"))
	windowDur, windowLabel := parseAdminWindow(windowKey)
	since := now.Add(-windowDur)

	resp := AdminOverviewResponse{
		TopClients:   []repository.AdminClientRow{},
		MTTDPriority: []repository.AdminMTTDRow{},
		MTTDVendor:   []repository.AdminMTTDRow{},
		Workers: AdminWorkersSection{
			ByPriority: map[string]int{},
			ByChannel:  map[string]int{},
			ByClient:   []AdminWorkerClientSummary{},
		},
		Kafka: AdminKafkaSection{Enabled: false, Mode: h.pubsubMode, Topics: []KafkaTopicLag{}},
		Cadence: AdminCadenceSection{
			Engine:          h.engineName,
			Throughput:      []repository.CadenceThroughputRow{},
			Retries:         []repository.CadenceRetryRow{},
			ScheduleToStart: []repository.CadenceScheduleRow{},
		},
	}

	// DB: delivery stats
	if s, err := h.notifRepo.GetDeliveryStats(ctx, since); err == nil {
		resp.Delivery.Window = s
		resp.Delivery.WindowLabel = windowLabel
	}
	// Back-compat: keep last-1h populated (used by older UIs).
	if s, err := h.notifRepo.GetDeliveryStats(ctx, now.Add(-1*time.Hour)); err == nil {
		resp.Delivery.Window1h = s
	}
	if s, err := h.notifRepo.GetDeliveryStats(ctx, now.Add(-24*time.Hour)); err == nil {
		resp.Delivery.Window24h = s
	}

	// DB: top clients (last 24h, top 10)
	if rows, err := h.notifRepo.GetTopClients(ctx, since, 10); err == nil {
		resp.TopClients = rows
	} else {
		h.log.Warn("admin overview: top clients query failed", zap.Error(err))
	}

	// DB: MTTD by priority and vendor (last 1h)
	if rows, err := h.notifRepo.GetMTTDByPriority(ctx, since); err == nil {
		resp.MTTDPriority = rows
	}
	if rows, err := h.notifRepo.GetMTTDByVendor(ctx, since); err == nil {
		resp.MTTDVendor = rows
	}

	// DB: Cadence / workflow orchestration metrics (last 1h)
	if rows, err := h.notifRepo.GetWorkflowThroughput(ctx, since); err == nil {
		resp.Cadence.Throughput = rows
	} else {
		h.log.Debug("admin overview: workflow throughput query failed", zap.Error(err))
	}
	if rows, err := h.notifRepo.GetRetryStats(ctx, since); err == nil {
		resp.Cadence.Retries = rows
	} else {
		h.log.Debug("admin overview: retry stats query failed", zap.Error(err))
	}
	if rows, err := h.notifRepo.GetScheduleToStartLatency(ctx, since); err == nil {
		resp.Cadence.ScheduleToStart = rows
	} else {
		h.log.Debug("admin overview: schedule-to-start query failed", zap.Error(err))
	}

	// Migration status: fetch active migrations and old/new worker breakdowns.
	resp.Migrations = AdminMigrationSection{
		OldByPriority: make(map[string]int),
		OldByChannel:  make(map[string]int),
		OldByClient:   []AdminWorkerClientSummary{},
		NewByPriority: make(map[string]int),
		NewByChannel:  make(map[string]int),
		NewByClient:   []AdminWorkerClientSummary{},
	}
	if h.migrationMgr != nil {
		activeMigs, err := h.migrationMgr.ListMigrations(ctx, true)
		if err == nil {
			resp.Migrations.Active = len(activeMigs)
			resp.Migrations.Migrations = activeMigs
		}
	}

	// Fetch migration worker breakdown from worker process.
	if mws, err := fetchMigrationWorkerState(workerServiceBase()); err == nil {
		resp.Migrations.OldWorkerTotal = mws.OldWorkerTotal
		resp.Migrations.NewWorkerTotal = mws.NewWorkerTotal
		if mws.OldByPriority != nil {
			resp.Migrations.OldByPriority = mws.OldByPriority
		}
		if mws.OldByChannel != nil {
			resp.Migrations.OldByChannel = mws.OldByChannel
		}
		if mws.NewByPriority != nil {
			resp.Migrations.NewByPriority = mws.NewByPriority
		}
		if mws.NewByChannel != nil {
			resp.Migrations.NewByChannel = mws.NewByChannel
		}
		for _, c := range mws.OldByClient {
			resp.Migrations.OldByClient = append(resp.Migrations.OldByClient, AdminWorkerClientSummary{
				ClientID:   c.ClientID,
				ClientName: c.ClientName,
				Total:      c.Total,
				ByPriority: c.ByPriority,
				ByChannel:  c.ByChannel,
			})
		}
		for _, c := range mws.NewByClient {
			resp.Migrations.NewByClient = append(resp.Migrations.NewByClient, AdminWorkerClientSummary{
				ClientID:   c.ClientID,
				ClientName: c.ClientName,
				Total:      c.Total,
				ByPriority: c.ByPriority,
				ByChannel:  c.ByChannel,
			})
		}
	} else {
		h.log.Debug("admin overview: migration worker state fetch skipped", zap.Error(err))
	}

	// Dry-run migration for the current client scope (from query param).
	if apiKeyID := c.Query("api_key_id"); apiKeyID != "" && h.migrationMgr != nil {
		result, err := h.migrationMgr.DryRunMigration(ctx, &apiKeyID)
		if err == nil {
			resp.Migrations.DryRunResult = result
		}
	}
	// Worker service: fetch live worker counts from the worker process HTTP server.
	if ws, err := fetchWorkerState(workerServiceBase()); err == nil {
		resp.Workers = adminWorkerSectionFrom(ws)
	} else {
		h.log.Debug("admin overview: worker state fetch skipped", zap.Error(err))
	}

	// Kafka: consumer group lag (unprocessed messages).
	// Queries Redpanda for each consumer group's committed offset and compares
	// it against the partition log-end-offset to report true lag, not topic depth.
	if strings.EqualFold(h.pubsubMode, "kafka") && len(h.kafkaBrokers) > 0 {
		resp.Kafka.Enabled = true
		topics := make([]KafkaTopicLag, 0, 32)
		var totalLag, totalEvents int64
		for key, topicName := range pubsub.TopicID {
			// Only include priority topics like "email-high" and skip internal topics.
			if key == "config" || key == "dlq" {
				continue
			}
			parts := strings.Split(key, "-")
			if len(parts) < 2 {
				continue
			}
			priority := parts[len(parts)-1]
			channel := strings.Join(parts[:len(parts)-1], "-")
			if priority != "high" && priority != "medium" && priority != "low" {
				continue
			}

			// Consumer group naming: notif-dispatcher-{channel}-{priority}-{channel}-{priority}
			groupID := fmt.Sprintf("notif-dispatcher-%s-%s-%s-%s", channel, priority, channel, priority)
			lag, evts, err := kafkaConsumerGroupLag(ctx, h.kafkaBrokers, topicName, groupID)
			if err != nil {
				h.log.Debug("admin overview: kafka consumer group lag fetch failed",
					zap.String("topic", topicName),
					zap.String("group", groupID),
					zap.Error(err),
				)
				// Fallback: report topic depth when consumer group query fails,
				// so the UI still shows something meaningful.
				depth, evts2, err2 := kafkaTopicDepth(ctx, h.kafkaBrokers, topicName)
				if err2 != nil {
					continue
				}
				totalLag += depth
				totalEvents += evts2
				topics = append(topics, KafkaTopicLag{
					Topic:       topicName,
					Channel:     channel,
					Priority:    priority,
					Lag:         depth,
					TotalEvents: evts2,
				})
				continue
			}
			totalLag += lag
			totalEvents += evts
			topics = append(topics, KafkaTopicLag{
				Topic:       topicName,
				Channel:     channel,
				Priority:    priority,
				Lag:         lag,
				TotalEvents: evts,
			})
		}
		resp.Kafka.Topics = topics
		resp.Kafka.TotalLag = totalLag
		resp.Kafka.TotalEvents = totalEvents
	}

	// Dead-letter queue stats
	if h.dlqRepo != nil {
		_, totalEntries, err := h.dlqRepo.List(ctx, 1, 1, false)
		if err == nil {
			resp.DLQ.TotalEntries = totalEntries
		}
		_, pendingReplay, err := h.dlqRepo.List(ctx, 1, 1, true)
		if err == nil {
			resp.DLQ.PendingReplay = pendingReplay
		}
		resp.DLQ.Replayed = resp.DLQ.TotalEntries - resp.DLQ.PendingReplay
	}

	c.JSON(http.StatusOK, resp)
}

func parseAdminWindow(key string) (time.Duration, string) {
	switch strings.ToLower(strings.TrimSpace(key)) {
	case "1h", "":
		return 1 * time.Hour, "Last 1 hour"
	case "3h":
		return 3 * time.Hour, "Last 3 hours"
	case "6h":
		return 6 * time.Hour, "Last 6 hours"
	case "12h":
		return 12 * time.Hour, "Last 12 hours"
	case "1d":
		return 24 * time.Hour, "Last 1 day"
	case "1w":
		return 7 * 24 * time.Hour, "Last 1 week"
	case "1mo":
		return 30 * 24 * time.Hour, "Last 1 month"
	case "3mo":
		return 90 * 24 * time.Hour, "Last 3 months"
	default:
		// Fail closed to a safe default rather than erroring the whole dashboard.
		return 1 * time.Hour, "Last 1 hour"
	}
}

// kafkaTopicDepth returns (depth, totalEvents, error).
// depth = messages currently in the queue (last - first offsets).
// totalEvents = high watermark i.e. total messages ever published (sum of last offsets).
// This is used as a fallback when consumer group lag queries fail.
func kafkaTopicDepth(ctx context.Context, brokers []string, topic string) (int64, int64, error) {
	// Use a short timeout so the admin overview doesn't hang.
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	// Discover partitions from any broker.
	conn, err := kafka.DialContext(ctx, "tcp", brokers[0])
	if err != nil {
		return 0, 0, err
	}
	defer conn.Close()

	partitions, err := conn.ReadPartitions(topic)
	if err != nil {
		return 0, 0, err
	}
	var depth, total int64
	for _, p := range partitions {
		leaderAddr := p.Leader.Host + ":" + fmt.Sprintf("%d", p.Leader.Port)
		pc, err := kafka.DialLeader(ctx, "tcp", leaderAddr, topic, p.ID)
		if err != nil {
			// Best-effort: skip partition if leader dial fails.
			continue
		}
		first, err1 := pc.ReadFirstOffset()
		last, err2 := pc.ReadLastOffset()
		_ = pc.Close()
		if err1 != nil || err2 != nil {
			continue
		}
		if last > first {
			depth += (last - first)
		}
		if last > 0 {
			total += last
		}
	}
	return depth, total, nil
}

// kafkaConsumerGroupLag returns (lag, totalEvents, error).
// lag = unprocessed messages (log_end_offset - committed_offset) for the consumer group.
// totalEvents = log-end-offset (total messages ever published to the topic).
func kafkaConsumerGroupLag(ctx context.Context, brokers []string, topic, groupID string) (int64, int64, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	// Step 1: Find the coordinator for this consumer group.
	coordAddr, err := kafkaCoordinator(ctx, brokers[0], groupID)
	if err != nil {
		return 0, 0, fmt.Errorf("find coordinator: %w", err)
	}

	// Step 2: Get committed offsets from the coordinator.
	committedOffsets, err := kafkaCommittedOffsets(ctx, coordAddr, groupID, topic)
	if err != nil {
		return 0, 0, fmt.Errorf("fetch committed offsets: %w", err)
	}

	// Step 3: Dial partition leaders to get log-end offsets.
	conn, err := kafka.DialContext(ctx, "tcp", brokers[0])
	if err != nil {
		return 0, 0, err
	}
	defer conn.Close()

	partitions, err := conn.ReadPartitions(topic)
	if err != nil {
		return 0, 0, err
	}

	var totalLag, totalEvents int64
	for _, p := range partitions {
		leaderAddr := p.Leader.Host + ":" + fmt.Sprintf("%d", p.Leader.Port)
		pc, err := kafka.DialLeader(ctx, "tcp", leaderAddr, topic, p.ID)
		if err != nil {
			continue
		}
		last, err2 := pc.ReadLastOffset()
		_ = pc.Close()
		if err2 != nil {
			continue
		}

		if last > 0 {
			totalEvents += last
		}

		// Get committed offset for this partition (-1 means no commit = start from latest).
		committed, exists := committedOffsets[p.ID]
		if !exists || committed == -1 {
			// Nothing committed yet — no lag if nobody has consumed, or consumer
			// hasn't committed. Treat as 0 lag (fresh group at end).
			continue
		}

		if last > committed {
			totalLag += (last - committed)
		}
	}

	return totalLag, totalEvents, nil
}

// kafkaCoordinator finds the coordinator broker address for a consumer group.
func kafkaCoordinator(ctx context.Context, brokerAddr, groupID string) (string, error) {
	client := &kafka.Client{
		Addr: kafka.TCP(brokerAddr),
	}
	resp, err := client.FindCoordinator(ctx, &kafka.FindCoordinatorRequest{
		Addr:    kafka.TCP(brokerAddr),
		Key:     groupID,
		KeyType: kafka.CoordinatorKeyTypeConsumer,
	})
	if err != nil {
		return "", err
	}
	if resp.Error != nil {
		return "", resp.Error
	}
	return fmt.Sprintf("%s:%d", resp.Coordinator.Host, resp.Coordinator.Port), nil
}

// kafkaCommittedOffsets fetches the committed offsets for all partitions of a topic
// within the given consumer group. Returns a map of partitionID → committedOffset.
func kafkaCommittedOffsets(ctx context.Context, coordinatorAddr, groupID, topic string) (map[int]int64, error) {
	client := &kafka.Client{
		Addr: kafka.TCP(coordinatorAddr),
	}
	resp, err := client.OffsetFetch(ctx, &kafka.OffsetFetchRequest{
		Addr:    kafka.TCP(coordinatorAddr),
		GroupID: groupID,
		Topics:  map[string][]int{topic: {}},
	})
	if err != nil {
		return nil, err
	}
	if resp.Error != nil {
		return nil, resp.Error
	}
	offsets := make(map[int]int64, len(resp.Topics[topic]))
	for _, p := range resp.Topics[topic] {
		if p.Error == nil {
			offsets[p.Partition] = p.CommittedOffset
		}
	}
	return offsets, nil
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

// migrationWorkerStateResponse mirrors worker.MigrationWorkerState for JSON unmarshalling.
type migrationWorkerStateResponse struct {
	OldWorkerTotal     int                   `json:"old_worker_total"`
	OldByPriority      map[string]int        `json:"old_by_priority"`
	OldByChannel       map[string]int        `json:"old_by_channel"`
	OldByClient        []workerClientSummary `json:"old_by_client"`
	NewWorkerTotal     int                   `json:"new_worker_total"`
	NewByPriority      map[string]int        `json:"new_by_priority"`
	NewByChannel       map[string]int        `json:"new_by_channel"`
	NewByClient        []workerClientSummary `json:"new_by_client"`
	ActiveMigrationIDs []string              `json:"active_migration_ids"`
}

type workerClientSummary struct {
	ClientID   string         `json:"client_id"`
	ClientName string         `json:"client_name"`
	Total      int            `json:"total"`
	ByPriority map[string]int `json:"by_priority"`
	ByChannel  map[string]int `json:"by_channel"`
}

func fetchMigrationWorkerState(base string) (migrationWorkerStateResponse, error) {
	var ws migrationWorkerStateResponse
	resp, err := http.Get(base + "/workers/migration") //nolint:noctx
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
	// Filter out internal vendor types that should not be exposed to the UI.
	// worker_pool and autoscaler are internal configurations.
	filtered := make([]*domain.VendorConfig, 0, len(configs))
	for _, cfg := range configs {
		if cfg != nil && cfg.VendorType != "worker_pool" && cfg.VendorType != "autoscaler" {
			filtered = append(filtered, cfg)
		}
	}
	// Managers can only view routing/delivery preference configs.
	if role == "manager" {
		managerFiltered := make([]*domain.VendorConfig, 0, len(filtered))
		for _, cfg := range filtered {
			if cfg != nil && managerAllowedVendorType(cfg.VendorType) {
				managerFiltered = append(managerFiltered, cfg)
			}
		}
		c.JSON(http.StatusOK, managerFiltered)
		return
	}
	c.JSON(http.StatusOK, filtered)
}

// GetApiDocsVisibility returns the api_docs_visibility configuration (public).
func (h *AdminHandler) GetApiDocsVisibility(c *gin.Context) {
	configs, err := h.configSvc.GetVendorConfigs(c.Request.Context(), nil)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"hidden_endpoints": []string{}})
		return
	}
	for _, cfg := range configs {
		if cfg.VendorType == "api_docs_visibility" {
			// Ensure it returns an object even if empty
			c.Data(http.StatusOK, "application/json", cfg.ConfigJSON)
			return
		}
	}
	c.JSON(http.StatusOK, gin.H{"hidden_endpoints": []string{}})
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

// ── AutoScaler Config Endpoints ────────────────────────────────────────────────

// autoscalerConfigResponse converts AutoScalerConfig to string-based JSON for the frontend.
type autoscalerConfigResponse struct {
	Enabled             bool    `json:"enabled"`
	EvaluationInterval  string  `json:"evaluation_interval"`
	HighMTTDThreshold   string  `json:"high_mttd_threshold"`
	MediumMTTDThreshold string `json:"medium_mttd_threshold"`
	LowMTTDThreshold    string  `json:"low_mttd_threshold"`
	MaxLagPerWorker     int     `json:"max_lag_per_worker"`
	CooldownPeriod      string  `json:"cooldown_period"`
	ScaleDownFactor     float64 `json:"scale_down_factor"`
	IdleThreshold       string  `json:"idle_threshold"`
	UnhealthyThreshold  int     `json:"unhealthy_threshold"`
	GlobalMaxWorkers    int     `json:"global_max_workers"`
	GlobalMinWorkers    int     `json:"global_min_workers"`
	MTTDLookback        string  `json:"mttd_lookback"`
}

func toConfigResponse(cfg *config.AutoScalerConfig) autoscalerConfigResponse {
	return autoscalerConfigResponse{
		Enabled:              cfg.Enabled,
		EvaluationInterval:   cfg.EvaluationInterval.String(),
		HighMTTDThreshold:    cfg.HighMTTDThreshold.String(),
		MediumMTTDThreshold:  cfg.MediumMTTDThreshold.String(),
		LowMTTDThreshold:     cfg.LowMTTDThreshold.String(),
		MaxLagPerWorker:      cfg.MaxLagPerWorker,
		CooldownPeriod:       cfg.CooldownPeriod.String(),
		ScaleDownFactor:      cfg.ScaleDownFactor,
		IdleThreshold:        cfg.IdleThreshold.String(),
		UnhealthyThreshold:   cfg.UnhealthyThreshold,
		GlobalMaxWorkers:     cfg.GlobalMaxWorkers,
		GlobalMinWorkers:     cfg.GlobalMinWorkers,
		MTTDLookback:         cfg.MTTDLookback.String(),
	}
}

// GetAutoScalerConfig returns the current autoscaler configuration (string-based durations).
func (h *AdminHandler) GetAutoScalerConfig(c *gin.Context) {
	cfg, err := h.configSvc.GetAutoScalerConfig(c.Request.Context())
	if err != nil {
		h.log.Error("failed to get autoscaler config", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get autoscaler config"})
		return
	}
	c.JSON(http.StatusOK, toConfigResponse(cfg))
}

// UpdateAutoScalerConfig updates the autoscaler configuration at runtime.
func (h *AdminHandler) UpdateAutoScalerConfig(c *gin.Context) {
	var cfg struct {
		Enabled            *bool    `json:"enabled"`
		EvaluationInterval *string  `json:"evaluation_interval"`
		HighMTTDThreshold  *string  `json:"high_mttd_threshold"`
		MediumMTTDThreshold *string `json:"medium_mttd_threshold"`
		LowMTTDThreshold   *string  `json:"low_mttd_threshold"`
		MaxLagPerWorker    *int     `json:"max_lag_per_worker"`
		CooldownPeriod     *string  `json:"cooldown_period"`
		ScaleDownFactor    *float64 `json:"scale_down_factor"`
		IdleThreshold      *string  `json:"idle_threshold"`
		UnhealthyThreshold *int     `json:"unhealthy_threshold"`
		GlobalMaxWorkers   *int     `json:"global_max_workers"`
		GlobalMinWorkers   *int     `json:"global_min_workers"`
		MTTDLookback       *string  `json:"mttd_lookback"`
	}

	if err := c.ShouldBindJSON(&cfg); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Fetch current, then overlay non-nil fields
	current, err := h.configSvc.GetAutoScalerConfig(c.Request.Context())
	if err != nil {
		h.log.Error("failed to load current autoscaler config for merge", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load current config"})
		return
	}

	if cfg.Enabled != nil {
		current.Enabled = *cfg.Enabled
	}
	if cfg.EvaluationInterval != nil {
		current.EvaluationInterval = mustParseDuration(*cfg.EvaluationInterval)
	}
	if cfg.HighMTTDThreshold != nil {
		current.HighMTTDThreshold = mustParseDuration(*cfg.HighMTTDThreshold)
	}
	if cfg.MediumMTTDThreshold != nil {
		current.MediumMTTDThreshold = mustParseDuration(*cfg.MediumMTTDThreshold)
	}
	if cfg.LowMTTDThreshold != nil {
		current.LowMTTDThreshold = mustParseDuration(*cfg.LowMTTDThreshold)
	}
	if cfg.MaxLagPerWorker != nil {
		current.MaxLagPerWorker = *cfg.MaxLagPerWorker
	}
	if cfg.CooldownPeriod != nil {
		current.CooldownPeriod = mustParseDuration(*cfg.CooldownPeriod)
	}
	if cfg.ScaleDownFactor != nil {
		current.ScaleDownFactor = *cfg.ScaleDownFactor
	}
	if cfg.IdleThreshold != nil {
		current.IdleThreshold = mustParseDuration(*cfg.IdleThreshold)
	}
	if cfg.UnhealthyThreshold != nil {
		current.UnhealthyThreshold = *cfg.UnhealthyThreshold
	}
	if cfg.GlobalMaxWorkers != nil {
		current.GlobalMaxWorkers = *cfg.GlobalMaxWorkers
	}
	if cfg.GlobalMinWorkers != nil {
		current.GlobalMinWorkers = *cfg.GlobalMinWorkers
	}
	if cfg.MTTDLookback != nil {
		current.MTTDLookback = mustParseDuration(*cfg.MTTDLookback)
	}

	if err := h.configSvc.UpdateAutoScalerConfig(c.Request.Context(), current); err != nil {
		h.log.Error("failed to update autoscaler config", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update autoscaler config"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "autoscaler config updated", "config": toConfigResponse(current)})
}

// ── Vendor Retry Config Endpoints ──────────────────────────────────────────

// GetVendorRetryConfigs returns all active vendor retry/backoff configurations.
func (h *AdminHandler) GetVendorRetryConfigs(c *gin.Context) {
	var apiKeyID *string
	if v := c.GetString("scoped_api_key_id"); v != "" {
		apiKeyID = &v
	} else if v := c.Query("api_key_id"); v != "" {
		apiKeyID = &v
	}

	configs, err := h.retryConfigRepo.ListActive(c.Request.Context(), apiKeyID)
	if err != nil {
		h.log.Error("failed to list vendor retry configs", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to retrieve retry configurations"})
		return
	}
	if configs == nil {
		configs = []*domain.VendorRetryConfig{}
	}
	c.JSON(http.StatusOK, configs)
}

// UpsertVendorRetryConfig creates or updates the retry config for a specific vendor.
func (h *AdminHandler) UpsertVendorRetryConfig(c *gin.Context) {
	vendorName := c.Param("vendor_name")
	if vendorName == "" {
		respondError(c, http.StatusBadRequest, "BAD_REQUEST", "vendor_name path param required")
		return
	}

	var req domain.UpsertVendorRetryConfigRequest
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

	// Load existing config to merge, or start with defaults
	var cfg *domain.VendorRetryConfig
	existing, _ := h.retryConfigRepo.Get(c.Request.Context(), vendorName, apiKeyID)
	if existing != nil {
		cfg = existing
	} else {
		cfg = domain.DefaultRetryConfig(vendorName)
	}

	if req.RetryInitialIntervalMs != nil {
		cfg.RetryInitialIntervalMs = *req.RetryInitialIntervalMs
	}
	if req.RetryMaxIntervalMs != nil {
		cfg.RetryMaxIntervalMs = *req.RetryMaxIntervalMs
	}
	if req.RetryMaxAttempts != nil {
		cfg.RetryMaxAttempts = *req.RetryMaxAttempts
	}
	if req.RetryBackoffCoefficient != nil {
		cfg.RetryBackoffCoefficient = *req.RetryBackoffCoefficient
	}
	if req.SLA != nil {
		cfg.SLA = *req.SLA
	}
	cfg.IsActive = true

	if err := h.retryConfigRepo.Upsert(c.Request.Context(), cfg, apiKeyID); err != nil {
		h.log.Error("failed to upsert vendor retry config", zap.String("vendor", vendorName), zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save retry configuration"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "retry configuration updated", "vendor": vendorName, "config": cfg})
}

// DeleteVendorRetryConfig removes the retry config for a specific vendor.
func (h *AdminHandler) DeleteVendorRetryConfig(c *gin.Context) {
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

	if err := h.retryConfigRepo.Delete(c.Request.Context(), vendorName, apiKeyID); err != nil {
		h.log.Error("failed to delete vendor retry config", zap.String("vendor", vendorName), zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete retry configuration"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "retry configuration removed", "vendor": vendorName})
}

func mustParseDuration(s string) time.Duration {
	d, err := time.ParseDuration(s)
	if err != nil {
		return 0
	}
	return d
}

// ── Migration Endpoints ─────────────────────────────────────────────────────────

// ListMigrations returns all orchestration migrations. Supports ?active_only=true.
func (h *AdminHandler) ListMigrations(c *gin.Context) {
	if h.migrationMgr == nil {
		c.JSON(http.StatusOK, []*domain.OrchestrationMigration{})
		return
	}
	activeOnly := c.Query("active_only") == "true"
	migrations, err := h.migrationMgr.ListMigrations(c.Request.Context(), activeOnly)
	if err != nil {
		h.log.Error("failed to list migrations", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list migrations"})
		return
	}
	if migrations == nil {
		migrations = []*domain.OrchestrationMigration{}
	}
	c.JSON(http.StatusOK, migrations)
}

// CancelMigration cancels an in-progress orchestration migration.
func (h *AdminHandler) CancelMigration(c *gin.Context) {
	if h.migrationMgr == nil {
		respondError(c, http.StatusNotFound, "NOT_FOUND", "migration manager not available")
		return
	}
	idStr := c.Param("id")
	migID, err := uuid.Parse(idStr)
	if err != nil {
		respondError(c, http.StatusBadRequest, "BAD_REQUEST", "invalid migration id")
		return
	}
	if err := h.migrationMgr.CancelMigration(c.Request.Context(), migID); err != nil {
		h.log.Error("failed to cancel migration", zap.String("id", idStr), zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to cancel migration"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "migration cancelled", "migration_id": idStr})
}

// ── Vendor Migration Endpoints ────────────────────────────────────────────────

// StartVendorMigration handles POST /v1/admin/config/vendors/migrations
// Initiates a vendor swap (cross-vendor or same-vendor config change).
func (h *AdminHandler) StartVendorMigration(c *gin.Context) {
	if h.vendorMigrationSvc == nil {
		respondError(c, http.StatusServiceUnavailable, "NOT_CONFIGURED", "vendor migration service not available")
		return
	}

	var req domain.StartVendorMigrationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "BAD_REQUEST", err.Error())
		return
	}

	var apiKeyID *string
	if v := c.GetString("scoped_api_key_id"); v != "" {
		apiKeyID = &v
	} else if v := c.Query("api_key_id"); v != "" {
		apiKeyID = &v
	}

	migration, err := h.vendorMigrationSvc.Start(c.Request.Context(), &req, apiKeyID)
	if err != nil {
		h.log.Error("failed to start vendor migration", zap.Error(err))
		respondError(c, http.StatusBadRequest, "MIGRATION_ERROR", err.Error())
		return
	}

	c.JSON(http.StatusCreated, migration)
}

// ListVendorMigrations handles GET /v1/admin/config/vendors/migrations
// Returns vendor migrations filtered by optional ?channel= and ?status= query params.
func (h *AdminHandler) ListVendorMigrations(c *gin.Context) {
	if h.vendorMigrationSvc == nil {
		c.JSON(http.StatusOK, []*domain.VendorMigration{})
		return
	}

	var apiKeyID *string
	if v := c.GetString("scoped_api_key_id"); v != "" {
		apiKeyID = &v
	} else if v := c.Query("api_key_id"); v != "" {
		apiKeyID = &v
	}

	channel := c.Query("channel")
	status := c.Query("status")

	migrations, err := h.vendorMigrationSvc.List(c.Request.Context(), apiKeyID, channel, status)
	if err != nil {
		h.log.Error("failed to list vendor migrations", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list vendor migrations"})
		return
	}
	c.JSON(http.StatusOK, migrations)
}

// CompleteVendorMigration handles POST /v1/admin/config/vendors/migrations/:id/complete
// Finalises a gradual migration: locks routing to the new vendor and marks it done.
func (h *AdminHandler) CompleteVendorMigration(c *gin.Context) {
	if h.vendorMigrationSvc == nil {
		respondError(c, http.StatusServiceUnavailable, "NOT_CONFIGURED", "vendor migration service not available")
		return
	}

	migID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		respondError(c, http.StatusBadRequest, "BAD_REQUEST", "invalid migration id")
		return
	}

	if err := h.vendorMigrationSvc.Complete(c.Request.Context(), migID); err != nil {
		h.log.Error("failed to complete vendor migration", zap.String("id", migID.String()), zap.Error(err))
		respondError(c, http.StatusBadRequest, "MIGRATION_ERROR", err.Error())
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "migration completed", "migration_id": migID.String()})
}

// RollbackVendorMigration handles POST /v1/admin/config/vendors/migrations/:id/rollback
// Restores the previous vendor config and routing, marking the migration as rolled_back.
func (h *AdminHandler) RollbackVendorMigration(c *gin.Context) {
	if h.vendorMigrationSvc == nil {
		respondError(c, http.StatusServiceUnavailable, "NOT_CONFIGURED", "vendor migration service not available")
		return
	}

	migID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		respondError(c, http.StatusBadRequest, "BAD_REQUEST", "invalid migration id")
		return
	}

	if err := h.vendorMigrationSvc.Rollback(c.Request.Context(), migID); err != nil {
		h.log.Error("failed to rollback vendor migration", zap.String("id", migID.String()), zap.Error(err))
		respondError(c, http.StatusBadRequest, "MIGRATION_ERROR", err.Error())
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "migration rolled back", "migration_id": migID.String()})
}
