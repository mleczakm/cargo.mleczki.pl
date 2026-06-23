package email

import (
	"context"
	"fmt"
	"log"
	"os"
)

// Sender delivers transactional HTML emails using SMTP or Brevo.
type Sender struct {
	impl Mailer
}

// NewMailer configures an email sender for Mailpit SMTP locally or Brevo in production.
func NewMailer() (*Sender, error) {
	if host := smtpHost(); host != "" {
		port := os.Getenv("SMTP_PORT")
		if port == "" {
			port = "1025"
		}

		log.Printf("Email mailer: SMTP (%s:%s) — open Mailpit at http://localhost:8025", host, port)
		return &Sender{impl: NewSMTPClient(host, port)}, nil
	}

	if os.Getenv("BREVO_API_KEY") == "" {
		log.Println("Transactional email disabled (set SMTP_HOST or BREVO_API_KEY to enable)")
		return &Sender{}, nil
	}

	brevoClient, err := NewBrevoClient()
	if err != nil {
		return nil, fmt.Errorf("create Brevo client: %w", err)
	}

	log.Println("Email mailer: Brevo")
	return &Sender{impl: brevoClient}, nil
}

// Configured reports whether outbound email delivery is available.
func (s *Sender) Configured() bool {
	return s != nil && s.impl != nil
}

// SendEmail delivers an HTML message.
func (s *Sender) SendEmail(ctx context.Context, sender *EmailSender, to []EmailRecipient, subject string, htmlContent string) error {
	if s == nil || s.impl == nil {
		return fmt.Errorf("email sender not configured")
	}
	return s.impl.SendEmail(ctx, sender, to, subject, htmlContent)
}
