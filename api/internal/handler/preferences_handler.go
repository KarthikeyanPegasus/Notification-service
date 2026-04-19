package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
	"github.com/spidey/notification-service/internal/domain"
	"github.com/spidey/notification-service/internal/service"
	"go.uber.org/zap"
)

// PreferencesHandler handles user notification preference routes.
type PreferencesHandler struct {
	prefsSvc *service.PreferencesService
	validate *validator.Validate
	log      *zap.Logger
}

func NewPreferencesHandler(prefsSvc *service.PreferencesService, log *zap.Logger) *PreferencesHandler {
	return &PreferencesHandler{
		prefsSvc: prefsSvc,
		validate: validator.New(),
		log:      log,
	}
}

// GetPreferences handles GET /v1/users/:user_id/notification-preferences
func (h *PreferencesHandler) GetPreferences(c *gin.Context) {
	userID := c.Param("user_id")
	if userID == "" {
		respondError(c, http.StatusBadRequest, "MISSING_PARAM", "user_id is required")
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
