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

const plivoAPIBase = "https://api.plivo.com/v1"

// PlivoSender sends SMS via Plivo REST API.
type PlivoSender struct {
	authID     string
	authToken  string
	fromNumber string
	client     *http.Client
}

func NewPlivoSender(cfg config.PlivoConfig) *PlivoSender {
	return &PlivoSender{
		authID:     cfg.AuthID,
		authToken:  cfg.AuthToken,
		fromNumber: cfg.FromNumber,
		client:     &http.Client{Timeout: 10 * time.Second},
	}
}

func (s *PlivoSender) ProviderName() string { return "plivo" }

func (s *PlivoSender) NormalizePhone(phone string) string {
	phone = strings.TrimSpace(phone)
	return strings.TrimPrefix(phone, "+")
}

func (s *PlivoSender) Send(ctx context.Context, n *domain.Notification) (domain.DeliveryResult, error) {
	start := time.Now()

	body := ""
	if n.RenderedContent != nil {
		body = n.RenderedContent.Body
	}

	payload := map[string]string{
		"src":  s.fromNumber,
		"dst":  s.NormalizePhone(n.Recipient),
		"text": body,
	}
	data, _ := json.Marshal(payload)

	apiURL := fmt.Sprintf("%s/Account/%s/Message/", plivoAPIBase, s.authID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, strings.NewReader(string(data)))
	if err != nil {
		return domain.DeliveryResult{Provider: s.ProviderName()}, err
	}

	req.SetBasicAuth(s.authID, s.authToken)
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
		}, fmt.Errorf("plivo returned HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		MessageUUID []string `json:"message_uuid"`
	}
	msgID := ""
	if err := json.Unmarshal(respBody, &result); err == nil && len(result.MessageUUID) > 0 {
		msgID = result.MessageUUID[0]
	}

	return domain.DeliveryResult{
		Success:       true,
		Provider:      s.ProviderName(),
		ProviderMsgID: msgID,
		LatencyMs:     latencyMs,
	}, nil
}

func (s *PlivoSender) GetStatus(ctx context.Context, providerMsgID string) (domain.DeliveryResult, error) {
	return domain.DeliveryResult{
		Provider:      s.ProviderName(),
		ProviderMsgID: providerMsgID,
		ErrorMessage:  "status polling not supported for plivo",
	}, nil
}
