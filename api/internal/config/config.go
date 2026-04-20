package config

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/viper"
)

type Config struct {
	Server   ServerConfig
	Database DatabaseConfig
	Redis    RedisConfig
	PubSub   PubSubConfig
	JWT      JWTConfig
	Cadence  CadenceConfig
	Providers ProviderConfig
	Log      LogConfig
	Security SecurityConfig
}

type SecurityConfig struct {
	RateLimit RateLimitConfig `mapstructure:"rate_limit"`
	DDoS      DDoSConfig      `mapstructure:"ddos"`
	Headers   HeadersConfig   `mapstructure:"headers"`
	Request   RequestConfig   `mapstructure:"request"`
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

type PubSubConfig struct {
	ProjectID            string `mapstructure:"project_id"`
	Mode                 string `mapstructure:"mode"` // gcp | mock
	TopicOverride        string `mapstructure:"topic_override"`
	SubscriptionOverride string `mapstructure:"subscription_override"`
	CredentialsFile      string `mapstructure:"credentials_file"`
	EventsTopic          string `mapstructure:"events_topic"`
	EventsSubscription   string `mapstructure:"events_subscription"`
}

type JWTConfig struct {
	Secret        string        `mapstructure:"secret"`
	Expiry        time.Duration `mapstructure:"expiry"`
	ServiceSecret string        `mapstructure:"service_secret"`
}

type CadenceConfig struct {
	// Use "standalone" for local dev without Cadence server
	Mode     string `mapstructure:"mode"` // cadence | standalone
	HostPort string `mapstructure:"host_port"`
	Domain   string `mapstructure:"domain"`
}

type ProviderConfig struct {
	Email   EmailProviderConfig
	EmailRouting RoutingConfig `mapstructure:"email_routing" json:"email_routing"`
	SMS     SMSProviderConfig
	SMSRouting RoutingConfig `mapstructure:"sms_routing" json:"sms_routing"`
	Push    PushProviderConfig
	PushRouting RoutingConfig `mapstructure:"push_routing" json:"push_routing"`
	Webhook WebhookProviderConfig
	Slack   SlackProviderConfig `mapstructure:"slack" json:"slack"`
}

// SlackProviderConfig holds defaults for Slack Incoming Webhooks (HTTP JSON API).
type SlackProviderConfig struct {
	WebhookURL      string `mapstructure:"webhook_url" json:"webhook_url"`
	TimeoutSeconds  int    `mapstructure:"timeout_seconds" json:"timeout_seconds"`
	DefaultUsername string `mapstructure:"default_username" json:"default_username"`
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
}

type EmailProviderConfig struct {
	// Primary provider: ses | mailgun | smtp
	Primary string            `mapstructure:"primary" json:"primary"`
	SES     SESConfig         `mapstructure:"ses"`
	Mailgun MailgunConfig     `mapstructure:"mailgun"`
	SMTP    SMTPConfig        `mapstructure:"smtp"`
}

type SESConfig struct {
	Region          string `mapstructure:"region" json:"region"`
	AccessKeyID     string `mapstructure:"access_key_id" json:"access_key_id"`
	SecretAccessKey string `mapstructure:"secret_access_key" json:"secret_access_key"`
	IAMUsername     string `mapstructure:"iam_username" json:"iam_username"`
	SMTPUsername    string `mapstructure:"smtp_username" json:"smtp_username"`
	SMTPPassword    string `mapstructure:"smtp_password" json:"smtp_password"`
	FromAddress     string `mapstructure:"from_address" json:"from_address"`
	FromName        string `mapstructure:"from_name" json:"from_name"`
}

type MailgunConfig struct {
	Domain  string `mapstructure:"domain"`
	APIKey  string `mapstructure:"api_key"`
	From    string `mapstructure:"from"`
}

type SMTPConfig struct {
	Host     string `mapstructure:"host"`
	Port     int    `mapstructure:"port"`
	Username string `mapstructure:"username"`
	Password string `mapstructure:"password"`
	From     string `mapstructure:"from"`
}

type SMSProviderConfig struct {
	Primary string       `mapstructure:"primary"` // twilio | plivo | vonage
	Twilio  TwilioConfig `mapstructure:"twilio"`
	Plivo   PlivoConfig  `mapstructure:"plivo"`
	Vonage  VonageConfig `mapstructure:"vonage"`
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

type PushProviderConfig struct {
	FCM       FCMConfig       `mapstructure:"fcm"`
	APNs      APNsConfig      `mapstructure:"apns"`
	Pushwoosh PushwooshConfig `mapstructure:"pushwoosh"`
}

type FCMConfig struct {
	ServiceAccountJSON string `mapstructure:"service_account_json"`
}

type APNsConfig struct {
	KeyFile   string `mapstructure:"key_file"`
	KeyID     string `mapstructure:"key_id"`
	TeamID    string `mapstructure:"team_id"`
	BundleID  string `mapstructure:"bundle_id"`
	Sandbox   bool   `mapstructure:"sandbox"`
}

type PushwooshConfig struct {
	ApplicationCode string `mapstructure:"application_code"`
	APIAccessToken  string `mapstructure:"api_access_token"`
}

type WebhookProviderConfig struct {
	SigningSecret    string        `mapstructure:"signing_secret"`
	TimeoutSeconds  int           `mapstructure:"timeout_seconds"`
	MaxRetries      int           `mapstructure:"max_retries"`
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
	v.SetDefault("pubsub.events_topic", "notifications-ingress")
	v.SetDefault("pubsub.events_subscription", "ingress")

	v.SetDefault("cadence.mode", "temporal")
	v.SetDefault("cadence.host_port", "localhost:7233")
	v.SetDefault("cadence.domain", "default")

	v.SetDefault("jwt.expiry", "24h")

	v.SetDefault("providers.webhook.timeout_seconds", 30)
	v.SetDefault("providers.webhook.max_retries", 5)
	v.SetDefault("providers.slack.timeout_seconds", 30)

	v.SetDefault("providers.email_routing.mode", "backup")
	v.SetDefault("providers.sms_routing.mode", "backup")
	v.SetDefault("providers.push_routing.mode", "backup")

	v.SetDefault("log.level", "info")
	v.SetDefault("log.format", "json")

	v.SetDefault("security.rate_limit.enabled", true)
	v.SetDefault("security.rate_limit.rps", 100.0)
	v.SetDefault("security.rate_limit.burst", 50)
	v.SetDefault("security.ddos.block_threshold", 500)
	v.SetDefault("security.ddos.block_duration", "5m")
	v.SetDefault("security.headers.enable_secure_headers", true)
	v.SetDefault("security.headers.allowed_origins", []string{"*"})
	v.SetDefault("security.request.max_body_size_mb", 2)
}

func validate(cfg *Config) error {
	if cfg.Database.DSN == "" {
		return fmt.Errorf("database.dsn is required")
	}
	if cfg.JWT.Secret == "" && cfg.Server.Mode == "release" {
		return fmt.Errorf("jwt.secret is required in release mode")
	}
	return nil
}
