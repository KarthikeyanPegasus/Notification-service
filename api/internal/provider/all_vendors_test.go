package provider_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/spidey/notification-service/internal/config"
	"github.com/spidey/notification-service/internal/domain"
	"github.com/spidey/notification-service/internal/provider/email"
	"github.com/spidey/notification-service/internal/provider/push"
	"github.com/spidey/notification-service/internal/provider/sms"
	"golang.org/x/time/rate"
)

func TestVendorIntegration(t *testing.T) {
	if os.Getenv("RUN_INTEGRATION_TESTS") != "true" {
		t.Skip("Skipping integration tests. Set RUN_INTEGRATION_TESTS=true to run.")
	}

	ctx := context.Background()
	cfg, err := config.Load("../../config")
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Rate limiter to prevent burst limit issues (1 request every 2 seconds)
	limiter := rate.NewLimiter(rate.Every(2*time.Second), 1)

	testCases := []struct {
		name      string
		channel   domain.Channel
		recipient string
		getSender func() (interface{}, error)
	}{
		{
			name:      "Twilio SMS",
			channel:   domain.ChannelSMS,
			recipient: os.Getenv("TEST_SMS_RECIPIENT"),
			getSender: func() (interface{}, error) {
				if cfg.Providers.SMS.Twilio.AccountSID == "" {
					return nil, nil
				}
				return sms.NewTwilioSender(cfg.Providers.SMS.Twilio), nil
			},
		},
		{
			name:      "Vonage SMS",
			channel:   domain.ChannelSMS,
			recipient: os.Getenv("TEST_SMS_RECIPIENT"),
			getSender: func() (interface{}, error) {
				if cfg.Providers.SMS.Vonage.APIKey == "" {
					return nil, nil
				}
				return sms.NewVonageSender(cfg.Providers.SMS.Vonage), nil
			},
		},
		{
			name:      "Plivo SMS",
			channel:   domain.ChannelSMS,
			recipient: os.Getenv("TEST_SMS_RECIPIENT"),
			getSender: func() (interface{}, error) {
				if cfg.Providers.SMS.Plivo.AuthID == "" {
					return nil, nil
				}
				return sms.NewPlivoSender(cfg.Providers.SMS.Plivo), nil
			},
		},
		{
			name:      "Amazon SES Email",
			channel:   domain.ChannelEmail,
			recipient: os.Getenv("TEST_EMAIL_RECIPIENT"),
			getSender: func() (interface{}, error) {
				if cfg.Providers.Email.SES.AccessKeyID == "" {
					return nil, nil
				}
				return email.NewSESSender(ctx, cfg.Providers.Email.SES)
			},
		},
		{
			name:      "Mailgun Email",
			channel:   domain.ChannelEmail,
			recipient: os.Getenv("TEST_EMAIL_RECIPIENT"),
			getSender: func() (interface{}, error) {
				if cfg.Providers.Email.Mailgun.APIKey == "" {
					return nil, nil
				}
				return email.NewMailgunSender(cfg.Providers.Email.Mailgun), nil
			},
		},
		{
			name:      "SMTP Email (Mailhog)",
			channel:   domain.ChannelEmail,
			recipient: os.Getenv("TEST_EMAIL_RECIPIENT"),
			getSender: func() (interface{}, error) {
				if cfg.Providers.Email.SMTP.Host == "" {
					return nil, nil
				}
				return email.NewSMTPSender(cfg.Providers.Email.SMTP), nil
			},
		},
		{
			name:      "Firebase Push (FCM)",
			channel:   domain.ChannelPush,
			recipient: os.Getenv("TEST_FCM_TOKEN"),
			getSender: func() (interface{}, error) {
				if cfg.Providers.Push.FCM.ServiceAccountJSON == "" {
					return nil, nil
				}
				return push.NewFCMSender(cfg.Providers.Push.FCM.ServiceAccountJSON)
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Respect rate limits
			if err := limiter.Wait(ctx); err != nil {
				t.Fatalf("Rate limiter error: %v", err)
			}

			senderObj, err := tc.getSender()
			if err != nil {
				t.Fatalf("Failed to initialize sender: %v", err)
			}
			if senderObj == nil {
				t.Skip("Credentials not configured for this provider, skipping.")
			}

			if tc.recipient == "" {
				t.Skip("No test recipient provided for this channel, skipping.")
			}

			sender := senderObj.(interface {
				Send(context.Context, *domain.Notification) (domain.DeliveryResult, error)
				ProviderName() string
			})

			notif := &domain.Notification{
				ID:        uuid.New(),
				Recipient: tc.recipient,
				Channel:   tc.channel,
				RenderedContent: &domain.RenderedContent{
					Subject: "Integration Test",
					Body:    "This is a real-world integration test sent from NotifyHub at " + time.Now().Format(time.RFC3339),
					HTML:    "<h1>Integration Test</h1><p>This is a real-world integration test sent from NotifyHub.</p>",
				},
			}

			t.Logf("Testing %s sending to %s...", sender.ProviderName(), tc.recipient)
			result, err := sender.Send(ctx, notif)

			if err != nil {
				t.Errorf("%s.Send() error: %v", tc.name, err)
				return
			}

			if !result.Success {
				t.Errorf("%s.Send() failed: %s (error code: %s)", tc.name, result.ErrorMessage, result.ErrorCode)
				return
			}

			t.Logf("Successfully sent message via %s. Provider ID: %s, Latency: %dms", 
				result.Provider, result.ProviderMsgID, result.LatencyMs)
		})
	}
}
