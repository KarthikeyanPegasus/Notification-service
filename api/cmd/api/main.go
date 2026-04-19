package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/spidey/notification-service/internal/cache"
	"github.com/spidey/notification-service/internal/circuit"
	"github.com/spidey/notification-service/internal/config"
	"github.com/spidey/notification-service/internal/handler"
	"github.com/spidey/notification-service/internal/pubsub"
	"github.com/spidey/notification-service/internal/repository"
	"github.com/spidey/notification-service/internal/service"
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
	if cfg.PubSub.Mode == "mock" {
		log.Info("using mock pubsub publisher")
		publisher = pubsub.NewMockPublisher(log)
	} else if cfg.PubSub.Mode == "redis" {
		log.Info("using redis pubsub publisher")
		publisher = pubsub.NewRedisPublisher(redisClient.RDB, log)
	} else {
		publisher, err = pubsub.NewGCPPublisher(ctx, cfg.PubSub)
		if err != nil {
			log.Fatal("creating pubsub publisher", zap.Error(err))
		}
	}
	defer publisher.Close()
	
	// Temporal Client
	temporalCli, err := workflow.NewClient(cfg, log)
	if err != nil {
		log.Fatal("creating temporal client", zap.Error(err))
	}
	if temporalCli != nil {
		defer temporalCli.Close()
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
	vendorConfigRepo := repository.NewVendorConfigRepository(db)
	govRepo := repository.NewGovernanceRepository(db)

	// Services
	templateSvc := service.NewTemplateService(templateRepo, redisClient)
	prefsSvc := service.NewPreferencesService(redisClient)
	otpSvc := service.NewOTPService(redisClient)
	notifSvc := service.NewNotificationService(
		notifRepo, schedRepo, eventRepo, attemptRepo,
		templateSvc, prefsSvc, temporalCli, publisher, cfg, log,
	)
	schedSvc := service.NewSchedulerService(schedRepo, notifRepo, eventRepo, temporalCli, log)
	reconSvc := service.NewReconciliationService(notifRepo, log)
	configSvc := service.NewConfigService(vendorConfigRepo, publisher, log)

	// Handlers
	notifHandler := handler.NewNotificationHandler(notifSvc, schedSvc, log)
	otpHandler := handler.NewOTPHandler(otpSvc, notifSvc, log)
	webhookHandler := handler.NewWebhookHandler(eventRepo, notifRepo, attemptRepo, webhookEventRepo, log)
	prefsHandler := handler.NewPreferencesHandler(prefsSvc, log)
	reportHandler := handler.NewReportHandler(webhookEventRepo, notifRepo, log)
	adminHandler := handler.NewAdminHandler(configSvc, log)
	govHandler := handler.NewGovernanceHandler(govRepo, log)

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
		GovernanceHandler:   govHandler,
		CircuitRegistry:     cbRegistry,
		Config:              cfg,
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
