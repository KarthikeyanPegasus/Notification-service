package email

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

const postmarkAPIBase = "https://api.postmarkapp.com"

type PostmarkSender struct {
	fromEmail   string
	fromName    string
	replyTo     string
	serverToken string
	httpClient  *http.Client
}

func NewPostmarkSender(cfg config.PostmarkConfig) *PostmarkSender {
	if strings.TrimSpace(cfg.ServerToken) == "" || strings.TrimSpace(cfg.FromEmail) == "" {
		return nil
	}
	return &PostmarkSender{
		fromEmail:   strings.TrimSpace(cfg.FromEmail),
		fromName:    strings.TrimSpace(cfg.FromName),
		replyTo:     strings.TrimSpace(cfg.ReplyTo),
		serverToken: strings.TrimSpace(cfg.ServerToken),
		httpClient:  &http.Client{Timeout: 10 * time.Second},
	}
}

func (s *PostmarkSender) ProviderName() string { return "postmark" }

func (s *PostmarkSender) ValidateEmail(email string) error {
	if strings.TrimSpace(email) == "" || !strings.Contains(email, "@") {
		return fmt.Errorf("invalid email")
	}
	return nil
}

func (s *PostmarkSender) Send(ctx context.Context, n *domain.Notification) (domain.DeliveryResult, error) {
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

	from := s.fromEmail
	if s.fromName != "" {
		from = fmt.Sprintf("%s <%s>", s.fromName, s.fromEmail)
	}

	payload := map[string]interface{}{
		"From":     from,
		"To":       toEmail,
		"Subject":  subject,
		"TextBody": textBody,
		"HtmlBody": htmlBody,
		"ReplyTo":  s.replyTo,
		"Metadata": map[string]string{
			"notification_id": n.ID.String(),
		},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return domain.DeliveryResult{Provider: s.ProviderName(), ErrorMessage: err.Error()}, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, postmarkAPIBase+"/email", bytes.NewReader(body))
	if err != nil {
		return domain.DeliveryResult{Provider: s.ProviderName(), ErrorMessage: err.Error()}, err
	}
	req.Header.Set("X-Postmark-Server-Token", s.serverToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := s.httpClient.Do(req)
	latencyMs := int(time.Since(start).Milliseconds())
	if err != nil {
		return domain.DeliveryResult{Provider: s.ProviderName(), LatencyMs: latencyMs, ErrorMessage: err.Error()}, err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 8192))
	if resp.StatusCode >= 400 {
		return domain.DeliveryResult{
			Provider:     s.ProviderName(),
			LatencyMs:    latencyMs,
			ErrorCode:    fmt.Sprintf("HTTP_%d", resp.StatusCode),
			ErrorMessage: string(respBody),
		}, fmt.Errorf("postmark returned HTTP %d", resp.StatusCode)
	}

	var result struct {
		MessageID string `json:"MessageID"`
		ErrorCode int    `json:"ErrorCode"`
		Message   string `json:"Message"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return domain.DeliveryResult{Provider: s.ProviderName(), LatencyMs: latencyMs, ErrorMessage: err.Error()}, err
	}
	if result.ErrorCode != 0 {
		return domain.DeliveryResult{
			Provider:     s.ProviderName(),
			LatencyMs:    latencyMs,
			ErrorCode:    fmt.Sprintf("POSTMARK_%d", result.ErrorCode),
			ErrorMessage: result.Message,
		}, fmt.Errorf("postmark error %d: %s", result.ErrorCode, result.Message)
	}

	msgID := strings.TrimSpace(result.MessageID)
	if msgID == "" {
		msgID = fmt.Sprintf("postmark-%d", time.Now().UnixMilli())
	}
	return domain.DeliveryResult{
		Success:       true,
		Provider:      s.ProviderName(),
		ProviderMsgID: msgID,
		LatencyMs:     latencyMs,
	}, nil
}

// GetStatus polls Postmark's outbound message details endpoint for delivery status.
func (s *PostmarkSender) GetStatus(ctx context.Context, providerMsgID string) (domain.DeliveryResult, error) {
	if s.serverToken == "" || providerMsgID == "" {
		return domain.DeliveryResult{
			Provider:      s.ProviderName(),
			ProviderMsgID: providerMsgID,
			ErrorMessage:  "postmark not configured for status polling",
		}, nil
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		postmarkAPIBase+"/messages/outbound/"+providerMsgID+"/details", nil)
	if err != nil {
		return domain.DeliveryResult{Provider: s.ProviderName(), ProviderMsgID: providerMsgID}, err
	}
	req.Header.Set("X-Postmark-Server-Token", s.serverToken)
	req.Header.Set("Accept", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return domain.DeliveryResult{Provider: s.ProviderName(), ProviderMsgID: providerMsgID}, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 8192))
	if resp.StatusCode >= 400 {
		return domain.DeliveryResult{
			Provider:      s.ProviderName(),
			ProviderMsgID: providerMsgID,
			ErrorCode:     fmt.Sprintf("HTTP_%d", resp.StatusCode),
			ErrorMessage:  string(body),
		}, fmt.Errorf("postmark returned HTTP %d", resp.StatusCode)
	}

	var result struct {
		MessageEvents []struct {
			RecordType string `json:"RecordType"`
			Type       string `json:"Type"`
		} `json:"MessageEvents"`
		Status string `json:"Status"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return domain.DeliveryResult{Provider: s.ProviderName(), ProviderMsgID: providerMsgID, ErrorMessage: err.Error()}, err
	}

	for _, ev := range result.MessageEvents {
		switch ev.RecordType {
		case "Delivery":
			return domain.DeliveryResult{
				Success:       true,
				Provider:      s.ProviderName(),
				ProviderMsgID: providerMsgID,
				VendorStatus:  "delivered",
			}, nil
		case "Bounce":
			return domain.DeliveryResult{
				Success:       false,
				Provider:      s.ProviderName(),
				ProviderMsgID: providerMsgID,
				ErrorCode:     ev.Type,
				VendorStatus:  "bounced",
			}, nil
		}
	}

	vendorStatus := result.Status
	if vendorStatus == "" {
		vendorStatus = "unknown"
	}
	return domain.DeliveryResult{
		Success:       result.Status == "Sent",
		Provider:      s.ProviderName(),
		ProviderMsgID: providerMsgID,
		VendorStatus:  vendorStatus,
	}, nil
}
