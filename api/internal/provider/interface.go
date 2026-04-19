package provider

import (
	"context"

	"github.com/spidey/notification-service/internal/domain"
)

// Sender is the universal contract for all notification providers.
type Sender interface {
	Send(ctx context.Context, n *domain.Notification) (domain.DeliveryResult, error)
	GetStatus(ctx context.Context, providerMsgID string) (domain.DeliveryResult, error)
	ProviderName() string
}

// EmailSender is extended for email-specific validation.
type EmailSender interface {
	Sender
	ValidateEmail(email string) error
}

// SMSSender is extended for SMS-specific features.
type SMSSender interface {
	Sender
	NormalizePhone(phone string) string
}

// PushSender handles push to a specific platform.
type PushSender interface {
	Sender
	Platform() string // ios | android | web
	DeactivateToken(ctx context.Context, token string) error
}

// WebhookSender handles outbound HTTP callbacks.
type WebhookSender interface {
	Sender
	SignRequest(body []byte, secret string) string
}
