package domain_test

import (
	"testing"

	"cargo.mleczki.pl/internal/domain"
)

func TestGeneratePaymentCode(t *testing.T) {
	code, err := domain.GeneratePaymentCode()
	if err != nil {
		t.Fatal(err)
	}
	if len(code) != domain.CodeLength {
		t.Fatalf("code %q has length %d", code, len(code))
	}
	for _, r := range code {
		if r == 'I' || r == 'O' {
			t.Fatalf("generated ambiguous character in %q", code)
		}
	}
}
