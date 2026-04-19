package handler

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"
	"github.com/spidey/notification-service/internal/domain"
	"github.com/spidey/notification-service/internal/service"
	"go.uber.org/zap"
)

// OTPHandler handles /v1/otp routes.
type OTPHandler struct {
	otpSvc   *service.OTPService
	notifSvc *service.NotificationService
	validate *validator.Validate
	log      *zap.Logger
}

func NewOTPHandler(
	otpSvc *service.OTPService,
	notifSvc *service.NotificationService,
	log *zap.Logger,
) *OTPHandler {
	return &OTPHandler{
		otpSvc:   otpSvc,
		notifSvc: notifSvc,
		validate: validator.New(),
		log:      log,
	}
}

// SendOTP handles POST /v1/otp/send
func (h *OTPHandler) SendOTP(c *gin.Context) {
	var req domain.OTPSendRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
		return
	}
	if err := h.validate.Struct(req); err != nil {
		respondError(c, http.StatusBadRequest, "VALIDATION_ERROR", formatValidationErrors(err))
		return
	}

	expiry := time.Duration(req.ExpirySeconds) * time.Second
	if expiry == 0 {
		expiry = 5 * time.Minute
	}

	otp, err := h.otpSvc.Generate(c.Request.Context(), req.UserID, req.Purpose, expiry)
	if err != nil {
		h.log.Warn("OTP generation failed", zap.String("user_id", req.UserID), zap.Error(err))
		respondDomainError(c, err)
		return
	}

	// Send OTP via SMS using the notification service
	otpID := uuid.New().String()
	sendReq := &domain.SendRequest{
		IdempotencyKey: "otp-" + otpID,
		UserID:         req.UserID,
		Channels:       []domain.Channel{domain.ChannelSMS},
		Type:           "otp",
		TemplateVariables: map[string]string{
			"otp":     otp,
			"purpose": req.Purpose,
			"expiry":  expiry.String(),
		},
		Recipient: req.PhoneNumber,
	}

	if _, err := h.notifSvc.Send(c.Request.Context(), sendReq); err != nil {
		h.log.Error("OTP delivery failed", zap.String("user_id", req.UserID), zap.Error(err))
		respondDomainError(c, err)
		return
	}

	expiryAt := time.Now().Add(expiry)
	c.JSON(http.StatusOK, gin.H{
		"otp_id":    otpID,
		"expiry_at": expiryAt.UTC().Format(time.RFC3339),
	})
}

// VerifyOTP handles POST /v1/otp/verify
func (h *OTPHandler) VerifyOTP(c *gin.Context) {
	var req domain.OTPVerifyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
		return
	}
	if err := h.validate.Struct(req); err != nil {
		respondError(c, http.StatusBadRequest, "VALIDATION_ERROR", formatValidationErrors(err))
		return
	}

	if err := h.otpSvc.Verify(c.Request.Context(), req.UserID, req.Purpose, req.OTP); err != nil {
		h.log.Warn("OTP verification failed",
			zap.String("user_id", req.UserID),
			zap.String("purpose", req.Purpose),
			zap.Error(err),
		)
		respondDomainError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"verified": true})
}
