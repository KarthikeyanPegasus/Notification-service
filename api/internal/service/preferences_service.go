package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/spidey/notification-service/internal/cache"
	"github.com/spidey/notification-service/internal/domain"
	"github.com/spidey/notification-service/internal/repository"
	"go.uber.org/zap"
)

const (
	prefsCacheTTL = 5 * time.Minute
)

// PreferencesService manages user notification preferences.
// PostgreSQL is the primary store; Redis is used as a read cache for fast access.
type PreferencesService struct {
	dbRepo *repository.UserPreferencesRepository
	cache   *cache.Client
	log     *zap.Logger
}

func NewPreferencesService(dbRepo *repository.UserPreferencesRepository, cacheClient *cache.Client, log *zap.Logger) *PreferencesService {
	return &PreferencesService{dbRepo: dbRepo, cache: cacheClient, log: log}
}

func prefsKey(userID string) string {
	return fmt.Sprintf("prefs:user:%s", userID)
}

// Get returns user preferences. Falls back to DB on cache miss. Returns permissive defaults if not set.
func (s *PreferencesService) Get(ctx context.Context, userID string) (*domain.UserPreferences, error) {
	// Try cache first
	var prefs domain.UserPreferences
	if err := s.cache.Get(ctx, prefsKey(userID), &prefs); err == nil {
		return &prefs, nil
	} else if !errors.Is(err, cache.ErrCacheMiss) {
		// Real cache failure — log a warning but continue to DB
		s.log.Warn("preferences cache fetch failed, falling back to DB",
			zap.String("user_id", userID),
			zap.Error(err),
		)
	}

	// Cache miss or error — fall back to PostgreSQL
	if s.dbRepo != nil {
		dbCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		dbPrefs, err := s.dbRepo.Get(dbCtx, userID)
		if err != nil {
			s.log.Warn("preferences DB fetch failed, using permissive defaults (fail-open)",
				zap.String("user_id", userID),
				zap.Error(err),
			)
			return &domain.UserPreferences{
				UserID:    userID,
				Channels:  map[domain.Channel]bool{},
				UpdatedAt: time.Now(),
			}, nil
		}
		if dbPrefs != nil {
			// Hydrate cache asynchronously
			if data, err := json.Marshal(dbPrefs); err == nil {
				_ = s.cache.Set(context.Background(), prefsKey(userID), data, prefsCacheTTL)
			}
			return dbPrefs, nil
		}
	}

	// No preferences found — return permissive defaults
	return &domain.UserPreferences{
		UserID:    userID,
		Channels:  map[domain.Channel]bool{},
		UpdatedAt: time.Now(),
	}, nil
}

// Set saves user preferences to both PostgreSQL (primary) and Redis (cache).
func (s *PreferencesService) Set(ctx context.Context, userID string, req *domain.UpdatePreferencesRequest) error {
	existing, err := s.Get(ctx, userID)
	if err != nil {
		return err
	}

	if existing.Channels == nil {
		existing.Channels = make(map[domain.Channel]bool)
	}

	for ch, enabled := range req.Channels {
		existing.Channels[ch] = enabled
	}

	if req.DoNotDisturb != nil {
		existing.DoNotDisturb = req.DoNotDisturb
	}

	existing.UserID = userID
	existing.UpdatedAt = time.Now()

	// Write to PostgreSQL (primary store)
	if s.dbRepo != nil {
		storeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		if err := s.dbRepo.Upsert(storeCtx, userID, existing); err != nil {
			return fmt.Errorf("persisting preferences to DB: %w", err)
		}
	}

	// Write to Redis (cache)
	data, err := json.Marshal(existing)
	if err != nil {
		return fmt.Errorf("marshalling preferences: %w", err)
	}
	return s.cache.Set(ctx, prefsKey(userID), data, prefsCacheTTL)
}

// IsInDND checks if the current time falls within the user's DND window.
func (s *PreferencesService) IsInDND(prefs *domain.UserPreferences) bool {
	if prefs == nil || prefs.DoNotDisturb == nil || !prefs.DoNotDisturb.Enabled {
		return false
	}

	dnd := prefs.DoNotDisturb
	loc, err := time.LoadLocation(dnd.Timezone)
	if err != nil {
		loc = time.UTC
	}

	now := time.Now().In(loc)
	hour := now.Hour()

	if dnd.StartHour <= dnd.EndHour {
		return hour >= dnd.StartHour && hour < dnd.EndHour
	}
	// Wraps midnight e.g. 22 → 8
	return hour >= dnd.StartHour || hour < dnd.EndHour
}

// IsRateLimited returns true if the user has exceeded frequency caps for promotional messages.
func (s *PreferencesService) IsRateLimited(ctx context.Context, userID string, channel domain.Channel, notifType string) (bool, error) {
	prefs, err := s.Get(ctx, userID)
	if err != nil {
		return false, err
	}

	cap, ok := prefs.FrequencyCaps[string(channel)+":"+notifType]
	if !ok || cap <= 0 {
		return false, nil
	}

	today := time.Now().UTC().Format("2006-01-02")
	key := fmt.Sprintf("rate:%s:%s:%s:%s", userID, channel, notifType, today)

	count, err := s.cache.Incr(ctx, key)
	if err != nil {
		return false, err
	}
	if count == 1 {
		_ = s.cache.Expire(ctx, key, 24*time.Hour)
	}

	return count > int64(cap), nil
}
