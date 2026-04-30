package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spidey/notification-service/internal/domain"
	"github.com/spidey/notification-service/internal/pubsub"
	"github.com/spidey/notification-service/internal/repository"
	"go.uber.org/zap"
)

type ConfigService interface {
	GetVendorConfigs(ctx context.Context, apiKeyID *string) ([]*domain.VendorConfig, error)
	GetVendorConfig(ctx context.Context, vendorType string, apiKeyID *string) (*domain.VendorConfig, error)
	UpdateVendorConfig(ctx context.Context, vendorType string, configJSON json.RawMessage, apiKeyID *string) error
	DeleteVendorConfig(ctx context.Context, vendorType string, apiKeyID *string) error
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

func (s *configService) GetVendorConfigs(ctx context.Context, apiKeyID *string) ([]*domain.VendorConfig, error) {
	return s.repo.ListActive(ctx, apiKeyID)
}

func (s *configService) GetVendorConfig(ctx context.Context, vendorType string, apiKeyID *string) (*domain.VendorConfig, error) {
	return s.repo.GetByType(ctx, vendorType, apiKeyID)
}

func (s *configService) UpdateVendorConfig(ctx context.Context, vendorType string, configJSON json.RawMessage, apiKeyID *string) error {
	// 1. Validate JSON (optional but recommended)
	if !json.Valid(configJSON) {
		return fmt.Errorf("invalid JSON for vendor config")
	}

	// 1b. Vendor-specific validation (keep minimal and only where required by UI/app-store).
	switch vendorType {
	case "mailgun":
		var payload struct {
			Domain string `json:"domain"`
			APIKey string `json:"api_key"`
		}
		if err := json.Unmarshal(configJSON, &payload); err != nil {
			return fmt.Errorf("invalid JSON for mailgun config")
		}
		if strings.TrimSpace(payload.Domain) == "" || strings.TrimSpace(payload.APIKey) == "" {
			return fmt.Errorf("mailgun config requires domain and api_key")
		}
	}

	// 2. Persist to DB
	cfg := &domain.VendorConfig{
		VendorType: vendorType,
		ConfigJSON: configJSON,
		IsActive:   true,
	}
	if err := s.repo.Upsert(ctx, cfg, apiKeyID); err != nil {
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

func (s *configService) DeleteVendorConfig(ctx context.Context, vendorType string, apiKeyID *string) error {
	if err := s.repo.SetActive(ctx, vendorType, false, apiKeyID); err != nil {
		return err
	}

	// Signal change via Pub/Sub (best-effort)
	msg := &pubsub.Message{
		NotificationID: "config-reload-" + vendorType,
		Channel:        "config",
		Payload: map[string]string{
			"vendor_type": vendorType,
		},
	}
	_, err := s.publisher.Publish(ctx, "config", msg)
	if err != nil {
		s.log.Error("failed to publish config reload event after delete", zap.Error(err), zap.String("vendor", vendorType))
	}

	s.log.Info("vendor config deactivated", zap.String("vendor", vendorType))
	return nil
}
