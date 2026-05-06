package worker

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/spidey/notification-service/internal/circuit"
	nsconfig "github.com/spidey/notification-service/internal/config"
	"github.com/spidey/notification-service/internal/domain"
	"github.com/spidey/notification-service/internal/provider"
	"github.com/spidey/notification-service/internal/pubsub"
	"github.com/spidey/notification-service/internal/repository"
	"go.uber.org/zap"
)

// EmailWorker delivers email notifications from the notifications-email Pub/Sub topic.
type EmailWorker struct {
	base    *BaseWorker
	mu      sync.RWMutex
	senders []provider.Sender
	routing nsconfig.RoutingConfig
	rr      uint64
}

func NewEmailWorker(
	subscriber pubsub.Subscriber,
	publisher pubsub.Publisher,
	senders []provider.Sender,
	notifRepo *repository.NotificationRepository,
	attemptRepo *repository.AttemptRepository,
	eventRepo *repository.EventRepository,
	govRepo *repository.GovernanceRepository,
	vendorRepo nsconfig.Repository,
	cfg *nsconfig.Config,
	registry *circuit.Registry,
	log *zap.Logger,
	opts ...WorkerOptions,
) *EmailWorker {
	priority := domain.PriorityLow
	if len(opts) > 0 && opts[0].Priority != "" {
		priority = opts[0].Priority
	}

	// Subscription key encodes the priority so the Kafka consumer group is unique per priority tier.
	subKey := pubsub.PriorityTopicKey(string(domain.ChannelEmail), string(priority))

	return &EmailWorker{
		base: newBaseWorker(
			domain.ChannelEmail,
			subKey,
			subscriber,
			publisher,
			notifRepo,
			attemptRepo,
			eventRepo,
			govRepo,
			vendorRepo,
			cfg,
			registry,
			log,
			opts...,
		),
		senders: senders,
	}
}

func (w *EmailWorker) Channel() domain.Channel { return domain.ChannelEmail }

func (w *EmailWorker) Start(ctx context.Context) error {
	w.base.log.Info("email worker started",
		zap.String("priority", string(w.base.priority)),
		zap.String("subscription", w.base.subscription),
	)
	return w.base.subscriber.Subscribe(ctx, w.base.subscription, func(ctx context.Context, msg *pubsub.Message) error {
		w.mu.RLock()
		routing := w.routing
		w.mu.RUnlock()

		if msg.ForcedVendor != "" {
			vendor := normalizeEmailVendor(msg.ForcedVendor)
			return w.base.dispatch(ctx, msg, func(ctx context.Context, n *domain.Notification) (domain.DeliveryResult, error) {
				w.mu.RLock()
				senders := w.senders
				w.mu.RUnlock()
				for _, s := range senders {
					if s.ProviderName() == vendor {
						return s.Send(ctx, n)
					}
				}
				return domain.DeliveryResult{}, fmt.Errorf("forced vendor %q not configured for email channel", vendor)
			}, vendor)
		}

		if normalizeRoutingMode(routing.Mode) == "publish_all" {
			return w.dispatchPublishAll(ctx, msg)
		}

		return w.base.dispatch(ctx, msg, func(ctx context.Context, n *domain.Notification) (domain.DeliveryResult, error) {
			effectiveCfg := w.base.getEffectiveConfig(ctx, n.APIKeyID)
			
			w.mu.RLock()
			senders := append([]provider.Sender(nil), w.senders...)
			routing := w.routing
			w.mu.RUnlock()

			// If we have a scoped config, use it to initialize senders/routing
			if effectiveCfg != w.base.cfg {
				senders = provider.InitializeEmailSenders(ctx, effectiveCfg.Providers.Email)
				routing = effectiveCfg.Providers.EmailRouting
				if routing.Prefer == "" {
					routing.Prefer = preferredProviderFromPrimary(effectiveCfg.Providers.Email.Primary)
				}
			}

			mode := normalizeRoutingMode(routing.Mode)
			senders = stableVendors(senders)

			prefer := normalizeEmailVendor(routing.Prefer)
			only := normalizeEmailVendor(routing.Only)
			if only == "" {
				only = prefer
			}

			switch mode {
			case "only":
				if only == "" {
					return domain.DeliveryResult{}, fmt.Errorf("email routing mode=only requires routing.only (or routing.prefer)")
				}
				for _, s := range senders {
					if s.ProviderName() == only {
						return s.Send(ctx, n)
					}
				}
				return domain.DeliveryResult{}, fmt.Errorf("email routing vendor %q not configured", only)

			case "round_robin":
				participants := make(map[string]struct{}, len(routing.Participants))
				for _, p := range routing.Participants {
					participants[normalizeEmailVendor(p)] = struct{}{}
				}
				rrSenders := senders
				if len(participants) > 0 {
					rrSenders = make([]provider.Sender, 0, len(senders))
					for _, s := range senders {
						if _, ok := participants[s.ProviderName()]; ok {
							rrSenders = append(rrSenders, s)
						}
					}
				}

				if len(rrSenders) == 0 {
					return domain.DeliveryResult{ErrorMessage: "no providers configured"}, domain.ErrAllProvidersOpen
				}
				idx := int(atomic.AddUint64(&w.rr, 1)-1) % len(rrSenders)
				s := rrSenders[idx]
				cb := w.base.registry.GetOrDefault(s.ProviderName())
				if cb.IsOpen() {
					return domain.DeliveryResult{ErrorMessage: "circuit breaker open"}, domain.ErrAllProvidersOpen
				}
				var result domain.DeliveryResult
				start := time.Now()
				_, execErr := cb.Execute(func() (any, error) {
					r, err := s.Send(ctx, n)
					result = r
					return r, err
				})
				result.LatencyMs = int(time.Since(start).Milliseconds())
				if execErr != nil {
					result.Success = false
					result.ErrorMessage = execErr.Error()
					return result, execErr
				}
				return result, nil

			case "backup":
				fallthrough
			default:
				// Put preferred first (if present) and fallback to others
				if prefer != "" || routing.Fallback != "" {
					fallback := normalizeEmailVendor(routing.Fallback)
					ordered := make([]provider.Sender, 0, len(senders))
					preferFallback := shouldPreferFallback(w.base.registry, prefer, routing)
					first := prefer
					second := fallback
					if preferFallback && fallback != "" {
						first, second = fallback, prefer
					}
					for _, s := range senders {
						if first != "" && s.ProviderName() == first {
							ordered = append(ordered, s)
						}
					}
					for _, s := range senders {
						if second != "" && s.ProviderName() == second && s.ProviderName() != first {
							ordered = append(ordered, s)
						}
					}
					for _, s := range senders {
						if s.ProviderName() != first && s.ProviderName() != second {
							ordered = append(ordered, s)
						}
					}
					senders = ordered
				}
				return withFallback(ctx, senders, n, w.base.registry, w.base.log)
			}
		}, "email")
	})
}

func (w *EmailWorker) Reload(ctx context.Context, cfg nsconfig.ProviderConfig) {
	newSenders := provider.InitializeEmailSenders(ctx, cfg.Email)
	w.mu.Lock()
	w.senders = newSenders
	routing := cfg.EmailRouting
	if routing.Prefer == "" {
		routing.Prefer = preferredProviderFromPrimary(cfg.Email.Primary)
	}
	w.routing = routing
	w.mu.Unlock()
	w.base.log.Info("email worker reloaded with new configuration",
		zap.String("routing_mode", cfg.EmailRouting.Mode),
		zap.String("routing_prefer", cfg.EmailRouting.Prefer),
		zap.String("routing_only", cfg.EmailRouting.Only),
	)
}

func normalizeRoutingMode(mode string) string {
	switch mode {
	case "preference":
		return "backup"
	case "prefer":
		return "backup"
	case "":
		return "backup"
	default:
		return mode
	}
}

func normalizeEmailVendor(v string) string {
	switch v {
	case "ses", "amazon-ses", "amazon_ses":
		return "amazon-ses"
	case "smtp", "smtp-relay", "smtp_relay":
		return "smtp-relay"
	case "mailgun":
		return "mailgun"
	default:
		return v
	}
}

func stableVendors(senders []provider.Sender) []provider.Sender {
	sort.SliceStable(senders, func(i, j int) bool {
		return senders[i].ProviderName() < senders[j].ProviderName()
	})
	return senders
}

func preferredProviderFromPrimary(primary string) string {
	switch primary {
	case "ses":
		return "amazon-ses"
	case "smtp":
		return "smtp-relay"
	case "mailgun":
		return "mailgun"
	default:
		return ""
	}
}

func (w *EmailWorker) dispatchPublishAll(ctx context.Context, msg *pubsub.Message) error {
	w.mu.RLock()
	senders := append([]provider.Sender(nil), w.senders...)
	w.mu.RUnlock()
	senders = stableVendors(senders)
	return w.base.dispatchPublishAll(ctx, msg, senders)
}
