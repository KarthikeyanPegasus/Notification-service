package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/clerk/clerk-sdk-go/v2"
	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/spidey/notification-service/internal/cache"
	"github.com/spidey/notification-service/internal/circuit"
	"github.com/spidey/notification-service/internal/config"
	"github.com/spidey/notification-service/internal/domain"
	"github.com/spidey/notification-service/internal/handler"
	ws "github.com/spidey/notification-service/internal/provider/websocket"
	"github.com/spidey/notification-service/internal/pubsub"
	"github.com/spidey/notification-service/internal/repository"
	"github.com/spidey/notification-service/internal/security"
	"github.com/spidey/notification-service/internal/service"
	"github.com/spidey/notification-service/internal/workflow"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"golang.org/x/crypto/bcrypt"
)

func main() {
	cfg, err := config.Load("")
	if err != nil {
		fmt.Fprintf(os.Stderr, "loading config: %v\n", err)
		os.Exit(1)
	}

	log := buildLogger(cfg.Log)
	defer log.Sync() //nolint:errcheck

	// Guard: debug mode must never run in a production or staging environment.
	if cfg.Server.Mode == "debug" {
		env := strings.ToLower(strings.TrimSpace(os.Getenv("ENVIRONMENT")))
		switch env {
		case "production", "prod", "staging", "stage":
			log.Fatal("debug mode is not allowed in production/staging; set NS_SERVER_MODE=release")
		}
	}

	ctx := context.Background()

	// Database
	db, err := repository.NewDB(ctx, cfg.Database)
	if err != nil {
		log.Fatal("connecting to database", zap.Error(err))
	}
	defer db.Close()

	// Run migrations
	if err := runMigrations(cfg.Database.DSN, cfg.Database.MigrationDir); err != nil {
		log.Fatal("running migrations", zap.Error(err))
	}

	// Redis
	redisClient, err := cache.NewClient(cfg.Redis)
	if err != nil {
		log.Fatal("connecting to redis", zap.Error(err))
	}
	defer redisClient.Close()

	// Pub/Sub publisher
	var publisher pubsub.Publisher
	switch cfg.PubSub.Mode {
	case "mock":
		log.Info("using mock pubsub publisher")
		publisher = pubsub.NewMockPublisher(log)
	case "redis":
		log.Info("using redis pubsub publisher")
		publisher = pubsub.NewRedisPublisher(redisClient.RDB, log)
	case "kafka":
		log.Info("using kafka pubsub publisher", zap.Strings("brokers", cfg.PubSub.Kafka.Brokers))
		publisher = pubsub.NewKafkaPublisher(cfg.PubSub.Kafka, log)
	default:
		publisher, err = pubsub.NewGCPPublisher(ctx, cfg.PubSub)
		if err != nil {
			log.Fatal("creating pubsub publisher", zap.Error(err))
		}
	}
	defer publisher.Close()

	// Workflow Engine
	engine, err := workflow.NewEngine(cfg, log)
	if err != nil {
		log.Fatal("creating workflow engine", zap.Error(err))
	}
	if engine != nil {
		defer engine.Close()
	}

	// Circuit breaker registry
	cbRegistry := circuit.NewRegistry(log)

	// Repositories
	notifRepo := repository.NewNotificationRepository(db)
	schedRepo := repository.NewScheduledRepository(db)
	eventRepo := repository.NewEventRepository(db)
	attemptRepo := repository.NewAttemptRepository(db)
	templateRepo := repository.NewTemplateRepository(db)
	webhookEventRepo := repository.NewWebhookEventRepository(db)
	migrationRepo := repository.NewMigrationRepository(db)
	key := cfg.Security.VendorConfigEncryptionKey
	key = strings.TrimSpace(key)
	// Treat the default placeholder as "unset" for dev ergonomics.
	if strings.Contains(key, "ENCRYPTION_KEY_HERE") || strings.Contains(key, "<SET_IN_ENV>") {
		key = ""
	}
	if key == "" {
		if cfg.Server.Mode == "release" {
			log.Fatal("NS_SECURITY_VENDOR_CONFIG_ENCRYPTION_KEY must be set in release mode; refusing to start with an insecure derived key")
		}
		// Generate a random ephemeral key for non-release environments.
		// This key is ephemeral — vendor configs encrypted with it will be
		// unrecoverable on next restart. Set NS_SECURITY_VENDOR_CONFIG_ENCRYPTION_KEY
		// explicitly for any environment that stores real vendor credentials.
		rawKey := make([]byte, 32)
		if _, err := rand.Read(rawKey); err != nil {
			log.Fatal("failed to generate random encryption key", zap.Error(err))
		}
		key = base64.StdEncoding.EncodeToString(rawKey)
		log.Warn("security.vendor_config_encryption_key is empty; generated an ephemeral random key for non-release mode — set NS_SECURITY_VENDOR_CONFIG_ENCRYPTION_KEY explicitly in production/staging")
	}
	vendorCfgCrypto, err := security.NewVendorConfigCrypto(key)
	if err != nil {
		log.Fatal("initializing vendor config encryption", zap.Error(err))
	}
	vendorConfigRepo := repository.NewVendorConfigRepository(db, vendorCfgCrypto)
	govRepo := repository.NewGovernanceRepository(db)
	apiKeyRepo := repository.NewAPIKeyRepository(db)
	userRepo := repository.NewUserRepository(db)
	rateLimitRepo := repository.NewVendorRateLimitRepository(db)
	prefsRepo := repository.NewUserPreferencesRepository(db)

	log.Info("Initializing Clerk SDK",
		zap.Bool("clerk_key_configured", strings.TrimSpace(cfg.Clerk.SecretKey) != ""),
	)
	if strings.TrimSpace(cfg.Clerk.SecretKey) != "" {
		clerk.SetKey(strings.TrimSpace(cfg.Clerk.SecretKey))
	} else {
		log.Warn("clerk.secret_key is empty; Clerk JWT verification will fail (set NS_CLERK_SECRET_KEY)")
	}


	// Bootstrap admin user (optional, config-driven)
	if cfg.Admin.Email != "" && cfg.Admin.Password != "" {
		if err := ensureAdminUser(ctx, userRepo, cfg.Admin.Email, cfg.Admin.Name, cfg.Admin.Password, log); err != nil {
			log.Fatal("bootstrapping admin user failed", zap.Error(err))
		}
	}

	if err := cfg.LoadDynamicOverrides(ctx, vendorConfigRepo); err != nil {
		log.Warn("failed to load dynamic vendor config into memory", zap.Error(err))
	}

	// Services
	templateSvc := service.NewTemplateService(templateRepo, redisClient)
	prefsSvc := service.NewPreferencesService(prefsRepo, redisClient, log)
	otpSvc := service.NewOTPService(redisClient)
	wfClients := service.NewWorkflowClientProvider(engine, vendorConfigRepo, cfg, log)
	notifSvc := service.NewNotificationService(
		notifRepo, schedRepo, eventRepo, attemptRepo,
		templateSvc, prefsSvc, wfClients, publisher, cfg, vendorConfigRepo, log,
	)
	schedSvc := service.NewSchedulerService(schedRepo, notifRepo, eventRepo, wfClients, log)
	reconSvc := service.NewReconciliationService(notifRepo, log)

	// Migration Manager for orchestration changes.
	migrationMgr := service.NewMigrationManager(
		migrationRepo, schedRepo, notifRepo, eventRepo, apiKeyRepo, wfClients, vendorConfigRepo, log,
	)

	// Create config service and wire migration manager for orchestration changes.
	configSvcImpl := service.NewConfigService(vendorConfigRepo, publisher, log)
	configSvcImpl.WithMigrationManager(migrationMgr)
	configSvc := configSvcImpl
	apiKeySvc := service.NewAPIKeyService(apiKeyRepo)

	// Clerk authentication
	var clerkWebhookHandler *handler.ClerkWebhookHandler
	if cfg.Clerk.WebhookSecret != "" {
		var err error
		clerkWebhookHandler, err = handler.NewClerkWebhookHandler(cfg.Clerk.WebhookSecret, userRepo, log)
		if err != nil {
			log.Warn("failed to init Clerk webhook handler", zap.Error(err))
		}
	} else {
		log.Warn("Clerk webhook secret not configured; user sync via webhooks disabled")
	}

	// WebSocket Hub — manages real-time notification delivery connections
	wsHub := ws.NewHub(log)
	go wsHub.Run()

	// DLQ Repository
	dlqRepo := repository.NewDLQRepository(db)

	// Handlers
	notifHandler := handler.NewNotificationHandler(notifSvc, schedSvc, userRepo, log)
	otpHandler := handler.NewOTPHandler(otpSvc, notifSvc, log)
	webhookHandler := handler.NewWebhookHandler(eventRepo, notifRepo, attemptRepo, webhookEventRepo, govRepo, cfg, log)
	prefsHandler := handler.NewPreferencesHandler(prefsSvc, userRepo, log)
	reportHandler := handler.NewReportHandler(webhookEventRepo, notifRepo, attemptRepo, userRepo, cfg, vendorConfigRepo, log)
	vendorMigrationRepo := repository.NewVendorMigrationRepository(db)
	vendorMigrationSvc := service.NewVendorMigrationService(vendorMigrationRepo, vendorConfigRepo, configSvc, publisher, log)

	adminHandler := handler.NewAdminHandler(
		configSvc,
		rateLimitRepo,
		notifRepo,
		cfg.Cadence.Mode,
		cfg.PubSub.Mode,
		cfg.PubSub.Kafka.Brokers,
		log,
	).WithMigrationManager(migrationMgr).WithVendorMigrationService(vendorMigrationSvc).WithDLQRepository(dlqRepo)
	testDeliveryHandler := handler.NewTestDeliveryHandler(vendorConfigRepo, notifSvc, log)
	apiKeyHandler := handler.NewAPIKeyHandler(apiKeySvc, log)
	userAdminHandler := handler.NewUserAdminHandler(userRepo, apiKeyRepo)
	invitationHandler := handler.NewInvitationHandler()
	meHandler := handler.NewMeHandler(userRepo, apiKeyRepo, apiKeySvc)
	govHandler := handler.NewGovernanceHandler(govRepo, log)
	tmplHandler := handler.NewTemplateHandler(templateRepo, templateSvc, log)
	wsHandler := handler.NewWebSocketHandler(wsHub, log)
	dlqHandler := handler.NewDLQHandler(dlqRepo, notifSvc, log)

	// Standalone scheduler is now replaced by Temporal's native scheduling logic.
	// No longer need a local goroutine ticker for ProcessDue.

	// Reconciliation: ticks every 1h to backfill stuck notifications
	go reconSvc.Start(ctx, 1*time.Hour)

	// Router
	router := handler.NewRouter(handler.Dependencies{
		NotificationHandler: notifHandler,
		OTPHandler:          otpHandler,
		WebhookHandler:      webhookHandler,
		PrefsHandler:        prefsHandler,
		ReportHandler:       reportHandler,
		AdminHandler:        adminHandler,
		TestDeliveryHandler: testDeliveryHandler,
		APIKeyHandler:       apiKeyHandler,
		ClerkWebhookHandler: clerkWebhookHandler,
		InvitationHandler:   invitationHandler,
		UserAdminHandler:    userAdminHandler,
		MeHandler:           meHandler,
		GovernanceHandler:   govHandler,
		TemplateHandler:     tmplHandler,
		WSHandler:           wsHandler,
		DLQHandler:          dlqHandler,
		CircuitRegistry:     cbRegistry,
		Config:              cfg,
		APIKeyVerifier:      apiKeySvc,
		UserRepo:            userRepo,
	})

	srv := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.Server.Port),
		Handler:      router,
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
	}

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	schedulerCtx, stopScheduler := context.WithCancel(context.Background())
	go notifSvc.StartStandaloneScheduler(schedulerCtx)

	go func() {
		log.Info("api server starting", zap.Int("port", cfg.Server.Port))
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatal("server error", zap.Error(err))
		}
	}()

	<-quit
	log.Info("shutting down api server")
	stopScheduler()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.Server.ShutdownTimeout)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Error("server shutdown error", zap.Error(err))
	}
	log.Info("api server stopped")
}

func runMigrations(dsn, dir string) error {
	m, err := migrate.New("file://"+dir, dsn)
	if err != nil {
		return fmt.Errorf("creating migrator: %w", err)
	}
	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("running up migrations: %w", err)
	}
	return nil
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
		Encoding:         cfg.Format,
		EncoderConfig:    encoderCfg,
		OutputPaths:      []string{"stdout"},
		ErrorOutputPaths: []string{"stderr"},
	}
	if zapCfg.Encoding == "" {
		zapCfg.Encoding = "json"
	}

	log, err := zapCfg.Build()
	if err != nil {
		panic("building logger: " + err.Error())
	}
	return log
}

func ensureAdminUser(
	ctx context.Context,
	users *repository.UserRepository,
	email, name, password string,
	log *zap.Logger,
) error {
	email = strings.TrimSpace(strings.ToLower(email))
	name = strings.TrimSpace(name)
	if name == "" {
		name = "Admin"
	}

	// If exists, do nothing.
	if _, err := users.GetByEmailWithHash(ctx, email); err == nil {
		log.Info("admin user already exists", zap.String("email", email))
		return nil
	} else if !errors.Is(err, domain.ErrNotFound) {
		return fmt.Errorf("lookup admin user: %w", err)
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}
	u, err := users.Create(ctx, email, name, hash, domain.UserRoleAdmin)
	if err != nil {
		return fmt.Errorf("create admin user: %w", err)
	}
	log.Info("bootstrapped admin user", zap.String("id", u.ID), zap.String("email", u.Email))
	return nil
}
