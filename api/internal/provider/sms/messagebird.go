package sms

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	messagebird "github.com/messagebird/go-rest-api/v9"
	mbsms "github.com/messagebird/go-rest-api/v9/sms"

	"github.com/spidey/notification-service/internal/config"
	"github.com/spidey/notification-service/internal/domain"
)

type MessageBirdSender struct {
	client     messagebird.Client
	originator string
	accessKey  string
	httpClient *http.Client
}

func NewMessageBirdSender(cfg config.MessageBirdConfig) *MessageBirdSender {
	if strings.TrimSpace(cfg.AccessKey) == "" || strings.TrimSpace(cfg.Originator) == "" {
		return nil
	}
	return &MessageBirdSender{
		client:     messagebird.New(cfg.AccessKey),
		originator: strings.TrimSpace(cfg.Originator),
		accessKey:  strings.TrimSpace(cfg.AccessKey),
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

func (s *MessageBirdSender) ProviderName() string { return "messagebird" }

func (s *MessageBirdSender) NormalizePhone(phone string) string {
	return strings.TrimPrefix(strings.TrimSpace(phone), "+")
}

func (s *MessageBirdSender) Send(ctx context.Context, n *domain.Notification) (domain.DeliveryResult, error) {
	start := time.Now()
	body := ""
	if n.RenderedContent != nil {
		body = strings.TrimSpace(n.RenderedContent.Body)
	}
	if body == "" {
		body = fmt.Sprintf("[%s] notification %s", n.Type, n.ID.String())
	}

	recipient := strings.TrimSpace(n.Recipient)
	if recipient == "" {
		return domain.DeliveryResult{Provider: s.ProviderName(), ErrorMessage: "missing recipient"}, fmt.Errorf("missing recipient")
	}

	resp, err := mbsms.Create(s.client, s.originator, []string{recipient}, body, nil)
	latencyMs := int(time.Since(start).Milliseconds())
	if err != nil {
		return domain.DeliveryResult{
			Provider:     s.ProviderName(),
			LatencyMs:    latencyMs,
			ErrorMessage: err.Error(),
		}, err
	}

	msgID := ""
	if resp != nil {
		msgID = resp.ID
	}
	if msgID == "" {
		msgID = fmt.Sprintf("messagebird-%d", time.Now().UnixMilli())
	}

	return domain.DeliveryResult{
		Success:       true,
		Provider:      s.ProviderName(),
		ProviderMsgID: msgID,
		LatencyMs:     latencyMs,
	}, nil
}

// GetStatus polls the MessageBird Messages API for delivery status.
func (s *MessageBirdSender) GetStatus(ctx context.Context, providerMsgID string) (domain.DeliveryResult, error) {
	if s.accessKey == "" || providerMsgID == "" {
		return domain.DeliveryResult{
			Provider:      s.ProviderName(),
			ProviderMsgID: providerMsgID,
			ErrorMessage:  "messagebird not configured for status polling",
		}, nil
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		"https://rest.messagebird.com/messages/"+providerMsgID, nil)
	if err != nil {
		return domain.DeliveryResult{Provider: s.ProviderName(), ProviderMsgID: providerMsgID}, err
	}
	req.Header.Set("Authorization", "AccessKey "+s.accessKey)
	req.Header.Set("Accept", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return domain.DeliveryResult{Provider: s.ProviderName(), ProviderMsgID: providerMsgID}, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if resp.StatusCode >= 400 {
		return domain.DeliveryResult{
			Provider:      s.ProviderName(),
			ProviderMsgID: providerMsgID,
			ErrorCode:     fmt.Sprintf("HTTP_%d", resp.StatusCode),
			ErrorMessage:  string(body),
		}, fmt.Errorf("messagebird returned HTTP %d", resp.StatusCode)
	}

	var result struct {
		ID         string `json:"id"`
		Recipients struct {
			Items []struct {
				Status          string `json:"status"`
				StatusErrorCode int    `json:"statusErrorCode"`
			} `json:"items"`
		} `json:"recipients"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return domain.DeliveryResult{Provider: s.ProviderName(), ProviderMsgID: providerMsgID, ErrorMessage: err.Error()}, err
	}

	status := ""
	errCode := ""
	if len(result.Recipients.Items) > 0 {
		item := result.Recipients.Items[0]
		status = item.Status
		if item.StatusErrorCode != 0 {
			errCode = fmt.Sprintf("%d", item.StatusErrorCode)
		}
	}

	success := status == "delivered"

	return domain.DeliveryResult{
		Success:       success,
		Provider:      s.ProviderName(),
		ProviderMsgID: providerMsgID,
		ErrorCode:     errCode,
		VendorStatus:  status,
	}, nil
}
