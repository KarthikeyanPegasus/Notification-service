using System;

namespace NotificationService.Sdk
{
    public class NotificationApiException : Exception
    {
        public int HttpStatus { get; }
        public string? ErrorCode { get; }

        public NotificationApiException(int httpStatus, string message, string? errorCode = null) 
            : base(message)
        {
            HttpStatus = httpStatus;
            ErrorCode = errorCode;
        }

        public override string ToString()
        {
            return $"NotificationApiException: {HttpStatus} {ErrorCode} - {Message}";
        }
    }
}
