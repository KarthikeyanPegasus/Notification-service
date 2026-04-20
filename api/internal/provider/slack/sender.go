package slack

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/spidey/notification-service/internal/config"
	"github.com/spidey/notification-service/internal/domain"
)

const slackTextMaxRunes = 40000

// Sender posts messages to Slack Incoming Webhooks (payload shape: {"text": "..."}).
type Sender struct {
	cfg    config.SlackProviderConfig
	client *http.Client
}

func NewSender(cfg config.SlackProviderConfig) *Sender {
	sec := cfg.TimeoutSeconds
	if sec <= 0 {
		sec = 30
	}
	return &Sender{
		cfg: cfg,
		client: &http.Client{
			Timeout: time.Duration(sec) * time.Second,
		},
	}
}

func (s *Sender) ProviderName() string { return "slack" }

func (s *Sender) resolveWebhookURL(n *domain.Notification) string {
	if n != nil {
		if u := strings.TrimSpace(n.Recipient); u != "" {
			return u
		}
	}
	return strings.TrimSpace(s.cfg.WebhookURL)
}

func truncateSlackText(s string) string {
	r := []rune(s)
	if len(r) <= slackTextMaxRunes {
		return s
	}
	return string(r[:slackTextMaxRunes]) + "…"
}

func (s *Sender) Send(ctx context.Context, n *domain.Notification) (domain.DeliveryResult, error) {
	start := time.Now()
	webhookURL := s.resolveWebhookURL(n)
	if webhookURL == "" {
		return domain.DeliveryResult{
			Provider:     s.ProviderName(),
			ErrorMessage: "slack webhook URL not configured (set recipient or providers.slack.webhook_url / App Store slack config)",
		}, fmt.Errorf("slack webhook URL not configured")
	}

	text := ""
	if n.RenderedContent != nil {
		text = strings.TrimSpace(n.RenderedContent.Body)
	}
	if text == "" {
		text = fmt.Sprintf("[%s] notification %s", n.Type, n.ID.String())
	}
	text = truncateSlackText(text)

	payload := map[string]any{
		"text": text,
	}
	if s.cfg.DefaultUsername != "" {
		payload["username"] = s.cfg.DefaultUsername
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return domain.DeliveryResult{Provider: s.ProviderName()}, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, webhookURL, bytes.NewReader(body))
	if err != nil {
		return domain.DeliveryResult{Provider: s.ProviderName()}, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.client.Do(req)
	latencyMs := int(time.Since(start).Milliseconds())
	if err != nil {
		return domain.DeliveryResult{
			Provider:     s.ProviderName(),
			LatencyMs:    latencyMs,
			ErrorMessage: err.Error(),
		}, err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return domain.DeliveryResult{
			Provider:     s.ProviderName(),
			LatencyMs:    latencyMs,
			ErrorCode:    fmt.Sprintf("HTTP_%d", resp.StatusCode),
			ErrorMessage: string(respBody),
		}, fmt.Errorf("slack returned HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	return domain.DeliveryResult{
		Success:       true,
		Provider:      s.ProviderName(),
		ProviderMsgID: fmt.Sprintf("slack-%d", time.Now().UnixMilli()),
		LatencyMs:     latencyMs,
	}, nil
}

func (s *Sender) GetStatus(ctx context.Context, providerMsgID string) (domain.DeliveryResult, error) {
	return domain.DeliveryResult{
		Provider:      s.ProviderName(),
		ProviderMsgID: providerMsgID,
		ErrorMessage:  "status polling not supported for slack",
	}, nil
}
