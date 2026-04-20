package provider

import (
	"context"

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

	// Add SES if configured
	if ses, err := email.NewSESSender(ctx, cfg.SES); err == nil && ses != nil {
		senders = append(senders, ses)
	}

	// Add Mailgun
	if mailgun := email.NewMailgunSender(cfg.Mailgun); mailgun != nil {
		senders = append(senders, mailgun)
	}

	// Add SMTP
	if smtp := email.NewSMTPSender(cfg.SMTP); smtp != nil {
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

	return senders
}

// InitializePushSenders creates a list of enabled push senders based on config.
func InitializePushSenders(cfg config.PushProviderConfig) []Sender {
	var senders []Sender

	if fcm, err := push.NewFCMSender(cfg.FCM.ServiceAccountJSON); err == nil && fcm != nil {
		senders = append(senders, fcm)
	}

	// Add APNs/Pushwoosh once implemented
	return senders
}

// InitializeWebhookDispatcher creates a webhook dispatcher based on config.
func InitializeWebhookDispatcher(cfg config.WebhookProviderConfig) *webhook.Dispatcher {
	return webhook.NewDispatcher(cfg)
}

// InitializeSlackSender creates a Slack Incoming Webhook sender.
func InitializeSlackSender(cfg config.SlackProviderConfig) *slack.Sender {
	return slack.NewSender(cfg)
}
