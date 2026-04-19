package sms

import (
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

const vonageAPIBase = "https://rest.nexmo.com/sms/json"

// VonageSender sends SMS via Vonage (formerly Nexmo) REST API.
type VonageSender struct {
	apiKey    string
	apiSecret string
	from      string
	client    *http.Client
}

func NewVonageSender(cfg config.VonageConfig) *VonageSender {
	return &VonageSender{
		apiKey:    cfg.APIKey,
		apiSecret: cfg.APISecret,
		from:      cfg.From,
		client:    &http.Client{Timeout: 10 * time.Second},
	}
}

func (s *VonageSender) ProviderName() string { return "vonage" }

func (s *VonageSender) NormalizePhone(phone string) string {
	return strings.TrimPrefix(strings.TrimSpace(phone), "+")
}

func (s *VonageSender) Send(ctx context.Context, n *domain.Notification) (domain.DeliveryResult, error) {
	start := time.Now()

	body := ""
	if n.RenderedContent != nil {
		body = n.RenderedContent.Body
	}

	payload := map[string]string{
		"api_key":    s.apiKey,
		"api_secret": s.apiSecret,
		"from":       s.from,
		"to":         s.NormalizePhone(n.Recipient),
		"text":       body,
	}
	data, _ := json.Marshal(payload)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, vonageAPIBase, strings.NewReader(string(data)))
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

	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode >= 400 {
		return domain.DeliveryResult{
			Provider:     s.ProviderName(),
			LatencyMs:    latencyMs,
			ErrorCode:    fmt.Sprintf("HTTP_%d", resp.StatusCode),
			ErrorMessage: string(respBody),
		}, fmt.Errorf("vonage returned HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Messages []struct {
			Status    string `json:"status"`
			MessageID string `json:"message-id"`
		} `json:"messages"`
	}
	msgID := ""
	if err := json.Unmarshal(respBody, &result); err == nil && len(result.Messages) > 0 {
		if result.Messages[0].Status != "0" {
			return domain.DeliveryResult{
				Provider:     s.ProviderName(),
				LatencyMs:    latencyMs,
				ErrorCode:    result.Messages[0].Status,
				ErrorMessage: "vonage delivery error",
			}, fmt.Errorf("vonage: non-zero status %s", result.Messages[0].Status)
		}
		msgID = result.Messages[0].MessageID
	}

	return domain.DeliveryResult{
		Success:       true,
		Provider:      s.ProviderName(),
		ProviderMsgID: msgID,
		LatencyMs:     latencyMs,
	}, nil
}
func (s *VonageSender) GetStatus(ctx context.Context, providerMsgID string) (domain.DeliveryResult, error) {
	return domain.DeliveryResult{
		Provider:      s.ProviderName(),
		ProviderMsgID: providerMsgID,
		ErrorMessage:  "status polling not supported for vonage",
	}, nil
}
