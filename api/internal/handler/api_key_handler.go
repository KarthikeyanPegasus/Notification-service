package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/spidey/notification-service/internal/service"
	"go.uber.org/zap"
)

type APIKeyHandler struct {
	svc *service.APIKeyService
	log *zap.Logger
}

func NewAPIKeyHandler(svc *service.APIKeyService, log *zap.Logger) *APIKeyHandler {
	return &APIKeyHandler{svc: svc, log: log}
}

func (h *APIKeyHandler) Create(c *gin.Context) {
	var req struct {
		Name string `json:"name" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
		return
	}
	out, err := h.svc.Create(c.Request.Context(), req.Name)
	if err != nil {
		respondError(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
		return
	}
	c.JSON(http.StatusCreated, out)
}

func (h *APIKeyHandler) List(c *gin.Context) {
	out, err := h.svc.List(c.Request.Context())
	if err != nil {
		h.log.Error("failed to list api keys", zap.Error(err))
		respondError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to list api keys")
		return
	}
	c.JSON(http.StatusOK, out)
}

func (h *APIKeyHandler) Revoke(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		respondError(c, http.StatusBadRequest, "VALIDATION_ERROR", "missing id")
		return
	}
	if err := h.svc.Revoke(c.Request.Context(), id); err != nil {
		h.log.Error("failed to revoke api key", zap.Error(err), zap.String("id", id))
		respondError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to revoke api key")
		return
	}
	c.Status(http.StatusNoContent)
}
