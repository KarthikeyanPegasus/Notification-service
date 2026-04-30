package webhook

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

type DiscordSender struct {
	cfg    config.DiscordConfig
	client *http.Client
}

func NewDiscordSender(cfg config.DiscordConfig) *DiscordSender {
	if strings.TrimSpace(cfg.WebhookURL) == "" {
		return nil
	}
	sec := cfg.TimeoutSeconds
	if sec <= 0 {
		sec = 30
	}
	return &DiscordSender{
		cfg: cfg,
		client: &http.Client{
			Timeout: time.Duration(sec) * time.Second,
		},
	}
}

func (s *DiscordSender) ProviderName() string { return "discord" }

func (s *DiscordSender) Send(ctx context.Context, n *domain.Notification) (domain.DeliveryResult, error) {
	start := time.Now()
	text := ""
	if n.RenderedContent != nil {
		text = strings.TrimSpace(n.RenderedContent.Body)
	}
	if text == "" {
		text = fmt.Sprintf("[%s] notification %s", n.Type, n.ID.String())
	}

	payload := map[string]any{
		"content": text,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return domain.DeliveryResult{Provider: s.ProviderName()}, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimSpace(s.cfg.WebhookURL), bytes.NewReader(body))
	if err != nil {
		return domain.DeliveryResult{Provider: s.ProviderName()}, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.client.Do(req)
	latencyMs := int(time.Since(start).Milliseconds())
	if err != nil {
		return domain.DeliveryResult{Provider: s.ProviderName(), LatencyMs: latencyMs, ErrorMessage: err.Error()}, err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return domain.DeliveryResult{
			Provider:     s.ProviderName(),
			LatencyMs:    latencyMs,
			ErrorCode:    fmt.Sprintf("HTTP_%d", resp.StatusCode),
			ErrorMessage: string(respBody),
		}, fmt.Errorf("discord returned HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	return domain.DeliveryResult{
		Success:       true,
		Provider:      s.ProviderName(),
		ProviderMsgID: fmt.Sprintf("discord-%d", time.Now().UnixMilli()),
		LatencyMs:     latencyMs,
	}, nil
}

func (s *DiscordSender) GetStatus(ctx context.Context, providerMsgID string) (domain.DeliveryResult, error) {
	return domain.DeliveryResult{
		Provider:      s.ProviderName(),
		ProviderMsgID: providerMsgID,
		ErrorMessage:  "status polling not supported for discord webhooks",
	}, nil
}

