package email_test

import (
	"strings"
	"testing"

	"cargo.mleczki.pl/internal/email"
)

func TestRenderPasswordResetIsPolishWithLinkAndAuthorFooter(t *testing.T) {
	t.Setenv("MAIL_REPLY_TO", "")
	html, err := email.RenderPasswordReset("https://cargo.mleczki.pl/reset-password?token=a&b=<x>")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"Reset hasła", "Ustaw nowe hasło", "ważny przez 1 godzinę", "English:",
		"https://cargo.mleczki.pl/reset-password?token=a&amp;b=%3cx%3e",
		"Michał Mleczko", "mailto:do@mleczki.pl",
	} {
		if !strings.Contains(html, want) {
			t.Errorf("reset email missing %q", want)
		}
	}
	if strings.Contains(html, "<x>") {
		t.Error("link must be escaped")
	}
}

func TestRenderOrderNoticeEscapesCustomerInput(t *testing.T) {
	html, err := email.RenderOrderNotice(email.OrderNotice{
		OrderID: "ORD-1", UserName: "<script>alert(1)</script>", UserEmail: "a@b.pl", PaymentMethod: "cash_pickup", Amount: 150,
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(html, "<script>") || !strings.Contains(html, "150.00 zł") || !strings.Contains(html, "Michał Mleczko") {
		t.Errorf("unexpected notice html: %s", html)
	}
}
