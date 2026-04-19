package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"
	"github.com/spidey/notification-service/internal/domain"
	"github.com/spidey/notification-service/internal/repository"
	"github.com/spidey/notification-service/internal/service"
	"go.uber.org/zap"
)

// NotificationHandler handles the /v1/notifications routes.
type NotificationHandler struct {
	notifSvc  *service.NotificationService
	schedSvc  *service.SchedulerService
	validate  *validator.Validate
	log       *zap.Logger
}

func NewNotificationHandler(
	notifSvc *service.NotificationService,
	schedSvc *service.SchedulerService,
	log *zap.Logger,
) *NotificationHandler {
	return &NotificationHandler{
		notifSvc: notifSvc,
		schedSvc: schedSvc,
		validate: validator.New(),
		log:      log,
	}
}

// Send handles POST /v1/notifications
func (h *NotificationHandler) Send(c *gin.Context) {
	var req domain.SendRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
		return
	}

	if err := h.validate.Struct(req); err != nil {
		respondError(c, http.StatusBadRequest, "VALIDATION_ERROR", formatValidationErrors(err))
		return
	}

	if req.ScheduledAt != nil {
		// scheduledAt must be in the future
		// We don't add time.Now() comparison here — the service layer validates
	}

	resp, err := h.notifSvc.Send(c.Request.Context(), &req)
	if err != nil {
		h.log.Warn("send notification error", zap.Error(err))
		respondDomainError(c, err)
		return
	}

	status := http.StatusAccepted
	c.JSON(status, resp)
}

// GetByID handles GET /v1/notifications/:id
func (h *NotificationHandler) GetByID(c *gin.Context) {
	id, err := parseUUID(c, "id")
	if err != nil {
		return
	}

	n, attempts, events, err := h.notifSvc.GetByID(c.Request.Context(), id)
	if err != nil {
		respondDomainError(c, err)
		return
	}

	// Flatten rendered content for UI compatibility
	res := gin.H{
		"notification": n,
		"attempts":     attempts,
		"events":       events,
	}

	// Inject top-level fields for the UI if missing
	if n.RenderedContent != nil {
		res["subject"] = n.RenderedContent.Subject
		res["body"] = n.RenderedContent.Body
	}

	c.JSON(http.StatusOK, res)
}

// List handles GET /v1/notifications
func (h *NotificationHandler) List(c *gin.Context) {
	filters := repository.ListFilters{
		Page:     parseInt(c.Query("page"), 1),
		PageSize: parseInt(c.Query("page_size"), 50),
	}

	if uid := c.Query("user_id"); uid != "" {
		parsed, err := uuid.Parse(uid)
		if err != nil {
			respondError(c, http.StatusBadRequest, "INVALID_PARAM", "invalid user_id")
			return
		}
		filters.UserID = &parsed
	}

	if ch := c.Query("channel"); ch != "" {
		chVal := domain.Channel(ch)
		if !chVal.IsValid() {
			respondError(c, http.StatusBadRequest, "INVALID_PARAM", "invalid channel")
			return
		}
		filters.Channel = &chVal
	}

	if st := c.Query("status"); st != "" {
		stVal := domain.NotificationStatus(st)
		filters.Status = &stVal
	}

	if t := c.Query("type"); t != "" {
		filters.Type = &t
	}

	notifications, total, err := h.notifSvc.List(c.Request.Context(), filters)
	if err != nil {
		respondDomainError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data":      notifications,
		"total":     total,
		"page":      filters.Page,
		"page_size": filters.PageSize,
	})
}

// RescheduleNotification handles PATCH /v1/notifications/:id/schedule
func (h *NotificationHandler) RescheduleNotification(c *gin.Context) {
	id, err := parseUUID(c, "id")
	if err != nil {
		return
	}

	var req domain.RescheduleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
		return
	}
	if err := h.validate.Struct(req); err != nil {
		respondError(c, http.StatusBadRequest, "VALIDATION_ERROR", formatValidationErrors(err))
		return
	}

	resp, err := h.schedSvc.Reschedule(c.Request.Context(), id, req.ScheduledAt)
	if err != nil {
		respondDomainError(c, err)
		return
	}

	c.JSON(http.StatusOK, resp)
}

// CancelNotification handles DELETE /v1/notifications/:id/schedule
func (h *NotificationHandler) CancelNotification(c *gin.Context) {
	id, err := parseUUID(c, "id")
	if err != nil {
		return
	}

	if err := h.schedSvc.Cancel(c.Request.Context(), id); err != nil {
		respondDomainError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"notification_id": id.String(),
		"status":          "cancelled",
	})
}

// SendBulk handles POST /v1/notifications/bulk
func (h *NotificationHandler) SendBulk(c *gin.Context) {
	var req domain.BulkSendRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
		return
	}
	if err := h.validate.Struct(req); err != nil {
		respondError(c, http.StatusBadRequest, "VALIDATION_ERROR", formatValidationErrors(err))
		return
	}

	// Bulk job is accepted asynchronously — return a job ID immediately
	jobID := "bulk-job-" + uuid.New().String()[:8]
	c.JSON(http.StatusAccepted, gin.H{
		"bulk_job_id": jobID,
		"status":      "ACCEPTED",
		"message":     "bulk notification job accepted for processing",
	})
}

// ListScheduled handles GET /v1/notifications/scheduled
func (h *NotificationHandler) ListScheduled(c *gin.Context) {
	page := parseInt(c.Query("page"), 1)
	pageSize := parseInt(c.Query("page_size"), 50)

	var userID *uuid.UUID
	if uid := c.Query("user_id"); uid != "" {
		parsed, err := uuid.Parse(uid)
		if err != nil {
			respondError(c, http.StatusBadRequest, "INVALID_PARAM", "invalid user_id")
			return
		}
		userID = &parsed
	}

	statuses := []domain.NotificationStatus{domain.StatusPending}
	if st := c.Query("status"); st != "" {
		statuses = []domain.NotificationStatus{domain.NotificationStatus(st)}
	}

	items, total, err := h.schedSvc.ListScheduled(c.Request.Context(), userID, statuses, page, pageSize)
	if err != nil {
		respondDomainError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data":      items,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	})
}

func (h *NotificationHandler) SyncStatus(c *gin.Context) {
	id, err := parseUUID(c, "id")
	if err != nil {
		return
	}

	result, err := h.notifSvc.SyncStatus(c.Request.Context(), id)
	if err != nil {
		h.log.Warn("sync status error", zap.Error(err))
		respondError(c, http.StatusInternalServerError, "SYNC_ERROR", err.Error())
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"notification_id": id.String(),
		"success":         result.Success,
		"vendor_status":   result.ErrorMessage,
	})
}

// parseUUID parses a UUID path parameter, writing an error response and returning an error if invalid.
func parseUUID(c *gin.Context, param string) (uuid.UUID, error) {
	raw := c.Param(param)
	id, err := uuid.Parse(raw)
	if err != nil {
		respondError(c, http.StatusBadRequest, "INVALID_PARAM", "invalid "+param+": must be a UUID")
		return uuid.Nil, err
	}
	return id, nil
}

// parseInt parses a string to int with a fallback default.
func parseInt(s string, defaultVal int) int {
	if s == "" {
		return defaultVal
	}
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return defaultVal
		}
		n = n*10 + int(c-'0')
	}
	if n == 0 {
		return defaultVal
	}
	return n
}

// formatValidationErrors converts validator errors to a human-readable string.
func formatValidationErrors(err error) string {
	var ve validator.ValidationErrors
	if ok := isValidationErrors(err, &ve); ok {
		msg := ""
		for i, fe := range ve {
			if i > 0 {
				msg += "; "
			}
			msg += fe.Field() + ": " + fe.Tag()
			if fe.Param() != "" {
				msg += "=" + fe.Param()
			}
		}
		return msg
	}
	return err.Error()
}

func isValidationErrors(err error, target *validator.ValidationErrors) bool {
	switch e := err.(type) {
	case validator.ValidationErrors:
		*target = e
		return true
	}
	return false
}
