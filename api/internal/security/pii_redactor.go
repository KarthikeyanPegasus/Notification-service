package security

import (
	"strings"
	"unicode"

	"github.com/spidey/notification-service/internal/domain"
)

// RedactEmail redacts an email address to show only domain.
// Example: "user@example.com" -> "***@example.com"
func RedactEmail(email string) string {
	if email == "" {
		return ""
	}

	// Try to parse email
	parts := strings.Split(email, "@")
	if len(parts) != 2 {
		// Not a valid email format, return masked version
		return "***@***"
	}

	// Redact local part completely
	return "***@" + parts[1]
}

// RedactPhone redacts a phone number to show only last 4 digits.
// Example: "+12345678901" -> "***-***-78901"
func RedactPhone(phone string) string {
	if phone == "" {
		return ""
	}

	// Remove common formatting characters
	cleaned := strings.Map(func(r rune) rune {
		if unicode.IsDigit(r) || r == '+' {
			return r
		}
		return -1
	}, phone)

	if len(cleaned) <= 4 {
		return "***"
	}

	// Keep last 4 digits
	last4 := cleaned[len(cleaned)-4:]
	return "***-***-" + last4
}

// RedactRecipient redacts recipient based on channel type.
func RedactRecipient(recipient string, channel domain.Channel) string {
	if recipient == "" {
		return ""
	}

	switch channel {
	case domain.ChannelEmail:
		return RedactEmail(recipient)
	case domain.ChannelSMS:
		return RedactPhone(recipient)
	default:
		// For other channels (Slack, WebSocket), don't redact
		// as they typically use identifiers, not PII
		return recipient
	}
}

// RedactMap redacts sensitive fields from a map.
func RedactMap(data map[string]any, sensitiveFields ...string) map[string]any {
	if data == nil {
		return nil
	}

	// Create a copy to avoid modifying original
	redacted := make(map[string]any, len(data))
	for k, v := range data {
		redacted[k] = v
	}

	// Redact specified sensitive fields
	for _, field := range sensitiveFields {
		if _, exists := redacted[field]; exists {
			if val, ok := redacted[field].(string); ok {
				// Try to detect if it's an email or phone
				if strings.Contains(val, "@") {
					redacted[field] = RedactEmail(val)
				} else if isPhoneNumber(val) {
					redacted[field] = RedactPhone(val)
				} else {
					redacted[field] = "***"
				}
			} else {
				redacted[field] = "***"
			}
		}
	}

	return redacted
}

// isPhoneNumber checks if a string looks like a phone number.
func isPhoneNumber(s string) bool {
	// Remove formatting characters
	cleaned := strings.Map(func(r rune) rune {
		if unicode.IsDigit(r) {
			return r
		}
		return -1
	}, s)

	// Phone numbers typically have 7-15 digits
	if len(cleaned) < 7 || len(cleaned) > 15 {
		return false
	}

	// Check if it contains mostly digits
	digitCount := 0
	for _, r := range s {
		if unicode.IsDigit(r) {
			digitCount++
		}
	}

	// At least 70% of characters should be digits
	return float64(digitCount)/float64(len(s)) >= 0.7
}

// SanitizeWebhookPayload removes or redacts PII from webhook payloads before storage.
func SanitizeWebhookPayload(payload map[string]any) map[string]any {
	if payload == nil {
		return nil
	}

	sanitized := make(map[string]any, len(payload))
	for k, v := range payload {
		switch k {
		// Known PII fields to redact
		case "email", "recipient", "to", "from", "email_address", "phone", "phone_number", "msisdn":
			if str, ok := v.(string); ok {
				if strings.Contains(str, "@") {
					sanitized[k] = RedactEmail(str)
				} else {
					sanitized[k] = RedactPhone(str)
				}
			} else {
				sanitized[k] = "***"
			}
		// Recursively sanitize nested objects
		case "event-data", "mail", "bounce", "message", "data":
			if nested, ok := v.(map[string]any); ok {
				sanitized[k] = SanitizeWebhookPayload(nested)
			} else {
				sanitized[k] = v
			}
		// Recursively sanitize arrays of objects
		case "bouncedRecipients", "recipients":
			if arr, ok := v.([]any); ok {
				sanitizedArr := make([]any, len(arr))
				for i, item := range arr {
					if nested, ok := item.(map[string]any); ok {
						sanitizedArr[i] = SanitizeWebhookPayload(nested)
					} else {
						sanitizedArr[i] = item
					}
				}
				sanitized[k] = sanitizedArr
			} else {
				sanitized[k] = v
			}
		default:
			sanitized[k] = v
		}
	}

	return sanitized
}

// SanitizeLogFields creates a safe-to-log version of notification fields.
func SanitizeLogFields(recipient string, channel domain.Channel) map[string]string {
	return map[string]string{
		"recipient": RedactRecipient(recipient, channel),
	}
}
