using System;
using System.Collections.Generic;
using System.Text.Json.Serialization;

namespace NotificationService.Sdk
{
    // ── Enumerations ─────────────────────────────────────────────────────────

    public enum Channel
    {
        [JsonPropertyName("email")]     Email,
        [JsonPropertyName("sms")]       Sms,
        [JsonPropertyName("push")]      Push,
        [JsonPropertyName("websocket")] WebSocket,
        [JsonPropertyName("webhook")]   Webhook,
        [JsonPropertyName("slack")]     Slack
    }

    public enum Priority
    {
        [JsonPropertyName("high")]   High,
        [JsonPropertyName("medium")] Medium,
        [JsonPropertyName("low")]    Low
    }

    public enum NotificationStatus
    {
        [JsonPropertyName("pending")]    Pending,
        [JsonPropertyName("queued")]     Queued,
        [JsonPropertyName("sent")]       Sent,
        [JsonPropertyName("delivered")]  Delivered,
        [JsonPropertyName("failed")]     Failed,
        [JsonPropertyName("cancelled")]  Cancelled,
        [JsonPropertyName("bounced")]    Bounced,
        [JsonPropertyName("suppressed")] Suppressed
    }

    // ── Core notification types ───────────────────────────────────────────────

    public class RenderedContent
    {
        [JsonPropertyName("subject")] public string? Subject { get; set; }
        [JsonPropertyName("body")]    public string? Body    { get; set; }
        [JsonPropertyName("html")]    public string? Html    { get; set; }
        [JsonPropertyName("data")]    public Dictionary<string, string>? Data { get; set; }
    }

    public class NotificationAttempt
    {
        [JsonPropertyName("id")]               public string  Id              { get; set; } = "";
        [JsonPropertyName("notification_id")]  public string  NotificationId  { get; set; } = "";
        [JsonPropertyName("attempt_number")]   public int     AttemptNumber   { get; set; }
        [JsonPropertyName("status")]           public string  Status          { get; set; } = "";
        [JsonPropertyName("provider")]         public string  Provider        { get; set; } = "";
        [JsonPropertyName("provider_msg_id")]  public string? ProviderMsgId   { get; set; }
        [JsonPropertyName("error_code")]       public string? ErrorCode        { get; set; }
        [JsonPropertyName("error_message")]    public string? ErrorMessage     { get; set; }
        [JsonPropertyName("latency_ms")]       public int?    LatencyMs        { get; set; }
        [JsonPropertyName("created_at")]       public DateTime CreatedAt       { get; set; }
    }

    public class NotificationEvent
    {
        [JsonPropertyName("id")]               public string  Id             { get; set; } = "";
        [JsonPropertyName("notification_id")]  public string  NotificationId { get; set; } = "";
        [JsonPropertyName("event_type")]       public string  EventType      { get; set; } = "";
        [JsonPropertyName("metadata")]         public Dictionary<string, object>? Metadata { get; set; }
        [JsonPropertyName("created_at")]       public DateTime CreatedAt     { get; set; }
    }

    public class Notification
    {
        [JsonPropertyName("id")]               public Guid   Id              { get; set; }
        [JsonPropertyName("idempotency_key")]  public string? IdempotencyKey  { get; set; }
        [JsonPropertyName("user_id")]          public Guid   UserId          { get; set; }
        [JsonPropertyName("api_key_id")]       public Guid?  ApiKeyId        { get; set; }
        [JsonPropertyName("client_name")]      public string? ClientName      { get; set; }

        [JsonPropertyName("channel")]
        [JsonConverter(typeof(JsonStringEnumConverter))]
        public Channel Channel { get; set; }

        [JsonPropertyName("priority")]
        [JsonConverter(typeof(JsonStringEnumConverter))]
        public Priority Priority { get; set; }

        [JsonPropertyName("type")]             public string? Type            { get; set; }
        [JsonPropertyName("template_id")]      public Guid?   TemplateId      { get; set; }
        [JsonPropertyName("rendered_content")] public RenderedContent? RenderedContent { get; set; }
        [JsonPropertyName("recipient")]        public string? Recipient        { get; set; }

        [JsonPropertyName("status")]
        [JsonConverter(typeof(JsonStringEnumConverter))]
        public NotificationStatus Status { get; set; }

        [JsonPropertyName("provider")]         public string? Provider         { get; set; }
        [JsonPropertyName("source")]           public string? Source           { get; set; }
        [JsonPropertyName("forced_vendor")]    public string? ForcedVendor     { get; set; }
        [JsonPropertyName("scheduled_at")]     public DateTime? ScheduledAt    { get; set; }
        [JsonPropertyName("sent_at")]          public DateTime? SentAt         { get; set; }
        [JsonPropertyName("delivered_at")]     public DateTime? DeliveredAt    { get; set; }
        [JsonPropertyName("created_at")]       public DateTime CreatedAt       { get; set; }
        [JsonPropertyName("updated_at")]       public DateTime UpdatedAt       { get; set; }
        [JsonPropertyName("attempts")]         public List<NotificationAttempt>? Attempts { get; set; }
        [JsonPropertyName("events")]           public List<NotificationEvent>?   Events   { get; set; }
    }

    public class ScheduledNotification
    {
        [JsonPropertyName("id")]               public string Id              { get; set; } = "";
        [JsonPropertyName("notification_id")]  public string NotificationId  { get; set; } = "";
        [JsonPropertyName("user_id")]          public string UserId          { get; set; } = "";

        [JsonPropertyName("channel")]
        [JsonConverter(typeof(JsonStringEnumConverter))]
        public Channel Channel { get; set; }

        [JsonPropertyName("template_id")]      public string? TemplateId     { get; set; }
        [JsonPropertyName("template_vars")]    public Dictionary<string, string>? TemplateVars { get; set; }
        [JsonPropertyName("scheduled_at")]     public DateTime ScheduledAt   { get; set; }
        [JsonPropertyName("original_at")]      public DateTime OriginalAt    { get; set; }

        [JsonPropertyName("status")]
        [JsonConverter(typeof(JsonStringEnumConverter))]
        public NotificationStatus Status { get; set; }

        [JsonPropertyName("reschedule_count")] public int      RescheduleCount { get; set; }
        [JsonPropertyName("created_at")]       public DateTime CreatedAt       { get; set; }
        [JsonPropertyName("updated_at")]       public DateTime UpdatedAt       { get; set; }
    }

    // ── Request / response types ──────────────────────────────────────────────

    public class SendRequest
    {
        [JsonPropertyName("idempotency_key")]    public string? IdempotencyKey    { get; set; }
        [JsonPropertyName("user_id")]            public string? UserId            { get; set; }
        [JsonPropertyName("channels")]           public List<string>? Channels    { get; set; }
        [JsonPropertyName("type")]               public string? Type              { get; set; }
        [JsonPropertyName("subject")]            public string? Subject           { get; set; }
        [JsonPropertyName("body")]               public string? Body              { get; set; }
        [JsonPropertyName("html")]               public string? Html              { get; set; }
        [JsonPropertyName("template_id")]        public string? TemplateId        { get; set; }
        [JsonPropertyName("template_variables")] public Dictionary<string, string>? TemplateVariables { get; set; }
        [JsonPropertyName("recipient")]          public string? Recipient         { get; set; }
        [JsonPropertyName("slack_channel")]      public string? SlackChannel      { get; set; }
        [JsonPropertyName("priority")]
        [JsonConverter(typeof(JsonStringEnumConverter))]
        public Priority? Priority { get; set; }
        [JsonPropertyName("scheduled_at")]       public DateTime? ScheduledAt     { get; set; }
    }

    public class SendResponse
    {
        [JsonPropertyName("notification_id")] public string? NotificationId { get; set; }
        [JsonPropertyName("status")]          public string? Status         { get; set; }
        [JsonPropertyName("workflow_id")]     public string? WorkflowId     { get; set; }
        [JsonPropertyName("scheduled_at")]    public DateTime? ScheduledAt  { get; set; }
    }

    public class NotifyOptions
    {
        public string? Subject           { get; set; }
        public string? Body              { get; set; }
        public string? Html              { get; set; }
        public string? TemplateId        { get; set; }
        public Dictionary<string, string>? TemplateVariables { get; set; }
        public Priority? Priority        { get; set; }
        public DateTime? ScheduledAt     { get; set; }
        public string? Recipient         { get; set; }
        public string? SlackChannel      { get; set; }
    }

    public class ListNotificationsParams
    {
        public int    Page      { get; set; } = 1;
        public int    PageSize  { get; set; } = 50;
        public string? UserId   { get; set; }
        public string? Channel  { get; set; }
        public string? Status   { get; set; }
        public string? Recipient { get; set; }
        public string? Search   { get; set; }
        public string? DateFrom { get; set; } // RFC3339
        public string? DateTo   { get; set; } // RFC3339
        public string? ApiKeyId { get; set; }
    }

    public class ListNotificationsResponse
    {
        [JsonPropertyName("data")]      public List<Notification>? Data     { get; set; }
        [JsonPropertyName("total")]     public int Total    { get; set; }
        [JsonPropertyName("page")]      public int Page     { get; set; }
        [JsonPropertyName("page_size")] public int PageSize { get; set; }
    }

    public class ListScheduledParams
    {
        public int    Page     { get; set; } = 1;
        public int    PageSize { get; set; } = 20;
        public string? Status  { get; set; }
    }

    public class ListScheduledResponse
    {
        [JsonPropertyName("data")]      public List<ScheduledNotification>? Data { get; set; }
        [JsonPropertyName("total")]     public int Total    { get; set; }
        [JsonPropertyName("page")]      public int Page     { get; set; }
        [JsonPropertyName("page_size")] public int PageSize { get; set; }
    }

    public class RescheduleRequest
    {
        [JsonPropertyName("scheduled_at")] public DateTime ScheduledAt { get; set; }
    }

    public class SyncResponse
    {
        [JsonPropertyName("status")]          public string? Status         { get; set; }
        [JsonPropertyName("provider_status")] public string? ProviderStatus { get; set; }
        [JsonPropertyName("synced_at")]       public DateTime SyncedAt      { get; set; }
    }

    public class RetriggerResponse
    {
        [JsonPropertyName("notification_id")] public string? NotificationId { get; set; }
        [JsonPropertyName("status")]          public string? Status         { get; set; }
    }

    // ── OTP ───────────────────────────────────────────────────────────────────

    public class OtpSendRequest
    {
        [JsonPropertyName("user_id")]       public string? UserId        { get; set; }
        [JsonPropertyName("phone_number")]  public string? PhoneNumber   { get; set; }
        [JsonPropertyName("purpose")]       public string? Purpose       { get; set; }
        [JsonPropertyName("expiry_seconds")] public int?  ExpirySeconds  { get; set; }
    }

    public class OtpSendResponse
    {
        [JsonPropertyName("otp_id")]   public string?   OtpId    { get; set; }
        [JsonPropertyName("expiry_at")] public DateTime ExpiryAt { get; set; }
    }

    public class OtpVerifyRequest
    {
        [JsonPropertyName("user_id")]  public string? UserId   { get; set; }
        [JsonPropertyName("purpose")]  public string? Purpose  { get; set; }
        [JsonPropertyName("otp")]      public string? Otp      { get; set; }
    }

    public class OtpVerifyResponse
    {
        [JsonPropertyName("verified")] public bool Verified { get; set; }
    }

    // ── Reports ───────────────────────────────────────────────────────────────

    public class ReportFilters
    {
        public string? DateFrom { get; set; } // RFC3339
        public string? DateTo   { get; set; } // RFC3339
        public string? ApiKeyId { get; set; }
    }

    public class ReportSummaryItem
    {
        [JsonPropertyName("channel")]
        [JsonConverter(typeof(JsonStringEnumConverter))]
        public Channel Channel { get; set; }

        [JsonPropertyName("date")]          public string  Date         { get; set; } = "";
        [JsonPropertyName("total")]         public int     Total        { get; set; }
        [JsonPropertyName("sent")]          public int     Sent         { get; set; }
        [JsonPropertyName("delivered")]     public int     Delivered    { get; set; }
        [JsonPropertyName("failed")]        public int     Failed       { get; set; }
        [JsonPropertyName("bounced")]       public int     Bounced      { get; set; }
        [JsonPropertyName("success_rate")]  public double  SuccessRate  { get; set; }
        [JsonPropertyName("p50_latency_ms")] public double P50LatencyMs { get; set; }
        [JsonPropertyName("p95_latency_ms")] public double P95LatencyMs { get; set; }
    }

    public class IngressBreakdownItem
    {
        [JsonPropertyName("source")] public string Source { get; set; } = "";
        [JsonPropertyName("count")]  public int    Count  { get; set; }
    }

    public class BreakdownRow
    {
        [JsonPropertyName("key")]   public string Key   { get; set; } = "";
        [JsonPropertyName("count")] public int    Count { get; set; }
    }

    public class VendorMetric
    {
        [JsonPropertyName("provider")]        public string Provider       { get; set; } = "";
        [JsonPropertyName("total")]           public int    Total          { get; set; }
        [JsonPropertyName("success_rate")]    public double SuccessRate    { get; set; }
        [JsonPropertyName("avg_latency_ms")]  public double AvgLatencyMs   { get; set; }
        [JsonPropertyName("latest_error")]    public string LatestError    { get; set; } = "";
        [JsonPropertyName("last_status")]     public string LastStatus     { get; set; } = "";
        [JsonPropertyName("cost_per_msg")]    public double CostPerMsg     { get; set; }
        [JsonPropertyName("estimated_cost")]  public double EstimatedCost  { get; set; }
        [JsonPropertyName("status")]          public string Status         { get; set; } = "";
        [JsonPropertyName("payment_status")]  public string PaymentStatus  { get; set; } = "";
    }

    public class VendorBilling
    {
        [JsonPropertyName("provider")]      public string  Provider    { get; set; } = "";
        [JsonPropertyName("total_cost")]    public double  TotalCost   { get; set; }
        [JsonPropertyName("balance")]       public double? Balance     { get; set; }
        [JsonPropertyName("currency")]      public string  Currency    { get; set; } = "";
        [JsonPropertyName("period")]        public string  Period      { get; set; } = "";
        [JsonPropertyName("is_actual")]     public bool    IsActual    { get; set; }
        [JsonPropertyName("source")]        public string  Source      { get; set; } = "";
        [JsonPropertyName("total_count")]   public long    TotalCount  { get; set; }
        [JsonPropertyName("cost_per_msg")]  public double  CostPerMsg  { get; set; }
        [JsonPropertyName("error")]         public string? Error       { get; set; }
    }

    public class ScheduledStats
    {
        [JsonPropertyName("total_scheduled")]          public int     TotalScheduled       { get; set; }
        [JsonPropertyName("pending")]                  public int     Pending              { get; set; }
        [JsonPropertyName("delivered")]                public int     Delivered            { get; set; }
        [JsonPropertyName("avg_delivery_latency_ms")]  public double? AvgDeliveryLatencyMs { get; set; }
    }

    // ── Auth & API keys ───────────────────────────────────────────────────────

    public class LoginRequest
    {
        [JsonPropertyName("email")]    public string Email    { get; set; } = "";
        [JsonPropertyName("password")] public string Password { get; set; } = "";
    }

    public class LoginResponse
    {
        [JsonPropertyName("token")] public string    Token { get; set; } = "";
        [JsonPropertyName("user")]  public UserInfo? User  { get; set; }
    }

    public class UserInfo
    {
        [JsonPropertyName("id")]    public string Id    { get; set; } = "";
        [JsonPropertyName("email")] public string Email { get; set; } = "";
        [JsonPropertyName("name")]  public string Name  { get; set; } = "";
        [JsonPropertyName("role")]  public string Role  { get; set; } = "";
    }

    public class ApiKeyClient
    {
        [JsonPropertyName("id")]         public string    Id        { get; set; } = "";
        [JsonPropertyName("name")]       public string    Name      { get; set; } = "";
        [JsonPropertyName("prefix")]     public string    Prefix    { get; set; } = "";
        [JsonPropertyName("created_at")] public DateTime  CreatedAt { get; set; }
        [JsonPropertyName("revoked_at")] public DateTime? RevokedAt { get; set; }
    }

    public class CreateApiKeyRequest
    {
        [JsonPropertyName("name")] public string Name { get; set; } = "";
    }

    public class CreateApiKeyResponse
    {
        [JsonPropertyName("key")]     public ApiKeyClient? Key    { get; set; }
        [JsonPropertyName("api_key")] public string        ApiKey { get; set; } = "";
    }
}
