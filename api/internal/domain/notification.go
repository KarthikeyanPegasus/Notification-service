package domain

import (
	"time"

	"github.com/google/uuid"
)

// Channel enumerates supported delivery channels.
type Channel string

const (
	ChannelEmail     Channel = "email"
	ChannelSMS       Channel = "sms"
	ChannelPush      Channel = "push"
	ChannelWebSocket Channel = "websocket"
	ChannelWebhook   Channel = "webhook"
)

func (c Channel) IsValid() bool {
	switch c {
	case ChannelEmail, ChannelSMS, ChannelPush, ChannelWebSocket, ChannelWebhook:
		return true
	}
	return false
}

// Priority determines dispatch order and retry aggressiveness.
type Priority string

const (
	PriorityHigh   Priority = "high"
	PriorityMedium Priority = "medium"
	PriorityLow    Priority = "low"
)

func PriorityFor(channel Channel, notifType string) Priority {
	switch notifType {
	case "otp", "payment_confirmation", "security_alert":
		return PriorityHigh
	case "order_update", "account_activity", "transactional":
		return PriorityMedium
	}
	return PriorityLow
}

// NotificationStatus tracks lifecycle state.
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

// AttemptStatus tracks individual provider call outcomes.
type AttemptStatus string

const (
	AttemptSent      AttemptStatus = "sent"
	AttemptFailed    AttemptStatus = "failed"
	AttemptDelivered AttemptStatus = "delivered"
	AttemptBounced   AttemptStatus = "bounced"
)

// EventType describes notification lifecycle events.
type EventType string

const (
	EventQueued    EventType = "queued"
	EventSent      EventType = "sent"
	EventDelivered EventType = "delivered"
	EventFailed    EventType = "failed"
	EventBounced   EventType = "bounced"
	EventClicked   EventType = "clicked"
	EventOpened    EventType = "opened"
	EventCancelled EventType = "cancelled"
)

// Notification is the core aggregate root.
type Notification struct {
	ID              uuid.UUID          `json:"id" db:"id"`
	IdempotencyKey  string             `json:"idempotency_key" db:"idempotency_key"`
	UserID          uuid.UUID          `json:"user_id" db:"user_id"`
	Channel         Channel            `json:"channel" db:"channel"`
	Priority        Priority           `json:"priority" db:"priority"`
	Type            string             `json:"type" db:"type"`
	TemplateID      *uuid.UUID         `json:"template_id,omitempty" db:"template_id"`
	RenderedContent *RenderedContent   `json:"rendered_content,omitempty" db:"rendered_content"`
	Recipient       string             `json:"recipient" db:"recipient"`
	Status          NotificationStatus `json:"status" db:"status"`
	ScheduledAt     *time.Time         `json:"scheduled_at,omitempty" db:"scheduled_at"`
	SentAt          *time.Time         `json:"sent_at,omitempty" db:"sent_at"`
	DeliveredAt     *time.Time         `json:"delivered_at,omitempty" db:"delivered_at"`
	CreatedAt       time.Time          `json:"created_at" db:"created_at"`
	UpdatedAt       time.Time          `json:"updated_at" db:"updated_at"`
}

// RenderedContent holds the channel-rendered message payload.
type RenderedContent struct {
	Subject  string            `json:"subject,omitempty"`
	Body     string            `json:"body"`
	HTML     string            `json:"html,omitempty"`
	Data     map[string]string `json:"data,omitempty"`
}

// NotificationAttempt records each provider delivery attempt.
type NotificationAttempt struct {
	ID              uuid.UUID     `json:"id" db:"id"`
	NotificationID  uuid.UUID     `json:"notification_id" db:"notification_id"`
	AttemptNumber   int           `json:"attempt_number" db:"attempt_number"`
	Status          AttemptStatus `json:"status" db:"status"`
	Provider        string        `json:"provider" db:"provider"`
	ProviderMsgID   *string       `json:"provider_msg_id,omitempty" db:"provider_msg_id"`
	ErrorCode       *string       `json:"error_code,omitempty" db:"error_code"`
	ErrorMessage    *string       `json:"error_message,omitempty" db:"error_message"`
	LatencyMs       *int          `json:"latency_ms,omitempty" db:"latency_ms"`
	CreatedAt       time.Time     `json:"created_at" db:"created_at"`
}

// NotificationEvent is an immutable timeline entry.
type NotificationEvent struct {
	ID             uuid.UUID `json:"id" db:"id"`
	NotificationID uuid.UUID `json:"notification_id" db:"notification_id"`
	EventType      EventType `json:"event_type" db:"event_type"`
	Metadata       map[string]any `json:"metadata,omitempty" db:"metadata"`
	CreatedAt      time.Time `json:"created_at" db:"created_at"`
}

// ScheduledNotification is the authoritative state for future delivery.
type ScheduledNotification struct {
	ID                uuid.UUID          `json:"id" db:"id"`
	NotificationID    uuid.UUID          `json:"notification_id" db:"notification_id"`
	UserID            uuid.UUID          `json:"user_id" db:"user_id"`
	Channel           Channel            `json:"channel" db:"channel"`
	TemplateID        *uuid.UUID         `json:"template_id,omitempty" db:"template_id"`
	TemplateVars      map[string]string  `json:"template_vars,omitempty" db:"template_vars"`
	ScheduledAt       time.Time          `json:"scheduled_at" db:"scheduled_at"`
	OriginalAt        time.Time          `json:"original_at" db:"original_at"`
	WorkflowID        string             `json:"workflow_id" db:"cadence_workflow_id"`
	RunID             string             `json:"run_id" db:"cadence_run_id"`
	Status            NotificationStatus `json:"status" db:"status"`
	RescheduleCount   int                `json:"reschedule_count" db:"reschedule_count"`
	CreatedAt         time.Time          `json:"created_at" db:"created_at"`
	UpdatedAt         time.Time          `json:"updated_at" db:"updated_at"`
}

// NotificationTemplate holds parameterised message templates.
type NotificationTemplate struct {
	ID        uuid.UUID `json:"id" db:"id"`
	Name      string    `json:"name" db:"name"`
	Channel   Channel   `json:"channel" db:"channel"`
	Subject   *string   `json:"subject,omitempty" db:"subject"`
	Body      string    `json:"body" db:"body"`
	Version   int       `json:"version" db:"version"`
	IsActive  bool      `json:"is_active" db:"is_active"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
}

// ProviderWebhookEvent stores raw inbound provider callbacks.
type ProviderWebhookEvent struct {
	ID             uuid.UUID      `json:"id" db:"id"`
	Provider       string         `json:"provider" db:"provider"`
	Channel        Channel        `json:"channel" db:"channel"`
	NotificationID *uuid.UUID     `json:"notification_id,omitempty" db:"notification_id"`
	EventType      string         `json:"event_type" db:"event_type"`
	RawPayload     map[string]any `json:"raw_payload" db:"raw_payload"`
	ReceivedAt     time.Time      `json:"received_at" db:"received_at"`
}

// DeviceToken maps users to push device registrations.
type DeviceToken struct {
	ID         uuid.UUID `json:"id" db:"id"`
	UserID     uuid.UUID `json:"user_id" db:"user_id"`
	Token      string    `json:"token" db:"token"`
	Platform   string    `json:"platform" db:"platform"` // ios | android | web
	AppVersion *string   `json:"app_version,omitempty" db:"app_version"`
	IsActive   bool      `json:"is_active" db:"is_active"`
	CreatedAt  time.Time `json:"created_at" db:"created_at"`
	LastSeenAt *time.Time `json:"last_seen_at,omitempty" db:"last_seen_at"`
}

// UserPreferences governs per-user channel opt-ins and DND windows.
type UserPreferences struct {
	UserID          string             `json:"user_id"`
	Channels        map[Channel]bool   `json:"channels"`
	DoNotDisturb    *DNDWindow         `json:"do_not_disturb,omitempty"`
	FrequencyCaps   map[string]int     `json:"frequency_caps,omitempty"`
	UnsubscribedTypes []string         `json:"unsubscribed_types,omitempty"`
	IsSuppressed    bool               `json:"is_suppressed,omitempty"`
	UpdatedAt       time.Time          `json:"updated_at"`
}

// DNDWindow defines quiet hours.
type DNDWindow struct {
	Enabled   bool   `json:"enabled"`
	StartHour int    `json:"start_hour"` // 0-23
	EndHour   int    `json:"end_hour"`   // 0-23
	Timezone  string `json:"timezone"`
}

func (p *UserPreferences) IsChannelEnabled(ch Channel) bool {
	if p == nil {
		return true
	}
	enabled, ok := p.Channels[ch]
	if !ok {
		return true // default allow
	}
	return enabled
}

// SendRequest is the API-facing input for a notification send.
type SendRequest struct {
	IdempotencyKey    string            `json:"idempotency_key" validate:"required,max=128"`
	UserID            string            `json:"user_id" validate:"required,uuid4"`
	Channels          []Channel         `json:"channels" validate:"required,min=1,dive,oneof=email sms push websocket webhook"`
	Type              string            `json:"type" validate:"required,max=50"`
	TemplateID        *string           `json:"template_id,omitempty" validate:"omitempty,uuid4"`
	TemplateVariables map[string]string `json:"template_variables,omitempty"`
	ScheduledAt       *time.Time        `json:"scheduled_at,omitempty"`
	Recipient         string            `json:"recipient,omitempty"`
}

// BulkSendRequest fans a notification out to a user segment.
type BulkSendRequest struct {
	Type              string            `json:"type" validate:"required"`
	TemplateID        string            `json:"template_id" validate:"required,uuid4"`
	TemplateVariables map[string]string `json:"template_variables,omitempty"`
	UserSegment       map[string]any    `json:"user_segment" validate:"required"`
	Channels          []Channel         `json:"channels" validate:"required,min=1"`
	ScheduledAt       *time.Time        `json:"scheduled_at,omitempty"`
}

// OTPSendRequest initiates OTP generation and delivery.
type OTPSendRequest struct {
	UserID        string `json:"user_id" validate:"required,uuid4"`
	PhoneNumber   string `json:"phone_number" validate:"required,e164"`
	Purpose       string `json:"purpose" validate:"required,oneof=login payment 2fa"`
	ExpirySeconds int    `json:"expiry_seconds" validate:"min=60,max=600"`
}

// OTPVerifyRequest verifies a submitted OTP.
type OTPVerifyRequest struct {
	UserID  string `json:"user_id" validate:"required,uuid4"`
	Purpose string `json:"purpose" validate:"required,oneof=login payment 2fa"`
	OTP     string `json:"otp" validate:"required,len=6,numeric"`
}

// RescheduleRequest updates delivery time for a scheduled notification.
type RescheduleRequest struct {
	ScheduledAt time.Time `json:"scheduled_at" validate:"required"`
}

// UpdatePreferencesRequest updates user channel preferences.
type UpdatePreferencesRequest struct {
	Channels     map[Channel]bool   `json:"channels,omitempty"`
	DoNotDisturb *DNDWindow         `json:"do_not_disturb,omitempty"`
}

// DeliveryResult is the outcome of a single provider send.
type DeliveryResult struct {
	Success       bool
	ProviderMsgID string
	Provider      string
	LatencyMs     int
	ErrorCode     string
	ErrorMessage  string
}

func (r DeliveryResult) IsSuccess() bool { return r.Success }
