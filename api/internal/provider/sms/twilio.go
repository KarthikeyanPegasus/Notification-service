package sms

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/spidey/notification-service/internal/config"
	"github.com/spidey/notification-service/internal/domain"
)

const twilioAPIBase = "https://api.twilio.com/2010-04-01"

// TwilioSender sends SMS via Twilio REST API.
type TwilioSender struct {
	accountSID string
	authToken  string
	fromNumber string
	client     *http.Client
}

func NewTwilioSender(cfg config.TwilioConfig) *TwilioSender {
	return &TwilioSender{
		accountSID: cfg.AccountSID,
		authToken:  cfg.AuthToken,
		fromNumber: cfg.FromNumber,
		client:     &http.Client{Timeout: 10 * time.Second},
	}
}

func (s *TwilioSender) ProviderName() string { return "twilio" }

func (s *TwilioSender) NormalizePhone(phone string) string {
	phone = strings.TrimSpace(phone)
	if !strings.HasPrefix(phone, "+") {
		phone = "+" + phone
	}
	return phone
}

func (s *TwilioSender) Send(ctx context.Context, n *domain.Notification) (domain.DeliveryResult, error) {
	start := time.Now()

	body := ""
	if n.RenderedContent != nil {
		body = strings.TrimSpace(n.RenderedContent.Body)
	}
	if body == "" {
		return domain.DeliveryResult{
			Provider:     s.ProviderName(),
			ErrorMessage: "sms body is empty",
		}, fmt.Errorf("sms body is empty")
	}

	to := s.NormalizePhone(n.Recipient)
	from := s.NormalizePhone(s.fromNumber)

	params := url.Values{}
	params.Set("To", to)
	params.Set("From", from)
	params.Set("Body", body)

	apiURL := fmt.Sprintf("%s/Accounts/%s/Messages.json", twilioAPIBase, s.accountSID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, strings.NewReader(params.Encode()))
	if err != nil {
		return domain.DeliveryResult{Provider: s.ProviderName()}, err
	}

	req.SetBasicAuth(s.accountSID, s.authToken)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

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
		}, fmt.Errorf("twilio returned HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	return domain.DeliveryResult{
		Success:       true,
		Provider:      s.ProviderName(),
		ProviderMsgID: extractTwilioMsgID(respBody),
		LatencyMs:     latencyMs,
	}, nil
}

func (s *TwilioSender) GetStatus(ctx context.Context, providerMsgID string) (domain.DeliveryResult, error) {
	if providerMsgID == "" {
		return domain.DeliveryResult{Provider: s.ProviderName()}, fmt.Errorf("provider_msg_id is empty")
	}

	apiURL := fmt.Sprintf("%s/Accounts/%s/Messages/%s.json", twilioAPIBase, s.accountSID, providerMsgID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return domain.DeliveryResult{Provider: s.ProviderName()}, err
	}

	req.SetBasicAuth(s.accountSID, s.authToken)
	req.Header.Set("Accept", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return domain.DeliveryResult{Provider: s.ProviderName()}, err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return domain.DeliveryResult{
			Provider:     s.ProviderName(),
			ErrorCode:    fmt.Sprintf("HTTP_%d", resp.StatusCode),
			ErrorMessage: string(respBody),
		}, fmt.Errorf("twilio returned HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	status := extractTwilioStatus(respBody)
	success := status == "delivered" || status == "sent" || status == "queued" || status == "sending"

	return domain.DeliveryResult{
		Success:       success,
		Provider:      s.ProviderName(),
		ProviderMsgID: providerMsgID,
		ErrorMessage:  status, // Using error message field to store vendor-specific status for now
	}, nil
}

func extractTwilioStatus(body []byte) string {
	s := string(body)
	key := `"status":"`
	idx := strings.Index(s, key)
	if idx == -1 {
		return "unknown"
	}
	start := idx + len(key)
	end := strings.Index(s[start:], `"`)
	if end == -1 {
		return "unknown"
	}
	return s[start : start+end]
}

func extractTwilioMsgID(body []byte) string {
	// Extract "sid" from Twilio JSON response without importing encoding/json
	// for performance — just a simple string scan
	s := string(body)
	key := `"sid":"`
	idx := strings.Index(s, key)
	if idx == -1 {
		return ""
	}
	start := idx + len(key)
	end := strings.Index(s[start:], `"`)
	if end == -1 {
		return ""
	}
	return s[start : start+end]
}
