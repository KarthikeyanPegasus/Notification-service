package email

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/mailgun/mailgun-go/v4"
	"github.com/spidey/notification-service/internal/config"
	"github.com/spidey/notification-service/internal/domain"
)

// MailgunSender sends email via Mailgun.
type MailgunSender struct {
	mg         *mailgun.MailgunImpl
	from       string
	replyTo    string
	domain     string
	apiKey     string
	httpClient *http.Client
}

func NewMailgunSender(cfg config.MailgunConfig) *MailgunSender {
	mg := mailgun.NewMailgun(cfg.Domain, cfg.APIKey)
	return &MailgunSender{
		mg:         mg,
		from:       cfg.From,
		replyTo:    cfg.ReplyTo,
		domain:     cfg.Domain,
		apiKey:     cfg.APIKey,
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

func (s *MailgunSender) ProviderName() string { return "mailgun" }

func (s *MailgunSender) ValidateEmail(email string) error { return validateEmailFormat(email) }

func (s *MailgunSender) Send(ctx context.Context, n *domain.Notification) (domain.DeliveryResult, error) {
	start := time.Now()

	if n.RenderedContent == nil {
		return domain.DeliveryResult{}, fmt.Errorf("mailgun: rendered content is nil for notification %s", n.ID)
	}

	message := s.mg.NewMessage(s.from, n.RenderedContent.Subject, n.RenderedContent.Body, n.Recipient)
	if n.RenderedContent.HTML != "" {
		message.SetHtml(n.RenderedContent.HTML)
	}
	if s.replyTo != "" {
		message.AddHeader("Reply-To", s.replyTo)
	}
	// Embed notification ID so it comes back in webhook event-data.user-variables
	_ = message.AddVariable("notification_id", n.ID.String())

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

func normalizeMailgunMessageID(id string) (withBrackets string, withoutBrackets string) {
	trimmed := strings.TrimSpace(id)
	without := strings.Trim(trimmed, "<>")
	if without == "" {
		return trimmed, trimmed
	}
	return "<" + without + ">", without
}

type mailgunEventItem struct {
	Event    string `json:"event"`
	Severity string `json:"severity"`
}

// GetStatus polls the Mailgun Events API for the latest delivery status of a message.
func (s *MailgunSender) GetStatus(ctx context.Context, providerMsgID string) (domain.DeliveryResult, error) {
	if s.domain == "" || s.apiKey == "" || providerMsgID == "" {
		return domain.DeliveryResult{
			Provider:      s.ProviderName(),
			ProviderMsgID: providerMsgID,
			ErrorMessage:  "mailgun not configured for status polling",
		}, nil
	}

	queryOnce := func(messageID string) (items []mailgunEventItem, httpStatus int, respBody []byte, _ error) {
		apiURL := "https://api.mailgun.net/v3/" + url.PathEscape(s.domain) + "/events"
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
		if err != nil {
			return nil, 0, nil, err
		}
		q := req.URL.Query()
		q.Set("message-id", messageID)
		q.Set("limit", "1")
		req.URL.RawQuery = q.Encode()
		req.SetBasicAuth("api", s.apiKey)

		resp, err := s.httpClient.Do(req)
		if err != nil {
			return nil, 0, nil, err
		}
		defer resp.Body.Close()

		body, _ := io.ReadAll(io.LimitReader(resp.Body, 8192))
		if resp.StatusCode >= 400 {
			return nil, resp.StatusCode, body, fmt.Errorf("mailgun events API returned HTTP %d", resp.StatusCode)
		}

		var result struct {
			Items []mailgunEventItem `json:"items"`
		}
		if err := json.Unmarshal(body, &result); err != nil {
			return nil, resp.StatusCode, body, err
		}
		return result.Items, resp.StatusCode, body, nil
	}

	withBrackets, withoutBrackets := normalizeMailgunMessageID(providerMsgID)
	candidates := []string{providerMsgID}
	if withBrackets != providerMsgID {
		candidates = append(candidates, withBrackets)
	}
	if withoutBrackets != providerMsgID {
		candidates = append(candidates, withoutBrackets)
	}

	var (
		items      []mailgunEventItem
		statusCode int
		body       []byte
		lastErr    error
	)
	for _, cand := range candidates {
		its, sc, b, err := queryOnce(cand)
		if err != nil {
			lastErr = err
			statusCode = sc
			body = b
			continue
		}
		if len(its) == 0 {
			statusCode = sc
			body = b
			continue
		}
		// Found at least one event
		items = its
		break
	}

	if lastErr != nil && statusCode >= 400 {
		return domain.DeliveryResult{
			Provider:      s.ProviderName(),
			ProviderMsgID: providerMsgID,
			ErrorCode:     fmt.Sprintf("HTTP_%d", statusCode),
			ErrorMessage:  string(body),
		}, lastErr
	}

	if len(items) == 0 {
		return domain.DeliveryResult{
			Provider:      s.ProviderName(),
			ProviderMsgID: providerMsgID,
			ErrorMessage:  "no events found for message-id",
		}, nil
	}

	ev := items[0]
	success := ev.Event == "delivered"
	errCode := ""
	if !success {
		if ev.Severity != "" {
			errCode = ev.Severity
		} else {
			errCode = ev.Event
		}
	}

	return domain.DeliveryResult{
		Success:       success,
		Provider:      s.ProviderName(),
		ProviderMsgID: providerMsgID,
		ErrorCode:     errCode,
		VendorStatus:  ev.Event,
	}, nil
}
