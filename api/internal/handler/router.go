package handler

import (
	"net/http"
	"os"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/spidey/notification-service/internal/circuit"
	"github.com/spidey/notification-service/internal/config"
	"github.com/spidey/notification-service/internal/repository"
	"gopkg.in/yaml.v3"
)

// Dependencies groups all handler dependencies for router setup.
type Dependencies struct {
	NotificationHandler *NotificationHandler
	OTPHandler          *OTPHandler
	WebhookHandler      *WebhookHandler
	PrefsHandler        *PreferencesHandler
	ReportHandler       *ReportHandler
	AdminHandler        *AdminHandler
	TestDeliveryHandler *TestDeliveryHandler
	APIKeyHandler       *APIKeyHandler
	AuthHandler         *AuthHandler
	ClerkWebhookHandler *ClerkWebhookHandler
	InvitationHandler   *InvitationHandler
	UserAdminHandler    *UserAdminHandler
	MeHandler           *MeHandler
	GovernanceHandler   *GovernanceHandler
	TemplateHandler     *TemplateHandler
	WSHandler           *WebSocketHandler
	DLQHandler          *DLQHandler
	CircuitRegistry     *circuit.Registry
	Config              *config.Config
	APIKeyVerifier      AuthAPIKeyVerifier
	UserRepo            *repository.UserRepository
}

// NewRouter creates and configures the Gin router.
func NewRouter(deps Dependencies) *gin.Engine {
	if deps.Config.Server.Mode == "release" {
		gin.SetMode(gin.ReleaseMode)
	}

	r := gin.New()
	// Security hardening: do not trust proxy headers by default.
	// This prevents X-Forwarded-For spoofing from untrusted clients.
	_ = r.SetTrustedProxies(nil)

	// Global middleware
	r.Use(RequestID())
	r.Use(Recovery(deps.NotificationHandler.log))
	r.Use(Logger(deps.NotificationHandler.log))
	r.Use(CORS(deps.Config.Security.Headers.AllowedOrigins))
	r.Use(SecurityHeaders(deps.Config.Security.Headers))
	r.Use(RequestSizeLimiter(deps.Config.Security.Request.MaxBodySizeMB))
	r.Use(RateLimiter(deps.Config.Security))

	// Health check — no auth required
	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	// Prometheus metrics — restricted to localhost / private network (Prometheus scraper)
	r.GET("/metrics", internalOnly(), gin.WrapH(promhttp.Handler()))

	// Circuit breaker status — restricted to localhost / private network
	r.GET("/internal/circuit-breakers", internalOnly(), func(c *gin.Context) {
		c.JSON(http.StatusOK, deps.CircuitRegistry.Snapshot())
	})

	v1 := r.Group("/v1")
	// Auth — Clerk handles authentication; login endpoint removed.
	// See /v1/webhooks/clerk for user lifecycle sync.

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
		v1.GET("/docs/visibility", deps.AdminHandler.GetApiDocsVisibility)

	// Notifications
	notif := v1.Group("/notifications")
	notif.Use(AnyAuth(deps.Config.JWT.Secret, deps.Config.Server.Mode == "debug", deps.APIKeyVerifier, deps.UserRepo))
	{
		// allow: admin/dev or API key callers
		// RequireClientScope enforces tenant isolation: API keys can only send to their own scope
		notif.POST("", RequireRole("admin", "manager", "dev", "api_key"), RequireClientScope(deps.UserRepo), deps.NotificationHandler.Send)
		notif.POST("/bulk", RequireRole("admin", "manager", "dev", "api_key"), RequireClientScope(deps.UserRepo), deps.NotificationHandler.SendBulk)
		notif.GET("", RequireRole("admin", "manager", "dev", "support", "api_key"), deps.NotificationHandler.List)
		notif.GET("/scheduled", RequireRole("admin", "manager", "dev", "support"), RequireClientScope(deps.UserRepo), deps.NotificationHandler.ListScheduled)
		notif.GET("/:id", RequireRole("admin", "manager", "dev", "support", "api_key"), deps.NotificationHandler.GetByID)
		notif.POST("/:id/sync", RequireRole("admin", "manager", "dev", "support", "api_key"), deps.NotificationHandler.SyncStatus)
		notif.POST("/:id/retrigger", RequireRole("admin", "manager", "dev", "api_key"), deps.NotificationHandler.Retrigger)
		notif.PATCH("/:id/schedule", RequireRole("admin", "manager", "dev"), RequireClientScope(deps.UserRepo), deps.NotificationHandler.RescheduleNotification)
		notif.DELETE("/:id/schedule", RequireRole("admin", "manager", "dev"), RequireClientScope(deps.UserRepo), deps.NotificationHandler.CancelNotification)
	}

	// OTP — service auth (internal callers only)
	otp := v1.Group("/otp")
	otp.Use(ServiceAuth(deps.Config.JWT.ServiceSecret))
	{
		otp.POST("/send", deps.OTPHandler.SendOTP)
		otp.POST("/verify", deps.OTPHandler.VerifyOTP)
	}

	// Provider webhooks — no auth, validated by HMAC signature per provider
	// GET is used by Plivo, Vonage, and MessageBird delivery receipt callbacks.
	webhooks := v1.Group("/webhooks")
	{
		webhooks.POST("/:provider", deps.WebhookHandler.HandleProviderEvent)
		webhooks.GET("/:provider", deps.WebhookHandler.HandleProviderCallback)
		// Clerk webhook for user lifecycle events (signed via Svix)
		// Nil guard: only register if Clerk webhook secret is configured and handler initialized.
		if deps.ClerkWebhookHandler != nil {
			webhooks.POST("/clerk", deps.ClerkWebhookHandler.HandleWebhook)
		}
	}

	// User preferences — authenticated users may access their own preferences.
	// Admin and support roles can access any user's preferences.
	users := v1.Group("/users")
	users.Use(AnyAuth(deps.Config.JWT.Secret, deps.Config.Server.Mode == "debug", deps.APIKeyVerifier, deps.UserRepo))
	users.Use(requireOwnershipOrRole("admin", "support"))
	{
		users.GET("/:user_id/notification-preferences", deps.PrefsHandler.GetPreferences)
		users.PUT("/:user_id/notification-preferences", deps.PrefsHandler.UpdatePreferences)
	}

	// WebSocket — authenticated long-lived connections for real-time notification delivery.
	v1.GET("/ws", AnyAuth(deps.Config.JWT.Secret, deps.Config.Server.Mode == "debug", deps.APIKeyVerifier, deps.UserRepo), deps.WSHandler.Upgrade)

	// Reports
	reports := v1.Group("/reports")
	reports.Use(AnyAuth(deps.Config.JWT.Secret, deps.Config.Server.Mode == "debug", deps.APIKeyVerifier, deps.UserRepo))
	reports.Use(RequireRole("admin", "manager", "dev", "support"))
	{
		reports.GET("/channel-metrics", deps.ReportHandler.ChannelMetrics)
		reports.GET("/summary", deps.ReportHandler.Summary)
		reports.GET("/ingress", deps.ReportHandler.IngressBreakdown)
		reports.GET("/sms-countries", deps.ReportHandler.SMSCountryBreakdown)
		reports.GET("/email-domains", deps.ReportHandler.EmailDomainBreakdown)
		reports.GET("/vendors", deps.ReportHandler.VendorMetrics)
		reports.GET("/billing", deps.ReportHandler.VendorBilling)
		reports.GET("/scheduled-stats", deps.ReportHandler.ScheduledStats)
	}

	// Current user helpers (for UI client scoping)
	me := v1.Group("/me")
	me.Use(AnyAuth(deps.Config.JWT.Secret, deps.Config.Server.Mode == "debug", deps.APIKeyVerifier, deps.UserRepo))
	{
		me.GET("/clients", deps.MeHandler.ListClients)
		me.POST("/clients", RequireRole("dev"), deps.MeHandler.CreateClient)
	}

	// Admin config — restricted to authorized admins (using same JWT secret for now)
	admin := v1.Group("/admin")
	admin.Use(AnyAuth(deps.Config.JWT.Secret, deps.Config.Server.Mode == "debug", deps.APIKeyVerifier, deps.UserRepo))
	{
		// Vendor config: admin or manager (manager must be scoped via api_key_id; enforced in handler/service layer usage)
		admin.GET("/config/vendors", RequireRole("admin", "manager", "dev"), RequireClientScope(deps.UserRepo), deps.AdminHandler.GetVendorConfigs)
		admin.PUT("/config/vendors/:vendor_type", RequireRole("admin", "manager", "dev"), RequireClientScope(deps.UserRepo), deps.AdminHandler.UpdateVendorConfig)
		admin.DELETE("/config/vendors/:vendor_type", RequireRole("admin", "dev"), RequireClientScope(deps.UserRepo), deps.AdminHandler.DeleteVendorConfig)

		// Vendor rate limits: rps, per_minute, per_10_min, per_hour, per_day — enforced per-vendor by workers.
		admin.GET("/config/rate-limits", RequireRole("admin", "manager", "dev"), RequireClientScope(deps.UserRepo), deps.AdminHandler.GetVendorRateLimits)
		admin.PUT("/config/rate-limits/:vendor_name", RequireRole("admin", "manager", "dev"), RequireClientScope(deps.UserRepo), deps.AdminHandler.UpsertVendorRateLimit)
		admin.DELETE("/config/rate-limits/:vendor_name", RequireRole("admin", "dev"), RequireClientScope(deps.UserRepo), deps.AdminHandler.DeleteVendorRateLimit)

		// Vendor retry configs: retry_initial_interval_ms, retry_max_interval_ms, retry_max_attempts, retry_backoff_coefficient, sla_seconds
		admin.GET("/config/retry-configs", RequireRole("admin", "manager", "dev"), RequireClientScope(deps.UserRepo), deps.AdminHandler.GetVendorRetryConfigs)
		admin.PUT("/config/retry-configs/:vendor_name", RequireRole("admin", "manager", "dev"), RequireClientScope(deps.UserRepo), deps.AdminHandler.UpsertVendorRetryConfig)
		admin.DELETE("/config/retry-configs/:vendor_name", RequireRole("admin", "dev"), RequireClientScope(deps.UserRepo), deps.AdminHandler.DeleteVendorRetryConfig)

		// AutoScaler config — runtime autoscaler settings.
		admin.GET("/config/autoscaler", RequireRole("admin"), deps.AdminHandler.GetAutoScalerConfig)
		admin.PUT("/config/autoscaler", RequireRole("admin"), deps.AdminHandler.UpdateAutoScalerConfig)

		// Admin dashboard overview — aggregated system metrics.
		admin.GET("/overview", RequireRole("admin"), deps.AdminHandler.GetAdminOverview)

		// API Keys (for server-to-server / programmatic access)
		// dev: can create/list their own sandbox keys (max 1 enforced in handler)
		// manager: read-only list of keys scoped to their assigned clients
		admin.POST("/api-keys", RequireRole("admin"), deps.APIKeyHandler.Create)
		admin.GET("/api-keys", RequireRole("admin", "manager", "dev"), deps.APIKeyHandler.List)
		admin.DELETE("/api-keys/:id", RequireRole("admin"), deps.APIKeyHandler.Revoke)

		// People / RBAC
		// dev: can invite up to 1 person (dev/support roles only — enforced in handler)
		admin.POST("/invitations", RequireRole("admin", "dev"), deps.InvitationHandler.Invite)

		admin.GET("/users", RequireRole("admin", "dev"), deps.UserAdminHandler.List)
		admin.POST("/users", RequireRole("admin", "dev"), deps.UserAdminHandler.Create)
		admin.DELETE("/users/:id", RequireRole("admin"), deps.UserAdminHandler.Delete)
		admin.PUT("/users/:id/role", RequireRole("admin"), deps.UserAdminHandler.SetRole)
		admin.GET("/users/:id/clients", RequireRole("admin", "manager", "dev"), deps.UserAdminHandler.ListAssignments)
		admin.PUT("/users/:id/clients", RequireRole("admin"), deps.UserAdminHandler.SetAssignments)

		// Test delivery (send a test message via a specific vendor)
		admin.POST("/test-delivery/:vendor_type",
			RequireRole("admin", "manager", "dev"),
			RequireClientScope(deps.UserRepo),
			deps.TestDeliveryHandler.Send,
		)

		// Orchestration migration status and control.
		admin.GET("/migrations", RequireRole("admin"), deps.AdminHandler.ListMigrations)
		admin.POST("/migrations/:id/cancel", RequireRole("admin"), deps.AdminHandler.CancelMigration)

		// Vendor migrations — swap vendors or hot-swap credentials within the same vendor.
		admin.POST("/config/vendors/migrations", RequireRole("admin", "manager"), RequireClientScope(deps.UserRepo), deps.AdminHandler.StartVendorMigration)
		admin.GET("/config/vendors/migrations", RequireRole("admin", "manager", "dev"), RequireClientScope(deps.UserRepo), deps.AdminHandler.ListVendorMigrations)
		admin.POST("/config/vendors/migrations/:id/complete", RequireRole("admin", "manager"), RequireClientScope(deps.UserRepo), deps.AdminHandler.CompleteVendorMigration)
		admin.POST("/config/vendors/migrations/:id/rollback", RequireRole("admin", "manager"), RequireClientScope(deps.UserRepo), deps.AdminHandler.RollbackVendorMigration)

		// Dead-Letter Queue — scoped by client for non-admin roles
		// admin: sees all,  manager/dev/support: scoped by assigned client
		admin.GET("/dlq", RequireRole("admin", "manager", "dev", "support"), RequireClientScope(deps.UserRepo), deps.DLQHandler.ListDLQ)
		admin.GET("/dlq/stats", RequireRole("admin", "manager", "dev", "support"), RequireClientScope(deps.UserRepo), deps.DLQHandler.DLQStats)
		admin.GET("/dlq/:id", RequireRole("admin", "manager", "dev", "support"), RequireClientScope(deps.UserRepo), deps.DLQHandler.GetDLQEntry)
		admin.POST("/dlq/:id/replay", RequireRole("admin", "manager", "dev", "support"), RequireClientScope(deps.UserRepo), deps.DLQHandler.ReplayDLQEntry)
		admin.POST("/dlq/replay-all", RequireRole("admin", "manager", "dev", "support"), RequireClientScope(deps.UserRepo), deps.DLQHandler.ReplayAllDLQ)
	}

	// Governance (Suppressions & Opt-outs)
	gov := v1.Group("/governance")
	gov.Use(AnyAuth(deps.Config.JWT.Secret, deps.Config.Server.Mode == "debug", deps.APIKeyVerifier, deps.UserRepo))
	gov.Use(RequireRole("admin", "manager", "dev", "support"))
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
	templates.Use(AnyAuth(deps.Config.JWT.Secret, deps.Config.Server.Mode == "debug", deps.APIKeyVerifier, deps.UserRepo))
	templates.Use(RequireRole("admin", "manager", "dev"))
	templates.Use(RequireClientScope(deps.UserRepo))
	{
		templates.GET("", deps.TemplateHandler.List)
		templates.GET("/:id", deps.TemplateHandler.GetByID)
		templates.POST("", deps.TemplateHandler.Create)
		templates.PUT("/:id", deps.TemplateHandler.Update)
		templates.DELETE("/:id", deps.TemplateHandler.Delete)
	}

	return r
}
