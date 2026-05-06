package worker

import (
	"context"
	"fmt"
	"sync"

	"github.com/spidey/notification-service/internal/circuit"
	nsconfig "github.com/spidey/notification-service/internal/config"
	"github.com/spidey/notification-service/internal/domain"
	"github.com/spidey/notification-service/internal/provider"
	webhookprovider "github.com/spidey/notification-service/internal/provider/webhook"
	"github.com/spidey/notification-service/internal/pubsub"
	"github.com/spidey/notification-service/internal/repository"
	"go.uber.org/zap"
)

// WebhookWorker delivers outbound HTTP callback notifications to partner endpoints.
type WebhookWorker struct {
	base    *BaseWorker
	mu      sync.RWMutex
	senders []provider.Sender
	routing nsconfig.RoutingConfig
}

func NewWebhookWorker(
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
) *WebhookWorker {
	priority := domain.PriorityLow
	if len(opts) > 0 && opts[0].Priority != "" {
		priority = opts[0].Priority
	}
	subKey := pubsub.PriorityTopicKey(string(domain.ChannelWebhook), string(priority))
	return &WebhookWorker{
		base: newBaseWorker(
			domain.ChannelWebhook, subKey,
			subscriber, publisher, notifRepo, attemptRepo, eventRepo, govRepo, vendorRepo, cfg, registry, log, opts...,
		),
		senders: senders,
	}
}

func (w *WebhookWorker) Channel() domain.Channel { return domain.ChannelWebhook }

func (w *WebhookWorker) Start(ctx context.Context) error {
	w.base.log.Info("webhook worker started",
		zap.String("priority", string(w.base.priority)),
		zap.String("subscription", w.base.subscription),
	)
	return w.base.subscriber.Subscribe(ctx, w.base.subscription, func(ctx context.Context, msg *pubsub.Message) error {

		if msg.ForcedVendor != "" {
			return w.base.dispatch(ctx, msg, func(ctx context.Context, n *domain.Notification) (domain.DeliveryResult, error) {
				w.mu.RLock()
				senders := w.senders
				w.mu.RUnlock()
				for _, s := range senders {
					if s.ProviderName() == msg.ForcedVendor {
						return s.Send(ctx, n)
					}
				}
				return domain.DeliveryResult{}, fmt.Errorf("forced vendor %q not configured for webhook channel", msg.ForcedVendor)
			}, msg.ForcedVendor)
		}

		w.mu.RLock()
		routing := w.routing
		w.mu.RUnlock()

		// Default vendor selection: prefer routing.only → routing.prefer → "webhooks".
		vendor := routing.Only
		if vendor == "" {
			vendor = routing.Prefer
		}
		if vendor == "" {
			vendor = "webhooks"
		}

		return w.base.dispatch(ctx, msg, func(ctx context.Context, n *domain.Notification) (domain.DeliveryResult, error) {
			effectiveCfg := w.base.getEffectiveConfig(ctx, n.APIKeyID)
			
			w.mu.RLock()
			senders := w.senders
			routing := w.routing
			w.mu.RUnlock()

			// If we have a scoped config, use it to initialize senders/routing
			if effectiveCfg != w.base.cfg {
				senders = provider.InitializeWebhookSenders(effectiveCfg.Providers)
				routing = effectiveCfg.Providers.WebhookRouting
			}

			// Re-determine vendor using effective routing
			v := routing.Only
			if v == "" {
				v = routing.Prefer
			}
			if v == "" {
				v = "webhooks"
			}

			for _, s := range senders {
				if s.ProviderName() == v {
					return s.Send(ctx, n)
				}
			}
			// If the preferred vendor isn't configured, fall back to the generic webhooks sender if present.
			for _, s := range senders {
				if s.ProviderName() == "webhooks" {
					return s.Send(ctx, n)
				}
			}
			return domain.DeliveryResult{}, fmt.Errorf("no webhook providers configured")
		}, vendor)
	})
}

func (w *WebhookWorker) Reload(ctx context.Context, cfg nsconfig.ProviderConfig) {
	newSenders := provider.InitializeWebhookSenders(cfg)
	w.mu.Lock()
	w.senders = newSenders
	w.routing = cfg.WebhookRouting
	w.mu.Unlock()
	w.base.log.Info("webhook worker reloaded with new configuration")
}

// ensure Dispatcher satisfies provider.Sender at compile time
var _ provider.Sender = (*webhookprovider.Dispatcher)(nil)
