// Package ratelimit provides per-vendor throughput enforcement using Redis sliding windows.
// All configured limit windows (RPS, per_minute, per_10_min, per_hour, per_day) must pass
// before a notification is sent; if any window is exhausted, Allow returns false.
package ratelimit

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/spidey/notification-service/internal/domain"
	"go.uber.org/zap"
)

// Window defines a sliding-window rate limit bucket.
type Window struct {
	Size  time.Duration // bucket window size
	Limit int           // max requests in the window
}

// VendorLimiter enforces per-vendor rate limits using Redis sorted sets
// (sliding window log approach). It is safe for concurrent use.
type VendorLimiter struct {
	rdb *redis.Client
	log *zap.Logger
}

func NewVendorLimiter(rdb *redis.Client, log *zap.Logger) *VendorLimiter {
	return &VendorLimiter{rdb: rdb, log: log}
}

// Allow checks whether a send to the given vendor (optionally scoped to clientID) is permitted
// under the configured rate limit. Returns (true, nil) when permitted, (false, nil) when throttled,
// or (false, err) on Redis errors (which default to allowing the request to avoid false negatives).
func (l *VendorLimiter) Allow(ctx context.Context, vendorName, clientID string, rl *domain.VendorRateLimit) (bool, error) {
	if rl == nil || !rl.IsActive {
		return true, nil
	}

	scope := vendorName
	if clientID != "" {
		scope = vendorName + ":" + clientID
	}

	now := time.Now()

	// Check all configured windows; all must pass.
	windows := l.buildWindows(rl)
	for _, w := range windows {
		allowed, err := l.checkWindow(ctx, scope, w.Size, w.Limit, now)
		if err != nil {
			// Redis errors are non-fatal: allow the request to avoid blocking delivery.
			l.log.Warn("rate limit check error — allowing request",
				zap.String("vendor", vendorName),
				zap.Duration("window", w.Size),
				zap.Error(err),
			)
			continue
		}
		if !allowed {
			l.log.Info("vendor rate limit exceeded",
				zap.String("vendor", vendorName),
				zap.String("client_id", clientID),
				zap.Duration("window", w.Size),
				zap.Int("limit", w.Limit),
			)
			return false, nil
		}
	}

	return true, nil
}

// buildWindows converts a VendorRateLimit into the set of windows to enforce.
func (l *VendorLimiter) buildWindows(rl *domain.VendorRateLimit) []Window {
	var windows []Window

	if rl.RPS != nil && *rl.RPS > 0 {
		// Convert RPS to "N requests per 1s window"
		limit := int(math.Ceil(*rl.RPS))
		windows = append(windows, Window{Size: time.Second, Limit: limit})
	}
	if rl.PerMinute != nil && *rl.PerMinute > 0 {
		windows = append(windows, Window{Size: time.Minute, Limit: *rl.PerMinute})
	}
	if rl.Per10Min != nil && *rl.Per10Min > 0 {
		windows = append(windows, Window{Size: 10 * time.Minute, Limit: *rl.Per10Min})
	}
	if rl.PerHour != nil && *rl.PerHour > 0 {
		windows = append(windows, Window{Size: time.Hour, Limit: *rl.PerHour})
	}
	if rl.PerDay != nil && *rl.PerDay > 0 {
		windows = append(windows, Window{Size: 24 * time.Hour, Limit: *rl.PerDay})
	}

	return windows
}

// checkWindow implements a Redis sliding window using a sorted set.
// Key: ratelimit:{scope}:{windowSeconds}
// Each entry is a unique timestamp+nonce as the member, scored by Unix time.
// We remove expired entries, count the remainder, and conditionally add the new one.
func (l *VendorLimiter) checkWindow(ctx context.Context, scope string, size time.Duration, limit int, now time.Time) (bool, error) {
	windowSeconds := int64(size.Seconds())
	key := fmt.Sprintf("ratelimit:%s:%d", scope, windowSeconds)
	cutoff := now.Add(-size).UnixNano()
	member := fmt.Sprintf("%d", now.UnixNano())

	pipe := l.rdb.Pipeline()
	// Remove entries older than the window
	pipe.ZRemRangeByScore(ctx, key, "-inf", fmt.Sprintf("%d", cutoff))
	// Count current entries
	countCmd := pipe.ZCard(ctx, key)
	// Set expiry so the key cleans itself up
	pipe.Expire(ctx, key, size*2)

	if _, err := pipe.Exec(ctx); err != nil {
		return false, fmt.Errorf("redis pipeline: %w", err)
	}

	count := countCmd.Val()
	if int(count) >= limit {
		return false, nil
	}

	// Add this request's entry; score = nanosecond timestamp for ordering
	if err := l.rdb.ZAdd(ctx, key, redis.Z{
		Score:  float64(now.UnixNano()),
		Member: member,
	}).Err(); err != nil {
		return false, fmt.Errorf("redis zadd: %w", err)
	}

	return true, nil
}

// AllowWithRetryDelay returns the minimum wait duration before the next request will be allowed,
// useful for workers that want to back-pressure rather than drop messages.
func (l *VendorLimiter) AllowWithRetryDelay(ctx context.Context, vendorName, clientID string, rl *domain.VendorRateLimit) (bool, time.Duration, error) {
	if rl == nil || !rl.IsActive {
		return true, 0, nil
	}

	scope := vendorName
	if clientID != "" {
		scope = vendorName + ":" + clientID
	}

	now := time.Now()
	windows := l.buildWindows(rl)

	maxWait := time.Duration(0)
	for _, w := range windows {
		windowSeconds := int64(w.Size.Seconds())
		key := fmt.Sprintf("ratelimit:%s:%d", scope, windowSeconds)
		cutoff := now.Add(-w.Size).UnixNano()

		pipe := l.rdb.Pipeline()
		pipe.ZRemRangeByScore(ctx, key, "-inf", fmt.Sprintf("%d", cutoff))
		oldestCmd := pipe.ZRangeWithScores(ctx, key, 0, 0) // oldest entry
		countCmd := pipe.ZCard(ctx, key)
		if _, err := pipe.Exec(ctx); err != nil {
			return true, 0, nil // fail open
		}

		if int(countCmd.Val()) >= w.Limit {
			// Wait until the oldest entry exits the window
			oldest := oldestCmd.Val()
			if len(oldest) > 0 {
				oldestTs := time.Duration(int64(oldest[0].Score))
				windowEnd := time.Duration(now.UnixNano()) - time.Duration(cutoff) // time until oldest exits
				wait := time.Unix(0, int64(oldest[0].Score)).Add(w.Size).Sub(now)
				_ = oldestTs
				_ = windowEnd
				if wait > maxWait {
					maxWait = wait
				}
			} else {
				if w.Size > maxWait {
					maxWait = w.Size
				}
			}
			return false, maxWait, nil
		}
	}

	allowed, err := l.Allow(ctx, vendorName, clientID, rl)
	return allowed, 0, err
}
