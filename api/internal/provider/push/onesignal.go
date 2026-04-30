package push

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

type OneSignalSender struct {
	appID  string
	apiKey string
	client *http.Client
}

func NewOneSignalSender(cfg config.OneSignalConfig) *OneSignalSender {
	if strings.TrimSpace(cfg.AppID) == "" || strings.TrimSpace(cfg.RestAPIKey) == "" {
		return nil
	}
	return &OneSignalSender{
		appID:  strings.TrimSpace(cfg.AppID),
		apiKey: strings.TrimSpace(cfg.RestAPIKey),
		client: &http.Client{Timeout: 20 * time.Second},
	}
}

func (s *OneSignalSender) ProviderName() string { return "onesignal" }

func (s *OneSignalSender) Send(ctx context.Context, n *domain.Notification) (domain.DeliveryResult, error) {
	start := time.Now()

	// Interpret recipient as OneSignal "external user id" by default.
	recipient := strings.TrimSpace(n.Recipient)
	if recipient == "" {
		return domain.DeliveryResult{Provider: s.ProviderName(), ErrorMessage: "missing recipient"}, fmt.Errorf("missing recipient")
	}

	text := ""
	if n.RenderedContent != nil {
		text = strings.TrimSpace(n.RenderedContent.Body)
	}
	if text == "" {
		text = fmt.Sprintf("[%s] notification %s", n.Type, n.ID.String())
	}

	// OneSignal v1 REST API: https://onesignal.com/api/v1/notifications
	payload := map[string]any{
		"app_id":                    s.appID,
		"include_external_user_ids": []string{recipient},
		"contents": map[string]string{
			"en": text,
		},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return domain.DeliveryResult{Provider: s.ProviderName(), ErrorMessage: err.Error()}, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://onesignal.com/api/v1/notifications", bytes.NewReader(body))
	if err != nil {
		return domain.DeliveryResult{Provider: s.ProviderName(), ErrorMessage: err.Error()}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Basic "+s.apiKey)

	httpResp, err := s.client.Do(req)
	latencyMs := int(time.Since(start).Milliseconds())
	if err != nil {
		return domain.DeliveryResult{Provider: s.ProviderName(), LatencyMs: latencyMs, ErrorMessage: err.Error()}, err
	}
	defer httpResp.Body.Close()

	respBody, _ := io.ReadAll(io.LimitReader(httpResp.Body, 4096))
	if httpResp.StatusCode < 200 || httpResp.StatusCode >= 300 {
		return domain.DeliveryResult{
			Provider:     s.ProviderName(),
			LatencyMs:    latencyMs,
			ErrorCode:    fmt.Sprintf("HTTP_%d", httpResp.StatusCode),
			ErrorMessage: string(respBody),
		}, fmt.Errorf("onesignal returned HTTP %d: %s", httpResp.StatusCode, string(respBody))
	}

	var parsed struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(respBody, &parsed)
	id := strings.TrimSpace(parsed.ID)
	if id == "" {
		id = fmt.Sprintf("onesignal-%d", time.Now().UnixMilli())
	}

	return domain.DeliveryResult{
		Success:       true,
		Provider:      s.ProviderName(),
		ProviderMsgID: id,
		LatencyMs:     latencyMs,
	}, nil
}

// GetStatus polls the OneSignal Notifications API for delivery status.
func (s *OneSignalSender) GetStatus(ctx context.Context, providerMsgID string) (domain.DeliveryResult, error) {
	if s.appID == "" || providerMsgID == "" {
		return domain.DeliveryResult{
			Provider:      s.ProviderName(),
			ProviderMsgID: providerMsgID,
			ErrorMessage:  "onesignal not configured for status polling",
		}, nil
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		"https://onesignal.com/api/v1/notifications/"+providerMsgID+"?app_id="+s.appID, nil)
	if err != nil {
		return domain.DeliveryResult{Provider: s.ProviderName(), ProviderMsgID: providerMsgID}, err
	}
	req.Header.Set("Authorization", "Basic "+s.apiKey)
	req.Header.Set("Accept", "application/json")

	httpResp, err := s.client.Do(req)
	if err != nil {
		return domain.DeliveryResult{Provider: s.ProviderName(), ProviderMsgID: providerMsgID}, err
	}
	defer httpResp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(httpResp.Body, 4096))
	if httpResp.StatusCode >= 400 {
		return domain.DeliveryResult{
			Provider:      s.ProviderName(),
			ProviderMsgID: providerMsgID,
			ErrorCode:     fmt.Sprintf("HTTP_%d", httpResp.StatusCode),
			ErrorMessage:  string(body),
		}, fmt.Errorf("onesignal returned HTTP %d", httpResp.StatusCode)
	}

	var result struct {
		Successful int `json:"successful"`
		Failed     int `json:"failed"`
		Errored    int `json:"errored"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return domain.DeliveryResult{Provider: s.ProviderName(), ProviderMsgID: providerMsgID, ErrorMessage: err.Error()}, err
	}

	success := result.Successful > 0 && result.Failed == 0 && result.Errored == 0
	errCode := ""
	vendorStatus := "queued"
	if success {
		vendorStatus = "delivered"
	} else if result.Failed > 0 {
		vendorStatus = "failed"
		errCode = "DELIVERY_FAILED"
	} else if result.Errored > 0 {
		vendorStatus = "errored"
		errCode = "ERRORED"
	}

	return domain.DeliveryResult{
		Success:      success,
		Provider:     s.ProviderName(),
		ProviderMsgID: providerMsgID,
		ErrorCode:    errCode,
		VendorStatus: vendorStatus,
		ErrorMessage: fmt.Sprintf("successful=%d failed=%d errored=%d", result.Successful, result.Failed, result.Errored),
	}, nil
}

