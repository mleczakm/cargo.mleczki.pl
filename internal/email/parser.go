package email

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// TransferNotification represents parsed transfer data from an email.
type TransferNotification struct {
	AccountNumber string
	Sender        string
	Title         string
	Amount        string
	Date          string
}

// Parser handles parsing bank transfer notification emails.
type Parser struct{}

// NewParser creates a new email parser.
func NewParser() *Parser {
	return &Parser{}
}

// ParseTransferNotification parses a bank transfer notification email.
// It extracts account number, sender, title, and amount from the email subject and body.
func (p *Parser) ParseTransferNotification(subject, body string) (*TransferNotification, error) {
	// Polish bank emails typically have format:
	// Subject: "Uznanie rachunku - Kwota: 123,45 PLN - Nadawca: Jan Kowalski"
	// Body contains details like account number, title, etc.

	notification := &TransferNotification{}

	// Extract amount from subject (format: "Kwota: 123,45 PLN")
	amountRegex := regexp.MustCompile(`Kwota:\s*([\d\s,]+)\s*PLN`)
	if match := amountRegex.FindStringSubmatch(subject); len(match) > 1 {
		notification.Amount = strings.ReplaceAll(match[1], " ", "")
	}

	// Extract sender from subject (format: "Nadawca: Jan Kowalski")
	senderRegex := regexp.MustCompile(`Nadawca:\s*(.+?)(?:\s*-\s*|$)`)
	if match := senderRegex.FindStringSubmatch(subject); len(match) > 1 {
		notification.Sender = strings.TrimSpace(match[1])
	}

	// Extract account number from body (Polish account numbers are 26 digits)
	accountRegex := regexp.MustCompile(`\b\d{2}\s*\d{4}\s*\d{4}\s*\d{4}\s*\d{4}\s*\d{4}\d{2}\b`)
	if match := accountRegex.FindStringSubmatch(body); len(match) > 0 {
		notification.AccountNumber = strings.ReplaceAll(match[0], " ", "")
	}

	// Extract title from body (look for "Tytuł:" or "Tytul:")
	titleRegex := regexp.MustCompile(`(?i)tytu[łl]:\s*(.+?)(?:\n|$)`)
	if match := titleRegex.FindStringSubmatch(body); len(match) > 1 {
		notification.Title = strings.TrimSpace(match[1])
	}

	// If title not found in body, try to extract from subject
	if notification.Title == "" {
		// Sometimes title is in subject after "Tytuł:"
		titleRegex = regexp.MustCompile(`(?i)tytu[łl]:\s*(.+?)(?:\s*-\s*|$)`)
		if match := titleRegex.FindStringSubmatch(subject); len(match) > 1 {
			notification.Title = strings.TrimSpace(match[1])
		}
	}

	// Validate that we have the required fields
	if notification.Amount == "" {
		return nil, fmt.Errorf("could not extract amount from email")
	}

	return notification, nil
}

// NormalizeAmount normalizes amount string to a standard format (e.g., "123,45" -> "123.45").
func (p *Parser) NormalizeAmount(amount string) string {
	// Replace comma with dot for decimal separator
	normalized := strings.ReplaceAll(amount, ",", ".")
	// Remove any spaces
	normalized = strings.ReplaceAll(normalized, " ", "")
	return normalized
}

// ParseAmountToGrosz converts amount string to grosz (integer cents).
// For example, "123.45" -> 12345 grosz.
func (p *Parser) ParseAmountToGrosz(amount string) (int, error) {
	normalized := p.NormalizeAmount(amount)

	// Parse as float
	amountFloat, err := strconv.ParseFloat(normalized, 64)
	if err != nil {
		return 0, fmt.Errorf("failed to parse amount: %w", err)
	}

	// Convert to grosz (multiply by 100)
	grosz := int(amountFloat * 100)
	return grosz, nil
}
