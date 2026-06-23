package email_test

import (
	"strings"
	"testing"

	"cargo.mleczki.pl/internal/email"
)

func TestBuildHTMLMessage(t *testing.T) {
	message := string(email.BuildHTMLMessageForTest(
		"Cargo Mleczki <noreply@cargo.mleczki.pl>",
		"jan@example.com",
		"Reset your password",
		"<p>Hello</p>",
	))

	for _, expected := range []string{
		"From: Cargo Mleczki <noreply@cargo.mleczki.pl>",
		"To: jan@example.com",
		"Subject: Reset your password",
		"Content-Type: text/html; charset=UTF-8",
		"<p>Hello</p>",
	} {
		if !strings.Contains(message, expected) {
			t.Fatalf("expected message to contain %q, got:\n%s", expected, message)
		}
	}
}

func TestSMTPClientSendEmailValidation(t *testing.T) {
	client := email.NewSMTPClient("localhost", "1025")

	err := client.SendEmail(t.Context(), nil, nil, "subject", "<p>body</p>")
	if err == nil || !strings.Contains(err.Error(), "sender email is required") {
		t.Fatalf("expected sender validation error, got %v", err)
	}

	err = client.SendEmail(
		t.Context(),
		&email.EmailSender{Name: "Cargo", Email: "noreply@cargo.mleczki.pl"},
		nil,
		"subject",
		"<p>body</p>",
	)
	if err == nil || !strings.Contains(err.Error(), "at least one recipient is required") {
		t.Fatalf("expected recipient validation error, got %v", err)
	}
}
