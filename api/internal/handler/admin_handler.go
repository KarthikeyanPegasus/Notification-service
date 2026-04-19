package handler

import (
	"encoding/json"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/spidey/notification-service/internal/service"
	"go.uber.org/zap"
)

type AdminHandler struct {
	configSvc service.ConfigService
	log       *zap.Logger
}

func NewAdminHandler(configSvc service.ConfigService, log *zap.Logger) *AdminHandler {
	return &AdminHandler{
		configSvc: configSvc,
		log:       log,
	}
}

// GetVendorConfigs returns all dynamic vendor configurations.
func (h *AdminHandler) GetVendorConfigs(c *gin.Context) {
	configs, err := h.configSvc.GetVendorConfigs(c.Request.Context())
	if err != nil {
		h.log.Error("failed to get vendor configs", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to retrieve configurations"})
		return
	}
	c.JSON(http.StatusOK, configs)
}

// UpdateVendorConfig updates or creates a dynamic vendor configuration.
func (h *AdminHandler) UpdateVendorConfig(c *gin.Context) {
	vendorType := c.Param("vendor_type")
	var req struct {
		Config json.RawMessage `json:"config" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	err := h.configSvc.UpdateVendorConfig(c.Request.Context(), vendorType, req.Config)
	if err != nil {
		h.log.Error("failed to update vendor config", zap.Error(err), zap.String("vendor", vendorType))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update configuration"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "configuration updated successfully"})
}
