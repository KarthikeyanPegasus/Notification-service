package circuit

import (
	"errors"
	"fmt"
	"time"

	"github.com/sony/gobreaker"
	"go.uber.org/zap"
)

// State mirrors gobreaker.State for external callers.
type State int

const (
	StateClosed   State = 0
	StateHalfOpen State = 1
	StateOpen     State = 2
)

// BreakerConfig tunes a circuit breaker for a specific vendor.
type BreakerConfig struct {
	// Consecutive failures to trip from Closed → Open
	ConsecutiveFailures uint32
	// Rolling window for failure-rate calculation
	Interval time.Duration
	// How long to stay Open before probing
	OpenTimeout time.Duration
	// Slow call threshold (calls slower than this count as failures)
	SlowCallThreshold time.Duration
}

// Breaker wraps gobreaker.CircuitBreaker with named vendor tracking.
type Breaker struct {
	cb   *gobreaker.CircuitBreaker
	name string
	log  *zap.Logger
}

func newBreaker(name string, cfg BreakerConfig, log *zap.Logger) *Breaker {
	settings := gobreaker.Settings{
		Name:        name,
		MaxRequests: 3, // half-open probes before closing
		Interval:    cfg.Interval,
		Timeout:     cfg.OpenTimeout,
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			return counts.ConsecutiveFailures >= cfg.ConsecutiveFailures
		},
		OnStateChange: func(name string, from, to gobreaker.State) {
			if log != nil {
				log.Info("circuit breaker state changed",
					zap.String("vendor", name),
					zap.String("from", from.String()),
					zap.String("to", to.String()),
				)
			}
		},
	}
	return &Breaker{cb: gobreaker.NewCircuitBreaker(settings), name: name, log: log}
}

// Execute runs fn through the circuit breaker. Returns ErrCircuitOpen if the breaker is open.
func (b *Breaker) Execute(fn func() (any, error)) (any, error) {
	result, err := b.cb.Execute(fn)
	if err != nil {
		if errors.Is(err, gobreaker.ErrOpenState) {
			return nil, fmt.Errorf("%w: vendor %s", ErrCircuitOpen, b.name)
		}
		if errors.Is(err, gobreaker.ErrTooManyRequests) {
			return nil, fmt.Errorf("%w: vendor %s half-open", ErrCircuitOpen, b.name)
		}
		return nil, err
	}
	return result, nil
}

// IsOpen returns true if the circuit breaker is in the Open state.
func (b *Breaker) IsOpen() bool {
	return b.cb.State() == gobreaker.StateOpen
}

// State returns the current breaker state.
func (b *Breaker) State() State {
	switch b.cb.State() {
	case gobreaker.StateClosed:
		return StateClosed
	case gobreaker.StateHalfOpen:
		return StateHalfOpen
	default:
		return StateOpen
	}
}

// Counts returns the current window counts.
func (b *Breaker) Counts() gobreaker.Counts {
	return b.cb.Counts()
}

// ErrCircuitOpen signals that a circuit breaker rejected the call.
var ErrCircuitOpen = errors.New("circuit breaker open")
