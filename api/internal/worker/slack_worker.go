package worker

import (
	"context"
	"fmt"
	"sync"

	"github.com/spidey/notification-service/internal/circuit"
	nsconfig "github.com/spidey/notification-service/internal/config"
	"github.com/spidey/notification-service/internal/domain"
	"github.com/spidey/notification-service/internal/provider"
	slackprovider "github.com/spidey/notification-service/internal/provider/slack"
	"github.com/spidey/notification-service/internal/pubsub"
	"github.com/spidey/notification-service/internal/repository"
	"go.uber.org/zap"
)

// SlackWorker delivers Slack Incoming Webhook messages.
type SlackWorker struct {
	base   *BaseWorker
	mu     sync.RWMutex
	sender *slackprovider.Sender
}

func NewSlackWorker(
	subscriber pubsub.Subscriber,
	publisher pubsub.Publisher,
	sender *slackprovider.Sender,
	notifRepo *repository.NotificationRepository,
	attemptRepo *repository.AttemptRepository,
	eventRepo *repository.EventRepository,
	govRepo *repository.GovernanceRepository,
	vendorRepo nsconfig.Repository,
	cfg *nsconfig.Config,
	registry *circuit.Registry,
	log *zap.Logger,
	opts ...WorkerOptions,
) *SlackWorker {
	priority := domain.PriorityLow
	if len(opts) > 0 && opts[0].Priority != "" {
		priority = opts[0].Priority
	}
	subKey := pubsub.PriorityTopicKey(string(domain.ChannelSlack), string(priority))
	return &SlackWorker{
		base: newBaseWorker(
			domain.ChannelSlack, subKey,
			subscriber, publisher, notifRepo, attemptRepo, eventRepo, govRepo, vendorRepo, cfg, registry, log, opts...,
		),
		sender: sender,
	}
}

func (w *SlackWorker) Channel() domain.Channel { return domain.ChannelSlack }

func (w *SlackWorker) Start(ctx context.Context) error {
	w.base.log.Info("slack worker started",
		zap.String("priority", string(w.base.priority)),
		zap.String("subscription", w.base.subscription),
	)
	return w.base.subscriber.Subscribe(ctx, w.base.subscription, func(ctx context.Context, msg *pubsub.Message) error {
		return w.base.dispatch(ctx, msg, func(ctx context.Context, n *domain.Notification) (domain.DeliveryResult, error) {
			effectiveCfg := w.base.getEffectiveConfig(ctx, n.APIKeyID)
			
			w.mu.RLock()
			snd := w.sender
			w.mu.RUnlock()

			// If we have a scoped config, use it to initialize sender
			if effectiveCfg != w.base.cfg {
				snd = provider.InitializeSlackSender(effectiveCfg.Providers.Slack)
			}

			if snd == nil {
				return domain.DeliveryResult{}, fmt.Errorf("slack vendor not configured")
			}
			return snd.Send(ctx, n)
		}, "slack")
	})
}

func (w *SlackWorker) Reload(ctx context.Context, cfg nsconfig.ProviderConfig) {
	newSender := provider.InitializeSlackSender(cfg.Slack)
	w.mu.Lock()
	w.sender = newSender
	w.mu.Unlock()
	w.base.log.Info("slack worker reloaded with new configuration")
}

var _ provider.Sender = (*slackprovider.Sender)(nil)
