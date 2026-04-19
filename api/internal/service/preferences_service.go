package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/spidey/notification-service/internal/cache"
	"github.com/spidey/notification-service/internal/domain"
)

const (
	prefsCacheTTL = 5 * time.Minute
)

// PreferencesService manages user notification preferences.
// Preferences are persisted in Redis (acting as primary store for this fast path).
// A production system would use Firestore/DynamoDB as primary with Redis as cache.
type PreferencesService struct {
	cache *cache.Client
}

func NewPreferencesService(cacheClient *cache.Client) *PreferencesService {
	return &PreferencesService{cache: cacheClient}
}

func prefsKey(userID string) string {
	return fmt.Sprintf("prefs:user:%s", userID)
}

// Get returns user preferences. Returns permissive defaults if not set.
func (s *PreferencesService) Get(ctx context.Context, userID string) (*domain.UserPreferences, error) {
	var prefs domain.UserPreferences
	if err := s.cache.Get(ctx, prefsKey(userID), &prefs); err != nil {
		if !errors.Is(err, cache.ErrCacheMiss) {
			return nil, fmt.Errorf("getting preferences for %s: %w", userID, err)
		}
		// Default: all channels enabled, no DND
		return &domain.UserPreferences{
			UserID:    userID,
			Channels:  map[domain.Channel]bool{},
			UpdatedAt: time.Now(),
		}, nil
	}
	return &prefs, nil
}

// Set saves user preferences.
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

	data, err := json.Marshal(existing)
	if err != nil {
		return fmt.Errorf("marshalling preferences: %w", err)
	}

	return s.cache.Set(ctx, prefsKey(userID), data, 0) // no TTL — persists until changed
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
