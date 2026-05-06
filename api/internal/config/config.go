package config

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/viper"
)

type Config struct {
	Server     ServerConfig
	Database   DatabaseConfig
	Redis      RedisConfig
	PubSub     PubSubConfig
	JWT        JWTConfig
	Clerk      ClerkConfig
	Admin      AdminBootstrapConfig `mapstructure:"admin"`
	Cadence    CadenceConfig
	Providers  ProviderConfig
	Log        LogConfig
	Security   SecurityConfig
	Worker     WorkerConfig           `mapstructure:"worker"`
	AutoScaler AutoScalerConfig      `mapstructure:"autoscaler"`
}

// AdminBootstrapConfig bootstraps an initial admin account on startup.
// If Email and Password are set, the API will ensure the admin user exists.
type ClerkConfig struct {
	SecretKey     string `mapstructure:"secret_key"`
	WebhookSecret string `mapstructure:"webhook_secret"`
	JWKSURL       string `mapstructure:"jwks_url"`
}

type AdminBootstrapConfig struct {
	Email    string `mapstructure:"email"`
	Name     string `mapstructure:"name"`
	Password string `mapstructure:"password"`
}

type SecurityConfig struct {
	// VendorConfigEncryptionKey is used to encrypt vendor_configs.config_json at rest.
	// Provide as base64 (recommended) or raw 32-byte string via NS_SECURITY_VENDOR_CONFIG_ENCRYPTION_KEY.
	VendorConfigEncryptionKey string          `mapstructure:"vendor_config_encryption_key"`
	RateLimit                 RateLimitConfig `mapstructure:"rate_limit"`
	DDoS                      DDoSConfig      `mapstructure:"ddos"`
	Headers                   HeadersConfig   `mapstructure:"headers"`
	Request                   RequestConfig   `mapstructure:"request"`
	SpamAssassin              SpamAssassinConfig   `mapstructure:"spam_assassin"`
	ContentSecurity           ContentSecurityConfig `mapstructure:"content_security"`
}

type SpamAssassinConfig struct {
	Enabled bool   `mapstructure:"enabled"`
	Host    string `mapstructure:"host"`
	Port    int    `mapstructure:"port"`
}

type ContentSecurityConfig struct {
	Enabled      bool     `mapstructure:"enabled"`
	SussyWords   []string `mapstructure:"sussy_words"`
	BlockedLinks []string `mapstructure:"blocked_links"`
}

type RateLimitConfig struct {
	Enabled bool    `mapstructure:"enabled"`
	RPS     float64 `mapstructure:"rps"`
	Burst   int     `mapstructure:"burst"`
}

type DDoSConfig struct {
	BlockThreshold int           `mapstructure:"block_threshold"`
	BlockDuration  time.Duration `mapstructure:"block_duration"`
}

type HeadersConfig struct {
	EnableSecureHeaders bool     `mapstructure:"enable_secure_headers"`
	AllowedOrigins      []string `mapstructure:"allowed_origins"`
}

type RequestConfig struct {
	MaxBodySizeMB int `mapstructure:"max_body_size_mb"`
}

type ServerConfig struct {
	Port            int           `mapstructure:"port"`
	ReadTimeout     time.Duration `mapstructure:"read_timeout"`
	WriteTimeout    time.Duration `mapstructure:"write_timeout"`
	ShutdownTimeout time.Duration `mapstructure:"shutdown_timeout"`
	Mode            string        `mapstructure:"mode"` // debug | release
}

type DatabaseConfig struct {
	DSN             string        `mapstructure:"dsn"`
	MaxOpenConns    int           `mapstructure:"max_open_conns"`
	MaxIdleConns    int           `mapstructure:"max_idle_conns"`
	ConnMaxLifetime time.Duration `mapstructure:"conn_max_lifetime"`
	MigrationDir    string        `mapstructure:"migration_dir"`
}

type RedisConfig struct {
	Addr     string `mapstructure:"addr"`
	Password string `mapstructure:"password"`
	DB       int    `mapstructure:"db"`
}

// PubSubConfig configures the message broker used for internal queuing.
// Clients interact with the system exclusively via the REST API.
// The API publishes directly to priority topics; workers consume from them.
type PubSubConfig struct {
	ProjectID            string      `mapstructure:"project_id"`
	Mode                 string      `mapstructure:"mode"` // kafka | redis | gcp | mock
	TopicOverride        string      `mapstructure:"topic_override"`
	SubscriptionOverride string      `mapstructure:"subscription_override"`
	CredentialsFile      string      `mapstructure:"credentials_file"`
	Kafka                KafkaConfig `mapstructure:"kafka"`
}

// KafkaConfig configures Kafka as the pubsub backend.
// Topics follow the pattern: notifications-{channel}-{priority}.
// Clients publish via the REST API only; workers consume from priority topics.
type KafkaConfig struct {
	Brokers           []string `mapstructure:"brokers"`
	ConsumerGroupID   string   `mapstructure:"consumer_group_id"`
	NumPartitions     int      `mapstructure:"num_partitions"`
	ReplicationFactor int      `mapstructure:"replication_factor"`
}

// AutoScalerConfig controls MTTD-based and Kafka-lag-based worker autoscaling.
type AutoScalerConfig struct {
	Enabled            bool          `mapstructure:"enabled"`
	EvaluationInterval time.Duration `mapstructure:"evaluation_interval"`

	HighMTTDThreshold   time.Duration `mapstructure:"high_mttd_threshold"`
	MediumMTTDThreshold time.Duration `mapstructure:"medium_mttd_threshold"`
	LowMTTDThreshold    time.Duration `mapstructure:"low_mttd_threshold"`

	MaxLagPerWorker int `mapstructure:"max_lag_per_worker"`

	CooldownPeriod    time.Duration `mapstructure:"cooldown_period"`
	ScaleDownFactor   float64       `mapstructure:"scale_down_factor"`
	IdleThreshold     time.Duration `mapstructure:"idle_threshold"`
	UnhealthyThreshold int          `mapstructure:"unhealthy_threshold"`

	GlobalMaxWorkers int           `mapstructure:"global_max_workers"`
	GlobalMinWorkers int           `mapstructure:"global_min_workers"`
	MTTDLookback     time.Duration `mapstructure:"mttd_lookback"`
}

// SlaMsForPriority returns the MTTD threshold in milliseconds for the given
// priority, respecting any per-priority overrides in the config.
func (c *AutoScalerConfig) SlaMsForPriority(priority string) float64 {
	switch priority {
	case "high":
		return c.HighMTTDThreshold.Seconds() * 1000
	case "medium":
		return c.MediumMTTDThreshold.Seconds() * 1000
	case "low":
		return c.LowMTTDThreshold.Seconds() * 1000
	default:
		return c.LowMTTDThreshold.Seconds() * 1000
	}
}

// WorkerConfig identifies a single worker instance in container-per-worker mode.
// These are set via environment variables (NS_WORKER_*) when the worker-manager
// spawns a dedicated container per channel × priority × vendor × client.
type WorkerConfig struct {
	Channel  string `mapstructure:"channel"`   // email | sms | push | webhook | slack | websocket
	Priority string `mapstructure:"priority"`  // high | medium | low
	Vendor   string `mapstructure:"vendor"`    // optional vendor name filter
	ClientID string `mapstructure:"client_id"` // optional api_key_id scope
}

type JWTConfig struct {
	Secret        string        `mapstructure:"secret"`
	Expiry        time.Duration `mapstructure:"expiry"`
	ServiceSecret string        `mapstructure:"service_secret"`
}

type CadenceConfig struct {
	// Mode selects the workflow engine: "temporal" (default), "cadence", or "standalone" (no orchestration).
	Mode     string `mapstructure:"mode"` // temporal | cadence | standalone
	HostPort string `mapstructure:"host_port"`
	Domain   string `mapstructure:"domain"`
}

type ProviderConfig struct {
	Email          EmailProviderConfig
	EmailRouting   RoutingConfig `mapstructure:"email_routing" json:"email_routing"`
	SMS            SMSProviderConfig
	SMSRouting     RoutingConfig `mapstructure:"sms_routing" json:"sms_routing"`
	Push           PushProviderConfig
	PushRouting    RoutingConfig `mapstructure:"push_routing" json:"push_routing"`
	WebhookRouting RoutingConfig `mapstructure:"webhook_routing" json:"webhook_routing"`
	Webhook        WebhookProviderConfig
	Slack          SlackProviderConfig `mapstructure:"slack" json:"slack"`

	// Webhook-based “social” vendors (used by webhook worker routing / forced-vendor tests).
	Discord  DiscordConfig  `mapstructure:"discord" json:"discord"`
	Teams    TeamsConfig    `mapstructure:"teams" json:"teams"`
	Telegram TelegramConfig `mapstructure:"telegram" json:"telegram"`
}

// SlackProviderConfig holds defaults for Slack Incoming Webhooks (HTTP JSON API).
type SlackProviderConfig struct {
	// WebhookURL is the legacy single-webhook configuration.
	// Prefer Channels for multi-channel Slack delivery.
	WebhookURL      string               `mapstructure:"webhook_url" json:"webhook_url"`
	Channels        []SlackChannelConfig `mapstructure:"channels" json:"channels"`
	TimeoutSeconds  int                  `mapstructure:"timeout_seconds" json:"timeout_seconds"`
	DefaultUsername string               `mapstructure:"default_username" json:"default_username"`
}

type SlackChannelConfig struct {
	ChannelName string `mapstructure:"channel_name" json:"channel_name"`
	WebhookURL  string `mapstructure:"webhook_url" json:"webhook_url"`
}

// RoutingConfig controls how a channel worker chooses vendors.
//
// Modes:
// - only: send only via `only` (or `prefer` if `only` is empty)
// - backup: try `prefer` first (or primary), then fall back to others
// - round_robin: pick exactly one vendor per message, rotating across configured vendors
// - publish_all: send the same message through all configured vendors
type RoutingConfig struct {
	Mode   string `mapstructure:"mode" json:"mode"`
	Prefer string `mapstructure:"prefer" json:"prefer"`
	// Fallback is used in backup mode to pick the second-choice vendor.
	// If empty, the worker falls back to any remaining configured vendors.
	Fallback string `mapstructure:"fallback" json:"fallback"`
	Only     string `mapstructure:"only" json:"only"`
	// Participants is used in round_robin mode to restrict which vendors participate.
	// If empty, all configured vendors participate.
	Participants []string `mapstructure:"participants" json:"participants"`

	// ErrorRateThreshold enables threshold-based fallback in backup/only/round_robin modes.
	// When the chosen/preferred vendor's recent error rate exceeds this value (0..1),
	// the worker will prefer the configured fallback vendor (or other vendors) instead.
	// If 0, the feature is disabled.
	ErrorRateThreshold float64 `mapstructure:"error_rate_threshold" json:"error_rate_threshold"`
	// MinRequests is the minimum number of (success+failure) executions observed by the circuit breaker
	// before applying error-rate based fallback.
	MinRequests int `mapstructure:"min_requests" json:"min_requests"`
}

type EmailProviderConfig struct {
	// Primary provider: ses | mailgun | smtp
	Primary     string         `mapstructure:"primary" json:"primary"`
	DefaultFrom string         `mapstructure:"default_from" json:"default_from"`
	SES         SESConfig      `mapstructure:"ses"`
	Mailgun  MailgunConfig  `mapstructure:"mailgun"`
	SendGrid SendGridConfig `mapstructure:"sendgrid" json:"sendgrid"`
	Postmark PostmarkConfig `mapstructure:"postmark" json:"postmark"`
	SMTP     SMTPConfig     `mapstructure:"smtp"`
}

type SESConfig struct {
	Region      string `mapstructure:"region" json:"region"`
	AccessKeyID string `mapstructure:"access_key_id" json:"access_key_id"`
	// AccessSecret is an alias for SecretAccessKey (used by some UIs/configs).
	AccessSecret    string `mapstructure:"access_secret" json:"access_secret"`
	SecretAccessKey string `mapstructure:"secret_access_key" json:"secret_access_key"`
	IAMUsername     string `mapstructure:"iam_username" json:"iam_username"`
	SMTPUsername    string `mapstructure:"smtp_username" json:"smtp_username"`
	SMTPPassword    string `mapstructure:"smtp_password" json:"smtp_password"`
	FromAddress     string `mapstructure:"from_address" json:"from_address"`
	FromName        string `mapstructure:"from_name" json:"from_name"`
	ReplyTo         string `mapstructure:"reply_to" json:"reply_to"`
}

type MailgunConfig struct {
	Domain            string `mapstructure:"domain" json:"domain"`
	APIKey            string `mapstructure:"api_key" json:"api_key"`
	From              string `mapstructure:"from" json:"from"`
	ReplyTo           string `mapstructure:"reply_to" json:"reply_to"`
	WebhookSigningKey string `mapstructure:"webhook_signing_key" json:"webhook_signing_key"`
}

type SendGridConfig struct {
	APIKey    string `mapstructure:"api_key" json:"api_key"`
	FromEmail string `mapstructure:"from_email" json:"from_email"`
	FromName  string `mapstructure:"from_name" json:"from_name"`
	ReplyTo   string `mapstructure:"reply_to" json:"reply_to"`
}

// Postmark uses a Server API Token for sending.
type PostmarkConfig struct {
	ServerToken string `mapstructure:"server_token" json:"server_token"`
	FromEmail   string `mapstructure:"from_email" json:"from_email"`
	FromName    string `mapstructure:"from_name" json:"from_name"`
	ReplyTo     string `mapstructure:"reply_to" json:"reply_to"`
}

type SMTPConfig struct {
	Host     string `mapstructure:"host" json:"host"`
	Port     int    `mapstructure:"port" json:"port"`
	Username string `mapstructure:"username" json:"username"`
	Password string `mapstructure:"password" json:"password"`
	From     string `mapstructure:"from" json:"from"`
	ReplyTo  string `mapstructure:"reply_to" json:"reply_to"`
}

type SMSProviderConfig struct {
	Primary     string            `mapstructure:"primary"` // twilio | plivo | vonage | messagebird
	Twilio      TwilioConfig      `mapstructure:"twilio"`
	Plivo       PlivoConfig       `mapstructure:"plivo"`
	Vonage      VonageConfig      `mapstructure:"vonage"`
	MessageBird MessageBirdConfig `mapstructure:"messagebird" json:"messagebird"`
}

type TwilioConfig struct {
	AccountSID string `mapstructure:"account_sid" json:"account_sid"`
	AuthToken  string `mapstructure:"auth_token" json:"auth_token"`
	FromNumber string `mapstructure:"from_number" json:"from_number"`
}

type PlivoConfig struct {
	AuthID     string `mapstructure:"auth_id" json:"auth_id"`
	AuthToken  string `mapstructure:"auth_token" json:"auth_token"`
	FromNumber string `mapstructure:"from_number" json:"from_number"`
}

type VonageConfig struct {
	APIKey    string `mapstructure:"api_key" json:"api_key"`
	APISecret string `mapstructure:"api_secret" json:"api_secret"`
	From      string `mapstructure:"from" json:"from"`
}

// MessageBird SMS uses an access key + originator (sender id/number).
type MessageBirdConfig struct {
	AccessKey  string `mapstructure:"access_key" json:"access_key"`
	Originator string `mapstructure:"originator" json:"originator"`
}

type PushProviderConfig struct {
	FCM       FCMConfig       `mapstructure:"fcm"`
	OneSignal OneSignalConfig `mapstructure:"onesignal" json:"onesignal"`
	Pusher    PusherConfig    `mapstructure:"pusher" json:"pusher"`
	APNs      APNsConfig      `mapstructure:"apns"`
	Pushwoosh PushwooshConfig `mapstructure:"pushwoosh"`
}

type FCMConfig struct {
	ServiceAccountJSON string `mapstructure:"service_account_json"`
}

// OneSignal supports App-level REST API keys.
type OneSignalConfig struct {
	AppID      string `mapstructure:"app_id" json:"app_id"`
	RestAPIKey string `mapstructure:"rest_api_key" json:"rest_api_key"`
}

// Pusher here refers to Pusher Beams (push notifications).
type PusherConfig struct {
	InstanceID string `mapstructure:"instance_id" json:"instance_id"`
	SecretKey  string `mapstructure:"secret_key" json:"secret_key"`
}

type APNsConfig struct {
	KeyFile  string `mapstructure:"key_file"`
	KeyID    string `mapstructure:"key_id"`
	TeamID   string `mapstructure:"team_id"`
	BundleID string `mapstructure:"bundle_id"`
	Sandbox  bool   `mapstructure:"sandbox"`
}

type PushwooshConfig struct {
	ApplicationCode string `mapstructure:"application_code"`
	APIAccessToken  string `mapstructure:"api_access_token"`
}

type WebhookProviderConfig struct {
	SigningSecret  string `mapstructure:"signing_secret"`
	TimeoutSeconds int    `mapstructure:"timeout_seconds"`
	MaxRetries     int    `mapstructure:"max_retries"`
}

// Discord incoming webhooks: POST JSON {"content": "..."} to a webhook URL.
type DiscordConfig struct {
	WebhookURL     string `mapstructure:"webhook_url" json:"webhook_url"`
	TimeoutSeconds int    `mapstructure:"timeout_seconds" json:"timeout_seconds"`
}

// Microsoft Teams incoming webhooks: POST JSON {"text": "..."} to the webhook URL.
type TeamsConfig struct {
	WebhookURL     string `mapstructure:"webhook_url" json:"webhook_url"`
	TimeoutSeconds int    `mapstructure:"timeout_seconds" json:"timeout_seconds"`
}

// Telegram Bot API: POST to https://api.telegram.org/bot<TOKEN>/sendMessage with chat_id + text.
type TelegramConfig struct {
	BotToken       string `mapstructure:"bot_token" json:"bot_token"`
	ChatID         string `mapstructure:"chat_id" json:"chat_id"`
	TimeoutSeconds int    `mapstructure:"timeout_seconds" json:"timeout_seconds"`
}

type LogConfig struct {
	Level  string `mapstructure:"level"`  // debug | info | warn | error
	Format string `mapstructure:"format"` // json | console
}

func Load(path string) (*Config, error) {
	v := viper.New()

	v.SetConfigName("config")
	v.SetConfigType("yaml")

	if path != "" {
		v.AddConfigPath(path)
	}
	v.AddConfigPath(".")
	v.AddConfigPath("./config")
	v.AddConfigPath("/etc/notification-service")

	v.SetEnvPrefix("NS")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	setDefaults(v)

	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("reading config: %w", err)
		}
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("unmarshalling config: %w", err)
	}

	// Viper delivers []string env vars as a single comma-joined string when set via
	// NS_PUBSUB_KAFKA_BROKERS="broker1:9092,broker2:9092". Re-split here.
	if len(cfg.PubSub.Kafka.Brokers) == 1 && strings.Contains(cfg.PubSub.Kafka.Brokers[0], ",") {
		cfg.PubSub.Kafka.Brokers = strings.Split(cfg.PubSub.Kafka.Brokers[0], ",")
	}

	if err := validate(&cfg); err != nil {
		return nil, fmt.Errorf("validating config: %w", err)
	}

	return &cfg, nil
}

func setDefaults(v *viper.Viper) {
	v.SetDefault("server.port", 8080)
	v.SetDefault("server.read_timeout", "30s")
	v.SetDefault("server.write_timeout", "30s")
	v.SetDefault("server.shutdown_timeout", "30s")
	v.SetDefault("server.mode", "release")

	v.SetDefault("database.max_open_conns", 25)
	v.SetDefault("database.max_idle_conns", 5)
	v.SetDefault("database.conn_max_lifetime", "5m")
	v.SetDefault("database.migration_dir", "migrations")

	v.SetDefault("redis.addr", "localhost:6379")
	v.SetDefault("redis.db", 0)

	v.SetDefault("pubsub.mode", "mock")
	v.SetDefault("pubsub.project_id", "local-project")
	v.SetDefault("pubsub.kafka.brokers", []string{"kafka:9092"})
	v.SetDefault("pubsub.kafka.consumer_group_id", "notification-service")
	v.SetDefault("pubsub.kafka.num_partitions", 3)
	v.SetDefault("pubsub.kafka.replication_factor", 1)

	v.SetDefault("cadence.mode", "standalone")
	v.SetDefault("cadence.host_port", "")
	v.SetDefault("cadence.domain", "")

	v.SetDefault("jwt.expiry", "24h")
	v.SetDefault("admin.name", "Admin")
	v.SetDefault("clerk.jwks_url", "https://clerk.example.com/.well-known/jwks.json")

	v.SetDefault("providers.webhook.timeout_seconds", 30)
	v.SetDefault("providers.webhook.max_retries", 5)
	v.SetDefault("providers.slack.timeout_seconds", 30)

	v.SetDefault("providers.email_routing.mode", "backup")
	v.SetDefault("providers.email.default_from", "no-reply@example.com")
	v.SetDefault("providers.sms_routing.mode", "backup")
	v.SetDefault("providers.push_routing.mode", "backup")
	v.SetDefault("providers.webhook_routing.mode", "backup")

	v.SetDefault("log.level", "info")
	v.SetDefault("log.format", "json")

	v.SetDefault("security.rate_limit.enabled", true)
	v.SetDefault("security.rate_limit.rps", 100.0)
	v.SetDefault("security.rate_limit.burst", 100)
	v.SetDefault("security.ddos.block_threshold", 500)
	v.SetDefault("security.ddos.block_duration", "5m")
	v.SetDefault("security.headers.enable_secure_headers", true)
	v.SetDefault("security.headers.allowed_origins", []string{"*"})
	v.SetDefault("security.request.max_body_size_mb", 2)

	v.SetDefault("security.spam_assassin.enabled", true)
	v.SetDefault("security.spam_assassin.host", "localhost")
	v.SetDefault("security.spam_assassin.port", 783)

	v.SetDefault("security.content_security.enabled", false)
	v.SetDefault("security.content_security.sussy_words", []string{"crypto", "casino", "free money", "winner", "urgent"})
	v.SetDefault("security.content_security.blocked_links", []string{"bit.ly", "tinyurl.com", "malicious-site.com"})

	// Autoscaler defaults
	v.SetDefault("autoscaler.enabled", false)
	v.SetDefault("autoscaler.evaluation_interval", "15s")
	v.SetDefault("autoscaler.high_mttd_threshold", "5s")
	v.SetDefault("autoscaler.medium_mttd_threshold", "10s")
	v.SetDefault("autoscaler.low_mttd_threshold", "15s")
	v.SetDefault("autoscaler.max_lag_per_worker", 50)
	v.SetDefault("autoscaler.cooldown_period", "60s")
	v.SetDefault("autoscaler.scale_down_factor", 0.5)
	v.SetDefault("autoscaler.idle_threshold", "5m")
	v.SetDefault("autoscaler.unhealthy_threshold", 6)
	v.SetDefault("autoscaler.global_max_workers", 20)
	v.SetDefault("autoscaler.global_min_workers", 1)
	v.SetDefault("autoscaler.mttd_lookback", "3m")
}

func validate(cfg *Config) error {
	if cfg.Database.DSN == "" {
		return fmt.Errorf("database.dsn is required")
	}

	if cfg.Server.Mode == "release" {
		if isDisallowedSecretValue(cfg.JWT.Secret) {
			return fmt.Errorf("jwt.secret is missing or placeholder in release mode; set NS_JWT_SECRET to a strong secret")
		}
		if len(strings.TrimSpace(cfg.JWT.Secret)) < 32 {
			return fmt.Errorf("jwt.secret must be at least 32 characters in release mode")
		}

		if isDisallowedSecretValue(cfg.Security.VendorConfigEncryptionKey) {
			return fmt.Errorf("security.vendor_config_encryption_key is missing or placeholder in release mode; set NS_SECURITY_VENDOR_CONFIG_ENCRYPTION_KEY")
		}
	}

	// If admin bootstrap is configured, require both email + password.
	if (cfg.Admin.Email != "" || cfg.Admin.Password != "") && (cfg.Admin.Email == "" || cfg.Admin.Password == "") {
		return fmt.Errorf("admin.email and admin.password must both be set")
	}
	return nil
}

// isDisallowedSecretValue returns true for empty/placeholder values that must
// never be accepted in release mode.
func isDisallowedSecretValue(s string) bool {
	v := strings.TrimSpace(strings.ToLower(s))
	if v == "" {
		return true
	}

	switch v {
	case "<set_in_env>", "change-me-in-production", "changeme", "set_in_env", "set-in-env", "secret", "password":
		return true
	}

	if strings.Contains(v, "encryption_key_here") {
		return true
	}
	if strings.Contains(v, "placeholder") {
		return true
	}
	return false
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
