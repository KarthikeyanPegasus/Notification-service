package domain

import (
	"time"

	"github.com/google/uuid"
)

type SuppressionType string

const (
	SuppressionTypeEmail SuppressionType = "email"
	SuppressionTypeSMS   SuppressionType = "sms"
)

type Suppression struct {
	ID        uuid.UUID       `json:"id" db:"id"`
	Type      SuppressionType `json:"type" db:"type"`
	Value     string          `json:"value" db:"value"`
	Reason    string          `json:"reason" db:"reason"`
	Metadata  map[string]any  `json:"metadata" db:"metadata"`
	CreatedAt time.Time       `json:"created_at" db:"created_at"`
}

type OptOut struct {
	ID        uuid.UUID        `json:"id" db:"id"`
	UserID    uuid.UUID        `json:"user_id" db:"user_id"`
	Channel   Channel          `json:"channel" db:"channel"`
	Reason    string           `json:"reason" db:"reason"`
	Source    string           `json:"source" db:"source"`
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
