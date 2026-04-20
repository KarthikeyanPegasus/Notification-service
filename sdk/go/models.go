package notification

import "time"

type Channel string

const (
	ChannelEmail     Channel = "email"
	ChannelSMS       Channel = "sms"
	ChannelPush      Channel = "push"
	ChannelWebSocket Channel = "websocket"
	ChannelWebhook   Channel = "webhook"
	ChannelSlack     Channel = "slack"
)

type Priority string

const (
	PriorityHigh   Priority = "high"
	PriorityMedium Priority = "medium"
	PriorityLow    Priority = "low"
)

type NotificationStatus string

const (
	StatusPending   NotificationStatus = "pending"
	StatusQueued    NotificationStatus = "queued"
	StatusSent      NotificationStatus = "sent"
	StatusDelivered NotificationStatus = "delivered"
	StatusFailed    NotificationStatus = "failed"
	StatusCancelled NotificationStatus = "cancelled"
	StatusBounced   NotificationStatus = "bounced"
)

type RenderedContent struct {
	Subject string            `json:"subject,omitempty"`
	Body    string            `json:"body,omitempty"`
	Data    map[string]string `json:"data,omitempty"`
}

type Notification struct {
	ID              string             `json:"id"`
	IdempotencyKey  string             `json:"idempotency_key"`
	UserID          string             `json:"user_id"`
	Channel         Channel            `json:"channel"`
	Priority        Priority           `json:"priority"`
	Type            string             `json:"type"`
	TemplateID      *string            `json:"template_id,omitempty"`
	RenderedContent *RenderedContent   `json:"rendered_content,omitempty"`
	Recipient       string             `json:"recipient"`
	Status          NotificationStatus `json:"status"`
	ScheduledAt     *time.Time         `json:"scheduled_at,omitempty"`
	SentAt          *time.Time         `json:"sent_at,omitempty"`
	DeliveredAt     *time.Time         `json:"delivered_at,omitempty"`
	CreatedAt       time.Time          `json:"created_at"`
	UpdatedAt       time.Time          `json:"updated_at"`
}

type SendRequest struct {
	IdempotencyKey    string            `json:"idempotency_key"`
	UserID            string            `json:"user_id"`
	Channels          []Channel         `json:"channels"`
	Type              string            `json:"type"`
	Body              string            `json:"body,omitempty"`
	TemplateID        string            `json:"template_id,omitempty"`
	TemplateVariables map[string]string `json:"template_variables,omitempty"`
	Recipient         string            `json:"recipient,omitempty"`
	ScheduledAt       *time.Time        `json:"scheduled_at,omitempty"`
}

// NotifyOptions carries optional fields for NotifyBy* helpers. Nil is treated as zero values.
type NotifyOptions struct {
	Body              string
	TemplateID        string
	TemplateVariables map[string]string
	ScheduledAt       *time.Time
}

type SendResponse struct {
	NotificationID string `json:"notification_id"`
	Status         string `json:"status"`
}

type ListNotificationsParams struct {
	Page     int
	PageSize int
	UserID   string
	Channel  Channel
	Status   NotificationStatus
}

type ListNotificationsResponse struct {
	Data     []*Notification `json:"data"`
	Total    int             `json:"total"`
	Page     int             `json:"page"`
	PageSize int             `json:"page_size"`
}

type NotificationDetailResponse struct {
	Notification *Notification    `json:"notification"`
	Attempts     []map[string]any `json:"attempts"`
	Events       []map[string]any `json:"events"`
}

type OTPSendRequest struct {
	UserID        string `json:"user_id"`
	PhoneNumber   string `json:"phone_number"`
	Purpose       string `json:"purpose"`
	ExpirySeconds int    `json:"expiry_seconds,omitempty"`
}

type OTPSendResponse struct {
	OTPID    string    `json:"otp_id"`
	ExpiryAt time.Time `json:"expiry_at"`
}

type OTPVerifyRequest struct {
	UserID  string `json:"user_id"`
	Purpose string `json:"purpose"`
	OTP     string `json:"otp"`
}

type OTPVerifyResponse struct {
	Verified bool `json:"verified"`
}

type ReportSummaryItem struct {
	Channel      Channel `json:"channel"`
	Total        int     `json:"total"`
	SuccessRate  float64 `json:"success_rate"`
	P50LatencyMs float64 `json:"p50_latency_ms"`
	P95LatencyMs float64 `json:"p95_latency_ms"`
}
