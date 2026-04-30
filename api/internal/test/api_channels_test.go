package test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/spidey/notification-service/internal/config"
	"github.com/spidey/notification-service/internal/domain"
	"github.com/spidey/notification-service/internal/handler"
	"go.uber.org/zap"
)

func TestNotificationAPI_Channels(t *testing.T) {
	// 1. Setup Mock Services
	mockNotifSvc := &MockNotificationService{}
	mockSchedSvc := &MockSchedulerService{}
	
	// 2. Setup Router Dependencies
	cfg := &config.Config{}
	cfg.Server.Mode = "debug" // Bypass Auth
	cfg.Security.RateLimit.Enabled = false
	
	logger := zap.NewNop()
	
	deps := handler.Dependencies{
		NotificationHandler: handler.NewNotificationHandler(mockNotifSvc, mockSchedSvc, nil, logger),
		Config:              cfg,
		// Other handlers can be nil for these tests as we only hit notification endpoints
	}
	
	router := handler.NewRouter(deps)
	
	// 3. Define Channel Test Cases
	testCases := []struct {
		name     string
		channel  domain.Channel
		payload  map[string]any
		expected int
	}{
		{
			name:    "Email Channel",
			channel: domain.ChannelEmail,
			payload: map[string]any{
				"idempotency_key": uuid.New().String(),
				"channels":        []string{"email"},
				"type":            "transactional",
				"recipient":       "test@example.com",
				"subject":         "Hello Email",
				"body":            "Email Body",
			},
			expected: http.StatusAccepted,
		},
		{
			name:    "SMS Channel",
			channel: domain.ChannelSMS,
			payload: map[string]any{
				"idempotency_key": uuid.New().String(),
				"channels":        []string{"sms"},
				"type":            "otp",
				"recipient":       "+1234567890",
				"body":            "Your OTP is 123456",
			},
			expected: http.StatusAccepted,
		},
		{
			name:    "Push Channel",
			channel: domain.ChannelPush,
			payload: map[string]any{
				"idempotency_key": uuid.New().String(),
				"channels":        []string{"push"},
				"type":            "promo",
				"recipient":       "device-token-abc",
				"body":            "Push Notification Content",
			},
			expected: http.StatusAccepted,
		},
		{
			name:    "Slack Channel",
			channel: domain.ChannelSlack,
			payload: map[string]any{
				"idempotency_key": uuid.New().String(),
				"channels":        []string{"slack"},
				"type":            "transactional",
				"slack_channel":   "#alerts",
				"body":            "Slack message",
			},
			expected: http.StatusAccepted,
		},
		{
			name:    "Webhook Channel",
			channel: domain.ChannelWebhook,
			payload: map[string]any{
				"idempotency_key": uuid.New().String(),
				"channels":        []string{"webhook"},
				"type":            "event",
				"recipient":       "https://webhook.site/test",
				"body":            `{"event": "test"}`,
			},
			expected: http.StatusAccepted,
		},
		{
			name:    "WebSocket Channel",
			channel: domain.ChannelWebSocket,
			payload: map[string]any{
				"idempotency_key": uuid.New().String(),
				"channels":        []string{"websocket"},
				"type":            "realtime",
				"user_id":         uuid.New().String(),
				"body":            "Live update",
			},
			expected: http.StatusAccepted,
		},
		{
			name:    "Invalid Channel",
			channel: "fax",
			payload: map[string]any{
				"idempotency_key": uuid.New().String(),
				"channels":        []string{"fax"},
				"type":            "legacy",
				"recipient":       "12345",
			},
			expected: http.StatusBadRequest,
		},
	}
	
	// 4. Run Tests
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			jsonPayload, _ := json.Marshal(tc.payload)
			req := httptest.NewRequest(http.MethodPost, "/v1/notifications", bytes.NewBuffer(jsonPayload))
			req.Header.Set("Content-Type", "application/json")
			
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)
			
			if w.Code != tc.expected {
				t.Errorf("expected status %d, got %d. Body: %s", tc.expected, w.Code, w.Body.String())
			}
			
			if tc.expected == http.StatusAccepted {
				var resp map[string]string
				if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
					t.Fatalf("failed to decode response: %v", err)
				}
				if resp["notification_id"] == "" {
					t.Error("expected notification_id in response")
				}
			}
		})
	}
}
