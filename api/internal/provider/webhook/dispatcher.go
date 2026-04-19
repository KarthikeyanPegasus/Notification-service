package webhook

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/spidey/notification-service/internal/config"
	"github.com/spidey/notification-service/internal/domain"
)

// Dispatcher sends HTTP webhook callbacks to partner endpoints.
type Dispatcher struct {
	signingSecret string
	client        *http.Client
}

func NewDispatcher(cfg config.WebhookProviderConfig) *Dispatcher {
	return &Dispatcher{
		signingSecret: cfg.SigningSecret,
		client: &http.Client{
			Timeout: time.Duration(cfg.TimeoutSeconds) * time.Second,
		},
	}
}

func (d *Dispatcher) ProviderName() string { return "webhook-delivery" }

func (d *Dispatcher) SignRequest(body []byte, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

func (d *Dispatcher) Send(ctx context.Context, n *domain.Notification) (domain.DeliveryResult, error) {
	start := time.Now()

	payload := map[string]any{
		"notification_id": n.ID.String(),
		"user_id":         n.UserID.String(),
		"type":            n.Type,
		"channel":         n.Channel,
		"timestamp":       time.Now().UTC().Format(time.RFC3339),
	}
	if n.RenderedContent != nil {
		payload["data"] = n.RenderedContent.Data
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return domain.DeliveryResult{Provider: d.ProviderName()}, err
	}

	idempotencyKey := n.IdempotencyKey
	if idempotencyKey == "" {
		idempotencyKey = n.ID.String()
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, n.Recipient, strings.NewReader(string(body)))
	if err != nil {
		return domain.DeliveryResult{Provider: d.ProviderName()}, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Webhook-Event", n.Type)
	req.Header.Set("X-Webhook-Id", uuid.New().String())
	req.Header.Set("X-Webhook-Timestamp", time.Now().UTC().Format(time.RFC3339))
	req.Header.Set("X-Webhook-Signature", d.SignRequest(body, d.signingSecret))
	req.Header.Set("Idempotency-Key", idempotencyKey)

	resp, err := d.client.Do(req)
	latencyMs := int(time.Since(start).Milliseconds())
	if err != nil {
		return domain.DeliveryResult{
			Provider:     d.ProviderName(),
			LatencyMs:    latencyMs,
			ErrorMessage: err.Error(),
		}, err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return domain.DeliveryResult{
			Provider:     d.ProviderName(),
			LatencyMs:    latencyMs,
			ErrorCode:    fmt.Sprintf("HTTP_%d", resp.StatusCode),
			ErrorMessage: string(respBody),
		}, fmt.Errorf("webhook returned HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	return domain.DeliveryResult{
		Success:       true,
		Provider:      d.ProviderName(),
		ProviderMsgID: fmt.Sprintf("wh-%d", time.Now().UnixMilli()),
		LatencyMs:     latencyMs,
	}, nil
}

func (d *Dispatcher) GetStatus(ctx context.Context, providerMsgID string) (domain.DeliveryResult, error) {
	return domain.DeliveryResult{
		Provider:      d.ProviderName(),
		ProviderMsgID: providerMsgID,
		ErrorMessage:  "status polling not supported for webhooks",
	}, nil
}
