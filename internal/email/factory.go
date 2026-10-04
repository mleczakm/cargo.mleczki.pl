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
	return "noreply@mleczki.pl"
}

// DefaultReplyToEmail returns the address customers' replies are routed to.
func DefaultReplyToEmail() string {
	if email := strings.TrimSpace(os.Getenv("MAIL_REPLY_TO")); email != "" {
		return email
	}
	return "do@mleczki.pl"
}

// DefaultSender returns the standard outbound sender with the Reply-To set.
func DefaultSender() *EmailSender {
	return &EmailSender{Name: "Cargo Mleczki", Email: DefaultSenderEmail(), ReplyTo: DefaultReplyToEmail()}
}

// FormatAddress formats a named email address for RFC 5322 headers.
func FormatAddress(name, email string) string {
	if name == "" {
		return email
	}
	return fmt.Sprintf("%s <%s>", name, email)
}
