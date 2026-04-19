package worker

import (
	"context"
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
	base       *BaseWorker
	mu         sync.RWMutex
	dispatcher *webhookprovider.Dispatcher
}

func NewWebhookWorker(
	subscriber pubsub.Subscriber,
	dispatcher *webhookprovider.Dispatcher,
	notifRepo *repository.NotificationRepository,
	attemptRepo *repository.AttemptRepository,
	eventRepo *repository.EventRepository,
	registry *circuit.Registry,
	log *zap.Logger,
) *WebhookWorker {
	return &WebhookWorker{
		base: newBaseWorker(
			domain.ChannelWebhook, "webhook-worker-sub",
			subscriber, notifRepo, attemptRepo, eventRepo, registry, log,
		),
		dispatcher: dispatcher,
	}
}

func (w *WebhookWorker) Channel() domain.Channel { return domain.ChannelWebhook }

func (w *WebhookWorker) Start(ctx context.Context) error {
	w.base.log.Info("webhook worker started")
	return w.base.subscriber.Subscribe(ctx, "webhook", func(ctx context.Context, msg *pubsub.Message) error {
		return w.base.dispatch(ctx, msg, func(ctx context.Context, n *domain.Notification) (domain.DeliveryResult, error) {
			w.mu.RLock()
			dispatcher := w.dispatcher
			w.mu.RUnlock()
			return dispatcher.Send(ctx, n)
		}, "webhook-delivery")
	})
}

func (w *WebhookWorker) Reload(ctx context.Context, cfg nsconfig.ProviderConfig) {
	newDispatcher := provider.InitializeWebhookDispatcher(cfg.Webhook)
	w.mu.Lock()
	w.dispatcher = newDispatcher
	w.mu.Unlock()
	w.base.log.Info("webhook worker reloaded with new configuration")
}

// ensure Dispatcher satisfies provider.Sender at compile time
var _ provider.Sender = (*webhookprovider.Dispatcher)(nil)
