package webhook

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/spidey/notification-service/internal/config"
	"github.com/spidey/notification-service/internal/domain"
)

type TelegramSender struct {
	cfg    config.TelegramConfig
	client *http.Client
}

func NewTelegramSender(cfg config.TelegramConfig) *TelegramSender {
	if strings.TrimSpace(cfg.BotToken) == "" || strings.TrimSpace(cfg.ChatID) == "" {
		return nil
	}
	sec := cfg.TimeoutSeconds
	if sec <= 0 {
		sec = 30
	}
	return &TelegramSender{
		cfg: cfg,
		client: &http.Client{
			Timeout: time.Duration(sec) * time.Second,
		},
	}
}

func (s *TelegramSender) ProviderName() string { return "telegram" }

func (s *TelegramSender) Send(ctx context.Context, n *domain.Notification) (domain.DeliveryResult, error) {
	start := time.Now()
	text := ""
	if n.RenderedContent != nil {
		text = strings.TrimSpace(n.RenderedContent.Body)
	}
	if text == "" {
		text = fmt.Sprintf("[%s] notification %s", n.Type, n.ID.String())
	}

	endpoint := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", strings.TrimSpace(s.cfg.BotToken))
	payload := url.Values{}
	payload.Set("chat_id", strings.TrimSpace(s.cfg.ChatID))
	payload.Set("text", text)
	payload.Set("disable_web_page_preview", "true")

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewBufferString(payload.Encode()))
	if err != nil {
		return domain.DeliveryResult{Provider: s.ProviderName()}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := s.client.Do(req)
	latencyMs := int(time.Since(start).Milliseconds())
	if err != nil {
		return domain.DeliveryResult{Provider: s.ProviderName(), LatencyMs: latencyMs, ErrorMessage: err.Error()}, err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return domain.DeliveryResult{
			Provider:     s.ProviderName(),
			LatencyMs:    latencyMs,
			ErrorCode:    fmt.Sprintf("HTTP_%d", resp.StatusCode),
			ErrorMessage: string(respBody),
		}, fmt.Errorf("telegram returned HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	// Best-effort parse for message id.
	var parsed struct {
		Ok     bool `json:"ok"`
		Result struct {
			MessageID int `json:"message_id"`
		} `json:"result"`
	}
	_ = json.Unmarshal(respBody, &parsed)
	msgID := fmt.Sprintf("telegram-%d", time.Now().UnixMilli())
	if parsed.Result.MessageID != 0 {
		msgID = fmt.Sprintf("telegram-%d", parsed.Result.MessageID)
	}

	return domain.DeliveryResult{
		Success:       true,
		Provider:      s.ProviderName(),
		ProviderMsgID: msgID,
		LatencyMs:     latencyMs,
	}, nil
}

func (s *TelegramSender) GetStatus(ctx context.Context, providerMsgID string) (domain.DeliveryResult, error) {
	return domain.DeliveryResult{
		Provider:      s.ProviderName(),
		ProviderMsgID: providerMsgID,
		ErrorMessage:  "status polling not supported for telegram",
	}, nil
}

