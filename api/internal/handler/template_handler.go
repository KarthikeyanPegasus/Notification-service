package handler

import (
	"net/http"
	"time"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/spidey/notification-service/internal/domain"
	"github.com/spidey/notification-service/internal/repository"
	"github.com/spidey/notification-service/internal/service"
	"go.uber.org/zap"
)

const smsTemplateMaxLen = 160

type TemplateHandler struct {
	repo        *repository.TemplateRepository
	templateSvc *service.TemplateService
	log         *zap.Logger
}

func NewTemplateHandler(repo *repository.TemplateRepository, templateSvc *service.TemplateService, log *zap.Logger) *TemplateHandler {
	return &TemplateHandler{
		repo:        repo,
		templateSvc: templateSvc,
		log:         log,
	}
}

func (h *TemplateHandler) List(c *gin.Context) {
	channelStr := c.Query("channel")
	var channel *domain.Channel
	if channelStr != "" {
		ch := domain.Channel(channelStr)
		if ch.IsValid() {
			channel = &ch
		}
	}

	templates, err := h.repo.List(c.Request.Context(), channel)
	if err != nil {
		h.log.Error("failed to list templates", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list templates"})
		return
	}

	c.JSON(http.StatusOK, templates)
}

func (h *TemplateHandler) GetByID(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid template id"})
		return
	}

	tmpl, err := h.repo.GetByID(c.Request.Context(), id)
	if err != nil {
		if err == domain.ErrNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "template not found"})
			return
		}
		h.log.Error("failed to get template", zap.Error(err), zap.String("id", id.String()))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get template"})
		return
	}

	c.JSON(http.StatusOK, tmpl)
}

func (h *TemplateHandler) Create(c *gin.Context) {
	var req struct {
		Name    string         `json:"name" binding:"required"`
		Channel domain.Channel `json:"channel" binding:"required"`
		Subject *string        `json:"subject"`
		Body    string         `json:"body" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if !req.Channel.IsValid() {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid channel"})
		return
	}

	if req.Channel == domain.ChannelSMS && utf8.RuneCountInString(req.Body) > smsTemplateMaxLen {
		c.JSON(http.StatusBadRequest, gin.H{"error": "sms template body must be 160 characters or fewer"})
		return
	}

	tmpl := &domain.NotificationTemplate{
		ID:        uuid.New(),
		Name:      req.Name,
		Channel:   req.Channel,
		Subject:   req.Subject,
		Body:      req.Body,
		Version:   1,
		IsActive:  true,
		CreatedAt: time.Now(),
	}

	if err := h.repo.Create(c.Request.Context(), tmpl); err != nil {
		h.log.Error("failed to create template", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create template"})
		return
	}

	c.JSON(http.StatusCreated, tmpl)
}

func (h *TemplateHandler) Update(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid template id"})
		return
	}

	var req struct {
		Name    string         `json:"name" binding:"required"`
		Channel domain.Channel `json:"channel" binding:"required"`
		Subject *string        `json:"subject"`
		Body    string         `json:"body" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if !req.Channel.IsValid() {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid channel"})
		return
	}

	if req.Channel == domain.ChannelSMS && utf8.RuneCountInString(req.Body) > smsTemplateMaxLen {
		c.JSON(http.StatusBadRequest, gin.H{"error": "sms template body must be 160 characters or fewer"})
		return
	}

	existing, err := h.repo.GetByID(c.Request.Context(), id)
	if err != nil {
		if err == domain.ErrNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "template not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch existing template"})
		return
	}

	existing.Name = req.Name
	existing.Channel = req.Channel
	existing.Subject = req.Subject
	existing.Body = req.Body
	existing.Version++

	if err := h.repo.Update(c.Request.Context(), existing); err != nil {
		h.log.Error("failed to update template", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update template"})
		return
	}

	// Invalidate cache
	h.templateSvc.InvalidateCache(c.Request.Context(), id)

	c.JSON(http.StatusOK, existing)
}

func (h *TemplateHandler) Delete(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid template id"})
		return
	}

	if err := h.repo.Delete(c.Request.Context(), id); err != nil {
		h.log.Error("failed to delete template", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete template"})
		return
	}

	// Invalidate cache
	h.templateSvc.InvalidateCache(c.Request.Context(), id)

	c.Status(http.StatusNoContent)
}
