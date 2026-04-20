package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/spidey/notification-service/internal/circuit"
	"github.com/spidey/notification-service/internal/config"
	"gopkg.in/yaml.v3"
	"os"
)

// Dependencies groups all handler dependencies for router setup.
type Dependencies struct {
	NotificationHandler *NotificationHandler
	OTPHandler          *OTPHandler
	WebhookHandler      *WebhookHandler
	PrefsHandler        *PreferencesHandler
	ReportHandler       *ReportHandler
	AdminHandler        *AdminHandler
	GovernanceHandler   *GovernanceHandler
	TemplateHandler     *TemplateHandler
	CircuitRegistry     *circuit.Registry
	Config              *config.Config
}

// NewRouter creates and configures the Gin router.
func NewRouter(deps Dependencies) *gin.Engine {
	if deps.Config.Server.Mode == "release" {
		gin.SetMode(gin.ReleaseMode)
	}

	r := gin.New()

	// Global middleware
	r.Use(RequestID())
	r.Use(Recovery(deps.NotificationHandler.log))
	r.Use(Logger(deps.NotificationHandler.log))
	r.Use(CORS())
	r.Use(Prometheus())
	r.Use(SecurityHeaders(deps.Config.Security.Headers))
	r.Use(RequestSizeLimiter(deps.Config.Security.Request.MaxBodySizeMB))
	r.Use(RateLimiter(deps.Config.Security))

	// Health check — no auth required
	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	// Metrics — internal only
	r.GET("/metrics", gin.WrapH(promhttp.Handler()))

	// Circuit breaker status — internal only
	r.GET("/internal/circuit-breakers", func(c *gin.Context) {
		c.JSON(http.StatusOK, deps.CircuitRegistry.Snapshot())
	})

	v1 := r.Group("/v1")

	// Static OpenAPI Spec
	v1.StaticFile("/openapi.yaml", "./docs/openapi.yaml")
	v1.GET("/openapi.json", func(c *gin.Context) {
		content, err := os.ReadFile("./docs/openapi.yaml")
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to read openapi.yaml"})
			return
		}
		var data interface{}
		if err := yaml.Unmarshal(content, &data); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to parse yaml"})
			return
		}
		c.JSON(http.StatusOK, data)
	})

	// Notifications
	notif := v1.Group("/notifications")
	notif.Use(JWTAuth(deps.Config.JWT.Secret, deps.Config.Server.Mode == "debug"))
	{
		notif.POST("", deps.NotificationHandler.Send)
		notif.POST("/bulk", deps.NotificationHandler.SendBulk)
		notif.GET("", deps.NotificationHandler.List)
		notif.GET("/scheduled", deps.NotificationHandler.ListScheduled)
		notif.GET("/:id", deps.NotificationHandler.GetByID)
		notif.POST("/:id/sync", deps.NotificationHandler.SyncStatus)
		notif.PATCH("/:id/schedule", deps.NotificationHandler.RescheduleNotification)
		notif.DELETE("/:id/schedule", deps.NotificationHandler.CancelNotification)
	}

	// OTP — service auth (internal callers only)
	otp := v1.Group("/otp")
	otp.Use(ServiceAuth(deps.Config.JWT.ServiceSecret))
	{
		otp.POST("/send", deps.OTPHandler.SendOTP)
		otp.POST("/verify", deps.OTPHandler.VerifyOTP)
	}

	// Provider webhooks — no auth, validated by HMAC signature per provider
	webhooks := v1.Group("/webhooks")
	{
		webhooks.POST("/:provider", deps.WebhookHandler.HandleProviderEvent)
	}

	// User preferences
	users := v1.Group("/users")
	users.Use(JWTAuth(deps.Config.JWT.Secret, deps.Config.Server.Mode == "debug"))
	{
		users.GET("/:user_id/notification-preferences", deps.PrefsHandler.GetPreferences)
		users.PUT("/:user_id/notification-preferences", deps.PrefsHandler.UpdatePreferences)
	}

	// Reports
	reports := v1.Group("/reports")
	reports.Use(JWTAuth(deps.Config.JWT.Secret, deps.Config.Server.Mode == "debug"))
	{
		reports.GET("/channel-metrics", deps.ReportHandler.ChannelMetrics)
		reports.GET("/summary", deps.ReportHandler.Summary)
		reports.GET("/ingress", deps.ReportHandler.IngressBreakdown)
	}

	// Admin config — restricted to authorized admins (using same JWT secret for now)
	admin := v1.Group("/admin")
	admin.Use(JWTAuth(deps.Config.JWT.Secret, deps.Config.Server.Mode == "debug"))
	{
		admin.GET("/config/vendors", deps.AdminHandler.GetVendorConfigs)
		admin.PUT("/config/vendors/:vendor_type", deps.AdminHandler.UpdateVendorConfig)
	}

	// Governance (Suppressions & Opt-outs)
	gov := v1.Group("/governance")
	gov.Use(JWTAuth(deps.Config.JWT.Secret, deps.Config.Server.Mode == "debug"))
	{
		gov.GET("/suppressions", deps.GovernanceHandler.ListSuppressions)
		gov.POST("/suppressions", deps.GovernanceHandler.AddSuppression)
		gov.DELETE("/suppressions/:id", deps.GovernanceHandler.DeleteSuppression)

		gov.GET("/opt-outs", deps.GovernanceHandler.ListOptOuts)
		gov.POST("/opt-outs", deps.GovernanceHandler.AddOptOut)
		gov.DELETE("/opt-outs/:id", deps.GovernanceHandler.DeleteOptOut)
	}
	
	// Templates
	templates := v1.Group("/templates")
	templates.Use(JWTAuth(deps.Config.JWT.Secret, deps.Config.Server.Mode == "debug"))
	{
		templates.GET("", deps.TemplateHandler.List)
		templates.GET("/:id", deps.TemplateHandler.GetByID)
		templates.POST("", deps.TemplateHandler.Create)
		templates.PUT("/:id", deps.TemplateHandler.Update)
		templates.DELETE("/:id", deps.TemplateHandler.Delete)
	}

	return r
}
