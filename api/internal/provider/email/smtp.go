package email

import (
	"context"
	"crypto/tls"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/spidey/notification-service/internal/config"
	"github.com/spidey/notification-service/internal/domain"
	gomail "gopkg.in/gomail.v2"
)

// SMTPSender sends email via SMTP relay.
type SMTPSender struct {
	cfg  config.SMTPConfig
	from string
}

func NewSMTPSender(cfg config.SMTPConfig) *SMTPSender {
	return &SMTPSender{cfg: cfg, from: cfg.From}
}

func (s *SMTPSender) ProviderName() string { return "smtp-relay" }

func (s *SMTPSender) ValidateEmail(_ string) error { return nil }

func (s *SMTPSender) Send(_ context.Context, n *domain.Notification) (domain.DeliveryResult, error) {
	start := time.Now()

	if n.RenderedContent == nil {
		return domain.DeliveryResult{}, fmt.Errorf("smtp: rendered content is nil for notification %s", n.ID)
	}

	m := gomail.NewMessage()
	m.SetHeader("From", s.from)
	m.SetHeader("To", n.Recipient)
	m.SetHeader("Subject", n.RenderedContent.Subject)
	m.SetHeader("Message-ID", fmt.Sprintf("<%s@notification-service>", uuid.New().String()))

	if n.RenderedContent.HTML != "" {
		m.SetBody("text/html", n.RenderedContent.HTML)
		m.AddAlternative("text/plain", n.RenderedContent.Body)
	} else {
		m.SetBody("text/plain", n.RenderedContent.Body)
	}

	d := gomail.NewDialer(s.cfg.Host, s.cfg.Port, s.cfg.Username, s.cfg.Password)
	d.TLSConfig = &tls.Config{ServerName: s.cfg.Host, MinVersion: tls.VersionTLS12}

	if err := d.DialAndSend(m); err != nil {
		latencyMs := int(time.Since(start).Milliseconds())
		return domain.DeliveryResult{
			Provider:     s.ProviderName(),
			LatencyMs:    latencyMs,
			ErrorMessage: err.Error(),
		}, err
	}

	return domain.DeliveryResult{
		Success:       true,
		ProviderMsgID: uuid.New().String(),
		Provider:      s.ProviderName(),
		LatencyMs:     int(time.Since(start).Milliseconds()),
	}, nil
}

func (s *SMTPSender) GetStatus(ctx context.Context, providerMsgID string) (domain.DeliveryResult, error) {
	return domain.DeliveryResult{
		Provider:      s.ProviderName(),
		ProviderMsgID: providerMsgID,
		ErrorMessage:  "status polling not supported for direct smtp",
	}, nil
}
