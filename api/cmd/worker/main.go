package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/spidey/notification-service/internal/cache"
	"github.com/spidey/notification-service/internal/circuit"
	"github.com/spidey/notification-service/internal/config"
	"github.com/spidey/notification-service/internal/provider"
	"github.com/spidey/notification-service/internal/pubsub"
	"github.com/spidey/notification-service/internal/repository"
	"github.com/spidey/notification-service/internal/service"
	"github.com/spidey/notification-service/internal/worker"
	"github.com/spidey/notification-service/internal/workflow"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	tworker "go.temporal.io/sdk/worker"
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

	// Database
	db, err := repository.NewDB(ctx, cfg.Database)
	if err != nil {
		log.Fatal("connecting to database", zap.Error(err))
	}
	defer db.Close()

	// Redis
	redisClient, err := cache.NewClient(cfg.Redis)
	if err != nil {
		log.Fatal("connecting to redis", zap.Error(err))
	}
	defer redisClient.Close()

	// Pub/Sub subscriber
	var subscriber pubsub.Subscriber
	var mockPublisher *pubsub.MockPublisher

	if cfg.PubSub.Mode == "mock" {
		log.Info("using mock pubsub")
		mockPublisher = pubsub.NewMockPublisher(log)
		subscriber = pubsub.NewMockSubscriber(mockPublisher, log)
	} else if cfg.PubSub.Mode == "redis" {
		log.Info("using redis pubsub")
		subscriber = pubsub.NewRedisSubscriber(redisClient.RDB, log)
	} else {
		gcpSub, err := pubsub.NewGCPSubscriber(ctx, cfg.PubSub)
		if err != nil {
			log.Fatal("creating pubsub subscriber", zap.Error(err))
		}
		subscriber = gcpSub
	}
	_ = mockPublisher

	// Circuit breaker registry
	cbRegistry := circuit.NewRegistry(log)
	
	// Temporal Client
	temporalCli, err := workflow.NewClient(cfg, log)
	if err != nil {
		log.Fatal("creating temporal client", zap.Error(err))
	}
	if temporalCli != nil {
		defer temporalCli.Close()
	}

	// Repositories
	notifRepo := repository.NewNotificationRepository(db)
	attemptRepo := repository.NewAttemptRepository(db)
	eventRepo := repository.NewEventRepository(db)
	vendorConfigRepo := repository.NewVendorConfigRepository(db)
	templateRepo := repository.NewTemplateRepository(db)
	govRepo := repository.NewGovernanceRepository(db)

	// Apply dynamic overrides on startup
	if err := cfg.LoadDynamicOverrides(ctx, vendorConfigRepo); err != nil {
		log.Warn("failed to load dynamic config overrides on startup", zap.Error(err))
	}

	// Providers — initialized dynamically via factory
	emailSenders := provider.InitializeEmailSenders(ctx, cfg.Providers.Email)
	smsSenders := provider.InitializeSMSSenders(cfg.Providers.SMS)
	pushSenders := provider.InitializePushSenders(cfg.Providers.Push)
	webhookDispatcher := provider.InitializeWebhookDispatcher(cfg.Providers.Webhook)


	// Services needed for activities
	templateSvc := service.NewTemplateService(templateRepo, redisClient)
	prefsSvc := service.NewPreferencesService(redisClient)
	notifSvc := service.NewNotificationService(
		notifRepo,
		nil, // schedRepo not needed for basic ingress
		eventRepo,
		attemptRepo,
		templateSvc,
		prefsSvc,
		temporalCli,
		nil, // publisher - service will use fallback if nil, or we can inject one
		cfg,
		log,
	)
	
	// Pub/Sub publisher for PublishToPubSubActivity
	var publisher pubsub.Publisher
	if cfg.PubSub.Mode == "mock" {
		publisher = pubsub.NewMockPublisher(log)
	} else if cfg.PubSub.Mode == "redis" {
		publisher = pubsub.NewRedisPublisher(redisClient.RDB, log)
	} else {
		publisher, _ = pubsub.NewGCPPublisher(ctx, cfg.PubSub)
	}

	// Temporal Worker
	var temporalWorker tworker.Worker
	if temporalCli != nil {
		temporalWorker = tworker.New(temporalCli, "notification-default", tworker.Options{})
		acts := workflow.NewActivities(
			redisClient,
			templateRepo,
			notifRepo,
			eventRepo,
			templateSvc,
			publisher,
			govRepo,
		)
		workflow.RegisterWorkflowsAndActivities(temporalWorker, acts)
	}

	// Workers
	workers := []worker.Worker{
		worker.NewEmailWorker(subscriber, emailSenders,
			notifRepo, attemptRepo, eventRepo, cbRegistry, log),
		worker.NewSMSWorker(subscriber, smsSenders,
			notifRepo, attemptRepo, eventRepo, cbRegistry, log),
		worker.NewPushWorker(subscriber, pushSenders,
			notifRepo, attemptRepo, eventRepo, cbRegistry, log),
		worker.NewWebSocketWorker(subscriber, redisClient,
			notifRepo, attemptRepo, eventRepo, cbRegistry, log),
		worker.NewWebhookWorker(subscriber, webhookDispatcher,
			notifRepo, attemptRepo, eventRepo, cbRegistry, log),
		worker.NewEventWorker(subscriber, notifSvc, cfg.PubSub, log),
	}

	// Metrics & Health server
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, "ok")
	})

	srv := &http.Server{
		Addr:         ":8081",
		Handler:      mux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	go func() {
		log.Info("metrics/health server starting", zap.String("port", "8081"))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error("metrics server error", zap.Error(err))
		}
	}()

	// Start all workers concurrently
	var wg sync.WaitGroup
	for _, w := range workers {
		wg.Add(1)
		go func(wkr worker.Worker) {
			defer wg.Done()
			log.Info("starting worker", zap.String("channel", string(wkr.Channel())))
			if err := wkr.Start(ctx); err != nil && ctx.Err() == nil {
				log.Error("worker exited with error",
					zap.String("channel", string(wkr.Channel())),
					zap.Error(err),
				)
			}
		}(w)
	}

	// Start Temporal Worker if initialized
	if temporalWorker != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			log.Info("starting temporal worker")
			if err := temporalWorker.Run(tworker.InterruptCh()); err != nil {
				log.Error("temporal worker exited with error", zap.Error(err))
			}
		}()
	}

	// Start a background listener for configuration reloads
	go func() {
		log.Info("starting dynamic config reload listener")
		err := subscriber.Subscribe(ctx, "config", func(ctx context.Context, msg *pubsub.Message) error {
			if msg.Channel != "config" {
				return nil
			}
			log.Info("received config reload signal", zap.String("vendor", msg.Payload["vendor_type"]))

			// 1. Fetch latest config from DB
			if err := cfg.LoadDynamicOverrides(ctx, vendorConfigRepo); err != nil {
				log.Error("failed to reload dynamic config", zap.Error(err))
				return nil // don't nack, we'll try again on next signal
			}

			// 2. Trigger reload on all workers
			for _, w := range workers {
				w.Reload(ctx, cfg.Providers)
			}
			return nil
		})
		if err != nil && ctx.Err() == nil {
			log.Error("config reload listener failed", zap.Error(err))
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Info("shutting down workers")
	cancel()

	shutdownCtx, sCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer sCancel()
	_ = srv.Shutdown(shutdownCtx)

	wg.Wait()
	log.Info("all workers stopped")
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

	log, _ := zapCfg.Build()
	return log
}
