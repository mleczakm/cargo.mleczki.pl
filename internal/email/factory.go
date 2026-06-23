package email

import (
	"fmt"
	"os"
	"strings"
)

func smtpHost() string {
	if host := strings.TrimSpace(os.Getenv("SMTP_HOST")); host != "" {
		return host
	}
	if strings.EqualFold(strings.TrimSpace(os.Getenv("MAILPIT")), "1") ||
		strings.EqualFold(strings.TrimSpace(os.Getenv("MAILPIT")), "true") {
		return "localhost"
	}
	return ""
}

// DefaultSenderEmail returns the configured sender address for outbound mail.
func DefaultSenderEmail() string {
	for _, key := range []string{"BREVO_SENDER_EMAIL", "SMTP_FROM"} {
		if email := strings.TrimSpace(os.Getenv(key)); email != "" {
			return email
		}
	}
	return "noreply@cargo.mleczki.pl"
}

// FormatAddress formats a named email address for RFC 5322 headers.
func FormatAddress(name, email string) string {
	if name == "" {
		return email
	}
	return fmt.Sprintf("%s <%s>", name, email)
}
