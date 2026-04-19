//go:build ignore

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/google/uuid"
)

const (
	baseURL = "http://127.0.0.1:8080"
)

type SendRequest struct {
	IdempotencyKey    string            `json:"idempotency_key"`
	UserID            string            `json:"user_id"`
	Channels          []string          `json:"channels"`
	Type              string            `json:"type"`
	Recipient         string            `json:"recipient"`
	TemplateVariables map[string]string `json:"template_variables"`
	ScheduledAt       *time.Time        `json:"scheduled_at,omitempty"`
}

type SendResponse struct {
	ID          string     `json:"notification_id"`
	Status      string     `json:"status"`
	ScheduledAt *time.Time `json:"scheduled_at"`
}

type NotificationDetail struct {
	Notification struct {
		ID          string     `json:"id"`
		Status      string     `json:"status"`
		Recipient   string     `json:"recipient"`
		Channel     string     `json:"channel"`
		ScheduledAt *time.Time `json:"scheduled_at"`
	} `json:"notification"`
}

func main() {
	// 1. Prepare Request
	reqBody := SendRequest{
		IdempotencyKey: uuid.New().String(),
		UserID:         uuid.New().String(),
		Channels:       []string{"email"},
		Type:           "transactional",
		Recipient:      "karthikeyan@spideysense.in",
		TemplateVariables: map[string]string{
			"name": "Karthikeyan",
		},
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		log.Fatalf("Failed to marshal request: %v", err)
	}

	fmt.Printf("📧 Triggering email notification flow for %s...\n", reqBody.Recipient)

	// 2. POST /v1/notifications
	resp, err := http.Post(baseURL+"/v1/notifications", "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		log.Fatalf("Failed to call API: %v. Is the server running?", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusAccepted && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		log.Fatalf("API returned error %d: %s", resp.StatusCode, string(body))
	}

	var sendResp SendResponse
	if err := json.NewDecoder(resp.Body).Decode(&sendResp); err != nil {
		log.Fatalf("Failed to decode response: %v", err)
	}

	notificationID := sendResp.ID
	fmt.Printf("✅ Email accepted! ID: %s\n", notificationID)

	// 3. Poll for status
	fmt.Println("⏳ Polling for status update...")
	timeout := time.After(45 * time.Second)
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			log.Fatalf("❌ Timeout reached waiting for email to be sent.")
		case <-ticker.C:
			detail, err := getNotificationDetail(notificationID)
			if err != nil {
				fmt.Printf("⚠️  Error fetching status: %v\n", err)
				continue
			}

			status := detail.Notification.Status
			fmt.Printf("🔄 Current status: [%s]\n", status)

			if status == "sent" || status == "delivered" {
				fmt.Printf("\n✨ SUCCESS! Email flow completed. Notification %s is now %s.\n", notificationID, status)
				return
			}

			if status == "failed" || status == "bounced" || status == "cancelled" {
				log.Fatalf("❌ Notification reached terminal state: %s", status)
			}
		}
	}
}

func getNotificationDetail(id string) (*NotificationDetail, error) {
	resp, err := http.Get(fmt.Sprintf("%s/v1/notifications/%s", baseURL, id))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API error %d: %s", resp.StatusCode, string(body))
	}

	var detail NotificationDetail
	if err := json.NewDecoder(resp.Body).Decode(&detail); err != nil {
		return nil, err
	}

	return &detail, nil
}
