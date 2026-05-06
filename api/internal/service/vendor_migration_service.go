package service

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/spidey/notification-service/internal/domain"
	"github.com/spidey/notification-service/internal/pubsub"
	"github.com/spidey/notification-service/internal/repository"
	"go.uber.org/zap"
)

// VendorMigrationService manages vendor-to-vendor (and same-vendor config) migrations
// while protecting sender reputation by preserving rollback state and enabling
// gradual traffic shifting via existing routing config.
type VendorMigrationService interface {
	Start(ctx context.Context, req *domain.StartVendorMigrationRequest, apiKeyID *string) (*domain.VendorMigration, error)
	Complete(ctx context.Context, id uuid.UUID) error
	Rollback(ctx context.Context, id uuid.UUID) error
	GetByID(ctx context.Context, id uuid.UUID) (*domain.VendorMigration, error)
	List(ctx context.Context, apiKeyID *string, channel string, status string) ([]*domain.VendorMigration, error)
}

type vendorMigrationService struct {
	migrationRepo repository.VendorMigrationRepository
	configRepo    repository.VendorConfigRepository
	configSvc     ConfigService
	publisher     pubsub.Publisher
	log           *zap.Logger
}

func NewVendorMigrationService(
	migrationRepo repository.VendorMigrationRepository,
	configRepo repository.VendorConfigRepository,
	configSvc ConfigService,
	publisher pubsub.Publisher,
	log *zap.Logger,
) VendorMigrationService {
	return &vendorMigrationService{
		migrationRepo: migrationRepo,
		configRepo:    configRepo,
		configSvc:     configSvc,
		publisher:     publisher,
		log:           log,
	}
}

// Start initiates a vendor migration.
//
// For cross-vendor migrations the new vendor config is applied immediately and the
// routing config is updated so the new vendor is preferred with the old as fallback
// (gradual strategy) or set as the only provider (instant strategy).
//
// For same-vendor config swaps the credentials are hot-swapped in-place; the
// provider name in notification_attempts stays the same so all-time reputation
// metrics remain continuous.
func (s *vendorMigrationService) Start(ctx context.Context, req *domain.StartVendorMigrationRequest, apiKeyID *string) (*domain.VendorMigration, error) {
	if err := validateMigrationRequest(req); err != nil {
		return nil, err
	}

	// Snapshot old config for rollback — tolerate missing (first-time setup).
	var fromConfigJSON json.RawMessage
	if oldCfg, err := s.configRepo.GetByType(ctx, req.FromVendor, apiKeyID); err == nil && oldCfg != nil {
		fromConfigJSON = oldCfg.ConfigJSON
	}

	// Apply the new vendor credentials immediately.
	if err := s.configSvc.UpdateVendorConfig(ctx, req.ToVendor, req.ToConfigJSON, apiKeyID); err != nil {
		return nil, fmt.Errorf("applying new vendor config: %w", err)
	}

	isSameVendor := req.FromVendor == req.ToVendor
	strategy := req.Strategy
	if strategy == "" {
		strategy = "instant"
	}

	// For cross-vendor migrations, adjust the channel routing config so traffic
	// is steered toward the new vendor.  Same-vendor swaps need no routing change
	// because the provider key in notification_attempts remains unchanged.
	if !isSameVendor {
		if err := s.applyRoutingChange(ctx, req.Channel, req.FromVendor, req.ToVendor, strategy, apiKeyID); err != nil {
			// Log but don't fail — routing update is best-effort; the config was already applied.
			s.log.Warn("failed to update routing config after migration start",
				zap.String("channel", req.Channel),
				zap.String("from", req.FromVendor),
				zap.String("to", req.ToVendor),
				zap.Error(err),
			)
		}
	}

	trafficPercent := 100
	if strategy == "gradual" {
		// Gradual: new vendor is preferred with old as fallback — not yet 100%.
		trafficPercent = 50
	}

	var apiKeyUUID *uuid.UUID
	if apiKeyID != nil && *apiKeyID != "" {
		u, err := uuid.Parse(*apiKeyID)
		if err == nil {
			apiKeyUUID = &u
		}
	}

	m := &domain.VendorMigration{
		APIKeyID:       apiKeyUUID,
		Channel:        req.Channel,
		FromVendor:     req.FromVendor,
		ToVendor:       req.ToVendor,
		FromConfigJSON: fromConfigJSON,
		ToConfigJSON:   req.ToConfigJSON,
		Strategy:       strategy,
		Status:         domain.VendorMigrationInProgress,
		TrafficPercent: trafficPercent,
	}

	if err := s.migrationRepo.Create(ctx, m); err != nil {
		return nil, fmt.Errorf("saving vendor migration record: %w", err)
	}

	s.log.Info("vendor migration started",
		zap.String("id", m.ID.String()),
		zap.String("channel", m.Channel),
		zap.String("from", m.FromVendor),
		zap.String("to", m.ToVendor),
		zap.String("strategy", m.Strategy),
	)
	return m, nil
}

// Complete finalises a gradual migration by removing the old vendor from routing
// and marking the record completed.  Only valid for in-progress gradual migrations.
func (s *vendorMigrationService) Complete(ctx context.Context, id uuid.UUID) error {
	m, err := s.migrationRepo.GetByID(ctx, id)
	if err != nil || m == nil {
		return fmt.Errorf("migration not found: %w", err)
	}
	if m.Status != domain.VendorMigrationInProgress {
		return fmt.Errorf("migration is not in progress (current status: %s)", m.Status)
	}

	// For cross-vendor gradual migrations: lock routing to only the new vendor.
	if !m.IsSameVendor() {
		var apiKeyIDStr *string
		if m.APIKeyID != nil {
			s := m.APIKeyID.String()
			apiKeyIDStr = &s
		}
		if err := s.lockRoutingToNewVendor(ctx, m.Channel, m.ToVendor, apiKeyIDStr); err != nil {
			s.log.Warn("failed to lock routing to new vendor on complete",
				zap.String("migration_id", id.String()),
				zap.Error(err),
			)
		}
	}

	now := time.Now()
	if err := s.migrationRepo.UpdateStatus(ctx, id, domain.VendorMigrationCompleted, nil, &now); err != nil {
		return fmt.Errorf("marking migration completed: %w", err)
	}

	s.log.Info("vendor migration completed", zap.String("id", id.String()))
	return nil
}

// Rollback restores the previous vendor config and routing, then marks the
// migration as rolled_back.  Only valid for in-progress migrations.
func (s *vendorMigrationService) Rollback(ctx context.Context, id uuid.UUID) error {
	m, err := s.migrationRepo.GetByID(ctx, id)
	if err != nil || m == nil {
		return fmt.Errorf("migration not found: %w", err)
	}
	if m.Status != domain.VendorMigrationInProgress {
		return fmt.Errorf("migration is not in progress (current status: %s)", m.Status)
	}

	var apiKeyIDStr *string
	if m.APIKeyID != nil {
		s := m.APIKeyID.String()
		apiKeyIDStr = &s
	}

	// Restore old credentials when we have a snapshot.
	if len(m.FromConfigJSON) > 0 {
		if err := s.configSvc.UpdateVendorConfig(ctx, m.FromVendor, m.FromConfigJSON, apiKeyIDStr); err != nil {
			return fmt.Errorf("restoring old vendor config: %w", err)
		}
	}

	// Revert routing to prefer the old vendor again.
	if !m.IsSameVendor() {
		if err := s.revertRouting(ctx, m.Channel, m.FromVendor, apiKeyIDStr); err != nil {
			s.log.Warn("failed to revert routing on rollback",
				zap.String("migration_id", id.String()),
				zap.Error(err),
			)
		}
	}

	now := time.Now()
	if err := s.migrationRepo.UpdateStatus(ctx, id, domain.VendorMigrationRolledBack, nil, &now); err != nil {
		return fmt.Errorf("marking migration rolled_back: %w", err)
	}

	s.log.Info("vendor migration rolled back", zap.String("id", id.String()))
	return nil
}

func (s *vendorMigrationService) GetByID(ctx context.Context, id uuid.UUID) (*domain.VendorMigration, error) {
	return s.migrationRepo.GetByID(ctx, id)
}

func (s *vendorMigrationService) List(ctx context.Context, apiKeyID *string, channel string, status string) ([]*domain.VendorMigration, error) {
	return s.migrationRepo.List(ctx, apiKeyID, channel, status)
}

// ── Routing helpers ───────────────────────────────────────────────────────────

// routingVendorType returns the vendor_type key used for routing config for a given channel.
func routingVendorType(channel string) string {
	switch channel {
	case "sms":
		return "sms_routing"
	case "push":
		return "push_routing"
	default:
		return "email_routing"
	}
}

// applyRoutingChange reads the current routing config and updates prefer/fallback
// to steer traffic toward the new vendor.
//   - gradual:  prefer=to, fallback=from  (workers auto-fallback on new-vendor errors)
//   - instant:  prefer=to, fallback=from  (same shape; Complete call locks it to only=to)
func (s *vendorMigrationService) applyRoutingChange(
	ctx context.Context,
	channel, fromVendor, toVendor, strategy string,
	apiKeyID *string,
) error {
	routingType := routingVendorType(channel)
	current, err := s.configRepo.GetByType(ctx, routingType, apiKeyID)
	if err != nil {
		return fmt.Errorf("reading current routing config: %w", err)
	}

	routing := map[string]interface{}{
		"mode":     "backup",
		"prefer":   toVendor,
		"fallback": fromVendor,
	}
	if current != nil && len(current.ConfigJSON) > 0 {
		var existing map[string]interface{}
		if jsonErr := json.Unmarshal(current.ConfigJSON, &existing); jsonErr == nil {
			// Merge: keep existing fields (participants, thresholds) and override prefer/fallback/mode.
			for k, v := range existing {
				routing[k] = v
			}
		}
	}
	routing["mode"] = "backup"
	routing["prefer"] = toVendor
	routing["fallback"] = fromVendor

	updated, err := json.Marshal(routing)
	if err != nil {
		return err
	}
	return s.configSvc.UpdateVendorConfig(ctx, routingType, updated, apiKeyID)
}

// lockRoutingToNewVendor sets the routing to only use the new vendor (called on Complete).
func (s *vendorMigrationService) lockRoutingToNewVendor(
	ctx context.Context,
	channel, toVendor string,
	apiKeyID *string,
) error {
	routingType := routingVendorType(channel)
	current, _ := s.configRepo.GetByType(ctx, routingType, apiKeyID)

	routing := map[string]interface{}{
		"mode":   "only",
		"prefer": toVendor,
		"only":   toVendor,
	}
	if current != nil && len(current.ConfigJSON) > 0 {
		var existing map[string]interface{}
		if json.Unmarshal(current.ConfigJSON, &existing) == nil {
			for k, v := range existing {
				routing[k] = v
			}
		}
	}
	routing["mode"] = "only"
	routing["prefer"] = toVendor
	routing["only"] = toVendor

	updated, err := json.Marshal(routing)
	if err != nil {
		return err
	}
	return s.configSvc.UpdateVendorConfig(ctx, routingType, updated, apiKeyID)
}

// revertRouting restores routing to prefer the old vendor.
func (s *vendorMigrationService) revertRouting(
	ctx context.Context,
	channel, fromVendor string,
	apiKeyID *string,
) error {
	routingType := routingVendorType(channel)
	current, _ := s.configRepo.GetByType(ctx, routingType, apiKeyID)

	routing := map[string]interface{}{
		"mode":   "backup",
		"prefer": fromVendor,
	}
	if current != nil && len(current.ConfigJSON) > 0 {
		var existing map[string]interface{}
		if json.Unmarshal(current.ConfigJSON, &existing) == nil {
			for k, v := range existing {
				routing[k] = v
			}
		}
	}
	routing["mode"] = "backup"
	routing["prefer"] = fromVendor

	updated, err := json.Marshal(routing)
	if err != nil {
		return err
	}
	return s.configSvc.UpdateVendorConfig(ctx, routingType, updated, apiKeyID)
}

// ── Validation ────────────────────────────────────────────────────────────────

var validChannels = map[string]bool{"email": true, "sms": true, "push": true}
var validStrategies = map[string]bool{"": true, "instant": true, "gradual": true}

func validateMigrationRequest(req *domain.StartVendorMigrationRequest) error {
	if !validChannels[req.Channel] {
		return fmt.Errorf("invalid channel %q: must be email, sms, or push", req.Channel)
	}
	if req.FromVendor == "" {
		return fmt.Errorf("from_vendor is required")
	}
	if req.ToVendor == "" {
		return fmt.Errorf("to_vendor is required")
	}
	if !validStrategies[req.Strategy] {
		return fmt.Errorf("invalid strategy %q: must be instant or gradual", req.Strategy)
	}
	if !json.Valid(req.ToConfigJSON) {
		return fmt.Errorf("to_config must be valid JSON")
	}
	return nil
}
