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

// SMSWorker delivers SMS notifications with Twilio → Plivo → Vonage fallback.
type SMSWorker struct {
	base    *BaseWorker
	mu      sync.RWMutex
	senders []provider.Sender
}

func NewSMSWorker(
	subscriber pubsub.Subscriber,
	senders []provider.Sender,
	notifRepo *repository.NotificationRepository,
	attemptRepo *repository.AttemptRepository,
	eventRepo *repository.EventRepository,
	registry *circuit.Registry,
	log *zap.Logger,
) *SMSWorker {
	return &SMSWorker{
		base: newBaseWorker(
			domain.ChannelSMS, "sms-worker-sub",
			subscriber, notifRepo, attemptRepo, eventRepo, registry, log,
		),
		senders: senders,
	}
}

func (w *SMSWorker) Channel() domain.Channel { return domain.ChannelSMS }

func (w *SMSWorker) Start(ctx context.Context) error {
	w.base.log.Info("sms worker started")
	return w.base.subscriber.Subscribe(ctx, "sms", func(ctx context.Context, msg *pubsub.Message) error {
		return w.base.dispatch(ctx, msg, func(ctx context.Context, n *domain.Notification) (domain.DeliveryResult, error) {
			w.mu.RLock()
			senders := w.senders
			w.mu.RUnlock()
			return withFallback(ctx, senders, n, w.base.registry, w.base.log)
		}, "sms")
	})
}

func (w *SMSWorker) Reload(ctx context.Context, cfg nsconfig.ProviderConfig) {
	newSenders := provider.InitializeSMSSenders(cfg.SMS)
	w.mu.Lock()
	w.senders = newSenders
	w.mu.Unlock()
	w.base.log.Info("sms worker reloaded with new configuration")
}
