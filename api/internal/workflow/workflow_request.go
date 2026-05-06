package workflow

import (
	"github.com/google/uuid"
	"github.com/spidey/notification-service/internal/domain"
)

// DirectContent carries inline message content, as an alternative to template-based rendering.
type DirectContent struct {
	Subject string
	Body    string
	HTML    string
}

// WorkflowRequest is the data passed to Temporal/Cadence Workflows.
type WorkflowRequest struct {
	ID                uuid.UUID
	Channel           domain.Channel
	Priority          domain.Priority // drives activity ScheduleToCloseTimeout (SLA)
	Recipient         string
	Type              string
	TemplateID        *string
	TemplateVariables map[string]string
	DirectContent     *DirectContent
	IdempotencyKey    string
	ForcedVendor      string
	// ClientID is the API key ID of the calling client, used for scoped vendor config
	// and rate limiting within delivery activities.
	ClientID string
	// TraceID is the originating HTTP X-Request-ID, propagated for end-to-end correlation.
	TraceID string
	// UserID is the optional user identifier used for preference and governance checks.
	UserID string
}
