package main

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/spidey/notification-service/internal/cache"
	"github.com/spidey/notification-service/internal/circuit"
	"github.com/spidey/notification-service/internal/config"
	"github.com/spidey/notification-service/internal/domain"
	"github.com/spidey/notification-service/internal/handler"
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
	key := cfg.Security.VendorConfigEncryptionKey
	key = strings.TrimSpace(key)
	// Treat the default placeholder as "unset" for dev ergonomics.
	if strings.Contains(key, "ENCRYPTION_KEY_HERE") {
		key = ""
	}
	if key == "" && cfg.Server.Mode != "release" {
		// Dev-friendly stable key derivation: if a user didn't set a dedicated vendor-config key,
		// derive a deterministic 32-byte key from the JWT secret so DB-stored configs survive restarts.
		// In production you MUST set NS_SECURITY_VENDOR_CONFIG_ENCRYPTION_KEY explicitly.
		sum := sha256.Sum256([]byte(strings.TrimSpace(cfg.JWT.Secret)))
		key = base64.StdEncoding.EncodeToString(sum[:])
		log.Warn("security.vendor_config_encryption_key is empty/placeholder; derived a stable key from jwt.secret for non-release mode (set NS_SECURITY_VENDOR_CONFIG_ENCRYPTION_KEY in production)")
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
	prefsSvc := service.NewPreferencesService(redisClient)
	otpSvc := service.NewOTPService(redisClient)
	wfClients := service.NewWorkflowClientProvider(engine, vendorConfigRepo, cfg, log)
	notifSvc := service.NewNotificationService(
		notifRepo, schedRepo, eventRepo, attemptRepo,
		templateSvc, prefsSvc, wfClients, publisher, cfg, vendorConfigRepo, log,
	)
	schedSvc := service.NewSchedulerService(schedRepo, notifRepo, eventRepo, wfClients, log)
	reconSvc := service.NewReconciliationService(notifRepo, log)
	configSvc := service.NewConfigService(vendorConfigRepo, publisher, log)
	apiKeySvc := service.NewAPIKeyService(apiKeyRepo)
	authSvc := service.NewAuthService(cfg, userRepo)

	// Handlers
	notifHandler := handler.NewNotificationHandler(notifSvc, schedSvc, userRepo, log)
	otpHandler := handler.NewOTPHandler(otpSvc, notifSvc, log)
	webhookHandler := handler.NewWebhookHandler(eventRepo, notifRepo, attemptRepo, webhookEventRepo, govRepo, cfg, log)
	prefsHandler := handler.NewPreferencesHandler(prefsSvc, log)
	reportHandler := handler.NewReportHandler(webhookEventRepo, notifRepo, attemptRepo, userRepo, cfg, vendorConfigRepo, log)
	adminHandler := handler.NewAdminHandler(configSvc, rateLimitRepo, notifRepo, cfg.Cadence.Mode, log)
	testDeliveryHandler := handler.NewTestDeliveryHandler(vendorConfigRepo, notifSvc, log)
	apiKeyHandler := handler.NewAPIKeyHandler(apiKeySvc, log)
	authHandler := handler.NewAuthHandler(authSvc)
	userAdminHandler := handler.NewUserAdminHandler(userRepo, apiKeyRepo)
	meHandler := handler.NewMeHandler(userRepo, apiKeyRepo, apiKeySvc)
	govHandler := handler.NewGovernanceHandler(govRepo, log)
	tmplHandler := handler.NewTemplateHandler(templateRepo, templateSvc, log)

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
		AuthHandler:         authHandler,
		UserAdminHandler:    userAdminHandler,
		MeHandler:           meHandler,
		GovernanceHandler:   govHandler,
		TemplateHandler:     tmplHandler,
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

	go func() {
		log.Info("api server starting", zap.Int("port", cfg.Server.Port))
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatal("server error", zap.Error(err))
		}
	}()

	<-quit
	log.Info("shutting down api server")

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
