package handler

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/clerk/clerk-sdk-go/v2"
	"github.com/clerk/clerk-sdk-go/v2/invitation"
	"github.com/gin-gonic/gin"
	"github.com/spidey/notification-service/internal/domain"
)

// InvitationHandler handles Clerk invitation management.
type InvitationHandler struct{}

// NewInvitationHandler creates a new InvitationHandler.
// Clerk SDK must already be initialized with SetKey() before using this handler.
func NewInvitationHandler() *InvitationHandler {
	return &InvitationHandler{}
}

// InviteRequest is the JSON body for creating an invitation.
type InviteRequest struct {
	Email string          `json:"email" binding:"required,email"`
	Role  domain.UserRole `json:"role" binding:"required"`
}

// Invite creates a Clerk invitation with the specified role in public_metadata.
func (h *InvitationHandler) Invite(c *gin.Context) {
	var req InviteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
		return
	}

	// Validate role
	validRoles := map[domain.UserRole]bool{
		domain.UserRoleAdmin:   true,
		domain.UserRoleManager: true,
		domain.UserRoleDev:     true,
		domain.UserRoleSupport: true,
	}
	if !validRoles[req.Role] {
		respondError(c, http.StatusBadRequest, "VALIDATION_ERROR", fmt.Sprintf("invalid role: %q. Must be one of: admin, manager, dev, support", req.Role))
		return
	}

	// Build public_metadata with the role
	metadata := json.RawMessage(fmt.Sprintf(`{"role":"%s"}`, req.Role))

	// Create the invitation via Clerk API
	inv, err := invitation.Create(c.Request.Context(), &invitation.CreateParams{
		EmailAddress:   req.Email,
		PublicMetadata: &metadata,
		RedirectURL:    clerk.String("/"),
	})
	if err != nil {
		respondError(c, http.StatusInternalServerError, "CLERK_ERROR", fmt.Sprintf("failed to create invitation: %v", err))
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"id":         inv.ID,
		"email":      inv.EmailAddress,
		"role":       req.Role,
		"status":     inv.Status,
		"url":        inv.URL,
		"created_at": inv.CreatedAt,
		"message":    "Invitation sent. The user will receive an email with instructions to sign up.",
	})
}
