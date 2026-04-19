package circuit

import (
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"
)

// Registry holds one circuit breaker per vendor.
type Registry struct {
	mu       sync.RWMutex
	breakers map[string]*Breaker
	log      *zap.Logger
}

// NewRegistry initialises all vendor circuit breakers with tuned configs.
func NewRegistry(log *zap.Logger) *Registry {
	r := &Registry{
		breakers: make(map[string]*Breaker),
		log:      log,
	}
	r.registerAll()
	return r
}

func (r *Registry) registerAll() {
	// Email providers: 50% failure rate, 30s open window, 5s slow threshold
	emailCfg := BreakerConfig{
		ConsecutiveFailures: 10,
		Interval:            60 * time.Second,
		OpenTimeout:         30 * time.Second,
		SlowCallThreshold:   5 * time.Second,
	}
	r.register("amazon-ses", emailCfg)
	r.register("mailgun", emailCfg)
	r.register("smtp-relay", emailCfg)

	// SMS providers: 50% failure rate, 20s open window, 3s slow threshold
	smsCfg := BreakerConfig{
		ConsecutiveFailures: 10,
		Interval:            60 * time.Second,
		OpenTimeout:         20 * time.Second,
		SlowCallThreshold:   3 * time.Second,
	}
	r.register("twilio", smsCfg)
	r.register("plivo", smsCfg)
	r.register("vonage", smsCfg)

	// OTP providers: tighter — 30% failure, 10s open (fail fast, user login at stake)
	otpCfg := BreakerConfig{
		ConsecutiveFailures: 6,
		Interval:            60 * time.Second,
		OpenTimeout:         10 * time.Second,
		SlowCallThreshold:   800 * time.Millisecond,
	}
	r.register("twilio-otp", otpCfg)
	r.register("plivo-otp", otpCfg)

	// Push providers: more lenient — 60% failure, 60s open
	pushCfg := BreakerConfig{
		ConsecutiveFailures: 12,
		Interval:            60 * time.Second,
		OpenTimeout:         60 * time.Second,
		SlowCallThreshold:   5 * time.Second,
	}
	r.register("fcm", pushCfg)
	r.register("apns", pushCfg)
	r.register("pushwoosh", pushCfg)

	// Other
	r.register("websocket-gateway", pushCfg)
	r.register("webhook-delivery", pushCfg)
}

func (r *Registry) register(vendor string, cfg BreakerConfig) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.breakers[vendor] = newBreaker(vendor, cfg, r.log)
}

// Get returns the circuit breaker for a vendor.
func (r *Registry) Get(vendor string) (*Breaker, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	b, ok := r.breakers[vendor]
	if !ok {
		return nil, fmt.Errorf("no circuit breaker registered for vendor: %s", vendor)
	}
	return b, nil
}

// GetOrDefault returns the breaker for vendor, or creates one with defaults.
func (r *Registry) GetOrDefault(vendor string) *Breaker {
	b, err := r.Get(vendor)
	if err != nil {
		defaultCfg := BreakerConfig{
			ConsecutiveFailures: 10,
			Interval:            60 * time.Second,
			OpenTimeout:         30 * time.Second,
			SlowCallThreshold:   5 * time.Second,
		}
		r.register(vendor, defaultCfg)
		b, _ = r.Get(vendor)
	}
	return b
}

// Snapshot returns the state of all registered breakers for observability.
func (r *Registry) Snapshot() map[string]State {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make(map[string]State, len(r.breakers))
	for name, b := range r.breakers {
		out[name] = b.State()
	}
	return out
}
