package transfers

import (
	"regexp"
	"strings"
	"unicode"
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
	// Tokenize the title and check each token against existing codes
	tokens := m.tokenizeTitle(title)

	for _, token := range tokens {
		for _, code := range existingCodes {
			if strings.EqualFold(token, code) {
				return code
			}
		}
	}

	return ""
}

// tokenizeTitle splits the title into tokens and generates variations
// to handle common OCR/input errors (like 0/O substitutions).
func (m *Matcher) tokenizeTitle(title string) []string {
	// Remove special characters and convert to uppercase
	cleaned := regexp.MustCompile(`[^A-Za-z0-9]`).ReplaceAllString(title, " ")
	cleaned = strings.ToUpper(strings.TrimSpace(cleaned))

	// Split into tokens
	tokens := strings.Fields(cleaned)

	// Generate variations for each token
	variations := make(map[string]bool)
	for _, token := range tokens {
		if token == "" {
			continue
		}

		// Add the original token
		variations[token] = true

		// Add 0/O substitutions
		withO := strings.ReplaceAll(token, "0", "O")
		if withO != token {
			variations[withO] = true
		}

		with0 := strings.ReplaceAll(token, "O", "0")
		if with0 != token {
			variations[with0] = true
		}

		// Add both substitutions
		swapped := strings.ReplaceAll(strings.ReplaceAll(token, "0", "O"), "O", "0")
		if swapped != token {
			variations[swapped] = true
		}
	}

	// Convert map to slice
	result := make([]string, 0, len(variations))
	for token := range variations {
		result = append(result, token)
	}

	return result
}

// IsValidPaymentCode checks if a string is a valid payment code format.
// Payment codes are 4 characters from the set: 0-9, A-Z (excluding I and O).
func (m *Matcher) IsValidPaymentCode(code string) bool {
	if len(code) != 4 {
		return false
	}

	for _, r := range strings.ToUpper(code) {
		if !unicode.IsDigit(r) && !unicode.IsUpper(r) {
			return false
		}
		// Exclude I and O for readability
		if r == 'I' || r == 'O' {
			return false
		}
	}

	return true
}
