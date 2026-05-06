package domain

import (
	"time"

	"github.com/google/uuid"
)

type SuppressionType string

const (
	SuppressionTypeEmail   SuppressionType = "email"
	SuppressionTypeSMS     SuppressionType = "sms"
	SuppressionTypePush    SuppressionType = "push"
	SuppressionTypeSlack   SuppressionType = "slack"
	SuppressionTypeWebhook SuppressionType = "webhook"
)

// SuppressionTypeForChannel returns the suppression type that applies to a given channel.
// Returns an empty string for channels that use opt-out rather than identifier suppression.
func SuppressionTypeForChannel(ch Channel) SuppressionType {
	switch ch {
	case ChannelEmail:
		return SuppressionTypeEmail
	case ChannelSMS:
		return SuppressionTypeSMS
	case ChannelPush:
		return SuppressionTypePush
	case ChannelSlack:
		return SuppressionTypeSlack
	case ChannelWebhook:
		return SuppressionTypeWebhook
	default:
		return ""
	}
}

type Suppression struct {
	ID        uuid.UUID       `json:"id" db:"id"`
	Type      SuppressionType `json:"type" db:"type"`
	Value     string          `json:"value" db:"value"`
	Reason    string          `json:"reason" db:"reason"`
	Metadata  map[string]any  `json:"metadata" db:"metadata"`
	CreatedBy string          `json:"created_by" db:"created_by"`
	CreatedAt time.Time       `json:"created_at" db:"created_at"`
}

type OptOut struct {
	ID        uuid.UUID        `json:"id" db:"id"`
	UserID    uuid.UUID        `json:"user_id" db:"user_id"`
	Channel   Channel          `json:"channel" db:"channel"`
	Reason    string           `json:"reason" db:"reason"`
	Source    string           `json:"source" db:"source"`
	CreatedBy string           `json:"created_by" db:"created_by"`
	CreatedAt time.Time        `json:"created_at" db:"created_at"`
}

type AddSuppressionRequest struct {
	Type     SuppressionType `json:"type" binding:"required"`
	Value    string          `json:"value" binding:"required"`
	Reason   string          `json:"reason"`
	Metadata map[string]any  `json:"metadata"`
}

type AddOptOutRequest struct {
	UserID  uuid.UUID `json:"user_id" binding:"required"`
	Channel Channel   `json:"channel" binding:"required"`
	Reason  string    `json:"reason"`
	Source  string    `json:"source"`
}
