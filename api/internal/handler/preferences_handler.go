package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
	"github.com/spidey/notification-service/internal/domain"
	"github.com/spidey/notification-service/internal/repository"
	"github.com/spidey/notification-service/internal/service"
	"go.uber.org/zap"
)

// PreferencesHandler handles user notification preference routes.
type PreferencesHandler struct {
	prefsSvc *service.PreferencesService
	users    *repository.UserRepository
	validate *validator.Validate
	log      *zap.Logger
}

func NewPreferencesHandler(prefsSvc *service.PreferencesService, users *repository.UserRepository, log *zap.Logger) *PreferencesHandler {
	return &PreferencesHandler{
		prefsSvc: prefsSvc,
		users:    users,
		validate: validator.New(),
		log:      log,
	}
}

// callerCanAccessUser returns true when the caller is allowed to read/write the given user_id.
// admin and support may access any user. All other authenticated callers can only access their own.
func (h *PreferencesHandler) callerCanAccessUser(c *gin.Context, userID string) bool {
	role, sub := getRoleAndSubject(c)
	if role == string(domain.UserRoleAdmin) || role == "support" {
		return true
	}
	// Non-privileged callers can only access their own user_id.
	return sub != "" && sub == userID
}

// GetPreferences handles GET /v1/users/:user_id/notification-preferences
func (h *PreferencesHandler) GetPreferences(c *gin.Context) {
	userID := c.Param("user_id")
	if userID == "" {
		respondError(c, http.StatusBadRequest, "MISSING_PARAM", "user_id is required")
		return
	}

	if !h.callerCanAccessUser(c, userID) {
		respondError(c, http.StatusForbidden, "FORBIDDEN", "cannot access preferences for another user")
		return
	}

	prefs, err := h.prefsSvc.Get(c.Request.Context(), userID)
	if err != nil {
		respondDomainError(c, err)
		return
	}

	c.JSON(http.StatusOK, prefs)
}

// UpdatePreferences handles PUT /v1/users/:user_id/notification-preferences
func (h *PreferencesHandler) UpdatePreferences(c *gin.Context) {
	userID := c.Param("user_id")
	if userID == "" {
		respondError(c, http.StatusBadRequest, "MISSING_PARAM", "user_id is required")
		return
	}

	if !h.callerCanAccessUser(c, userID) {
		respondError(c, http.StatusForbidden, "FORBIDDEN", "cannot modify preferences for another user")
		return
	}

	var req domain.UpdatePreferencesRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
		return
	}

	if err := h.prefsSvc.Set(c.Request.Context(), userID, &req); err != nil {
		respondDomainError(c, err)
		return
	}

	prefs, err := h.prefsSvc.Get(c.Request.Context(), userID)
	if err != nil {
		respondDomainError(c, err)
		return
	}

	c.JSON(http.StatusOK, prefs)
}
