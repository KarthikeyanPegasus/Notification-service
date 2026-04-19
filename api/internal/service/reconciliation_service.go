package service

import (
	"context"
	"time"

	"github.com/spidey/notification-service/internal/domain"
	"github.com/spidey/notification-service/internal/repository"
	"go.uber.org/zap"
)

// ReconciliationService checks for stuck notifications and reconciles their states.
type ReconciliationService struct {
	notifRepo *repository.NotificationRepository
	log       *zap.Logger
}

func NewReconciliationService(notifRepo *repository.NotificationRepository, log *zap.Logger) *ReconciliationService {
	return &ReconciliationService{
		notifRepo: notifRepo,
		log:       log,
	}
}

// Start begins the reconciliation background loop.
func (s *ReconciliationService) Start(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			s.log.Info("stopping reconciliation service")
			return
		case <-ticker.C:
			s.reconcile(ctx)
		}
	}
}

func (s *ReconciliationService) reconcile(ctx context.Context) {
	s.log.Info("starting reconciliation cycle")
	
	// Query notifications stuck for over 2 hours
	stuckThreshold := 2 * time.Hour
	stuckNotifs, err := s.notifRepo.GetStuckNotifications(ctx, stuckThreshold, 100)
	if err != nil {
		s.log.Error("failed to query stuck notifications", zap.Error(err))
		return
	}

	if len(stuckNotifs) == 0 {
		return
	}

	s.log.Info("found stuck notifications", zap.Int("count", len(stuckNotifs)))

	for _, n := range stuckNotifs {
		// Log and theoretically we would ping the provider APIs (SES, Twilio, etc) here to fetch real status.
		// For now, if it's stuck for 24+ hours, we mark it failed. Otherwise we just log a warning.
		if time.Since(n.UpdatedAt) > 24*time.Hour {
			s.log.Warn("notification stuck for over 24 hours, marking as failed", zap.String("id", n.ID.String()))
			_ = s.notifRepo.UpdateStatus(ctx, n.ID, domain.StatusFailed)
		} else {
			s.log.Warn("notification still stuck after 2 hours", zap.String("id", n.ID.String()), zap.String("status", string(n.Status)))
		}
	}
}
