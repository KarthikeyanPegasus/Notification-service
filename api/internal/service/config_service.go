package service

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/spidey/notification-service/internal/domain"
	"github.com/spidey/notification-service/internal/pubsub"
	"github.com/spidey/notification-service/internal/repository"
	"go.uber.org/zap"
)

type ConfigService interface {
	GetVendorConfigs(ctx context.Context) ([]*domain.VendorConfig, error)
	GetVendorConfig(ctx context.Context, vendorType string) (*domain.VendorConfig, error)
	UpdateVendorConfig(ctx context.Context, vendorType string, configJSON json.RawMessage) error
}

type configService struct {
	repo      repository.VendorConfigRepository
	publisher pubsub.Publisher
	log       *zap.Logger
}

func NewConfigService(repo repository.VendorConfigRepository, publisher pubsub.Publisher, log *zap.Logger) ConfigService {
	return &configService{
		repo:      repo,
		publisher: publisher,
		log:       log,
	}
}

func (s *configService) GetVendorConfigs(ctx context.Context) ([]*domain.VendorConfig, error) {
	return s.repo.ListActive(ctx)
}

func (s *configService) GetVendorConfig(ctx context.Context, vendorType string) (*domain.VendorConfig, error) {
	return s.repo.GetByType(ctx, vendorType)
}

func (s *configService) UpdateVendorConfig(ctx context.Context, vendorType string, configJSON json.RawMessage) error {
	// 1. Validate JSON (optional but recommended)
	if !json.Valid(configJSON) {
		return fmt.Errorf("invalid JSON for vendor config")
	}

	// 2. Persist to DB
	cfg := &domain.VendorConfig{
		VendorType: vendorType,
		ConfigJSON: configJSON,
		IsActive:   true,
	}
	if err := s.repo.Upsert(ctx, cfg); err != nil {
		return err
	}

	// 3. Signal change via Pub/Sub
	msg := &pubsub.Message{
		NotificationID: "config-reload-" + vendorType,
		Channel:        "config", // Matches the internal-config-reload topic
		Payload: map[string]string{
			"vendor_type": vendorType,
		},
	}

	_, err := s.publisher.Publish(ctx, "config", msg)
	if err != nil {
		s.log.Error("failed to publish config reload event", zap.Error(err), zap.String("vendor", vendorType))
		// We don't return error here because the DB update was successful.
		// Worse case, the worker reloads on next restart or periodic sync.
	}

	s.log.Info("vendor config updated and reload event published", zap.String("vendor", vendorType))
	return nil
}
