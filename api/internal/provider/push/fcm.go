package push

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/spidey/notification-service/internal/domain"
	"google.golang.org/api/option"
	googleoauth "google.golang.org/api/transport/http"
)

const fcmAPIBase = "https://fcm.googleapis.com/v1/projects"

// FCMSender sends push notifications to Android devices via Firebase Cloud Messaging.
type FCMSender struct {
	projectID           string
	serviceAccountJSON  string
	client              *http.Client
}

func NewFCMSender(serviceAccountJSON string) (*FCMSender, error) {
	if serviceAccountJSON == "" {
		return &FCMSender{}, nil
	}

	var sa struct {
		ProjectID string `json:"project_id"`
	}
	if err := json.Unmarshal([]byte(serviceAccountJSON), &sa); err != nil {
		return nil, fmt.Errorf("parsing service account: %w", err)
	}

	ctx := context.Background()
	httpClient, _, err := googleoauth.NewClient(ctx,
		option.WithCredentialsJSON([]byte(serviceAccountJSON)),
		option.WithScopes("https://www.googleapis.com/auth/firebase.messaging"),
	)
	if err != nil {
		return nil, fmt.Errorf("creating FCM HTTP client: %w", err)
	}

	return &FCMSender{
		projectID:          sa.ProjectID,
		serviceAccountJSON: serviceAccountJSON,
		client:             httpClient,
	}, nil
}

func (s *FCMSender) ProviderName() string { return "fcm" }
func (s *FCMSender) Platform() string     { return "android" }

func (s *FCMSender) DeactivateToken(ctx context.Context, token string) error {
	// Caller handles deactivation based on FCM UNREGISTERED response
	return nil
}

func (s *FCMSender) Send(ctx context.Context, n *domain.Notification) (domain.DeliveryResult, error) {
	start := time.Now()

	if s.client == nil {
		return domain.DeliveryResult{Provider: s.ProviderName(), ErrorMessage: "FCM not configured"}, fmt.Errorf("FCM client not configured")
	}

	title, body := "", ""
	if n.RenderedContent != nil {
		title = n.RenderedContent.Subject
		body = n.RenderedContent.Body
	}

	payload := map[string]any{
		"message": map[string]any{
			"token": n.Recipient,
			"notification": map[string]string{
				"title": title,
				"body":  body,
			},
			"data": n.RenderedContent.Data,
		},
	}
	data, _ := json.Marshal(payload)

	apiURL := fmt.Sprintf("%s/%s/messages:send", fcmAPIBase, s.projectID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, strings.NewReader(string(data)))
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

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		errMsg := string(respBody)
		errCode := fmt.Sprintf("HTTP_%d", resp.StatusCode)
		if resp.StatusCode == 404 || strings.Contains(errMsg, "UNREGISTERED") {
			errCode = "UNREGISTERED"
		}
		return domain.DeliveryResult{
			Provider:     s.ProviderName(),
			LatencyMs:    latencyMs,
			ErrorCode:    errCode,
			ErrorMessage: errMsg,
		}, fmt.Errorf("FCM returned HTTP %d: %s", resp.StatusCode, errMsg)
	}

	var result struct {
		Name string `json:"name"`
	}
	msgID := ""
	if err := json.Unmarshal(respBody, &result); err == nil {
		msgID = result.Name
	}

	return domain.DeliveryResult{
		Success:       true,
		Provider:      s.ProviderName(),
		ProviderMsgID: msgID,
		LatencyMs:     latencyMs,
	}, nil
}

func (s *FCMSender) GetStatus(ctx context.Context, providerMsgID string) (domain.DeliveryResult, error) {
	return domain.DeliveryResult{
		Provider:      s.ProviderName(),
		ProviderMsgID: providerMsgID,
		ErrorMessage:  "status polling not supported for fcm",
	}, nil
}
