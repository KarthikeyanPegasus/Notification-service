import { Badge } from '@/components/ui/badge'

function CodeBlock({ children, lang = 'go' }: { children: string; lang?: string }) {
  return (
    <pre className={`language-${lang} rounded-lg bg-muted p-4 text-sm overflow-x-auto leading-relaxed`}>
      <code>{children.trim()}</code>
    </pre>
  )
}

function Section({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <section className="space-y-3">
      <h2 className="text-xl font-semibold tracking-tight">{title}</h2>
      {children}
    </section>
  )
}

function MethodRow({ method, signature, description }: { method: string; signature: string; description: string }) {
  return (
    <div className="rounded-lg border bg-card p-4 space-y-1">
      <div className="flex items-start gap-3">
        <Badge variant="outline" className="mt-0.5 shrink-0 font-mono text-xs">{method}</Badge>
        <code className="text-sm font-mono text-foreground leading-snug">{signature}</code>
      </div>
      <p className="text-sm text-muted-foreground pl-16">{description}</p>
    </div>
  )
}

export default function GoSdkPage() {
  return (
    <div className="space-y-10 max-w-4xl">
      <div>
        <h1 className="text-3xl font-bold tracking-tight">SDK — Go</h1>
        <p className="text-muted-foreground mt-1">
          Idiomatic Go client for the NotifyHub API. Wraps all REST endpoints with typed models, automatic
          JSON serialization, and bearer / API-key auth.
        </p>
      </div>

      <Section title="Installation">
        <CodeBlock lang="bash">{`go get github.com/spidey/notification-service/sdk/go`}</CodeBlock>
        <p className="text-sm text-muted-foreground">Requires Go 1.21+. No external runtime dependencies.</p>
      </Section>

      <Section title="Initializing the client">
        <p className="text-sm text-muted-foreground">
          Create a client with <code className="text-xs">notification.New()</code> and pass options to configure
          the base URL and authentication.
        </p>
        <CodeBlock>{`import notification "github.com/spidey/notification-service/sdk/go"

// API key auth (recommended for server-to-server)
client := notification.New(
    notification.WithBaseURL("https://api.yourhost.com/v1"),
    notification.WithAPIKey("<api-key>"),
)

// JWT bearer token
client := notification.New(
    notification.WithBaseURL("https://api.yourhost.com/v1"),
    notification.WithBearerToken("<jwt-token>"),
)

// Service token (required for OTP endpoints)
client := notification.New(
    notification.WithServiceToken("<service-token>"),
)`}
        </CodeBlock>
      </Section>

      <Section title="Sending notifications">
        <p className="text-sm text-muted-foreground mb-2">
          Use the per-channel helpers for the common case. All helpers accept an optional{' '}
          <code className="text-xs">*NotifyOptions</code> for subject, body, template, and scheduling.
        </p>
        <CodeBlock>{`ctx := context.Background()

// Email
resp, err := client.Notifications.NotifyByEmail(ctx,
    "user-uuid",          // userID
    "order-confirm-001",  // idempotency key
    "transactional",      // notification type
    "alice@example.com",  // recipient
    &notification.NotifyOptions{
        Subject: "Your order is confirmed",
        Body:    "Hi Alice, order #1234 is confirmed.",
    },
)

// SMS
resp, err = client.Notifications.NotifyBySMS(ctx,
    "user-uuid", "sms-verify-001", "otp", "+15555550100",
    &notification.NotifyOptions{Body: "Your code is 482910"},
)

// Push (FCM device token or user ID depending on provider)
resp, err = client.Notifications.NotifyByPush(ctx,
    "user-uuid", "push-promo-001", "promotional", "<device-token>",
    &notification.NotifyOptions{
        Subject: "Flash sale — 50% off",
        Body:    "Shop now before midnight.",
    },
)

// Slack
resp, err = client.Notifications.NotifyBySlack(ctx,
    "user-uuid", "slack-alert-001", "transactional",
    "https://hooks.slack.com/services/XXX/YYY/ZZZ",
    &notification.NotifyOptions{Body: ":rocket: Deploy succeeded"},
)

// Webhook callback
resp, err = client.Notifications.NotifyByWebhook(ctx,
    "user-uuid", "webhook-evt-001", "transactional",
    "https://partner.example.com/events",
    &notification.NotifyOptions{Body: \`{"event":"payment.completed"}\`},
)

fmt.Println(resp.NotificationID, resp.Status)`}
        </CodeBlock>

        <p className="text-sm text-muted-foreground mt-4 mb-2">
          Use <code className="text-xs">Send()</code> directly when targeting multiple channels in one call.
        </p>
        <CodeBlock>{`resp, err := client.Notifications.Send(ctx, &notification.SendRequest{
    IdempotencyKey: "multi-ch-001",
    UserID:         "user-uuid",
    Channels:       []notification.Channel{notification.ChannelEmail, notification.ChannelSMS},
    Type:           "transactional",
    Recipient:      "alice@example.com",
    Body:           "Your account has been verified.",
})`}
        </CodeBlock>
      </Section>

      <Section title="Scheduled delivery">
        <CodeBlock>{`scheduledAt := time.Now().Add(2 * time.Hour)

resp, err := client.Notifications.NotifyByEmail(ctx,
    "user-uuid", "reminder-001", "promotional", "bob@example.com",
    &notification.NotifyOptions{
        Subject:     "Don't forget your trial ends soon",
        Body:        "Your free trial expires in 24 hours.",
        ScheduledAt: &scheduledAt,
    },
)`}
        </CodeBlock>
      </Section>

      <Section title="Templates">
        <CodeBlock>{`resp, err := client.Notifications.Send(ctx, &notification.SendRequest{
    IdempotencyKey: "tmpl-order-001",
    UserID:         "user-uuid",
    Channels:       []notification.Channel{notification.ChannelEmail},
    Type:           "transactional",
    Recipient:      "alice@example.com",
    TemplateID:     "template-uuid",
    TemplateVariables: map[string]string{
        "order_id":       "1234",
        "customer_name":  "Alice",
        "delivery_date":  "May 3",
    },
})`}
        </CodeBlock>
      </Section>

      <Section title="Listing &amp; retrieving notifications">
        <CodeBlock>{`// List with filters
list, err := client.Notifications.List(ctx, &notification.ListNotificationsParams{
    Page:     1,
    PageSize: 25,
    UserID:   "user-uuid",
    Channel:  notification.ChannelEmail,
    Status:   notification.StatusSent,
})
fmt.Printf("total=%d results=%d\\n", list.Total, len(list.Data))

// Get details (notification + attempt history + events)
detail, err := client.Notifications.Get(ctx, "notification-uuid")`}
        </CodeBlock>
      </Section>

      <Section title="Governance — suppressions &amp; opt-outs">
        <CodeBlock>{`// List suppressions
suppressions, err := client.Governance.ListSuppressions(ctx)

// Suppress an email address (e.g. on hard bounce)
s, err := client.Governance.AddSuppression(ctx, &notification.AddSuppressionRequest{
    Recipient: "bounced@example.com",
    Channel:   notification.ChannelEmail,
    Reason:    "hard_bounce",
})`}
        </CodeBlock>
      </Section>

      <Section title="OTP">
        <p className="text-sm text-muted-foreground mb-2">
          OTP endpoints require a <code className="text-xs">WithServiceToken</code> client.
        </p>
        <CodeBlock>{`// Send OTP
otpResp, err := client.OTP.Send(ctx, &notification.OTPSendRequest{
    UserID:        "user-uuid",
    PhoneNumber:   "+15555550100",
    Purpose:       "login",
    ExpirySeconds: 300,
})

// Verify OTP
verifyResp, err := client.OTP.Verify(ctx, &notification.OTPVerifyRequest{
    UserID:  "user-uuid",
    Purpose: "login",
    OTP:     "482910",
})
fmt.Println("verified:", verifyResp.Verified)`}
        </CodeBlock>
      </Section>

      <Section title="Pub/Sub ingress">
        <p className="text-sm text-muted-foreground mb-2">
          Publish a <code className="text-xs">SendRequest</code> directly to the GCP Pub/Sub ingress topic
          when bypassing HTTP is preferred.
        </p>
        <CodeBlock>{`pub, err := notification.NewIngressPublisher(ctx, "<gcp-project-id>",
    notification.WithIngressTopic("notifications-ingress"),
    notification.WithCredentialsJSONRaw([]byte(\`{...service account json...}\`)),
)
if err != nil {
    log.Fatal(err)
}
defer pub.Close()

_, err = pub.Publish(ctx, &notification.SendRequest{
    IdempotencyKey: "ingress-evt-001",
    UserID:         "user-uuid",
    Channels:       []notification.Channel{notification.ChannelSMS},
    Type:           "transactional",
    Recipient:      "+15555550123",
    Body:           "Order shipped!",
})`}
        </CodeBlock>
      </Section>

      <Section title="API surface">
        <div className="space-y-2">
          <MethodRow method="Notifications.Send" signature="Send(ctx, *SendRequest) (*SendResponse, error)" description="Send to one or more channels in a single request." />
          <MethodRow method="Notifications.NotifyByEmail" signature="NotifyByEmail(ctx, userID, key, type, recipient, *NotifyOptions) (*SendResponse, error)" description="Send an email notification." />
          <MethodRow method="Notifications.NotifyBySMS" signature="NotifyBySMS(ctx, userID, key, type, recipient, *NotifyOptions) (*SendResponse, error)" description="Send an SMS notification." />
          <MethodRow method="Notifications.NotifyByPush" signature="NotifyByPush(ctx, userID, key, type, recipient, *NotifyOptions) (*SendResponse, error)" description="Send a push notification." />
          <MethodRow method="Notifications.NotifyBySlack" signature="NotifyBySlack(ctx, userID, key, type, recipient, *NotifyOptions) (*SendResponse, error)" description="Send a Slack Incoming Webhook message." />
          <MethodRow method="Notifications.NotifyByWebhook" signature="NotifyByWebhook(ctx, userID, key, type, recipient, *NotifyOptions) (*SendResponse, error)" description="Dispatch a webhook callback to the recipient URL." />
          <MethodRow method="Notifications.NotifyByWebSocket" signature="NotifyByWebSocket(ctx, userID, key, type, recipient, *NotifyOptions) (*SendResponse, error)" description="Push a real-time WebSocket notification." />
          <MethodRow method="Notifications.List" signature="List(ctx, *ListNotificationsParams) (*ListNotificationsResponse, error)" description="Paginated list with user_id, channel, and status filters." />
          <MethodRow method="Notifications.Get" signature="Get(ctx, id) (*NotificationDetailResponse, error)" description="Fetch notification, attempt history, and lifecycle events by ID." />
          <MethodRow method="OTP.Send" signature="Send(ctx, *OTPSendRequest) (*OTPSendResponse, error)" description="Generate and deliver a one-time password via SMS." />
          <MethodRow method="OTP.Verify" signature="Verify(ctx, *OTPVerifyRequest) (*OTPVerifyResponse, error)" description="Verify a previously issued OTP." />
          <MethodRow method="Reports.Summary" signature="Summary(ctx) ([]*ReportSummaryItem, error)" description="Delivery success rates and latency percentiles per channel." />
          <MethodRow method="Governance.ListSuppressions" signature="ListSuppressions(ctx) ([]*Suppression, error)" description="List identifier-level suppression rules." />
          <MethodRow method="Governance.AddSuppression" signature="AddSuppression(ctx, *AddSuppressionRequest) (*Suppression, error)" description="Block an identifier from receiving notifications." />
        </div>
      </Section>
    </div>
  )
}
