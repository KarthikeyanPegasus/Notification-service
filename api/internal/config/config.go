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
	SMS     SMSProviderConfig
	Push    PushProviderConfig
	Webhook WebhookProviderConfig
}

type EmailProviderConfig struct {
	// Primary provider: ses | mailgun | smtp
	Primary string            `mapstructure:"primary"`
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
	AccountSID string `mapstructure:"account_sid"`
	AuthToken  string `mapstructure:"auth_token"`
	FromNumber string `mapstructure:"from_number"`
}

type PlivoConfig struct {
	AuthID    string `mapstructure:"auth_id"`
	AuthToken string `mapstructure:"auth_token"`
	FromNumber string `mapstructure:"from_number"`
}

type VonageConfig struct {
	APIKey    string `mapstructure:"api_key"`
	APISecret string `mapstructure:"api_secret"`
	From      string `mapstructure:"from"`
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
	v.SetDefault("pubsub.events_subscription", "notif-service-ingress")

	v.SetDefault("cadence.mode", "temporal")
	v.SetDefault("cadence.host_port", "localhost:7233")
	v.SetDefault("cadence.domain", "default")

	v.SetDefault("jwt.expiry", "24h")

	v.SetDefault("providers.webhook.timeout_seconds", 30)
	v.SetDefault("providers.webhook.max_retries", 5)

	v.SetDefault("log.level", "info")
	v.SetDefault("log.format", "json")
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
