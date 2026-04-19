package handler

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/spidey/notification-service/internal/domain"
	"github.com/spidey/notification-service/internal/repository"
	"go.uber.org/zap"
)

type GovernanceHandler struct {
	repo *repository.GovernanceRepository
	log  *zap.Logger
}

func NewGovernanceHandler(repo *repository.GovernanceRepository, log *zap.Logger) *GovernanceHandler {
	return &GovernanceHandler{repo: repo, log: log}
}

// Suppressions

func (h *GovernanceHandler) ListSuppressions(c *gin.Context) {
	list, err := h.repo.ListSuppressions(c.Request.Context())
	if err != nil {
		h.log.Error("failed to list suppressions", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}
	c.JSON(http.StatusOK, list)
}

func (h *GovernanceHandler) AddSuppression(c *gin.Context) {
	var req domain.AddSuppressionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	s := &domain.Suppression{
		ID:        uuid.New(),
		Type:      req.Type,
		Value:     req.Value,
		Reason:    req.Reason,
		Metadata:  req.Metadata,
		CreatedAt: time.Now(),
	}

	if err := h.repo.AddSuppression(c.Request.Context(), s); err != nil {
		h.log.Error("failed to add suppression", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}

	c.JSON(http.StatusCreated, s)
}

func (h *GovernanceHandler) DeleteSuppression(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	if err := h.repo.DeleteSuppression(c.Request.Context(), id); err != nil {
		h.log.Error("failed to delete suppression", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}

	c.Status(http.StatusNoContent)
}

// Opt-outs

func (h *GovernanceHandler) ListOptOuts(c *gin.Context) {
	list, err := h.repo.ListOptOuts(c.Request.Context())
	if err != nil {
		h.log.Error("failed to list opt-outs", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}
	c.JSON(http.StatusOK, list)
}

func (h *GovernanceHandler) AddOptOut(c *gin.Context) {
	var req domain.AddOptOutRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	o := &domain.OptOut{
		ID:        uuid.New(),
		UserID:    req.UserID,
		Channel:   req.Channel,
		Reason:    req.Reason,
		Source:    req.Source,
		CreatedAt: time.Now(),
	}

	if err := h.repo.AddOptOut(c.Request.Context(), o); err != nil {
		h.log.Error("failed to add opt-out", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}

	c.JSON(http.StatusCreated, o)
}

func (h *GovernanceHandler) DeleteOptOut(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	if err := h.repo.DeleteOptOut(c.Request.Context(), id); err != nil {
		h.log.Error("failed to delete opt-out", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}

	c.Status(http.StatusNoContent)
}
