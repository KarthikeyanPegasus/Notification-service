package notification

import "time"

// ── Enumerations ─────────────────────────────────────────────────────────────

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
	StatusPending    NotificationStatus = "pending"
	StatusQueued     NotificationStatus = "queued"
	StatusSent       NotificationStatus = "sent"
	StatusDelivered  NotificationStatus = "delivered"
	StatusFailed     NotificationStatus = "failed"
	StatusCancelled  NotificationStatus = "cancelled"
	StatusBounced    NotificationStatus = "bounced"
	StatusSuppressed NotificationStatus = "suppressed"
)

// ── Core notification types ───────────────────────────────────────────────────

type RenderedContent struct {
	Subject string            `json:"subject,omitempty"`
	Body    string            `json:"body,omitempty"`
	Data    map[string]string `json:"data,omitempty"`
}

type NotificationAttempt struct {
	ID             string     `json:"id"`
	NotificationID string     `json:"notification_id"`
	AttemptNumber  int        `json:"attempt_number"`
	Status         string     `json:"status"`
	Provider       string     `json:"provider"`
	ProviderMsgID  *string    `json:"provider_msg_id,omitempty"`
	ErrorCode      *string    `json:"error_code,omitempty"`
	ErrorMessage   *string    `json:"error_message,omitempty"`
	LatencyMs      *int       `json:"latency_ms,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
}

type NotificationEvent struct {
	ID             string         `json:"id"`
	NotificationID string         `json:"notification_id"`
	EventType      string         `json:"event_type"`
	Metadata       map[string]any `json:"metadata,omitempty"`
	CreatedAt      time.Time      `json:"created_at"`
}

type Notification struct {
	ID              string             `json:"id"`
	IdempotencyKey  string             `json:"idempotency_key"`
	UserID          string             `json:"user_id"`
	APIKeyID        *string            `json:"api_key_id,omitempty"`
	ClientName      string             `json:"client_name,omitempty"`
	Channel         Channel            `json:"channel"`
	Priority        Priority           `json:"priority"`
	Type            string             `json:"type"`
	TemplateID      *string            `json:"template_id,omitempty"`
	RenderedContent *RenderedContent   `json:"rendered_content,omitempty"`
	Recipient       string             `json:"recipient"`
	Status          NotificationStatus `json:"status"`
	Provider        string             `json:"provider,omitempty"`
	Source          string             `json:"source,omitempty"`
	ForcedVendor    string             `json:"forced_vendor,omitempty"`
	ScheduledAt     *time.Time         `json:"scheduled_at,omitempty"`
	SentAt          *time.Time         `json:"sent_at,omitempty"`
	DeliveredAt     *time.Time         `json:"delivered_at,omitempty"`
	CreatedAt       time.Time          `json:"created_at"`
	UpdatedAt       time.Time          `json:"updated_at"`
	Attempts        []NotificationAttempt `json:"attempts,omitempty"`
	Events          []NotificationEvent   `json:"events,omitempty"`
}

type ScheduledNotification struct {
	ID              string             `json:"id"`
	NotificationID  string             `json:"notification_id"`
	UserID          string             `json:"user_id"`
	Channel         Channel            `json:"channel"`
	TemplateID      *string            `json:"template_id,omitempty"`
	TemplateVars    map[string]string  `json:"template_vars,omitempty"`
	ScheduledAt     time.Time          `json:"scheduled_at"`
	OriginalAt      time.Time          `json:"original_at"`
	Status          NotificationStatus `json:"status"`
	RescheduleCount int                `json:"reschedule_count"`
	CreatedAt       time.Time          `json:"created_at"`
	UpdatedAt       time.Time          `json:"updated_at"`
}

// ── Request / response types ──────────────────────────────────────────────────

type SendRequest struct {
	IdempotencyKey    string            `json:"idempotency_key"`
	UserID            string            `json:"user_id"`
	Channels          []Channel         `json:"channels"`
	Type              string            `json:"type"`
	Subject           string            `json:"subject,omitempty"`
	Body              string            `json:"body,omitempty"`
	HTML              string            `json:"html,omitempty"`
	TemplateID        string            `json:"template_id,omitempty"`
	TemplateVariables map[string]string `json:"template_variables,omitempty"`
	Recipient         string            `json:"recipient,omitempty"`
	SlackChannel      string            `json:"slack_channel,omitempty"`
	Priority          Priority          `json:"priority,omitempty"`
	ScheduledAt       *time.Time        `json:"scheduled_at,omitempty"`
}

// NotifyOptions carries optional fields for NotifyBy* helpers. Nil is treated as zero values.
type NotifyOptions struct {
	Subject           string
	Body              string
	HTML              string
	TemplateID        string
	TemplateVariables map[string]string
	Priority          Priority
	ScheduledAt       *time.Time
}

type SendResponse struct {
	NotificationID string     `json:"notification_id"`
	Status         string     `json:"status"`
	WorkflowID     string     `json:"workflow_id,omitempty"`
	ScheduledAt    *time.Time `json:"scheduled_at,omitempty"`
}

type ListNotificationsParams struct {
	Page     int
	PageSize int
	UserID   string
	Channel  Channel
	Status   NotificationStatus
	Recipient string
	Search   string
	DateFrom string // RFC3339
	DateTo   string // RFC3339
	APIKeyID string
}

type ListNotificationsResponse struct {
	Data     []*Notification `json:"data"`
	Total    int             `json:"total"`
	Page     int             `json:"page"`
	PageSize int             `json:"page_size"`
}

type ListScheduledParams struct {
	Page     int
	PageSize int
	Status   NotificationStatus
}

type ListScheduledResponse struct {
	Data     []*ScheduledNotification `json:"data"`
	Total    int                      `json:"total"`
	Page     int                      `json:"page"`
	PageSize int                      `json:"page_size"`
}

type RescheduleRequest struct {
	ScheduledAt time.Time `json:"scheduled_at"`
}

type SyncResponse struct {
	Status         string    `json:"status"`
	ProviderStatus string    `json:"provider_status"`
	SyncedAt       time.Time `json:"synced_at"`
}

type RetriggerResponse struct {
	NotificationID string `json:"notification_id"`
	Status         string `json:"status"`
}

// ── OTP ───────────────────────────────────────────────────────────────────────

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

// ── Reports ───────────────────────────────────────────────────────────────────

type ReportFilters struct {
	DateFrom string // RFC3339
	DateTo   string // RFC3339
	APIKeyID string
}

type ReportSummaryItem struct {
	Channel      Channel `json:"channel"`
	Date         string  `json:"date"`
	Total        int     `json:"total"`
	Sent         int     `json:"sent"`
	Delivered    int     `json:"delivered"`
	Failed       int     `json:"failed"`
	Bounced      int     `json:"bounced"`
	SuccessRate  float64 `json:"success_rate"`
	P50LatencyMs float64 `json:"p50_latency_ms"`
	P95LatencyMs float64 `json:"p95_latency_ms"`
}

type IngressBreakdownItem struct {
	Source string `json:"source"`
	Count  int    `json:"count"`
}

type BreakdownRow struct {
	Key   string `json:"key"`
	Count int    `json:"count"`
}

type VendorMetric struct {
	Provider      string  `json:"provider"`
	Total         int     `json:"total"`
	SuccessRate   float64 `json:"success_rate"`
	AvgLatencyMs  float64 `json:"avg_latency_ms"`
	LatestError   string  `json:"latest_error"`
	LastStatus    string  `json:"last_status"`
	CostPerMsg    float64 `json:"cost_per_msg"`
	EstimatedCost float64 `json:"estimated_cost"`
	Status        string  `json:"status"` // healthy | degraded | down
	PaymentStatus string  `json:"payment_status"`
}

type VendorBilling struct {
	Provider     string   `json:"provider"`
	TotalCost    float64  `json:"total_cost"`
	Balance      *float64 `json:"balance,omitempty"`
	Currency     string   `json:"currency"`
	Period       string   `json:"period"`
	IsActual     bool     `json:"is_actual"`
	Source       string   `json:"source"`
	TotalCount   int64    `json:"total_count"`
	CostPerMsg   float64  `json:"cost_per_msg"`
	Error        string   `json:"error,omitempty"`
}

type ScheduledStats struct {
	TotalScheduled       int      `json:"total_scheduled"`
	Pending              int      `json:"pending"`
	Delivered            int      `json:"delivered"`
	AvgDeliveryLatencyMs *float64 `json:"avg_delivery_latency_ms"`
}

// ── Auth & API keys ───────────────────────────────────────────────────────────

type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type LoginResponse struct {
	Token string   `json:"token"`
	User  UserInfo `json:"user"`
}

type UserInfo struct {
	ID    string `json:"id"`
	Email string `json:"email"`
	Name  string `json:"name"`
	Role  string `json:"role"`
}

type ApiKeyClient struct {
	ID        string     `json:"id"`
	Name      string     `json:"name"`
	Prefix    string     `json:"prefix"`
	CreatedAt time.Time  `json:"created_at"`
	RevokedAt *time.Time `json:"revoked_at,omitempty"`
}

type CreateApiKeyResponse struct {
	Key    ApiKeyClient `json:"key"`
	ApiKey string       `json:"api_key"`
}
