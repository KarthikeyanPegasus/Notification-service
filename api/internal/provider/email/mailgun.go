package email

import (
	"context"
	"fmt"
	"time"

	"github.com/mailgun/mailgun-go/v4"
	"github.com/spidey/notification-service/internal/config"
	"github.com/spidey/notification-service/internal/domain"
)

// MailgunSender sends email via Mailgun.
type MailgunSender struct {
	mg   *mailgun.MailgunImpl
	from string
}

func NewMailgunSender(cfg config.MailgunConfig) *MailgunSender {
	mg := mailgun.NewMailgun(cfg.Domain, cfg.APIKey)
	return &MailgunSender{mg: mg, from: cfg.From}
}

func (s *MailgunSender) ProviderName() string { return "mailgun" }

func (s *MailgunSender) ValidateEmail(_ string) error { return nil }

func (s *MailgunSender) Send(ctx context.Context, n *domain.Notification) (domain.DeliveryResult, error) {
	start := time.Now()

	if n.RenderedContent == nil {
		return domain.DeliveryResult{}, fmt.Errorf("mailgun: rendered content is nil for notification %s", n.ID)
	}

	message := s.mg.NewMessage(s.from, n.RenderedContent.Subject, n.RenderedContent.Body, n.Recipient)
	if n.RenderedContent.HTML != "" {
		message.SetHtml(n.RenderedContent.HTML)
	}

	_, msgID, err := s.mg.Send(ctx, message)
	latencyMs := int(time.Since(start).Milliseconds())
	if err != nil {
		return domain.DeliveryResult{
			Provider:     s.ProviderName(),
			LatencyMs:    latencyMs,
			ErrorMessage: err.Error(),
		}, err
	}

	return domain.DeliveryResult{
		Success:       true,
		ProviderMsgID: msgID,
		Provider:      s.ProviderName(),
		LatencyMs:     latencyMs,
	}, nil
}

func (s *MailgunSender) GetStatus(ctx context.Context, providerMsgID string) (domain.DeliveryResult, error) {
	return domain.DeliveryResult{
		Provider:      s.ProviderName(),
		ProviderMsgID: providerMsgID,
		ErrorMessage:  "status polling not supported for mailgun (use webhooks)",
	}, nil
}
