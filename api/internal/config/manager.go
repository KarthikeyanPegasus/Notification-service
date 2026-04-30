package config

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/spidey/notification-service/internal/domain"
)

// Repository is a subset of VendorConfigRepository to avoid circular dependency.
type Repository interface {
	ListActive(ctx context.Context, apiKeyID *string) ([]*domain.VendorConfig, error)
	// ListPreferredActive returns one active config per vendor_type, preferring global
	// (api_key_id IS NULL) over client-scoped configs.
	ListPreferredActive(ctx context.Context) ([]*domain.VendorConfig, error)
}

// LoadDynamicOverrides fetches configurations from the DB and applies them to the base config.
func (cfg *Config) LoadDynamicOverrides(ctx context.Context, repo Repository) error {
	return cfg.LoadDynamicOverridesScoped(ctx, repo, nil)
}

// LoadDynamicOverridesScoped fetches configurations from the DB and applies them to the base config.
// If apiKeyID is provided, it loads the client-scoped configs for that API key.
func (cfg *Config) LoadDynamicOverridesScoped(ctx context.Context, repo Repository, apiKeyID *string) error {
	overrides, err := repo.ListActive(ctx, apiKeyID)
	if err != nil {
		return fmt.Errorf("listing overrides: %w", err)
	}

	for _, o := range overrides {
		switch o.VendorType {
		case "sms":
			var smsCfg SMSProviderConfig
			if err := json.Unmarshal(o.ConfigJSON, &smsCfg); err == nil {
				cfg.Providers.SMS = smsCfg
			}
		case "twilio":
			var s TwilioConfig
			if err := json.Unmarshal(o.ConfigJSON, &s); err == nil {
				cfg.Providers.SMS.Twilio = s
			}
		case "plivo":
			var s PlivoConfig
			if err := json.Unmarshal(o.ConfigJSON, &s); err == nil {
				cfg.Providers.SMS.Plivo = s
			}
		case "vonage":
			var s VonageConfig
			if err := json.Unmarshal(o.ConfigJSON, &s); err == nil {
				cfg.Providers.SMS.Vonage = s
			}
		case "messagebird":
			var s MessageBirdConfig
			if err := json.Unmarshal(o.ConfigJSON, &s); err == nil {
				cfg.Providers.SMS.MessageBird = s
			}

		case "email":
			var emailCfg EmailProviderConfig
			if err := json.Unmarshal(o.ConfigJSON, &emailCfg); err == nil {
				cfg.Providers.Email = emailCfg
			}
		case "email_routing":
			var r RoutingConfig
			if err := json.Unmarshal(o.ConfigJSON, &r); err == nil {
				cfg.Providers.EmailRouting = r
			}
		case "sms_routing":
			var r RoutingConfig
			if err := json.Unmarshal(o.ConfigJSON, &r); err == nil {
				cfg.Providers.SMSRouting = r
			}
		case "ses":
			var s SESConfig
			if err := json.Unmarshal(o.ConfigJSON, &s); err == nil {
				cfg.Providers.Email.SES = s
			}
		case "mailgun":
			var s MailgunConfig
			if err := json.Unmarshal(o.ConfigJSON, &s); err == nil {
				cfg.Providers.Email.Mailgun = s
			}
		case "sendgrid":
			var s SendGridConfig
			if err := json.Unmarshal(o.ConfigJSON, &s); err == nil {
				cfg.Providers.Email.SendGrid = s
			}
		case "postmark":
			var s PostmarkConfig
			if err := json.Unmarshal(o.ConfigJSON, &s); err == nil {
				cfg.Providers.Email.Postmark = s
			}

		case "push":
			var pushCfg PushProviderConfig
			if err := json.Unmarshal(o.ConfigJSON, &pushCfg); err == nil {
				cfg.Providers.Push = pushCfg
			}
		case "push_routing":
			var r RoutingConfig
			if err := json.Unmarshal(o.ConfigJSON, &r); err == nil {
				cfg.Providers.PushRouting = r
			}
		case "fcm":
			var s FCMConfig
			if err := json.Unmarshal(o.ConfigJSON, &s); err == nil {
				cfg.Providers.Push.FCM = s
			}
		case "onesignal":
			var s OneSignalConfig
			if err := json.Unmarshal(o.ConfigJSON, &s); err == nil {
				cfg.Providers.Push.OneSignal = s
			}
		case "pusher":
			var s PusherConfig
			if err := json.Unmarshal(o.ConfigJSON, &s); err == nil {
				cfg.Providers.Push.Pusher = s
			}

		case "webhook":
			var webhookCfg WebhookProviderConfig
			if err := json.Unmarshal(o.ConfigJSON, &webhookCfg); err == nil {
				cfg.Providers.Webhook = webhookCfg
			}
		case "webhooks":
			// UI vendor id is "webhooks" (Custom Webhooks). Map to providers.webhook.
			var webhookCfg WebhookProviderConfig
			if err := json.Unmarshal(o.ConfigJSON, &webhookCfg); err == nil {
				cfg.Providers.Webhook = webhookCfg
			}
		case "webhook_routing":
			var r RoutingConfig
			if err := json.Unmarshal(o.ConfigJSON, &r); err == nil {
				cfg.Providers.WebhookRouting = r
			}
		case "slack":
			var slackCfg SlackProviderConfig
			if err := json.Unmarshal(o.ConfigJSON, &slackCfg); err == nil {
				cfg.Providers.Slack = slackCfg
			}
		case "discord":
			var d DiscordConfig
			if err := json.Unmarshal(o.ConfigJSON, &d); err == nil {
				cfg.Providers.Discord = d
			}
		case "teams":
			var t TeamsConfig
			if err := json.Unmarshal(o.ConfigJSON, &t); err == nil {
				cfg.Providers.Teams = t
			}
		case "telegram":
			var tg TelegramConfig
			if err := json.Unmarshal(o.ConfigJSON, &tg); err == nil {
				cfg.Providers.Telegram = tg
			}
		}
	}
	return nil
}

// LoadPreferredDynamicOverrides applies one active config per vendor_type to cfg,
// preferring global configs over client-scoped ones. Use this for admin-level
// endpoints (billing, vendor connectivity) that must show all configured vendors
// regardless of which client scope they were saved under.
func (cfg *Config) LoadPreferredDynamicOverrides(ctx context.Context, repo Repository) error {
	overrides, err := repo.ListPreferredActive(ctx)
	if err != nil {
		return fmt.Errorf("listing preferred overrides: %w", err)
	}
	// Re-use the same switch logic by constructing a VendorConfig slice and calling
	// the existing scoped loader with a nil apiKeyID stand-in.
	// We apply each override directly here to avoid the api_key_id filter in
	// LoadDynamicOverridesScoped.
	for _, o := range overrides {
		switch o.VendorType {
		case "sms":
			var s SMSProviderConfig
			if err := json.Unmarshal(o.ConfigJSON, &s); err == nil {
				cfg.Providers.SMS = s
			}
		case "twilio":
			var s TwilioConfig
			if err := json.Unmarshal(o.ConfigJSON, &s); err == nil {
				cfg.Providers.SMS.Twilio = s
			}
		case "plivo":
			var s PlivoConfig
			if err := json.Unmarshal(o.ConfigJSON, &s); err == nil {
				cfg.Providers.SMS.Plivo = s
			}
		case "vonage":
			var s VonageConfig
			if err := json.Unmarshal(o.ConfigJSON, &s); err == nil {
				cfg.Providers.SMS.Vonage = s
			}
		case "messagebird":
			var s MessageBirdConfig
			if err := json.Unmarshal(o.ConfigJSON, &s); err == nil {
				cfg.Providers.SMS.MessageBird = s
			}
		case "email":
			var emailCfg EmailProviderConfig
			if err := json.Unmarshal(o.ConfigJSON, &emailCfg); err == nil {
				cfg.Providers.Email = emailCfg
			}
		case "ses":
			var s SESConfig
			if err := json.Unmarshal(o.ConfigJSON, &s); err == nil {
				cfg.Providers.Email.SES = s
			}
		case "mailgun":
			var s MailgunConfig
			if err := json.Unmarshal(o.ConfigJSON, &s); err == nil {
				cfg.Providers.Email.Mailgun = s
			}
		case "sendgrid":
			var s SendGridConfig
			if err := json.Unmarshal(o.ConfigJSON, &s); err == nil {
				cfg.Providers.Email.SendGrid = s
			}
		case "postmark":
			var s PostmarkConfig
			if err := json.Unmarshal(o.ConfigJSON, &s); err == nil {
				cfg.Providers.Email.Postmark = s
			}
		case "push":
			var pushCfg PushProviderConfig
			if err := json.Unmarshal(o.ConfigJSON, &pushCfg); err == nil {
				cfg.Providers.Push = pushCfg
			}
		case "fcm":
			var s FCMConfig
			if err := json.Unmarshal(o.ConfigJSON, &s); err == nil {
				cfg.Providers.Push.FCM = s
			}
		case "onesignal":
			var s OneSignalConfig
			if err := json.Unmarshal(o.ConfigJSON, &s); err == nil {
				cfg.Providers.Push.OneSignal = s
			}
		case "pusher":
			var s PusherConfig
			if err := json.Unmarshal(o.ConfigJSON, &s); err == nil {
				cfg.Providers.Push.Pusher = s
			}
		case "slack":
			var s SlackProviderConfig
			if err := json.Unmarshal(o.ConfigJSON, &s); err == nil {
				cfg.Providers.Slack = s
			}
		case "discord":
			var d DiscordConfig
			if err := json.Unmarshal(o.ConfigJSON, &d); err == nil {
				cfg.Providers.Discord = d
			}
		case "teams":
			var t TeamsConfig
			if err := json.Unmarshal(o.ConfigJSON, &t); err == nil {
				cfg.Providers.Teams = t
			}
		case "telegram":
			var tg TelegramConfig
			if err := json.Unmarshal(o.ConfigJSON, &tg); err == nil {
				cfg.Providers.Telegram = tg
			}
		}
	}
	return nil
}
