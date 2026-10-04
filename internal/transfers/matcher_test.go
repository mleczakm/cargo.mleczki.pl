package transfers_test

import (
	"testing"

	"cargo.mleczki.pl/internal/transfers"
)

func TestMatcherUsesSharedCodeMatchingAndValidation(t *testing.T) {
	matcher := transfers.NewMatcher()
	codes := []string{"AKWU", "10ZX"}
	for title, want := range map[string]string{"zamówienie akwu": "AKWU", "płatność IOZX": "10ZX", "AKWUX": ""} {
		if got := matcher.MatchByPaymentCode(title, codes); got != want {
			t.Errorf("MatchByPaymentCode(%q) = %q, want %q", title, got, want)
		}
	}
	for code, want := range map[string]bool{"A1Z9": true, "io00": false, "A12345": false, "ĄBCD": false} {
		if got := matcher.IsValidPaymentCode(code); got != want {
			t.Errorf("IsValidPaymentCode(%q) = %v, want %v", code, got, want)
		}
	}
}
