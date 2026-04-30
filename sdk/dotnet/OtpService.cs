using System.Threading;
using System.Threading.Tasks;

namespace NotificationService.Sdk
{
    public class OtpService
    {
        private readonly NotificationClient _client;

        internal OtpService(NotificationClient client)
        {
            _client = client;
        }

        public Task<OtpSendResponse> SendAsync(OtpSendRequest request, CancellationToken ct = default)
        {
            return _client.DoRequestAsync<OtpSendResponse>("POST", "/otp/send", request, ct);
        }

        public Task<OtpVerifyResponse> VerifyAsync(OtpVerifyRequest request, CancellationToken ct = default)
        {
            return _client.DoRequestAsync<OtpVerifyResponse>("POST", "/otp/verify", request, ct);
        }

        public Task<OtpSendResponse> SendOtpAsync(string userID, string phoneNumber, string purpose, int? expirySeconds = null, CancellationToken ct = default)
        {
            return SendAsync(new OtpSendRequest
            {
                UserID = userID,
                PhoneNumber = phoneNumber,
                Purpose = purpose,
                ExpirySeconds = expirySeconds
            }, ct);
        }

        public Task<OtpVerifyResponse> VerifyOtpAsync(string userID, string purpose, string otp, CancellationToken ct = default)
        {
            return VerifyAsync(new OtpVerifyRequest
            {
                UserID = userID,
                Purpose = purpose,
                Otp = otp
            }, ct);
        }
    }
}
