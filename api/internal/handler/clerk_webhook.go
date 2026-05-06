package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/spidey/notification-service/internal/domain"
	"github.com/spidey/notification-service/internal/repository"
	svix "github.com/svix/svix-webhooks/go"
	"go.uber.org/zap"
)

// ClerkWebhookHandler handles incoming Clerk webhooks for user lifecycle events.
type ClerkWebhookHandler struct {
	wh         *svix.Webhook
	userRepo   *repository.UserRepository
	log        *zap.Logger
}

// NewClerkWebhookHandler creates a new ClerkWebhookHandler.
// webhookSecret is your Clerk webhook signing secret (starts with "whsec_").
func NewClerkWebhookHandler(webhookSecret string, userRepo *repository.UserRepository, log *zap.Logger) (*ClerkWebhookHandler, error) {
	wh, err := svix.NewWebhook(webhookSecret)
	if err != nil {
		return nil, fmt.Errorf("create svix webhook: %w", err)
	}
	return &ClerkWebhookHandler{
		wh:       wh,
		userRepo: userRepo,
		log:      log,
	}, nil
}

// clerkWebhookPayload represents the structure of a Clerk webhook event.
type clerkWebhookPayload struct {
	Data   clerkUserData `json:"data"`
	Object string        `json:"object"`
	Type   string        `json:"type"`
}

// clerkUserData represents user data in a Clerk webhook event.
type clerkUserData struct {
	ID                 string                 `json:"id"`
	EmailAddresses     []clerkEmail           `json:"email_addresses"`
	FirstName          string                 `json:"first_name"`
	LastName           string                 `json:"last_name"`
	PublicMetadata     map[string]interface{} `json:"public_metadata"`
	PrivateMetadata    map[string]interface{} `json:"private_metadata"`
	UnsafeMetadata     map[string]interface{} `json:"unsafe_metadata"`
	Username           string                 `json:"username"`
	ImageURL           string                 `json:"image_url"`
	CreatedAt          int64                  `json:"created_at"`
	UpdatedAt          int64                  `json:"updated_at"`
}

type clerkEmail struct {
	ID         string `json:"id"`
	EmailAddress string `json:"email_address"`
	Verified   bool   `json:"verified"`
	Primary    bool   `json:"primary"`
}

// HandleWebhook processes an incoming Clerk webhook event.
func (h *ClerkWebhookHandler) HandleWebhook(c *gin.Context) {
	// Read the raw body
	bodyBytes, err := io.ReadAll(c.Request.Body)
	if err != nil {
		h.log.Warn("clerk webhook: failed to read body", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": "failed to read request body"})
		return
	}

	// Verify the webhook signature using Svix
	err = h.wh.Verify(bodyBytes, c.Request.Header)
	if err != nil {
		h.log.Warn("clerk webhook: invalid signature", zap.Error(err))
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid webhook signature"})
		return
	}

	// Parse the event
	var event clerkWebhookPayload
	if err := json.Unmarshal(bodyBytes, &event); err != nil {
		h.log.Warn("clerk webhook: failed to parse body", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid JSON"})
		return
	}

	ctx := c.Request.Context()

	switch event.Type {
	case "user.created":
		h.handleUserCreated(ctx, event.Data)
	case "user.updated":
		h.handleUserUpdated(ctx, event.Data)
	case "user.deleted":
		h.handleUserDeleted(ctx, event.Data)
	default:
		h.log.Debug("clerk webhook: unhandled event type", zap.String("type", event.Type))
	}

	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func (h *ClerkWebhookHandler) handleUserCreated(ctx context.Context, data clerkUserData) {
	email := ""
	for _, e := range data.EmailAddresses {
		if e.Primary {
			email = e.EmailAddress
			break
		}
	}
	if email == "" && len(data.EmailAddresses) > 0 {
		email = data.EmailAddresses[0].EmailAddress
	}

	name := data.FirstName
	if data.LastName != "" {
		if name != "" {
			name = name + " " + data.LastName
		} else {
			name = data.LastName
		}
	}
	if name == "" {
		name = data.Username
	}
	if name == "" {
		name = email
	}

	role := domain.UserRoleDev
	if data.PublicMetadata != nil {
		if r, ok := data.PublicMetadata["role"].(string); ok {
			role = domain.UserRole(r)
		}
	}

	existing, err := h.userRepo.GetByClerkID(ctx, data.ID)
	if err == nil && existing != nil {
		// User already exists — treat as update
		h.handleUserUpdated(ctx, data)
		return
	}

	_, err = h.userRepo.CreateFromClerk(ctx, data.ID, email, name, role)
	if err != nil {
		h.log.Warn("clerk webhook: failed to create user", zap.String("clerk_id", data.ID), zap.Error(err))
		return
	}
	h.log.Info("clerk webhook: user created", zap.String("clerk_id", data.ID), zap.String("email", email))
}

func (h *ClerkWebhookHandler) handleUserUpdated(ctx context.Context, data clerkUserData) {
	email := ""
	for _, e := range data.EmailAddresses {
		if e.Primary {
			email = e.EmailAddress
			break
		}
	}
	if email == "" && len(data.EmailAddresses) > 0 {
		email = data.EmailAddresses[0].EmailAddress
	}

	name := data.FirstName
	if data.LastName != "" {
		if name != "" {
			name = name + " " + data.LastName
		} else {
			name = data.LastName
		}
	}
	if name == "" {
		name = data.Username
	}
	if name == "" {
		name = email
	}

	role := domain.UserRole("")
	if data.PublicMetadata != nil {
		if r, ok := data.PublicMetadata["role"].(string); ok {
			role = domain.UserRole(r)
		}
	}

	existing, err := h.userRepo.GetByClerkID(ctx, data.ID)
	if err != nil {
		h.log.Warn("clerk webhook: user not found for update", zap.String("clerk_id", data.ID))
		return
	}

	if role != "" {
		err = h.userRepo.UpdateFromClerk(ctx, existing.ID, email, name, role)
	} else {
		err = h.userRepo.UpdateFromClerk(ctx, existing.ID, email, name, existing.Role)
	}
	if err != nil {
		h.log.Warn("clerk webhook: failed to update user", zap.String("clerk_id", data.ID), zap.Error(err))
		return
	}
	h.log.Info("clerk webhook: user updated", zap.String("clerk_id", data.ID), zap.String("email", email))
}

func (h *ClerkWebhookHandler) handleUserDeleted(ctx context.Context, data clerkUserData) {
	existing, err := h.userRepo.GetByClerkID(ctx, data.ID)
	if err != nil {
		h.log.Debug("clerk webhook: user not found for deletion", zap.String("clerk_id", data.ID))
		return
	}

	if err := h.userRepo.Delete(ctx, existing.ID); err != nil {
		h.log.Warn("clerk webhook: failed to delete user", zap.String("clerk_id", data.ID), zap.Error(err))
		return
	}
	h.log.Info("clerk webhook: user deleted", zap.String("clerk_id", data.ID))
}
