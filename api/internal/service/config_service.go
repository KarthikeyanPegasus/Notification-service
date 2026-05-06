package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	nsconfig "github.com/spidey/notification-service/internal/config"
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

	// AutoScalerConfig provides runtime autoscaler settings.
	GetAutoScalerConfig(ctx context.Context) (*nsconfig.AutoScalerConfig, error)
	UpdateAutoScalerConfig(ctx context.Context, cfg *nsconfig.AutoScalerConfig) error
}

type configService struct {
	repo      repository.VendorConfigRepository
	publisher pubsub.Publisher
	log       *zap.Logger

	// migrationMgr triggers orchestration migration when workflow_orchestration config changes.
	migrationMgr *MigrationManager
}

func NewConfigService(repo repository.VendorConfigRepository, publisher pubsub.Publisher, log *zap.Logger) *configService {
	return &configService{
		repo:      repo,
		publisher: publisher,
		log:       log,
	}
}

// WithMigrationManager wires the MigrationManager for orchestration migration support.
func (s *configService) WithMigrationManager(mgr *MigrationManager) *configService {
	s.migrationMgr = mgr
	return s
}

func (s *configService) GetVendorConfigs(ctx context.Context, apiKeyID *string) ([]*domain.VendorConfig, error) {
	return s.repo.ListActive(ctx, apiKeyID)
}

func (s *configService) GetVendorConfig(ctx context.Context, vendorType string, apiKeyID *string) (*domain.VendorConfig, error) {
	return s.repo.GetByType(ctx, vendorType, apiKeyID)
}

func (s *configService) UpdateVendorConfig(ctx context.Context, vendorType string, configJSON json.RawMessage, apiKeyID *string) error {
	// 1. Validate JSON
	if !json.Valid(configJSON) {
		return fmt.Errorf("invalid JSON for vendor config")
	}

	// 1b. Vendor-specific validation
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
	case workflowOrchestrationVendorType:
		// Validate workflow orchestration config structure.
		var wfCfg workflowOrchestrationConfig
		if err := json.Unmarshal(configJSON, &wfCfg); err != nil {
			return fmt.Errorf("invalid workflow orchestration config: %w", err)
		}
		provider := wfCfg.Provider
		hostPort := ""
		namespace := ""
		switch provider {
		case "cadence":
			hostPort = normalizeHostPort(wfCfg.Cadence.HostPort)
			namespace = wfCfg.Cadence.Domain
		case "go_routines":
			// go_routines uses in-process goroutines — no host/port needed.
		case "standalone":
			// standalone mode means no workflow engine — nothing to migrate or validate.
		default:
			provider = "temporal"
			hostPort = normalizeHostPort(wfCfg.Temporal.HostPort)
			namespace = wfCfg.Temporal.Namespace
		}
		if provider != "standalone" && provider != "go_routines" && (hostPort == "" || namespace == "") {
			return fmt.Errorf("workflow orchestration config requires host_port and namespace/domain for provider %s", provider)
		}
	}

	// 2. Fetch old config before overwriting (for migration detection).
	var oldConfigJSON json.RawMessage
	if vendorType == workflowOrchestrationVendorType && s.migrationMgr != nil {
		oldVC, err := s.repo.GetByType(ctx, vendorType, apiKeyID)
		if err == nil && oldVC != nil && len(oldVC.ConfigJSON) > 0 {
			oldConfigJSON = oldVC.ConfigJSON
		}
	}

	// 3. Persist to DB
	cfg := &domain.VendorConfig{
		VendorType: vendorType,
		ConfigJSON: configJSON,
		IsActive:   true,
	}
	if err := s.repo.Upsert(ctx, cfg, apiKeyID); err != nil {
		return err
	}

	// 4. Trigger orchestration migration if the config changed meaningfully.
	if vendorType == workflowOrchestrationVendorType && s.migrationMgr != nil && len(oldConfigJSON) > 0 {
		if hasOrchestrationChanged(oldConfigJSON, configJSON) {
			mgr := s.migrationMgr
			// Fire migration asynchronously so the HTTP response is not blocked.
			go func() {
				if err := mgr.TriggerMigration(context.Background(), apiKeyID, oldConfigJSON, configJSON); err != nil {
					s.log.Error("failed to trigger orchestration migration",
						zap.String("api_key_id", safeString(apiKeyID)),
						zap.Error(err),
					)
				}
			}()
		}
	}

	// 5. Signal change via Pub/Sub
	msg := &pubsub.Message{
		NotificationID: "config-reload-" + vendorType,
		Channel:        "config",
		Payload: map[string]string{
			"vendor_type": vendorType,
		},
	}

	_, err := s.publisher.Publish(ctx, "config", msg)
	if err != nil {
		s.log.Error("failed to publish config reload event", zap.Error(err), zap.String("vendor", vendorType))
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

// ── AutoScaler Config ──────────────────────────────────────────────────────────

// autoscalerConfigJSON is the JSON shape stored in vendor_configs.config_json.
type autoscalerConfigJSON struct {
	Enabled            bool    `json:"enabled"`
	EvaluationInterval string  `json:"evaluation_interval"`
	HighMTTDThreshold  string  `json:"high_mttd_threshold"`
	MediumMTTDThreshold string `json:"medium_mttd_threshold"`
	LowMTTDThreshold   string  `json:"low_mttd_threshold"`
	MaxLagPerWorker    int     `json:"max_lag_per_worker"`
	CooldownPeriod     string  `json:"cooldown_period"`
	ScaleDownFactor    float64 `json:"scale_down_factor"`
	IdleThreshold      string  `json:"idle_threshold"`
	UnhealthyThreshold int     `json:"unhealthy_threshold"`
	GlobalMaxWorkers   int     `json:"global_max_workers"`
	GlobalMinWorkers   int     `json:"global_min_workers"`
	MTTDLookback       string  `json:"mttd_lookback"`
}

// defaultAutoScalerConfig returns default values, used when no config stored yet.
func defaultAutoScalerConfig() *nsconfig.AutoScalerConfig {
	return &nsconfig.AutoScalerConfig{
		Enabled:             false,
		EvaluationInterval:  15 * time.Second,
		HighMTTDThreshold:   5 * time.Second,
		MediumMTTDThreshold: 10 * time.Second,
		LowMTTDThreshold:    15 * time.Second,
		MaxLagPerWorker:     50,
		CooldownPeriod:      60 * time.Second,
		ScaleDownFactor:     0.5,
		IdleThreshold:       5 * time.Minute,
		UnhealthyThreshold:  6,
		GlobalMaxWorkers:    20,
		GlobalMinWorkers:    1,
		MTTDLookback:        3 * time.Minute,
	}
}

func parseDuration(s string, fallback time.Duration) time.Duration {
	if s == "" {
		return fallback
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return fallback
	}
	return d
}

func (s *configService) GetAutoScalerConfig(ctx context.Context) (*nsconfig.AutoScalerConfig, error) {
	defaults := defaultAutoScalerConfig()

	vc, err := s.repo.GetByType(ctx, autoscalerConfigVendorType, nil)
	if err != nil || vc == nil || len(vc.ConfigJSON) == 0 {
		// No stored config → return defaults
		return defaults, nil
	}

	var stored autoscalerConfigJSON
	if err := json.Unmarshal(vc.ConfigJSON, &stored); err != nil {
		return defaults, nil
	}

	return &nsconfig.AutoScalerConfig{
		Enabled:              stored.Enabled,
		EvaluationInterval:   parseDuration(stored.EvaluationInterval, defaults.EvaluationInterval),
		HighMTTDThreshold:    parseDuration(stored.HighMTTDThreshold, defaults.HighMTTDThreshold),
		MediumMTTDThreshold:   parseDuration(stored.MediumMTTDThreshold, defaults.MediumMTTDThreshold),
		LowMTTDThreshold:     parseDuration(stored.LowMTTDThreshold, defaults.LowMTTDThreshold),
		MaxLagPerWorker:      intPtrOr(stored.MaxLagPerWorker, defaults.MaxLagPerWorker, 1),
		CooldownPeriod:       parseDuration(stored.CooldownPeriod, defaults.CooldownPeriod),
		ScaleDownFactor:      float64PtrOr(stored.ScaleDownFactor, defaults.ScaleDownFactor, 0.1),
		IdleThreshold:        parseDuration(stored.IdleThreshold, defaults.IdleThreshold),
		UnhealthyThreshold:   intPtrOr(stored.UnhealthyThreshold, defaults.UnhealthyThreshold, 1),
		GlobalMaxWorkers:     intPtrOr(stored.GlobalMaxWorkers, defaults.GlobalMaxWorkers, 1),
		GlobalMinWorkers:     intPtrOr(stored.GlobalMinWorkers, defaults.GlobalMinWorkers, 1),
		MTTDLookback:         parseDuration(stored.MTTDLookback, defaults.MTTDLookback),
	}, nil
}

func intPtrOr(v, fallback, min int) int {
	if v <= 0 {
		return fallback
	}
	return v
}

func float64PtrOr(v, fallback, min float64) float64 {
	if v <= 0 {
		return fallback
	}
	return v
}

func (s *configService) UpdateAutoScalerConfig(ctx context.Context, cfg *nsconfig.AutoScalerConfig) error {
	stored := autoscalerConfigJSON{
		Enabled:             cfg.Enabled,
		EvaluationInterval:  cfg.EvaluationInterval.String(),
		HighMTTDThreshold:   cfg.HighMTTDThreshold.String(),
		MediumMTTDThreshold:  cfg.MediumMTTDThreshold.String(),
		LowMTTDThreshold:    cfg.LowMTTDThreshold.String(),
		MaxLagPerWorker:     cfg.MaxLagPerWorker,
		CooldownPeriod:      cfg.CooldownPeriod.String(),
		ScaleDownFactor:     cfg.ScaleDownFactor,
		IdleThreshold:       cfg.IdleThreshold.String(),
		UnhealthyThreshold:  cfg.UnhealthyThreshold,
		GlobalMaxWorkers:    cfg.GlobalMaxWorkers,
		GlobalMinWorkers:    cfg.GlobalMinWorkers,
		MTTDLookback:        cfg.MTTDLookback.String(),
	}

	b, err := json.Marshal(stored)
	if err != nil {
		return fmt.Errorf("marshal autoscaler config: %w", err)
	}

	vc := &domain.VendorConfig{
		VendorType: autoscalerConfigVendorType,
		ConfigJSON: b,
		IsActive:   true,
	}
	if err := s.repo.Upsert(ctx, vc, nil); err != nil {
		return fmt.Errorf("upsert autoscaler config: %w", err)
	}

	// Publish notification for live reload
	msg := &pubsub.Message{
		NotificationID: "config-reload-autoscaler",
		Channel:        "config",
		Payload: map[string]string{
			"vendor_type": autoscalerConfigVendorType,
		},
	}
	_, pubErr := s.publisher.Publish(ctx, "config", msg)
	if pubErr != nil {
		s.log.Warn("failed to publish autoscaler config reload event", zap.Error(pubErr))
	}

	s.log.Info("autoscaler config updated",
		zap.Bool("enabled", cfg.Enabled),
		zap.Duration("evaluation_interval", cfg.EvaluationInterval),
		zap.Duration("high_threshold", cfg.HighMTTDThreshold),
	)
	return nil
}

// hasOrchestrationChanged compares old and new workflow orchestration configs
// to determine whether a migration is needed.
func hasOrchestrationChanged(oldJSON, newJSON json.RawMessage) bool {
	var oldCfg, newCfg workflowOrchestrationConfig
	if err := json.Unmarshal(oldJSON, &oldCfg); err != nil {
		return false
	}
	if err := json.Unmarshal(newJSON, &newCfg); err != nil {
		return false
	}

	oldProvider := oldCfg.Provider
	newProvider := newCfg.Provider

	// Determine effective host/port and namespace for comparison.
	oldHost, oldNS := extractEndpoint(oldCfg)
	newHost, newNS := extractEndpoint(newCfg)

	return oldProvider != newProvider || oldHost != newHost || oldNS != newNS
}

func extractEndpoint(cfg workflowOrchestrationConfig) (hostPort, namespace string) {
	switch cfg.Provider {
	case "cadence":
		return normalizeHostPort(cfg.Cadence.HostPort), cfg.Cadence.Domain
	case "go_routines":
		return "go_routines", "inline"
	case "standalone":
		return "", ""
	default:
		return normalizeHostPort(cfg.Temporal.HostPort), cfg.Temporal.Namespace
	}
}
