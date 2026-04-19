package handler

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/spidey/notification-service/internal/domain"
	"github.com/spidey/notification-service/internal/repository"
	"go.uber.org/zap"
)

// WebhookHandler receives inbound delivery callbacks from email/SMS/push providers.
type WebhookHandler struct {
	eventRepo   *repository.EventRepository
	notifRepo   *repository.NotificationRepository
	attemptRepo *repository.AttemptRepository
	webhookRepo *repository.WebhookEventRepository
	log         *zap.Logger
}

func NewWebhookHandler(
	eventRepo *repository.EventRepository,
	notifRepo *repository.NotificationRepository,
	attemptRepo *repository.AttemptRepository,
	webhookRepo *repository.WebhookEventRepository,
	log *zap.Logger,
) *WebhookHandler {
	return &WebhookHandler{
		eventRepo:   eventRepo,
		notifRepo:   notifRepo,
		attemptRepo: attemptRepo,
		webhookRepo: webhookRepo,
		log:         log,
	}
}

// HandleProviderEvent handles POST /v1/webhooks/:provider
func (h *WebhookHandler) HandleProviderEvent(c *gin.Context) {
	provider := c.Param("provider")

	bodyBytes, err := io.ReadAll(io.LimitReader(c.Request.Body, 1<<20)) // 1MB limit
	if err != nil {
		respondError(c, http.StatusBadRequest, "READ_ERROR", "failed to read request body")
		return
	}

	var rawPayload map[string]any
	if err := json.Unmarshal(bodyBytes, &rawPayload); err != nil {
		respondError(c, http.StatusBadRequest, "PARSE_ERROR", "invalid JSON body")
		return
	}

	// Extract notification_id and event_type from common provider fields
	notifIDStr := extractString(rawPayload, "notification_id", "custom_id", "tag")
	eventTypeStr := extractString(rawPayload, "event", "status", "Type")
	channel := domain.Channel(extractString(rawPayload, "channel", "type"))

	// Store raw webhook event
	webhookEvent := &domain.ProviderWebhookEvent{
		ID:         uuid.New(),
		Provider:   provider,
		Channel:    channel,
		EventType:  eventTypeStr,
		RawPayload: rawPayload,
		ReceivedAt: time.Now(),
	}

	if notifIDStr != "" {
		if notifID, err := uuid.Parse(notifIDStr); err == nil {
			webhookEvent.NotificationID = &notifID

			// Update notification status based on provider event
			if err := h.handleDeliveryEvent(c.Request.Context(), notifID, provider, eventTypeStr, rawPayload); err != nil {
				h.log.Warn("processing webhook delivery event",
					zap.String("provider", provider),
					zap.String("notification_id", notifIDStr),
					zap.Error(err),
				)
			}
		}
	}

	if err := h.webhookRepo.Create(c.Request.Context(), webhookEvent); err != nil {
		h.log.Error("storing webhook event", zap.Error(err))
	}

	c.JSON(http.StatusOK, gin.H{"received": true})
}

func (h *WebhookHandler) handleDeliveryEvent(
	ctx context.Context,
	notifID uuid.UUID,
	provider, eventType string,
	rawPayload map[string]any,
) error {
	var domainEvent domain.EventType
	var status domain.NotificationStatus

	switch eventType {
	case "delivered", "delivery":
		domainEvent = domain.EventDelivered
		status = domain.StatusDelivered
	case "bounced", "bounce", "permanent_failure":
		domainEvent = domain.EventBounced
		status = domain.StatusBounced
	case "failed", "failure":
		domainEvent = domain.EventFailed
		status = domain.StatusFailed
	case "opened", "open":
		domainEvent = domain.EventOpened
		// Don't change overall status for open
		if err := h.eventRepo.Append(ctx, &domain.NotificationEvent{
			ID:             uuid.New(),
			NotificationID: notifID,
			EventType:      domainEvent,
			Metadata:       map[string]any{"provider": provider},
			CreatedAt:      time.Now(),
		}); err != nil {
			return err
		}
		return nil
	default:
		return nil
	}

	if err := h.notifRepo.UpdateStatus(ctx, notifID, status); err != nil {
		return err
	}

	return h.eventRepo.Append(ctx, &domain.NotificationEvent{
		ID:             uuid.New(),
		NotificationID: notifID,
		EventType:      domainEvent,
		Metadata:       map[string]any{"provider": provider, "raw": rawPayload},
		CreatedAt:      time.Now(),
	})
}

// extractString checks multiple keys in order and returns the first non-empty string value.
func extractString(payload map[string]any, keys ...string) string {
	for _, k := range keys {
		if v, ok := payload[k]; ok {
			if s, ok := v.(string); ok && s != "" {
				return s
			}
		}
	}
	return ""
}
