package workflow

import (
	"github.com/google/uuid"
	"github.com/spidey/notification-service/internal/domain"
)

// WorkflowRequest is the data passed to Temporal Workflows.
type WorkflowRequest struct {
	ID                uuid.UUID
	UserID            string
	Channel           domain.Channel
	Recipient         string
	Type              string
	TemplateID        *string
	TemplateVariables map[string]string
	IdempotencyKey    string
}
