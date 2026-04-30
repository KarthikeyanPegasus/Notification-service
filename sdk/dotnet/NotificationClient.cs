using System;
using System.Collections.Generic;
using System.Net.Http;
using System.Net.Http.Headers;
using System.Text;
using System.Text.Json;
using System.Threading;
using System.Threading.Tasks;

namespace NotificationService.Sdk
{
    public class NotificationClient : IDisposable
    {
        private readonly HttpClient _httpClient;
        private readonly bool _ownHttpClient;
        private readonly string _baseUrl;
        private string? _bearerToken;
        private string? _serviceToken;
        private string? _apiKey;

        public NotificationsService Notifications { get; }
        public OtpService            Otp          { get; }
        public ReportsService        Reports       { get; }

        public NotificationClient(string baseUrl = "http://localhost:8080/v1", HttpClient? httpClient = null)
        {
            _baseUrl       = baseUrl.TrimEnd('/');
            _ownHttpClient = httpClient == null;
            _httpClient    = httpClient ?? new HttpClient();
            Notifications  = new NotificationsService(this);
            Otp            = new OtpService(this);
            Reports        = new ReportsService(this);
        }

        /// <summary>Set the JWT bearer token used for authenticated requests.</summary>
        public void UseBearerToken(string token) => _bearerToken = token;

        /// <summary>Set the X-Service-Token header used for OTP endpoints.</summary>
        public void UseServiceToken(string token) => _serviceToken = token;

        /// <summary>Set the X-API-Key header for SDK API key auth.</summary>
        public void UseApiKey(string apiKey) => _apiKey = apiKey;

        // ── Auth helpers ──────────────────────────────────────────────────────

        /// <summary>
        /// Authenticates with email + password and stores the returned JWT automatically.
        /// </summary>
        public async Task<LoginResponse> LoginAsync(string email, string password, CancellationToken ct = default)
        {
            var resp = await DoRequestAsync<LoginResponse>("POST", "/auth/login",
                new LoginRequest { Email = email, Password = password }, ct);
            if (!string.IsNullOrEmpty(resp.Token))
                _bearerToken = resp.Token;
            return resp;
        }

        // ── API key management ────────────────────────────────────────────────

        /// <summary>Admin-only: returns all active API key clients.</summary>
        public Task<List<ApiKeyClient>> ListApiKeysAsync(CancellationToken ct = default)
            => DoRequestAsync<List<ApiKeyClient>>("GET", "/admin/api-keys", null, ct);

        /// <summary>Admin-only: creates a new API key. The raw key is returned once only.</summary>
        public Task<CreateApiKeyResponse> CreateApiKeyAsync(string name, CancellationToken ct = default)
            => DoRequestAsync<CreateApiKeyResponse>("POST", "/admin/api-keys", new CreateApiKeyRequest { Name = name }, ct);

        /// <summary>Admin-only: permanently revokes an API key.</summary>
        public Task RevokeApiKeyAsync(string id, CancellationToken ct = default)
            => DoVoidRequestAsync("DELETE", $"/admin/api-keys/{id}", ct);

        /// <summary>Returns the API key clients visible to the current user (admin: all; others: assigned).</summary>
        public Task<List<ApiKeyClient>> ListMyClientsAsync(CancellationToken ct = default)
            => DoRequestAsync<List<ApiKeyClient>>("GET", "/me/clients", null, ct);

        // ── HTTP internals ────────────────────────────────────────────────────

        internal async Task<T> DoRequestAsync<T>(string method, string path, object? body = null, CancellationToken ct = default)
        {
            using var request = BuildRequest(method, path, body);
            var response = await _httpClient.SendAsync(request, ct);
            var content  = await response.Content.ReadAsStringAsync();

            if (!response.IsSuccessStatusCode)
                throw BuildException((int)response.StatusCode, content);

            if (string.IsNullOrWhiteSpace(content))
                return default!;

            return JsonSerializer.Deserialize<T>(content)!;
        }

        internal async Task DoVoidRequestAsync(string method, string path, CancellationToken ct = default)
        {
            using var request = BuildRequest(method, path, null);
            var response = await _httpClient.SendAsync(request, ct);
            if (!response.IsSuccessStatusCode)
            {
                var content = await response.Content.ReadAsStringAsync();
                throw BuildException((int)response.StatusCode, content);
            }
        }

        private HttpRequestMessage BuildRequest(string method, string path, object? body)
        {
            var request = new HttpRequestMessage(new HttpMethod(method), _baseUrl + path);
            if (body != null)
            {
                var json = JsonSerializer.Serialize(body);
                request.Content = new StringContent(json, Encoding.UTF8, "application/json");
            }
            request.Headers.Accept.Add(new MediaTypeWithQualityHeaderValue("application/json"));
            if (!string.IsNullOrEmpty(_bearerToken))
                request.Headers.Authorization = new AuthenticationHeaderValue("Bearer", _bearerToken);
            if (!string.IsNullOrEmpty(_serviceToken))
                request.Headers.Add("X-Service-Token", _serviceToken);
            if (!string.IsNullOrEmpty(_apiKey))
                request.Headers.Add("X-API-Key", _apiKey);
            return request;
        }

        private static NotificationApiException BuildException(int statusCode, string responseBody)
        {
            string message   = responseBody;
            string? code     = null;
            try
            {
                using var doc = JsonDocument.Parse(responseBody);
                if (doc.RootElement.TryGetProperty("message", out var m)) message = m.GetString() ?? message;
                if (doc.RootElement.TryGetProperty("code",    out var c)) code    = c.GetString();
            }
            catch { /* ignore JSON parse errors */ }
            return new NotificationApiException(statusCode, message, code);
        }

        public void Dispose()
        {
            if (_ownHttpClient)
                _httpClient.Dispose();
        }
    }
}
