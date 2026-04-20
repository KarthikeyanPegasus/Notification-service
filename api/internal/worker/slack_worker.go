package worker

import (
	"context"
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
	sender *slackprovider.Sender,
	notifRepo *repository.NotificationRepository,
	attemptRepo *repository.AttemptRepository,
	eventRepo *repository.EventRepository,
	registry *circuit.Registry,
	log *zap.Logger,
) *SlackWorker {
	return &SlackWorker{
		base: newBaseWorker(
			domain.ChannelSlack, "slack-worker-sub",
			subscriber, notifRepo, attemptRepo, eventRepo, registry, log,
		),
		sender: sender,
	}
}

func (w *SlackWorker) Channel() domain.Channel { return domain.ChannelSlack }

func (w *SlackWorker) Start(ctx context.Context) error {
	w.base.log.Info("slack worker started")
	return w.base.subscriber.Subscribe(ctx, "slack", func(ctx context.Context, msg *pubsub.Message) error {
		return w.base.dispatch(ctx, msg, func(ctx context.Context, n *domain.Notification) (domain.DeliveryResult, error) {
			w.mu.RLock()
			snd := w.sender
			w.mu.RUnlock()
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
