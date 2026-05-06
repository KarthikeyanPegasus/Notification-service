package email

import (
	"fmt"
	"net/mail"
	"strings"
)

// validateEmailFormat returns an error if addr is not a valid RFC 5322 email address.
func validateEmailFormat(addr string) error {
	trimmed := strings.TrimSpace(addr)
	if trimmed == "" {
		return fmt.Errorf("email address is empty")
	}
	if _, err := mail.ParseAddress(trimmed); err != nil {
		return fmt.Errorf("invalid email address %q: %w", trimmed, err)
	}
	return nil
}
