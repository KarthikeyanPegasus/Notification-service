package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/spidey/notification-service/internal/domain"
	"github.com/spidey/notification-service/internal/repository"
	"github.com/spidey/notification-service/internal/service"
)

// MeHandler exposes "current user" helper endpoints for the UI.
type MeHandler struct {
	users   *repository.UserRepository
	apiKeys *repository.APIKeyRepository
	apiKeySvc *service.APIKeyService
}

func NewMeHandler(users *repository.UserRepository, apiKeys *repository.APIKeyRepository, apiKeySvc *service.APIKeyService) *MeHandler {
	return &MeHandler{users: users, apiKeys: apiKeys, apiKeySvc: apiKeySvc}
}

// ListClients returns API keys visible to the current user.
// - admin: all api keys
// - dev/support: only assigned api keys
func (h *MeHandler) ListClients(c *gin.Context) {
	role, sub := getRoleAndSubject(c)
	if sub == "" {
		respondError(c, http.StatusUnauthorized, "UNAUTHORIZED", "missing subject")
		return
	}

	// Admin sees all keys (same as admin API keys list but without needing /admin permissions).
	if role == string(domain.UserRoleAdmin) {
		out, err := h.apiKeys.List(c.Request.Context())
		if err != nil {
			respondError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to list api keys")
			return
		}
		c.JSON(http.StatusOK, out)
		return
	}

	ids, err := h.users.ListAPIKeysForUser(c.Request.Context(), sub)
	if err != nil {
		respondError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to list assignments")
		return
	}
	out, err := h.apiKeys.ListByIDs(c.Request.Context(), ids)
	if err != nil {
		respondError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to list api keys")
		return
	}
	c.JSON(http.StatusOK, out)
}

// CreateClient allows dev users to create their own isolated client (API key) and auto-assign it to themselves.
func (h *MeHandler) CreateClient(c *gin.Context) {
	role, sub := getRoleAndSubject(c)
	if role != string(domain.UserRoleDev) {
		respondError(c, http.StatusForbidden, "FORBIDDEN", "only dev users can create clients")
		return
	}
	if sub == "" {
		respondError(c, http.StatusUnauthorized, "UNAUTHORIZED", "missing subject")
		return
	}
	if h.apiKeySvc == nil {
		respondError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "api key service not configured")
		return
	}

	var req struct {
		Name string `json:"name" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
		return
	}

	ids, err := h.users.ListAPIKeysForUser(c.Request.Context(), sub)
	if err != nil {
		respondError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to load assignments")
		return
	}
	if len(ids) >= 1 {
		respondError(c, http.StatusForbidden, "FORBIDDEN", "sandbox accounts are limited to 1 active client")
		return
	}

	created, err := h.apiKeySvc.Create(c.Request.Context(), req.Name)
	if err != nil {
		respondError(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
		return
	}

	ids = append(ids, created.Key.ID)
	if err := h.users.SetUserAPIKeys(c.Request.Context(), sub, ids); err != nil {
		respondError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to assign client to user")
		return
	}

	c.JSON(http.StatusCreated, created)
}

