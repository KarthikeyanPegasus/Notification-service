package main

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/spidey/notification-service/internal/cache"
	"github.com/spidey/notification-service/internal/circuit"
	"github.com/spidey/notification-service/internal/config"
	"github.com/spidey/notification-service/internal/pubsub"
	"github.com/spidey/notification-service/internal/ratelimit"
	"github.com/spidey/notification-service/internal/repository"
	"github.com/spidey/notification-service/internal/security"
	"github.com/spidey/notification-service/internal/service"
	"github.com/spidey/notification-service/internal/worker"
	"github.com/spidey/notification-service/internal/workflow"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func main() {
	cfg, err := config.Load("")
	if err != nil {
		fmt.Fprintf(os.Stderr, "loading config: %v\n", err)
		os.Exit(1)
	}

	log := buildLogger(cfg.Log)
	defer log.Sync() //nolint:errcheck

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// ── Database ─────────────────────────────────────────────────────────────
	db, err := repository.NewDB(ctx, cfg.Database)
	if err != nil {
		log.Fatal("connecting to database", zap.Error(err))
	}
	defer db.Close()

	// ── Redis ────────────────────────────────────────────────────────────────
	redisClient, err := cache.NewClient(cfg.Redis)
	if err != nil {
		log.Fatal("connecting to redis", zap.Error(err))
	}
	defer redisClient.Close()

	// ── Repositories ─────────────────────────────────────────────────────────
	notifRepo := repository.NewNotificationRepository(db)
	schedRepo := repository.NewScheduledRepository(db)
	attemptRepo := repository.NewAttemptRepository(db)
	eventRepo := repository.NewEventRepository(db)
	templateRepo := repository.NewTemplateRepository(db)
	govRepo := repository.NewGovernanceRepository(db)
	apiKeyRepo := repository.NewAPIKeyRepository(db)
	rateLimitRepo := repository.NewVendorRateLimitRepository(db)

	key := cfg.Security.VendorConfigEncryptionKey
	key = strings.TrimSpace(key)
	if strings.Contains(key, "ENCRYPTION_KEY_HERE") {
		key = ""
	}
	if key == "" && cfg.Server.Mode != "release" {
		sum := sha256.Sum256([]byte(strings.TrimSpace(cfg.JWT.Secret)))
		key = base64.StdEncoding.EncodeToString(sum[:])
		log.Warn("derived vendor config encryption key from jwt.secret (set NS_SECURITY_VENDOR_CONFIG_ENCRYPTION_KEY in production)")
	}
	vendorCfgCrypto, err := security.NewVendorConfigCrypto(key)
	if err != nil {
		log.Fatal("initializing vendor config encryption", zap.Error(err))
	}
	vendorConfigRepo := repository.NewVendorConfigRepository(db, vendorCfgCrypto)

	// Apply dynamic overrides on startup.
	if err := cfg.LoadDynamicOverrides(ctx, vendorConfigRepo); err != nil {
		log.Warn("failed to load dynamic config overrides on startup", zap.Error(err))
	}

	// ── Circuit breakers ─────────────────────────────────────────────────────
	cbRegistry := circuit.NewRegistry(log)

	// ── Delivery service ─────────────────────────────────────────────────────
	deliverySvc := service.NewDeliveryService(cbRegistry, log)
	deliverySvc.Reload(ctx, cfg.Providers)

	// ── Rate limiter ─────────────────────────────────────────────────────────
	rateLimiter := ratelimit.NewVendorLimiter(redisClient.RDB, log)

	// ── Services ─────────────────────────────────────────────────────────────
	templateSvc := service.NewTemplateService(templateRepo, redisClient)

	// ── Workflow Engine ───────────────────────────────────────────────────────
	engine, err := workflow.NewEngine(cfg, log)
	if err != nil {
		log.Fatal("creating workflow engine", zap.Error(err))
	}
	if engine != nil {
		defer engine.Close()
	}

	// Workflow client provider: chooses temporal vs cadence per API key scope
	// based on DB vendor config `workflow_orchestration`.
	wfClients := service.NewWorkflowClientProvider(engine, vendorConfigRepo, cfg, log)

	// ── Publisher (for PublishToPubSubActivity, config reload signals) ────────
	var publisher pubsub.Publisher
	switch cfg.PubSub.Mode {
	case "mock":
		publisher = pubsub.NewMockPublisher(log)
	case "redis":
		publisher = pubsub.NewRedisPublisher(redisClient.RDB, log)
	case "kafka":
		publisher = pubsub.NewKafkaPublisher(cfg.PubSub.Kafka, log)
	default:
		publisher, _ = pubsub.NewGCPPublisher(ctx, cfg.PubSub)
	}
	if publisher != nil {
		defer publisher.Close()
	}

	// ── Workflow Activities ───────────────────────────────────────────────────
	acts := workflow.NewActivities(
		redisClient,
		templateRepo,
		notifRepo,
		schedRepo,
		eventRepo,
		attemptRepo,
		templateSvc,
		deliverySvc,
		publisher,
		govRepo,
		cfg,
		vendorConfigRepo,
		rateLimitRepo,
		rateLimiter,
	)

	// ── WorkerManager ─────────────────────────────────────────────────────────
	// Dynamically creates workflow workers per client × channel × priority.
	// Each client scope can choose temporal vs cadence via `workflow_orchestration` vendor config.
	wm := worker.NewWorkerManager(wfClients, acts, apiKeyRepo, vendorConfigRepo, log)

	// ── Kafka Dispatchers ─────────────────────────────────────────────────────
	// One Dispatcher per channel × priority reads from the Kafka priority topic
	// and starts a Temporal workflow on the correct client-scoped task queue.
	// In non-Kafka modes the dispatcher is a no-op (API starts workflows directly).
	var dispatchers []*worker.Dispatcher
	if cfg.PubSub.Mode == "kafka" {
		subFactory := pubsub.NewKafkaSubscriberFactory(cfg.PubSub.Kafka, log)
		for _, ch := range worker.AllChannels() {
			for _, p := range worker.AllPriorities() {
				groupID := fmt.Sprintf("notif-dispatcher-%s-%s", ch, p)
				sub := subFactory.NewForTopic(groupID)
				d := worker.NewDispatcher(ch, p, sub, engine, log)
				dispatchers = append(dispatchers, d)
			}
		}
		log.Info("kafka dispatchers created", zap.Int("count", len(dispatchers)))
	}

	// ── Config-reload subscriber (non-Kafka modes) ────────────────────────────
	// In Kafka mode, config changes are detected via the WorkerManager reconcile loop.
	var configSubscriber pubsub.Subscriber
	if cfg.PubSub.Mode == "redis" {
		configSubscriber = pubsub.NewRedisSubscriber(redisClient.RDB, log)
	} else if cfg.PubSub.Mode == "mock" {
		mp := pubsub.NewMockPublisher(log)
		configSubscriber = pubsub.NewMockSubscriber(mp, log)
	}

	// ── Metrics & Health ──────────────────────────────────────────────────────
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, "ok")
	})
	mux.HandleFunc("/workers", func(w http.ResponseWriter, r *http.Request) {
		state := wm.GetState()
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(state); err != nil {
			http.Error(w, "encode error", http.StatusInternalServerError)
		}
	})
	srv := &http.Server{
		Addr:         ":8081",
		Handler:      mux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
	}
	go func() {
		log.Info("metrics/health server starting on :8081")
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error("metrics server error", zap.Error(err))
		}
	}()

	// ── Start everything ──────────────────────────────────────────────────────
	var wg sync.WaitGroup

	// WorkerManager: reconciles and manages Temporal workers.
	wg.Add(1)
	go func() {
		defer wg.Done()
		wm.Run(ctx)
	}()

	// Kafka Dispatchers: one goroutine per channel × priority.
	for _, d := range dispatchers {
		wg.Add(1)
		go func(disp *worker.Dispatcher) {
			defer wg.Done()
			if err := disp.Run(ctx); err != nil && ctx.Err() == nil {
				log.Error("dispatcher exited with error", zap.Error(err))
			}
		}(d)
	}

	// Config-reload listener (non-Kafka modes: redis or mock).
	if configSubscriber != nil {
		go func() {
			err := configSubscriber.Subscribe(ctx, "config", func(ctx context.Context, msg *pubsub.Message) error {
				log.Info("received config reload signal", zap.String("vendor", msg.Payload["vendor_type"]))
				if err := cfg.LoadDynamicOverrides(ctx, vendorConfigRepo); err != nil {
					log.Error("failed to reload dynamic config", zap.Error(err))
					return nil
				}
				deliverySvc.Reload(ctx, cfg.Providers)
				// Trigger WorkerManager reconcile to pick up any new keys.
				wm.Reload(ctx)
				return nil
			})
			if err != nil && ctx.Err() == nil {
				log.Error("config reload listener failed", zap.Error(err))
			}
		}()
	}

	// DB poll for config changes (mock mode — separate API/worker processes can't share in-memory broker).
	if cfg.PubSub.Mode == "mock" {
		go func() {
			t := time.NewTicker(10 * time.Second)
			defer t.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-t.C:
					if err := cfg.LoadDynamicOverrides(ctx, vendorConfigRepo); err != nil {
						log.Warn("failed to reload dynamic config (poll)", zap.Error(err))
						continue
					}
					deliverySvc.Reload(ctx, cfg.Providers)
					wm.Reload(ctx)
				}
			}
		}()
	}

	// ── Graceful shutdown ─────────────────────────────────────────────────────
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Info("shutting down worker process")
	cancel()

	shutdownCtx, sCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer sCancel()
	_ = srv.Shutdown(shutdownCtx)

	wg.Wait()
	log.Info("worker process stopped")
}

func buildLogger(cfg config.LogConfig) *zap.Logger {
	level := zap.InfoLevel
	switch cfg.Level {
	case "debug":
		level = zap.DebugLevel
	case "warn":
		level = zap.WarnLevel
	case "error":
		level = zap.ErrorLevel
	}

	encoderCfg := zap.NewProductionEncoderConfig()
	encoderCfg.TimeKey = "timestamp"
	encoderCfg.EncodeTime = zapcore.ISO8601TimeEncoder

	zapCfg := zap.Config{
		Level:            zap.NewAtomicLevelAt(level),
		Development:      false,
		Sampling:         &zap.SamplingConfig{Initial: 100, Thereafter: 100},
		Encoding:         "json",
		EncoderConfig:    encoderCfg,
		OutputPaths:      []string{"stdout"},
		ErrorOutputPaths: []string{"stderr"},
	}
	if cfg.Format == "console" {
		zapCfg.Encoding = "console"
	}

	l, _ := zapCfg.Build()
	return l
}
