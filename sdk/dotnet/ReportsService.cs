using System.Collections.Generic;
using System.Threading;
using System.Threading.Tasks;

namespace NotificationService.Sdk
{
    public class ReportsService
    {
        private readonly NotificationClient _client;

        internal ReportsService(NotificationClient client) => _client = client;

        private static string BuildQuery(ReportFilters? f)
        {
            if (f == null) return "";
            var parts = new List<string>();
            if (!string.IsNullOrEmpty(f.DateFrom)) parts.Add($"date_from={System.Uri.EscapeDataString(f.DateFrom)}");
            if (!string.IsNullOrEmpty(f.DateTo))   parts.Add($"date_to={System.Uri.EscapeDataString(f.DateTo)}");
            if (!string.IsNullOrEmpty(f.ApiKeyId)) parts.Add($"api_key_id={System.Uri.EscapeDataString(f.ApiKeyId)}");
            return parts.Count > 0 ? "?" + string.Join("&", parts) : "";
        }

        /// <summary>Returns delivery stats grouped by channel and date for the given range.</summary>
        public Task<List<ReportSummaryItem>> SummaryAsync(ReportFilters? filters = null, CancellationToken ct = default)
            => _client.DoRequestAsync<List<ReportSummaryItem>>("GET", $"/reports/summary{BuildQuery(filters)}", null, ct);

        /// <summary>Returns notification counts grouped by ingress source (api, pubsub, redis).</summary>
        public Task<List<IngressBreakdownItem>> IngressBreakdownAsync(ReportFilters? filters = null, CancellationToken ct = default)
            => _client.DoRequestAsync<List<IngressBreakdownItem>>("GET", $"/reports/ingress{BuildQuery(filters)}", null, ct);

        /// <summary>Returns SMS counts grouped by destination country prefix (top 10).</summary>
        public Task<List<BreakdownRow>> SmsCountriesAsync(ReportFilters? filters = null, CancellationToken ct = default)
            => _client.DoRequestAsync<List<BreakdownRow>>("GET", $"/reports/sms-countries{BuildQuery(filters)}", null, ct);

        /// <summary>Returns email counts grouped by recipient domain (top 10).</summary>
        public Task<List<BreakdownRow>> EmailDomainsAsync(ReportFilters? filters = null, CancellationToken ct = default)
            => _client.DoRequestAsync<List<BreakdownRow>>("GET", $"/reports/email-domains{BuildQuery(filters)}", null, ct);

        /// <summary>Returns real-time per-provider delivery metrics computed over the last 12 hours.</summary>
        public Task<List<VendorMetric>> VendorHealthAsync(CancellationToken ct = default)
            => _client.DoRequestAsync<List<VendorMetric>>("GET", "/reports/vendors", null, ct);

        /// <summary>Returns per-vendor cost data. Live from vendor APIs where available, estimated otherwise.</summary>
        public Task<List<VendorBilling>> VendorBillingAsync(CancellationToken ct = default)
            => _client.DoRequestAsync<List<VendorBilling>>("GET", "/reports/billing", null, ct);

        /// <summary>Returns aggregate metrics for scheduled notifications in the given date range.</summary>
        public Task<ScheduledStats> ScheduledStatsAsync(ReportFilters? filters = null, CancellationToken ct = default)
            => _client.DoRequestAsync<ScheduledStats>("GET", $"/reports/scheduled-stats{BuildQuery(filters)}", null, ct);
    }
}
