package provider

import (
	"context"
	"strings"

	"github.com/spidey/notification-service/internal/config"
	"github.com/spidey/notification-service/internal/provider/email"
	"github.com/spidey/notification-service/internal/provider/push"
	"github.com/spidey/notification-service/internal/provider/slack"
	"github.com/spidey/notification-service/internal/provider/sms"
	"github.com/spidey/notification-service/internal/provider/webhook"
)

// InitializeEmailSenders creates a list of enabled email senders based on config.
func InitializeEmailSenders(ctx context.Context, cfg config.EmailProviderConfig) []Sender {
	var senders []Sender

	// Helper to pick from address: provider-specific > default_from
	pickFrom := func(providerFrom string) string {
		trimmed := strings.TrimSpace(providerFrom)
		if trimmed != "" {
			return trimmed
		}
		return strings.TrimSpace(cfg.DefaultFrom)
	}

	// Add SES if configured
	sesCfg := cfg.SES
	if strings.TrimSpace(sesCfg.FromAddress) == "" {
		sesCfg.FromAddress = strings.TrimSpace(cfg.DefaultFrom)
	}
	if ses, err := email.NewSESSender(ctx, sesCfg); err == nil && ses != nil {
		senders = append(senders, ses)
	}

	// Add Mailgun
	mgCfg := cfg.Mailgun
	mgCfg.From = pickFrom(mgCfg.From)
	if mailgun := email.NewMailgunSender(mgCfg); mailgun != nil {
		senders = append(senders, mailgun)
	}

	// Add SendGrid
	sgCfg := cfg.SendGrid
	sgCfg.FromEmail = pickFrom(sgCfg.FromEmail)
	if sendgrid := email.NewSendGridSender(sgCfg); sendgrid != nil {
		senders = append(senders, sendgrid)
	}

	// Add Postmark
	pmCfg := cfg.Postmark
	pmCfg.FromEmail = pickFrom(pmCfg.FromEmail)
	if postmark := email.NewPostmarkSender(pmCfg); postmark != nil {
		senders = append(senders, postmark)
	}

	// Add SMTP
	smtpCfg := cfg.SMTP
	smtpCfg.From = pickFrom(smtpCfg.From)
	if smtp := email.NewSMTPSender(smtpCfg); smtp != nil {
		senders = append(senders, smtp)
	}

	return senders
}

// InitializeSMSSenders creates a list of enabled SMS senders based on config.
func InitializeSMSSenders(cfg config.SMSProviderConfig) []Sender {
	var senders []Sender

	if twilio := sms.NewTwilioSender(cfg.Twilio); twilio != nil {
		senders = append(senders, twilio)
	}

	if plivo := sms.NewPlivoSender(cfg.Plivo); plivo != nil {
		senders = append(senders, plivo)
	}

	if vonage := sms.NewVonageSender(cfg.Vonage); vonage != nil {
		senders = append(senders, vonage)
	}

	if mb := sms.NewMessageBirdSender(cfg.MessageBird); mb != nil {
		senders = append(senders, mb)
	}

	return senders
}

// InitializePushSenders creates a list of enabled push senders based on config.
func InitializePushSenders(cfg config.PushProviderConfig) []Sender {
	var senders []Sender

	if fcm, err := push.NewFCMSender(cfg.FCM.ServiceAccountJSON); err == nil && fcm != nil {
		senders = append(senders, fcm)
	}

	if os := push.NewOneSignalSender(cfg.OneSignal); os != nil {
		senders = append(senders, os)
	}

	if pb := push.NewPusherBeamsSender(cfg.Pusher); pb != nil {
		senders = append(senders, pb)
	}

	// Add APNs/Pushwoosh once implemented
	return senders
}

// InitializeWebhookDispatcher creates a webhook dispatcher based on config.
func InitializeWebhookDispatcher(cfg config.WebhookProviderConfig) *webhook.Dispatcher {
	return webhook.NewDispatcher(cfg)
}

// InitializeWebhookSenders returns webhook-channel senders (custom webhooks + social webhooks).
func InitializeWebhookSenders(cfg config.ProviderConfig) []Sender {
	var senders []Sender

	// Generic “Custom Webhooks” sender (recipient is the URL).
	if d := webhook.NewDispatcher(cfg.Webhook); d != nil {
		senders = append(senders, d)
	}

	if discord := webhook.NewDiscordSender(cfg.Discord); discord != nil {
		senders = append(senders, discord)
	}
	if teams := webhook.NewTeamsSender(cfg.Teams); teams != nil {
		senders = append(senders, teams)
	}
	if telegram := webhook.NewTelegramSender(cfg.Telegram); telegram != nil {
		senders = append(senders, telegram)
	}

	return senders
}

// InitializeSlackSender creates a Slack Incoming Webhook sender.
func InitializeSlackSender(cfg config.SlackProviderConfig) *slack.Sender {
	return slack.NewSender(cfg)
}
