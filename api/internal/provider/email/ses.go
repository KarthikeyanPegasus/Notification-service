package email

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/mail"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/sesv2"
	"github.com/aws/aws-sdk-go-v2/service/sesv2/types"
	"github.com/google/uuid"
	"github.com/spidey/notification-service/internal/config"
	"github.com/spidey/notification-service/internal/domain"
	gomail "gopkg.in/gomail.v2"
)

// SESSender sends email via Amazon SES v2.
type SESSender struct {
	client   *sesv2.Client
	from     string
	fromName string
	replyTo  string
	mode     string // "api" | "smtp"
	smtpHost string
	smtpUser string
	smtpPass string
}

func NewSESSender(ctx context.Context, cfg config.SESConfig) (*SESSender, error) {
	if cfg.Region == "" {
		return nil, nil
	}
	// Back-compat: allow "access_secret" to stand in for "secret_access_key".
	if cfg.SecretAccessKey == "" && cfg.AccessSecret != "" {
		cfg.SecretAccessKey = cfg.AccessSecret
	}

	resolver := aws.EndpointResolverWithOptionsFunc(
		func(service, region string, options ...interface{}) (aws.Endpoint, error) {
			return aws.Endpoint{}, &aws.EndpointNotFoundError{}
		},
	)

	opts := []func(*awsconfig.LoadOptions) error{
		awsconfig.WithRegion(cfg.Region),
		awsconfig.WithEndpointResolverWithOptions(resolver),
	}

	from := cfg.FromAddress
	if cfg.FromName != "" {
		from = fmt.Sprintf("%s <%s>", cfg.FromName, cfg.FromAddress)
	}

	accessKey := strings.TrimSpace(cfg.AccessKeyID)
	secretKey := strings.TrimSpace(cfg.SecretAccessKey)
	if secretKey == "" {
		secretKey = strings.TrimSpace(cfg.AccessSecret)
	}

	smtpUser := strings.TrimSpace(cfg.SMTPUsername)
	smtpPass := strings.TrimSpace(cfg.SMTPPassword)

	// If API credentials are provided, use SES API mode.
	// Otherwise, if SMTP credentials are provided, use SES SMTP mode.
	if accessKey != "" && secretKey != "" {
		opts = append(opts, awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(
			accessKey, secretKey, "",
		)))
	} else if smtpUser != "" && smtpPass != "" {
		return &SESSender{
			client:   nil,
			from:     from,
			fromName: cfg.FromName,
			replyTo:  cfg.ReplyTo,
			mode:     "smtp",
			smtpHost: fmt.Sprintf("email-smtp.%s.amazonaws.com", cfg.Region),
			smtpUser: smtpUser,
			smtpPass: smtpPass,
		}, nil
	}

	// If no explicit credentials are provided, the AWS SDK will try multiple sources including EC2 IMDS.
	// In local/dev environments (like laptops), IMDS calls can hang/time out and make test deliveries appear stuck.
	// Use anonymous credentials to fail fast with a clear auth error, instead of waiting on IMDS.
	if cfg.AccessKeyID == "" || cfg.SecretAccessKey == "" {
		opts = append(opts, awsconfig.WithCredentialsProvider(aws.AnonymousCredentials{}))
	}

	awsCfg, err := awsconfig.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("loading aws config: %w", err)
	}

	return &SESSender{
		client:   sesv2.NewFromConfig(awsCfg),
		from:     from,
		fromName: cfg.FromName,
		replyTo:  cfg.ReplyTo,
		mode:     "api",
	}, nil
}

func (s *SESSender) ProviderName() string { return "amazon-ses" }

func (s *SESSender) ValidateEmail(email string) error { return validateEmailFormat(email) }

func (s *SESSender) Send(ctx context.Context, n *domain.Notification) (domain.DeliveryResult, error) {
	start := time.Now()

	if n.RenderedContent == nil {
		return domain.DeliveryResult{}, fmt.Errorf("SES: rendered content is nil for notification %s", n.ID)
	}

	if s.mode == "smtp" {
		from := strings.TrimSpace(s.from)
		if from == "" {
			return domain.DeliveryResult{}, fmt.Errorf("SES SMTP: from address is empty")
		}
		if _, err := mail.ParseAddress(from); err != nil {
			return domain.DeliveryResult{}, fmt.Errorf("SES SMTP: invalid from address %q: %w", from, err)
		}

		to := strings.TrimSpace(n.Recipient)
		if to == "" {
			return domain.DeliveryResult{}, fmt.Errorf("SES SMTP: recipient address is empty")
		}
		if _, err := mail.ParseAddress(to); err != nil {
			return domain.DeliveryResult{}, fmt.Errorf("SES SMTP: invalid recipient address %q: %w", to, err)
		}

		m := gomail.NewMessage()
		m.SetHeader("From", from)
		m.SetHeader("To", to)
		m.SetHeader("Subject", n.RenderedContent.Subject)
		if s.replyTo != "" {
			m.SetHeader("Reply-To", s.replyTo)
		}
		m.SetHeader("Message-ID", fmt.Sprintf("<%s@notification-service>", uuid.New().String()))

		if n.RenderedContent.HTML != "" {
			m.SetBody("text/html", n.RenderedContent.HTML)
			m.AddAlternative("text/plain", n.RenderedContent.Body)
		} else {
			m.SetBody("text/plain", n.RenderedContent.Body)
		}

		d := gomail.NewDialer(s.smtpHost, 587, s.smtpUser, s.smtpPass)
		d.TLSConfig = &tls.Config{ServerName: s.smtpHost, MinVersion: tls.VersionTLS12}

		if err := d.DialAndSend(m); err != nil {
			latencyMs := int(time.Since(start).Milliseconds())
			return domain.DeliveryResult{
				Provider:     s.ProviderName(),
				LatencyMs:    latencyMs,
				ErrorMessage: err.Error(),
			}, err
		}

		return domain.DeliveryResult{
			Success:       true,
			ProviderMsgID: uuid.New().String(),
			Provider:      s.ProviderName(),
			LatencyMs:     int(time.Since(start).Milliseconds()),
		}, nil
	}

	input := &sesv2.SendEmailInput{
		FromEmailAddress: aws.String(s.from),
		Destination: &types.Destination{
			ToAddresses: []string{n.Recipient},
		},
		// Embed notification ID as an SES message tag; SNS event payloads include mail.tags
		EmailTags: []types.MessageTag{
			{Name: aws.String("notification-id"), Value: aws.String(n.ID.String())},
		},
		Content: &types.EmailContent{
			Simple: &types.Message{
				Subject: &types.Content{
					Data:    aws.String(n.RenderedContent.Subject),
					Charset: aws.String("UTF-8"),
				},
				Body: &types.Body{
					Html: &types.Content{
						Data:    aws.String(n.RenderedContent.HTML),
						Charset: aws.String("UTF-8"),
					},
					Text: &types.Content{
						Data:    aws.String(n.RenderedContent.Body),
						Charset: aws.String("UTF-8"),
					},
				},
			},
		},
	}

	if s.replyTo != "" {
		input.ReplyToAddresses = []string{s.replyTo}
	}

	output, err := s.client.SendEmail(ctx, input)
	latencyMs := int(time.Since(start).Milliseconds())
	if err != nil {
		return domain.DeliveryResult{
			Provider:     s.ProviderName(),
			LatencyMs:    latencyMs,
			ErrorMessage: err.Error(),
		}, err
	}

	msgID := ""
	if output.MessageId != nil {
		msgID = *output.MessageId
	}

	return domain.DeliveryResult{
		Success:       true,
		ProviderMsgID: msgID,
		Provider:      s.ProviderName(),
		LatencyMs:     latencyMs,
	}, nil
}

func (s *SESSender) GetStatus(ctx context.Context, providerMsgID string) (domain.DeliveryResult, error) {
	return domain.DeliveryResult{
		Provider:      s.ProviderName(),
		ProviderMsgID: providerMsgID,
		ErrorMessage:  "status polling not supported for ses (use webhooks)",
	}, nil
}
