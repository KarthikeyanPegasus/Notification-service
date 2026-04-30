package handler

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/spidey/notification-service/internal/domain"
	"github.com/spidey/notification-service/internal/repository"
	"github.com/spidey/notification-service/internal/service"
	"go.uber.org/zap"
)

type TestDeliveryHandler struct {
	vendors  repository.VendorConfigRepository
	notifSvc *service.NotificationService
	log      *zap.Logger
}

func NewTestDeliveryHandler(vendors repository.VendorConfigRepository, notifSvc *service.NotificationService, log *zap.Logger) *TestDeliveryHandler {
	return &TestDeliveryHandler{vendors: vendors, notifSvc: notifSvc, log: log}
}

type testDeliveryRequest struct {
	Channel      string `json:"channel" binding:"required"`   // sms | email | slack
	Recipient    string `json:"recipient" binding:"required"` // phone/email/slack_channel_name/webhook_url
	Body         string `json:"body" binding:"required"`
	Subject      string `json:"subject,omitempty"`
	SlackChannel string `json:"slack_channel,omitempty"`
}

func (h *TestDeliveryHandler) Send(c *gin.Context) {
	vendorType := strings.TrimSpace(c.Param("vendor_type"))
	if vendorType == "" {
		respondError(c, http.StatusBadRequest, "VALIDATION_ERROR", "missing vendor_type")
		return
	}

	// Scoped API key id (enforced by middleware for non-admin roles).
	var apiKeyID *uuid.UUID
	if v := strings.TrimSpace(c.GetString("scoped_api_key_id")); v != "" {
		parsed, _ := uuid.Parse(v)
		apiKeyID = &parsed
	} else if v := strings.TrimSpace(c.Query("api_key_id")); v != "" {
		parsed, _ := uuid.Parse(v)
		apiKeyID = &parsed
	}

	var req testDeliveryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
		return
	}

	ctx := c.Request.Context()

	// Use NotificationService to trigger a proper workflow for this test send.
	// This ensures it appears in history and attempts, while ForcedVendor ensures
	// it bypasses routing rules to test the specific infrastructure requested.
	sendReq := &domain.SendRequest{
		IdempotencyKey: fmt.Sprintf("test:%s:%d", vendorType, uuid.New().ID()),
		UserID:         uuid.Nil.String(),
		Channels:       []domain.Channel{domain.Channel(req.Channel)},
		Type:           "test",
		Recipient:      req.Recipient,
		Subject:        req.Subject,
		Body:           req.Body,
		SlackChannel:   req.SlackChannel,
		ForcedVendor:   vendorType,
	}

	res, err := h.notifSvc.Send(ctx, sendReq, "test-delivery-handler", apiKeyID)
	if err != nil {
		h.log.Warn("test delivery workflow trigger failed", zap.String("vendor", vendorType), zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"vendor":  vendorType,
			"error":   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success":         true,
		"vendor":          vendorType,
		"notification_id": res.NotificationID,
		"status":          res.Status,
		"workflow_id":     res.WorkflowID,
	})
}

