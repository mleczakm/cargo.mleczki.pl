package email_test

import (
	"testing"

	"cargo.mleczki.pl/internal/email"
)

func TestSMTPHostFromEnv(t *testing.T) {
	t.Setenv("SMTP_HOST", "")
	t.Setenv("MAILPIT", "")

	if got := email.SMTPHostFromEnv(); got != "" {
		t.Fatalf("expected empty host, got %q", got)
	}

	t.Setenv("MAILPIT", "1")
	if got := email.SMTPHostFromEnv(); got != "localhost" {
		t.Fatalf("expected localhost for MAILPIT=1, got %q", got)
	}

	t.Setenv("MAILPIT", "")
	t.Setenv("SMTP_HOST", "mailpit")
	if got := email.SMTPHostFromEnv(); got != "mailpit" {
		t.Fatalf("expected explicit SMTP_HOST, got %q", got)
	}
}

func TestDefaultSenderEmail(t *testing.T) {
	t.Setenv("BREVO_SENDER_EMAIL", "")
	t.Setenv("SMTP_FROM", "")

	if got := email.DefaultSenderEmail(); got != "noreply@cargo.mleczki.pl" {
		t.Fatalf("expected default sender, got %q", got)
	}

	t.Setenv("SMTP_FROM", "dev@example.com")
	if got := email.DefaultSenderEmail(); got != "dev@example.com" {
		t.Fatalf("expected SMTP_FROM to win over default, got %q", got)
	}

	t.Setenv("BREVO_SENDER_EMAIL", "prod@example.com")
	if got := email.DefaultSenderEmail(); got != "prod@example.com" {
		t.Fatalf("expected BREVO_SENDER_EMAIL to take precedence, got %q", got)
	}
}

func TestNewMailerPrefersSMTP(t *testing.T) {
	t.Setenv("SMTP_HOST", "localhost")
	t.Setenv("SMTP_PORT", "1025")
	t.Setenv("BREVO_API_KEY", "test-key-should-not-be-used")

	mailer, err := email.NewMailer()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !mailer.Configured() {
		t.Fatal("expected configured SMTP mailer")
	}
}

func TestNewMailerUnconfigured(t *testing.T) {
	t.Setenv("SMTP_HOST", "")
	t.Setenv("MAILPIT", "")
	t.Setenv("BREVO_API_KEY", "")

	mailer, err := email.NewMailer()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if mailer.Configured() {
		t.Fatal("expected unconfigured mailer")
	}
}
