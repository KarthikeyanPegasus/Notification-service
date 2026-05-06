package email

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/mail"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/spidey/notification-service/internal/config"
	"github.com/spidey/notification-service/internal/domain"
	gomail "gopkg.in/gomail.v2"
)

// SMTPSender sends email via SMTP relay.
type SMTPSender struct {
	cfg     config.SMTPConfig
	from    string
	replyTo string
}

func NewSMTPSender(cfg config.SMTPConfig) *SMTPSender {
	return &SMTPSender{cfg: cfg, from: cfg.From, replyTo: cfg.ReplyTo}
}

func (s *SMTPSender) ProviderName() string { return "smtp-relay" }

func (s *SMTPSender) ValidateEmail(email string) error { return validateEmailFormat(email) }

func (s *SMTPSender) Send(_ context.Context, n *domain.Notification) (domain.DeliveryResult, error) {
	start := time.Now()

	if n.RenderedContent == nil {
		return domain.DeliveryResult{}, fmt.Errorf("smtp: rendered content is nil for notification %s", n.ID)
	}

	from := strings.TrimSpace(s.from)
	if from == "" {
		return domain.DeliveryResult{}, fmt.Errorf("smtp: from address is empty")
	}
	if _, err := mail.ParseAddress(from); err != nil {
		return domain.DeliveryResult{}, fmt.Errorf("smtp: invalid from address %q: %w", from, err)
	}

	to := strings.TrimSpace(n.Recipient)
	if to == "" {
		return domain.DeliveryResult{}, fmt.Errorf("smtp: recipient address is empty")
	}
	if _, err := mail.ParseAddress(to); err != nil {
		return domain.DeliveryResult{}, fmt.Errorf("smtp: invalid recipient address %q: %w", to, err)
	}

	m := gomail.NewMessage()
	m.SetHeader("From", from)
	m.SetHeader("To", to)
	m.SetHeader("Subject", n.RenderedContent.Subject)
	if s.replyTo != "" {
		m.SetHeader("Reply-To", s.replyTo)
	}
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
