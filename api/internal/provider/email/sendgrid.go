package email

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/sendgrid/sendgrid-go"
	"github.com/sendgrid/sendgrid-go/helpers/mail"

	"github.com/spidey/notification-service/internal/config"
	"github.com/spidey/notification-service/internal/domain"
)

type SendGridSender struct {
	apiKey    string
	fromEmail string
	fromName  string
	replyTo   string
}

func NewSendGridSender(cfg config.SendGridConfig) *SendGridSender {
	if strings.TrimSpace(cfg.APIKey) == "" || strings.TrimSpace(cfg.FromEmail) == "" {
		return nil
	}
	return &SendGridSender{
		apiKey:    strings.TrimSpace(cfg.APIKey),
		fromEmail: strings.TrimSpace(cfg.FromEmail),
		fromName:  strings.TrimSpace(cfg.FromName),
		replyTo:   strings.TrimSpace(cfg.ReplyTo),
	}
}

func (s *SendGridSender) ProviderName() string { return "sendgrid" }

func (s *SendGridSender) ValidateEmail(email string) error {
	if strings.TrimSpace(email) == "" || !strings.Contains(email, "@") {
		return fmt.Errorf("invalid email")
	}
	return nil
}

func (s *SendGridSender) Send(ctx context.Context, n *domain.Notification) (domain.DeliveryResult, error) {
	start := time.Now()

	toEmail := strings.TrimSpace(n.Recipient)
	if err := s.ValidateEmail(toEmail); err != nil {
		return domain.DeliveryResult{Provider: s.ProviderName(), ErrorMessage: err.Error()}, err
	}

	subject := ""
	textBody := ""
	htmlBody := ""
	if n.RenderedContent != nil {
		subject = strings.TrimSpace(n.RenderedContent.Subject)
		textBody = strings.TrimSpace(n.RenderedContent.Body)
		htmlBody = strings.TrimSpace(n.RenderedContent.HTML)
	}
	if subject == "" {
		subject = fmt.Sprintf("[%s] notification %s", n.Type, n.ID.String())
	}
	if textBody == "" && htmlBody == "" {
		textBody = fmt.Sprintf("[%s] notification %s", n.Type, n.ID.String())
	}

	from := mail.NewEmail(s.fromName, s.fromEmail)
	to := mail.NewEmail("", toEmail)

	var m *mail.SGMailV3
	if htmlBody != "" && textBody != "" {
		m = mail.NewV3MailInit(from, subject, to, mail.NewContent("text/plain", textBody))
		m.AddContent(mail.NewContent("text/html", htmlBody))
	} else if htmlBody != "" {
		m = mail.NewV3MailInit(from, subject, to, mail.NewContent("text/html", htmlBody))
	} else {
		m = mail.NewV3MailInit(from, subject, to, mail.NewContent("text/plain", textBody))
	}
	if s.replyTo != "" {
		m.SetReplyTo(mail.NewEmail("", s.replyTo))
	}
	// Embed notification ID in custom args so it appears in webhook event payloads
	if len(m.Personalizations) > 0 {
		m.Personalizations[0].SetCustomArg("notification_id", n.ID.String())
	}

	req := sendgrid.GetRequest(s.apiKey, "/v3/mail/send", "https://api.sendgrid.com")
	req.Method = "POST"
	req.Body = mail.GetRequestBody(m)

	resp, err := sendgrid.API(req)
	latencyMs := int(time.Since(start).Milliseconds())
	if err != nil {
		return domain.DeliveryResult{Provider: s.ProviderName(), LatencyMs: latencyMs, ErrorMessage: err.Error()}, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return domain.DeliveryResult{
			Provider:     s.ProviderName(),
			LatencyMs:    latencyMs,
			ErrorCode:    fmt.Sprintf("HTTP_%d", resp.StatusCode),
			ErrorMessage: resp.Body,
		}, fmt.Errorf("sendgrid returned HTTP %d: %s", resp.StatusCode, resp.Body)
	}

	msgID := ""
	if resp.Headers != nil {
		if v, ok := resp.Headers["X-Message-Id"]; ok && len(v) > 0 {
			msgID = strings.TrimSpace(v[0])
		}
	}
	if msgID == "" {
		msgID = fmt.Sprintf("sendgrid-%d", time.Now().UnixMilli())
	}

	return domain.DeliveryResult{
		Success:       true,
		Provider:      s.ProviderName(),
		ProviderMsgID: msgID,
		LatencyMs:     latencyMs,
	}, nil
}

func (s *SendGridSender) GetStatus(_ context.Context, providerMsgID string) (domain.DeliveryResult, error) {
	// SendGrid does not provide a message status polling API; use webhooks for delivery events.
	return domain.DeliveryResult{
		Provider:      s.ProviderName(),
		ProviderMsgID: providerMsgID,
		ErrorMessage:  "sendgrid does not support status polling; configure webhook delivery events",
	}, nil
}
