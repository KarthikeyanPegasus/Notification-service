package worker

import (
	"context"
	"sync"

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
}

func NewEmailWorker(
	subscriber pubsub.Subscriber,
	senders []provider.Sender,
	notifRepo *repository.NotificationRepository,
	attemptRepo *repository.AttemptRepository,
	eventRepo *repository.EventRepository,
	registry *circuit.Registry,
	log *zap.Logger,
) *EmailWorker {
	return &EmailWorker{
		base: newBaseWorker(
			domain.ChannelEmail,
			"email-worker-sub",
			subscriber,
			notifRepo,
			attemptRepo,
			eventRepo,
			registry,
			log,
		),
		senders: senders,
	}
}

func (w *EmailWorker) Channel() domain.Channel { return domain.ChannelEmail }

func (w *EmailWorker) Start(ctx context.Context) error {
	w.base.log.Info("email worker started")
	return w.base.subscriber.Subscribe(ctx, "email", func(ctx context.Context, msg *pubsub.Message) error {
		return w.base.dispatch(ctx, msg, func(ctx context.Context, n *domain.Notification) (domain.DeliveryResult, error) {
			w.mu.RLock()
			senders := w.senders
			w.mu.RUnlock()
			return withFallback(ctx, senders, n, w.base.registry, w.base.log)
		}, "email")
	})
}

func (w *EmailWorker) Reload(ctx context.Context, cfg nsconfig.ProviderConfig) {
	newSenders := provider.InitializeEmailSenders(ctx, cfg.Email)
	w.mu.Lock()
	w.senders = newSenders
	w.mu.Unlock()
	w.base.log.Info("email worker reloaded with new configuration")
}
