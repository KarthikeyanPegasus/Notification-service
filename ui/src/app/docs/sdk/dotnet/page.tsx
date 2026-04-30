import { Badge } from '@/components/ui/badge'

function CodeBlock({ children, lang = 'csharp' }: { children: string; lang?: string }) {
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

export default function DotnetSdkPage() {
  return (
    <div className="space-y-10 max-w-4xl">
      <div>
        <h1 className="text-3xl font-bold tracking-tight">SDK — .NET</h1>
        <p className="text-muted-foreground mt-1">
          Async .NET client for the NotifyHub API. Targets .NET 6+ with{' '}
          <code className="text-xs">System.Text.Json</code> serialization, cancellation token support, and
          bearer / API-key auth.
        </p>
      </div>

      <Section title="Installation">
        <CodeBlock lang="bash">{`dotnet add package NotificationService.Sdk`}</CodeBlock>
        <p className="text-sm text-muted-foreground">
          Or reference the project directly from{' '}
          <code className="text-xs">sdk/dotnet/NotificationService.Sdk.csproj</code>.
          Targets .NET 6+. No external runtime dependencies.
        </p>
      </Section>

      <Section title="Initializing the client">
        <p className="text-sm text-muted-foreground">
          Create a <code className="text-xs">NotificationClient</code> and configure authentication before
          making requests. The client is designed to be registered as a singleton in DI.
        </p>
        <CodeBlock>{`using NotificationService.Sdk;

// API key auth (recommended for server-to-server)
var client = new NotificationClient("https://api.yourhost.com/v1");
client.UseApiKey("<api-key>");

// JWT bearer token
var client = new NotificationClient("https://api.yourhost.com/v1");
client.UseBearerToken("<jwt-token>");

// Service token (required for OTP endpoints)
var client = new NotificationClient("https://api.yourhost.com/v1");
client.UseServiceToken("<service-token>");

// With a custom HttpClient (e.g. from IHttpClientFactory)
var client = new NotificationClient("https://api.yourhost.com/v1", httpClient);`}
        </CodeBlock>

        <p className="text-sm text-muted-foreground mt-2">ASP.NET Core DI registration:</p>
        <CodeBlock>{`builder.Services.AddSingleton(sp => {
    var client = new NotificationClient("https://api.yourhost.com/v1");
    client.UseApiKey(builder.Configuration["NotifyHub:ApiKey"]!);
    return client;
});`}
        </CodeBlock>
      </Section>

      <Section title="Sending notifications">
        <p className="text-sm text-muted-foreground mb-2">
          Use the per-channel async helpers for the common case. All helpers accept an optional{' '}
          <code className="text-xs">NotifyOptions</code> for subject, body, template, and scheduling.
        </p>
        <CodeBlock>{`// Email
var resp = await client.Notifications.NotifyByEmailAsync(
    userID:         "user-uuid",
    idempotencyKey: "order-confirm-001",
    type:           "transactional",
    recipient:      "alice@example.com",
    opts: new NotifyOptions
    {
        Subject = "Your order is confirmed",
        Body    = "Hi Alice, order #1234 is confirmed.",
    }
);

// SMS
var resp = await client.Notifications.NotifyBySmsAsync(
    "user-uuid", "sms-verify-001", "otp", "+15555550100",
    new NotifyOptions { Body = "Your code is 482910" }
);

// Push
var resp = await client.Notifications.NotifyByPushAsync(
    "user-uuid", "push-promo-001", "promotional", "<device-token>",
    new NotifyOptions
    {
        Subject = "Flash sale — 50% off",
        Body    = "Shop now before midnight.",
    }
);

// Slack
var resp = await client.Notifications.NotifyBySlackAsync(
    "user-uuid", "slack-alert-001", "transactional",
    "https://hooks.slack.com/services/XXX/YYY/ZZZ",
    new NotifyOptions { Body = ":rocket: Deploy succeeded" }
);

Console.WriteLine($"ID={resp.NotificationID}  Status={resp.Status}");`}
        </CodeBlock>

        <p className="text-sm text-muted-foreground mt-4 mb-2">
          Use <code className="text-xs">SendAsync()</code> directly for multi-channel or advanced requests.
        </p>
        <CodeBlock>{`var resp = await client.Notifications.SendAsync(new SendRequest
{
    IdempotencyKey = "multi-ch-001",
    UserID         = "user-uuid",
    Channels       = new List<string> { "email", "sms" },
    Type           = "transactional",
    Recipient      = "alice@example.com",
    Body           = "Your account has been verified.",
});`}
        </CodeBlock>
      </Section>

      <Section title="Scheduled delivery">
        <CodeBlock>{`var resp = await client.Notifications.NotifyByEmailAsync(
    "user-uuid", "reminder-001", "promotional", "bob@example.com",
    new NotifyOptions
    {
        Subject     = "Don't forget — trial ends soon",
        Body        = "Your free trial expires in 24 hours.",
        ScheduledAt = DateTime.UtcNow.AddHours(2),
    }
);`}
        </CodeBlock>
      </Section>

      <Section title="Templates">
        <CodeBlock>{`var resp = await client.Notifications.SendAsync(new SendRequest
{
    IdempotencyKey     = "tmpl-order-001",
    UserID             = "user-uuid",
    Channels           = new List<string> { "email" },
    Type               = "transactional",
    Recipient          = "alice@example.com",
    TemplateID         = "template-uuid",
    TemplateVariables  = new Dictionary<string, string>
    {
        ["order_id"]      = "1234",
        ["customer_name"] = "Alice",
        ["delivery_date"] = "May 3",
    },
});`}
        </CodeBlock>
      </Section>

      <Section title="Listing &amp; retrieving notifications">
        <CodeBlock>{`// List with filters
var list = await client.Notifications.ListAsync(
    page:     1,
    pageSize: 25,
    userID:   "user-uuid",
    channel:  "email",
    status:   "sent"
);
Console.WriteLine($"total={list.Total}");

// Get details (notification + attempt history + events)
var detail = await client.Notifications.GetAsync("notification-uuid");
Console.WriteLine(detail.Notification?.Status);`}
        </CodeBlock>
      </Section>

      <Section title="OTP">
        <p className="text-sm text-muted-foreground mb-2">
          OTP endpoints require a <code className="text-xs">UseServiceToken</code> client.
        </p>
        <CodeBlock>{`// Send OTP
var otpResp = await client.Otp.SendAsync(new OtpSendRequest
{
    UserID        = "user-uuid",
    PhoneNumber   = "+15555550100",
    Purpose       = "login",
    ExpirySeconds = 300,
});

// Verify OTP
var verifyResp = await client.Otp.VerifyAsync(new OtpVerifyRequest
{
    UserID  = "user-uuid",
    Purpose = "login",
    Otp     = "482910",
});
Console.WriteLine($"verified={verifyResp.Verified}");`}
        </CodeBlock>
      </Section>

      <Section title="Error handling">
        <p className="text-sm text-muted-foreground mb-2">
          All methods throw <code className="text-xs">NotificationApiException</code> on HTTP 4xx/5xx responses.
        </p>
        <CodeBlock>{`try
{
    var resp = await client.Notifications.NotifyByEmailAsync(/* ... */);
}
catch (NotificationApiException ex)
{
    Console.WriteLine($"HTTP {ex.StatusCode}: {ex.Message}  code={ex.ErrorCode}");
}`}
        </CodeBlock>
      </Section>

      <Section title="API surface">
        <div className="space-y-2">
          <MethodRow method="Notifications.SendAsync" signature="SendAsync(SendRequest, ct) → SendResponse" description="Send to one or more channels in a single request." />
          <MethodRow method="Notifications.NotifyByEmailAsync" signature="NotifyByEmailAsync(userID, key, type, recipient, opts?, ct) → SendResponse" description="Send an email notification." />
          <MethodRow method="Notifications.NotifyBySmsAsync" signature="NotifyBySmsAsync(userID, key, type, recipient, opts?, ct) → SendResponse" description="Send an SMS notification." />
          <MethodRow method="Notifications.NotifyByPushAsync" signature="NotifyByPushAsync(userID, key, type, recipient, opts?, ct) → SendResponse" description="Send a push notification." />
          <MethodRow method="Notifications.NotifyBySlackAsync" signature="NotifyBySlackAsync(userID, key, type, recipient, opts?, ct) → SendResponse" description="Send a Slack Incoming Webhook message." />
          <MethodRow method="Notifications.ListAsync" signature="ListAsync(page, pageSize, userID?, channel?, status?, ct) → ListNotificationsResponse" description="Paginated list with optional filters." />
          <MethodRow method="Notifications.GetAsync" signature="GetAsync(id, ct) → NotificationDetailResponse" description="Fetch notification, attempt history, and lifecycle events." />
          <MethodRow method="Otp.SendAsync" signature="SendAsync(OtpSendRequest, ct) → OtpSendResponse" description="Generate and deliver a one-time password via SMS." />
          <MethodRow method="Otp.VerifyAsync" signature="VerifyAsync(OtpVerifyRequest, ct) → OtpVerifyResponse" description="Verify a previously issued OTP." />
        </div>
      </Section>
    </div>
  )
}
