package autoscaler

import (
	"context"
	"fmt"
	"math"
	"sync"
	"time"

	nsconfig "github.com/spidey/notification-service/internal/config"
	"github.com/spidey/notification-service/internal/repository"
	"go.uber.org/zap"
)

// ConfigProvider returns the current autoscaler config. Implementations
// should fetch from DB on each call so changes take effect without restart.
type ConfigProvider interface {
	GetAutoScalerConfig(ctx context.Context) (*nsconfig.AutoScalerConfig, error)
}

// scaleKey uniquely identifies a scaling target: one client's notifications
// for one channel at one priority level.
type scaleKey struct {
	ClientID string `json:"client_id"`
	Channel  string `json:"channel"`
	Priority string `json:"priority"`
}

func (k scaleKey) String() string {
	return fmt.Sprintf("%s/%s/%s", k.ClientID, k.Channel, k.Priority)
}

// MTTDProvider abstracts the DB query for per-client MTTD.
type MTTDProvider interface {
	GetMTTDByClientAndPriority(ctx context.Context, since time.Time) ([]repository.MTTDClientRow, error)
}

// LagProvider abstracts the Kafka consumer-lag query.
type LagProvider interface {
	GetConsumerLag(ctx context.Context) (map[string]int64, error)
	Close() error
}

// ScalingState holds the current desired parallelism for a scaleKey and
// metadata used for scale-down cooldowns.
type ScalingState struct {
	Desired      int32     `json:"desired"`
	LastScaleUp  time.Time `json:"last_scale_up"`
	LastScaleDown time.Time `json:"last_scale_down"`
	UnhealthyCount int     `json:"unhealthy_count"`
	UpdatedAt    time.Time `json:"updated_at"`
	// Reason records why the current desired count was chosen.
	Reason string `json:"reason"`
}

// AutoScaler evaluates MTTD and Kafka consumer lag on a periodic interval
// and computes the desired worker parallelism per (client, channel, priority).
//
// Scale-up triggers:
//   1. MTTD > SLA threshold → desired = max(desired, ceil(MTTD/threshold))
//   2. Kafka lag > MaxLagPerWorker → desired = max(desired, ceil(lag/MaxLagPerWorker))
//
// Scale-down triggers (all must be true):
//   1. CooldownPeriod has elapsed since last scale-up
//   2. MTTD < SLA_threshold * ScaleDownFactor
//   3. Kafka lag is zero or below threshold
//   4. No recent scale-down within a similar cooldown
//
// Idle scale-down:
//   - If no traffic seen for IdleThreshold → scale to GlobalMinWorkers
type AutoScaler struct {
	mu           sync.RWMutex
	states       map[scaleKey]*ScalingState
	cfgProv      ConfigProvider
	defaultCfg   nsconfig.AutoScalerConfig // fallback if provider unavailable
	mttdProv     MTTDProvider
	lagProv      LagProvider
	log          *zap.Logger
}

// NewAutoScaler creates a new AutoScaler.
// cfgProv provides runtime config (read each evaluation cycle).
// defaultCfg is used as fallback when cfgProv returns an error.
func NewAutoScaler(
	cfgProv ConfigProvider,
	defaultCfg *nsconfig.AutoScalerConfig,
	mttdProv MTTDProvider,
	lagProv LagProvider,
	log *zap.Logger,
) *AutoScaler {
	def := nsconfig.AutoScalerConfig{}
	if defaultCfg != nil {
		def = *defaultCfg
	}
	return &AutoScaler{
		states:     make(map[scaleKey]*ScalingState),
		cfgProv:    cfgProv,
		defaultCfg: def,
		mttdProv:   mttdProv,
		lagProv:    lagProv,
		log:        log.With(zap.String("component", "autoscaler")),
	}
}

// loadConfig fetches the current config from provider, falling back to defaults.
func (a *AutoScaler) loadConfig(ctx context.Context) *nsconfig.AutoScalerConfig {
	if a.cfgProv != nil {
		cfg, err := a.cfgProv.GetAutoScalerConfig(ctx)
		if err == nil && cfg != nil {
			return cfg
		}
		a.log.Warn("failed to load autoscaler config from provider, using defaults", zap.Error(err))
	}
	return &a.defaultCfg
}

// Run starts the autoscaler evaluation loop. Blocks until ctx is cancelled.
// The loop always runs; enabled/disabled is checked each cycle in evaluateAll.
func (a *AutoScaler) Run(ctx context.Context) {
	interval := 15 * time.Second
	initialCfg := a.loadConfig(ctx)
	if initialCfg != nil && initialCfg.EvaluationInterval > 0 {
		interval = initialCfg.EvaluationInterval
	}

	a.log.Info("autoscaler started",
		zap.Duration("interval", interval),
		zap.Float64("high_threshold_ms", initialCfg.SlaMsForPriority("high")),
		zap.Float64("medium_threshold_ms", initialCfg.SlaMsForPriority("medium")),
		zap.Float64("low_threshold_ms", initialCfg.SlaMsForPriority("low")),
		zap.Int("max_lag_per_worker", initialCfg.MaxLagPerWorker),
		zap.Int("min_workers", initialCfg.GlobalMinWorkers),
		zap.Int("max_workers", initialCfg.GlobalMaxWorkers),
	)

	t := time.NewTicker(interval)
	defer t.Stop()

	// Run an initial evaluation immediately.
	a.evaluateAll(ctx)

	for {
		select {
		case <-ctx.Done():
			a.log.Info("autoscaler stopped")
			return
		case <-t.C:
			a.evaluateAll(ctx)
		}
	}
}

// evaluateAll runs one full evaluation cycle:
//   1. Reload config from provider (dynamic runtime)
//   2. Fetch MTTD from DB
//   3. Fetch Kafka lag
//   4. Compute desired parallelism per client-channel-priority
//   5. Update states map
func (a *AutoScaler) evaluateAll(ctx context.Context) {
	cfg := a.loadConfig(ctx)
	if !cfg.Enabled {
		// If disabled at runtime, clear all states
		a.mu.Lock()
		a.states = make(map[scaleKey]*ScalingState)
		a.mu.Unlock()
		a.log.Info("autoscaler disabled at runtime — all states cleared")
		return
	}

	lookback := cfg.MTTDLookback
	if lookback <= 0 {
		lookback = 3 * time.Minute
	}
	since := time.Now().Add(-lookback)

	// 1. Fetch MTTD
	mttdRows, err := a.mttdProv.GetMTTDByClientAndPriority(ctx, since)
	if err != nil {
		a.log.Warn("failed to fetch MTTD data", zap.Error(err))
		// Don't bail — we might still have lag data to act on
		mttdRows = nil
	}

	// Build a lookup: clientID+priority → avgMs
	mttdMap := make(map[string]float64) // key: "clientID|priority"
	for _, row := range mttdRows {
		key := row.ClientID + "|" + row.Priority
		mttdMap[key] = row.AvgMs
	}

	// 2. Fetch Kafka lag
	lagMap := make(map[string]int64)
	if a.lagProv != nil {
		lagMap, err = a.lagProv.GetConsumerLag(ctx)
		if err != nil {
			a.log.Warn("failed to fetch Kafka lag", zap.Error(err))
			lagMap = nil
		}
	}
	// Build a lag lookup by (channel, priority)
	// lagMap keys are topic names like "notifications-email-high"
	lagByChannelPriority := make(map[string]int64) // key: "channel|priority"
	for topic, lag := range lagMap {
		ch, pr := parseTopicToChannelPriority(topic)
		if ch != "" {
			lagByChannelPriority[ch+"|"+pr] = lag
		}
	}

	// 3. Build the set of known scaleKeys from MTTD data + any existing states
	keys := make(map[scaleKey]bool)

	// Derive keys from MTTD rows
	for _, row := range mttdRows {
		// We don't know the channel from MTTD alone, so merge with existing
		// states or use a wildcard approach — iterate over all channels
		for _, ch := range allChannels() {
			sk := scaleKey{ClientID: row.ClientID, Channel: ch, Priority: row.Priority}
			keys[sk] = true
		}
	}

	// Merge in any existing keys (so we keep tracking even with zero traffic)
	a.mu.RLock()
	for k := range a.states {
		keys[k] = true
	}
	a.mu.RUnlock()

	// 4. Evaluate and update states
	now := time.Now()
	newStates := make(map[scaleKey]*ScalingState, len(keys))

	for sk := range keys {
		mttdKey := sk.ClientID + "|" + sk.Priority
		lagKey := sk.Channel + "|" + sk.Priority

		mttdMs := mttdMap[mttdKey]
		lag := lagByChannelPriority[lagKey]

		desired, reason := a.evaluate(sk, mttdMs, lag, now, cfg)

		// Merge with previous state for cooldown tracking
		prev := a.states[sk]
		state := &ScalingState{
			Desired:    desired,
			UpdatedAt:  now,
			Reason:     reason,
		}
		if prev != nil {
			state.LastScaleUp = prev.LastScaleUp
			state.LastScaleDown = prev.LastScaleDown
			state.UnhealthyCount = prev.UnhealthyCount
		}

		// Track scale-up events for cooldown
		if prev != nil && desired > prev.Desired {
			state.LastScaleUp = now
		}
		if prev != nil && desired < prev.Desired {
			state.LastScaleDown = now
		}

		// Track consecutive unhealthy cycles
		threshold := cfg.SlaMsForPriority(sk.Priority)
		if mttdMs > threshold && threshold > 0 {
			state.UnhealthyCount++
		} else if prev != nil {
			state.UnhealthyCount = 0
		}

		newStates[sk] = state

		a.log.Debug("autoscaler evaluation",
			zap.String("key", sk.String()),
			zap.Float64("mttd_ms", mttdMs),
			zap.Int64("lag", lag),
			zap.Int32("desired", desired),
			zap.String("reason", reason),
		)
	}

	a.mu.Lock()
	a.states = newStates
	a.mu.Unlock()

	// 5. Export metrics
	exportMetrics(newStates, lagByChannelPriority, mttdMap)
}

// evaluate computes the desired worker parallelism for a single scaleKey.
func (a *AutoScaler) evaluate(sk scaleKey, mttdMs float64, lag int64, now time.Time, cfg *nsconfig.AutoScalerConfig) (int32, string) {
	thresholdMs := cfg.SlaMsForPriority(sk.Priority)

	prev := a.states[sk]
	prevDesired := int32(1)
	if prev != nil {
		prevDesired = prev.Desired
	}

	// Start from base minimum
	desired := int32(cfg.GlobalMinWorkers)
	reasons := []string{}

	// ── Scale-up evaluation ────────────────────────────────────────────────

	// 1a. MTTD-based scale-up
	if mttdMs > thresholdMs && thresholdMs > 0 {
		factor := int32(math.Ceil(mttdMs / thresholdMs))
		if factor > desired {
			desired = factor
		}
		reasons = append(reasons, fmt.Sprintf("mttd=%.0fms > threshold=%.0fms → factor=%d", mttdMs, thresholdMs, factor))
	}

	// 1b. Kafka lag-based scale-up
	if cfg.MaxLagPerWorker > 0 && lag > int64(cfg.MaxLagPerWorker) {
		lagWorkers := int32(math.Ceil(float64(lag) / float64(cfg.MaxLagPerWorker)))
		if lagWorkers > desired {
			desired = lagWorkers
		}
		reasons = append(reasons, fmt.Sprintf("lag=%d > maxPerWorker=%d → workers=%d", lag, cfg.MaxLagPerWorker, lagWorkers))
	}

	// ── Scale-down evaluation ──────────────────────────────────────────────
	// We only scale down if all of these hold:
	//   a) Cooldown has elapsed since last scale-up
	//   b) MTTD is below threshold * ScaleDownFactor
	//   c) Lag is below MaxLagPerWorker (or lag scaling is disabled)
	//   d) Not in an unhealthy streak

	canScaleDown := true

	if prev != nil {
		// a) Cooldown check
		if !prev.LastScaleUp.IsZero() && now.Sub(prev.LastScaleUp) < cfg.CooldownPeriod {
			canScaleDown = false
			reasons = append(reasons, "cooldown_active")
		}

		// d) Unhealthy check
		if cfg.UnhealthyThreshold > 0 && prev.UnhealthyCount >= cfg.UnhealthyThreshold {
			canScaleDown = false
			reasons = append(reasons, "unhealthy_streak")
		}

		// Guard: if previous desired is already at minimum, no need to scale down further
		if prevDesired <= int32(cfg.GlobalMinWorkers) {
			canScaleDown = false
		}
	}

	// b) MTTD below scaledown threshold
	if thresholdMs > 0 && mttdMs > thresholdMs*cfg.ScaleDownFactor {
		canScaleDown = false
	}

	// c) Lag below threshold
	if cfg.MaxLagPerWorker > 0 && lag > int64(cfg.MaxLagPerWorker) {
		canScaleDown = false
	}

	// Also, if there's zero traffic — idle scale-down
	hasTraffic := mttdMs > 0
	if !hasTraffic && prev != nil {
		// Check how long we've had zero traffic
		idleThreshold := cfg.IdleThreshold
		if idleThreshold <= 0 {
			idleThreshold = 5 * time.Minute
		}
		// If we've been at minimum already, stay there
		if prevDesired > int32(cfg.GlobalMinWorkers) && prev.LastScaleDown.Add(idleThreshold).Before(now) {
			// Check if the last MTTD query returned any results for this client+priority
			// We need to check if this client has had any recent traffic at all
			desired = int32(cfg.GlobalMinWorkers)
			reasons = append(reasons, "idle_scale_down")
		}
	}

	if canScaleDown && desired > prevDesired {
		// Don't scale down if evaluation says we need MORE workers
		// Only scale down if desired is already <= prevDesired
	}

	// If we can scale down and the computed desired is less than current
	if canScaleDown && desired < prevDesired {
		reasons = append(reasons, fmt.Sprintf("scale_down from=%d to=%d", prevDesired, desired))
	} else if !canScaleDown && prevDesired > desired && desired >= int32(cfg.GlobalMinWorkers) {
		// Maintain current level if we can't scale down
		desired = prevDesired
	}

	// ── Clamp to absolute bounds ───────────────────────────────────────────
	if desired > int32(cfg.GlobalMaxWorkers) {
		desired = int32(cfg.GlobalMaxWorkers)
		reasons = append(reasons, fmt.Sprintf("clamped_to_max=%d", cfg.GlobalMaxWorkers))
	}
	if desired < int32(cfg.GlobalMinWorkers) {
		desired = int32(cfg.GlobalMinWorkers)
		reasons = append(reasons, fmt.Sprintf("clamped_to_min=%d", cfg.GlobalMinWorkers))
	}

	reason := "base_min"
	if len(reasons) > 0 {
		reason = ""
		for i, r := range reasons {
			if i > 0 {
				reason += "; "
			}
			reason += r
		}
	}

	return desired, reason
}

// GetDesiredParallelism returns the desired worker count for the given key.
// Returns 0 if no scaling decision exists (caller should use 1 as default).
func (a *AutoScaler) GetDesiredParallelism(clientID, channel, priority string) int32 {
	a.mu.RLock()
	defer a.mu.RUnlock()

	sk := scaleKey{ClientID: clientID, Channel: channel, Priority: priority}
	if s, ok := a.states[sk]; ok {
		return s.Desired
	}
	return 0
}

// GetAllStates returns a snapshot of all current scaling states.
func (a *AutoScaler) GetAllStates() map[scaleKey]*ScalingState {
	a.mu.RLock()
	defer a.mu.RUnlock()

	out := make(map[scaleKey]*ScalingState, len(a.states))
	for k, v := range a.states {
		// Copy
		c := *v
		out[k] = &c
	}
	return out
}

// Config returns the static default config (for dashboard display when
// the provider is unavailable). For live config, call loadConfig.
func (a *AutoScaler) DefaultConfig() nsconfig.AutoScalerConfig {
	return a.defaultCfg
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func parseTopicToChannelPriority(topic string) (channel, priority string) {
	// Topic format: notifications-{channel}-{priority}
	// e.g. notifications-email-high
	var ch, pr string
	n, err := fmt.Sscanf(topic, "notifications-%s-%s", &ch, &pr)
	if n == 2 && err == nil {
		return ch, pr
	}
	return "", ""
}

func allChannels() []string {
	return []string{"email", "sms", "push", "webhook", "slack", "websocket"}
}

func allPriorities() []string {
	return []string{"high", "medium", "low"}
}
