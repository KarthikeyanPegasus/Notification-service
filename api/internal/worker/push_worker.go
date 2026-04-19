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

// PushWorker delivers push notifications to iOS/Android/Web via FCM, APNs, Pushwoosh.
type PushWorker struct {
	base    *BaseWorker
	mu      sync.RWMutex
	senders []provider.Sender
}

func NewPushWorker(
	subscriber pubsub.Subscriber,
	senders []provider.Sender,
	notifRepo *repository.NotificationRepository,
	attemptRepo *repository.AttemptRepository,
	eventRepo *repository.EventRepository,
	registry *circuit.Registry,
	log *zap.Logger,
) *PushWorker {
	return &PushWorker{
		base: newBaseWorker(
			domain.ChannelPush, "push-worker-sub",
			subscriber, notifRepo, attemptRepo, eventRepo, registry, log,
		),
		senders: senders,
	}
}

func (w *PushWorker) Channel() domain.Channel { return domain.ChannelPush }

func (w *PushWorker) Start(ctx context.Context) error {
	w.base.log.Info("push worker started")
	return w.base.subscriber.Subscribe(ctx, "push", func(ctx context.Context, msg *pubsub.Message) error {
		return w.base.dispatch(ctx, msg, func(ctx context.Context, n *domain.Notification) (domain.DeliveryResult, error) {
			w.mu.RLock()
			senders := w.senders
			w.mu.RUnlock()
			return withFallback(ctx, senders, n, w.base.registry, w.base.log)
		}, "push")
	})
}

func (w *PushWorker) Reload(ctx context.Context, cfg nsconfig.ProviderConfig) {
	newSenders := provider.InitializePushSenders(cfg.Push)
	w.mu.Lock()
	w.senders = newSenders
	w.mu.Unlock()
	w.base.log.Info("push worker reloaded with new configuration")
}
