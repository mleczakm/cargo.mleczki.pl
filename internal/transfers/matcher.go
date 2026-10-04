package transfers

import (
	sharedpayments "github.com/mleczakm/blik-phone-payments-go"
)

// Matcher handles matching transfers to orders by payment code.
type Matcher struct{}

// NewMatcher creates a new transfer payment matcher.
func NewMatcher() *Matcher {
	return &Matcher{}
}

// MatchByPaymentCode attempts to match a transfer to an order by payment code.
// It tokenizes the transfer title and looks for matching payment codes.
// Returns the payment code if found, empty string otherwise.
func (m *Matcher) MatchByPaymentCode(title string, existingCodes []string) string {
	return sharedpayments.FindCode(title, existingCodes)
}

// IsValidPaymentCode checks if a string is a valid payment code format.
// Payment codes are 4 characters from the set: 0-9, A-Z (excluding I and O).
func (m *Matcher) IsValidPaymentCode(code string) bool {
	return sharedpayments.IsValidCode(code)
}
