package config

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/spidey/notification-service/internal/domain"
)

// Repository is a subset of VendorConfigRepository to avoid circular dependency.
type Repository interface {
	ListActive(ctx context.Context) ([]*domain.VendorConfig, error)
}

// LoadDynamicOverrides fetches configurations from the DB and applies them to the base config.
func (cfg *Config) LoadDynamicOverrides(ctx context.Context, repo Repository) error {
	overrides, err := repo.ListActive(ctx)
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

		case "webhook":
			var webhookCfg WebhookProviderConfig
			if err := json.Unmarshal(o.ConfigJSON, &webhookCfg); err == nil {
				cfg.Providers.Webhook = webhookCfg
			}
		}
	}
	return nil
}
