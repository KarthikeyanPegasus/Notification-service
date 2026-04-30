using System;
using System.Collections.Generic;
using System.Threading;
using System.Threading.Tasks;

namespace NotificationService.Sdk
{
    public class NotificationsService
    {
        private readonly NotificationClient _client;

        internal NotificationsService(NotificationClient client) => _client = client;

        // ── Send ─────────────────────────────────────────────────────────────

        public Task<SendResponse> SendAsync(SendRequest request, CancellationToken ct = default)
            => _client.DoRequestAsync<SendResponse>("POST", "/notifications", request, ct);

        public Task<SendResponse> NotifyByEmailAsync(string userId, string idempotencyKey, string type, string recipient, NotifyOptions? opts = null, CancellationToken ct = default)
            => SendAsync(BuildRequest("email", userId, idempotencyKey, type, recipient, opts), ct);

        public Task<SendResponse> NotifyBySmsAsync(string userId, string idempotencyKey, string type, string recipient, NotifyOptions? opts = null, CancellationToken ct = default)
            => SendAsync(BuildRequest("sms", userId, idempotencyKey, type, recipient, opts), ct);

        public Task<SendResponse> NotifyByPushAsync(string userId, string idempotencyKey, string type, string recipient, NotifyOptions? opts = null, CancellationToken ct = default)
            => SendAsync(BuildRequest("push", userId, idempotencyKey, type, recipient, opts), ct);

        public Task<SendResponse> NotifyByWebSocketAsync(string userId, string idempotencyKey, string type, string recipient, NotifyOptions? opts = null, CancellationToken ct = default)
            => SendAsync(BuildRequest("websocket", userId, idempotencyKey, type, recipient, opts), ct);

        public Task<SendResponse> NotifyByWebhookAsync(string userId, string idempotencyKey, string type, string recipient, NotifyOptions? opts = null, CancellationToken ct = default)
            => SendAsync(BuildRequest("webhook", userId, idempotencyKey, type, recipient, opts), ct);

        public Task<SendResponse> NotifyBySlackAsync(string userId, string idempotencyKey, string type, string recipient, NotifyOptions? opts = null, CancellationToken ct = default)
            => SendAsync(BuildRequest("slack", userId, idempotencyKey, type, recipient, opts), ct);

        // ── Query ────────────────────────────────────────────────────────────

        /// <summary>Returns full notification details including delivery attempts and events.</summary>
        public Task<Notification> GetAsync(string id, CancellationToken ct = default)
            => _client.DoRequestAsync<Notification>("GET", $"/notifications/{id}", null, ct);

        /// <summary>Returns a paginated list of notifications matching the given filters.</summary>
        public Task<ListNotificationsResponse> ListAsync(ListNotificationsParams? p = null, CancellationToken ct = default)
        {
            p ??= new ListNotificationsParams();
            var q = BuildListQuery(p);
            return _client.DoRequestAsync<ListNotificationsResponse>("GET", $"/notifications{q}", null, ct);
        }

        // ── Lifecycle operations ──────────────────────────────────────────────

        /// <summary>Polls the originating provider for the latest delivery status and updates the record.</summary>
        public Task<SyncResponse> SyncAsync(string id, CancellationToken ct = default)
            => _client.DoRequestAsync<SyncResponse>("POST", $"/notifications/{id}/sync", null, ct);

        /// <summary>Starts a new delivery workflow for an existing failed or stuck notification.</summary>
        public Task<RetriggerResponse> RetriggerAsync(string id, CancellationToken ct = default)
            => _client.DoRequestAsync<RetriggerResponse>("POST", $"/notifications/{id}/retrigger", null, ct);

        // ── Scheduled notifications ───────────────────────────────────────────

        /// <summary>Returns a paginated list of pending scheduled notifications.</summary>
        public Task<ListScheduledResponse> ListScheduledAsync(ListScheduledParams? p = null, CancellationToken ct = default)
        {
            p ??= new ListScheduledParams();
            var q = $"?page={p.Page}&page_size={p.PageSize}";
            if (!string.IsNullOrEmpty(p.Status)) q += $"&status={p.Status}";
            return _client.DoRequestAsync<ListScheduledResponse>("GET", $"/notifications/scheduled{q}", null, ct);
        }

        /// <summary>Updates the dispatch time of a scheduled notification.</summary>
        public Task<Notification> RescheduleAsync(string id, DateTime scheduledAt, CancellationToken ct = default)
            => _client.DoRequestAsync<Notification>("PATCH", $"/notifications/{id}/schedule", new RescheduleRequest { ScheduledAt = scheduledAt }, ct);

        /// <summary>Cancels a pending scheduled notification.</summary>
        public Task CancelScheduledAsync(string id, CancellationToken ct = default)
            => _client.DoVoidRequestAsync("DELETE", $"/notifications/{id}/schedule", ct);

        // ── Helpers ───────────────────────────────────────────────────────────

        private static SendRequest BuildRequest(string channel, string userId, string idempotencyKey, string type, string recipient, NotifyOptions? opts)
        {
            var req = new SendRequest
            {
                IdempotencyKey = idempotencyKey,
                UserId         = userId,
                Channels       = new List<string> { channel },
                Type           = type,
                Recipient      = recipient,
            };
            if (opts == null) return req;
            req.Subject            = opts.Subject;
            req.Body               = opts.Body;
            req.Html               = opts.Html;
            req.TemplateId         = opts.TemplateId;
            req.TemplateVariables  = opts.TemplateVariables;
            req.Priority           = opts.Priority;
            req.ScheduledAt        = opts.ScheduledAt;
            req.SlackChannel       = opts.SlackChannel;
            if (!string.IsNullOrEmpty(opts.Recipient)) req.Recipient = opts.Recipient;
            return req;
        }

        private static string BuildListQuery(ListNotificationsParams p)
        {
            var q = $"?page={p.Page}&page_size={p.PageSize}";
            if (!string.IsNullOrEmpty(p.UserId))    q += $"&user_id={Uri.EscapeDataString(p.UserId)}";
            if (!string.IsNullOrEmpty(p.Channel))   q += $"&channel={p.Channel}";
            if (!string.IsNullOrEmpty(p.Status))    q += $"&status={p.Status}";
            if (!string.IsNullOrEmpty(p.Recipient)) q += $"&recipient={Uri.EscapeDataString(p.Recipient)}";
            if (!string.IsNullOrEmpty(p.Search))    q += $"&search={Uri.EscapeDataString(p.Search)}";
            if (!string.IsNullOrEmpty(p.DateFrom))  q += $"&date_from={Uri.EscapeDataString(p.DateFrom)}";
            if (!string.IsNullOrEmpty(p.DateTo))    q += $"&date_to={Uri.EscapeDataString(p.DateTo)}";
            if (!string.IsNullOrEmpty(p.ApiKeyId))  q += $"&api_key_id={Uri.EscapeDataString(p.ApiKeyId)}";
            return q;
        }
    }
}
