package handler

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/spidey/notification-service/internal/repository"
	"go.uber.org/zap"
)

// DLQHandler handles dead-letter queue admin endpoints.
type DLQHandler struct {
	dlqRepo  *repository.DLQRepository
	notifSvc NotificationServiceProvider
	log      *zap.Logger
}

// NotificationServiceProvider defines the subset of notificationService used by DLQHandler.
type NotificationServiceProvider interface {
	Retrigger(ctx context.Context, id uuid.UUID) error
}

func NewDLQHandler(dlqRepo *repository.DLQRepository, notifSvc NotificationServiceProvider, log *zap.Logger) *DLQHandler {
	return &DLQHandler{
		dlqRepo:  dlqRepo,
		notifSvc: notifSvc,
		log:      log,
	}
}

// ListDLQ handles GET /v1/admin/dlq
func (h *DLQHandler) ListDLQ(c *gin.Context) {
	page := parseInt(c.Query("page"), 1)
	pageSize := parseInt(c.Query("page_size"), 50)
	unreplayedOnly := c.Query("unreplayed") == "true"

	entries, total, err := h.dlqRepo.List(c.Request.Context(), page, pageSize, unreplayedOnly, c.GetString("scoped_api_key_id"))
	if err != nil {
		h.log.Error("listing DLQ entries", zap.Error(err))
		respondError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to list DLQ entries")
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data":  entries,
		"total": total,
		"page":  page,
		"page_size": pageSize,
	})
}

// GetDLQEntry handles GET /v1/admin/dlq/:id
func (h *DLQHandler) GetDLQEntry(c *gin.Context) {
	id, err := parseUUID(c, "id")
	if err != nil {
		return
	}

	entry, err := h.dlqRepo.GetByID(c.Request.Context(), id)
	if err != nil {
		h.log.Error("getting DLQ entry", zap.Error(err))
		respondError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to get DLQ entry")
		return
	}
	if entry == nil {
		respondError(c, http.StatusNotFound, "NOT_FOUND", "DLQ entry not found")
		return
	}

	c.JSON(http.StatusOK, entry)
}

// ReplayDLQEntry handles POST /v1/admin/dlq/:id/replay
// Attempts to retrigger the original notification and marks the DLQ entry as replayed.
func (h *DLQHandler) ReplayDLQEntry(c *gin.Context) {
	id, err := parseUUID(c, "id")
	if err != nil {
		return
	}

	entry, err := h.dlqRepo.GetByID(c.Request.Context(), id)
	if err != nil {
		h.log.Error("getting DLQ entry for replay", zap.Error(err))
		respondError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to get DLQ entry")
		return
	}
	if entry == nil {
		respondError(c, http.StatusNotFound, "NOT_FOUND", "DLQ entry not found")
		return
	}

	if entry.Replayed {
		respondError(c, http.StatusConflict, "ALREADY_REPLAYED", "this DLQ entry has already been replayed")
		return
	}

	if entry.NotificationID != nil {
		if err := h.notifSvc.Retrigger(c.Request.Context(), *entry.NotificationID); err != nil {
			h.log.Error("replaying DLQ entry: retrigger failed",
				zap.String("dlq_id", id.String()),
				zap.String("notification_id", entry.NotificationID.String()),
				zap.Error(err),
			)
			respondError(c, http.StatusInternalServerError, "REPLAY_FAILED", err.Error())
			return
		}
	}

	if err := h.dlqRepo.MarkReplayed(c.Request.Context(), id); err != nil {
		h.log.Error("marking DLQ entry as replayed", zap.Error(err))
		respondError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to mark DLQ entry as replayed")
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"dlq_id":          id.String(),
		"notification_id": entry.NotificationID,
		"status":          "replayed",
		"message":         "notification re-queued for delivery",
	})
}

// ReplayAllDLQ handles POST /v1/admin/dlq/replay-all
// Retriggers all un-replayed DLQ entries.
func (h *DLQHandler) ReplayAllDLQ(c *gin.Context) {
	entries, total, err := h.dlqRepo.List(c.Request.Context(), 1, 10000, true, c.GetString("scoped_api_key_id"))
	if err != nil {
		h.log.Error("listing un-replayed DLQ entries for replay-all", zap.Error(err))
		respondError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to list DLQ entries")
		return
	}

	replayed := 0
	errors := 0
	for _, entry := range entries {
		if entry.NotificationID != nil {
			if err := h.notifSvc.Retrigger(c.Request.Context(), *entry.NotificationID); err != nil {
				h.log.Error("replay-all: retrigger failed",
					zap.String("dlq_id", entry.ID.String()),
					zap.Error(err),
				)
				errors++
				continue
			}
		}
		if err := h.dlqRepo.MarkReplayed(c.Request.Context(), entry.ID); err != nil {
			h.log.Error("replay-all: failed to mark replayed", zap.Error(err))
			errors++
			continue
		}
		replayed++
	}

	c.JSON(http.StatusOK, gin.H{
		"total":    total,
		"replayed": replayed,
		"errors":   errors,
		"status":   "completed",
	})
}

// DLQStats handles GET /v1/admin/dlq/stats
func (h *DLQHandler) DLQStats(c *gin.Context) {
	scopedID := c.GetString("scoped_api_key_id")

	_, total, err := h.dlqRepo.List(c.Request.Context(), 1, 1, false, scopedID)
	if err != nil {
		h.log.Error("getting DLQ stats", zap.Error(err))
		respondError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to get DLQ stats")
		return
	}

	_, pendingCount, err := h.dlqRepo.List(c.Request.Context(), 1, 1, true, scopedID)
	if err != nil {
		h.log.Error("getting DLQ stats for unreplayed", zap.Error(err))
		respondError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to get unreplayed DLQ stats")
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"total_entries":     total,
		"pending_replay":    pendingCount,
		"replayed_entries":  total - pendingCount,
	})
}
