//go:build ignore

package main


import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"
	"github.com/spidey/notification-service/internal/circuit"
	"github.com/spidey/notification-service/internal/config"
	"github.com/spidey/notification-service/internal/domain"
	"github.com/spidey/notification-service/internal/provider"
	"github.com/spidey/notification-service/internal/pubsub"
	"github.com/spidey/notification-service/internal/repository"
	"github.com/spidey/notification-service/internal/worker"
	"go.uber.org/zap"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// 1. Load config
	cfg, err := config.Load("api/config")
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	logger, _ := zap.NewDevelopment()
	defer logger.Sync()

	// 2. Setup DB and Repos
	db, err := repository.NewDB(ctx, cfg.Database)
	if err != nil {
		log.Fatalf("failed to connect to db: %v", err)
	}
	defer db.Close()

	notifRepo := repository.NewNotificationRepository(db)
	attemptRepo := repository.NewAttemptRepository(db)
	eventRepo := repository.NewEventRepository(db)

	// 3. Setup Mock PubSub
	mockPub := pubsub.NewMockPublisher(logger)
	mockSub := pubsub.NewMockSubscriber(mockPub, logger)

	// 4. Setup SMS Worker
	smsSenders := provider.InitializeSMSSenders(cfg.Providers.SMS)
	registry := circuit.NewRegistry(logger)
	smsWorker := worker.NewSMSWorker(mockSub, smsSenders, notifRepo, attemptRepo, eventRepo, registry, logger)

	// Start worker
	go func() {
		if err := smsWorker.Start(ctx); err != nil && ctx.Err() == nil {
			logger.Error("sms worker error", zap.Error(err))
		}
	}()

	fmt.Println("SMS Worker started in background...")

	// 5. Create a fake notification in DB (needed by worker dispatch)
	notifID := uuid.New()
	notif := &domain.Notification{
		ID:             notifID,
		UserID:         uuid.New(),
		Channel:        domain.ChannelSMS,
		Priority:       domain.PriorityHigh,
		Recipient:      "+18777804236", //twilio's virtual phone number
		Status:         domain.StatusPending,
		IdempotencyKey: uuid.New().String(),
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}

	if err := notifRepo.Create(ctx, notif); err != nil {
		log.Fatalf("failed to create notification record: %v", err)
	}
	fmt.Printf("Created notification record with ID: %s\n", notifID)

	// 6. Publish message to PubSub
	msg := &pubsub.Message{
		NotificationID: notifID.String(),
		Channel:        "sms",
		Recipient:      notif.Recipient,
	}

	fmt.Println("Publishing message to mock PubSub...")
	msgID, err := mockPub.Publish(ctx, "sms", msg)
	if err != nil {
		log.Fatalf("failed to publish message: %v", err)
	}
	fmt.Printf("Published message with ID: %s\n", msgID)

	// 7. Wait and observe
	fmt.Println("Waiting for worker to process...")
	time.Sleep(5 * time.Second)

	// Check status in DB
	updatedNotif, err := notifRepo.GetByID(ctx, notifID)
	if err != nil {
		log.Fatalf("failed to get updated notification: %v", err)
	}

	fmt.Printf("Final status in DB: %s\n", updatedNotif.Status)
	fmt.Println("Done.")
}
