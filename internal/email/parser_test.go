package email_test

import (
	"testing"

	"cargo.mleczki.pl/internal/email"
)

func TestNewParser(t *testing.T) {
	parser := email.NewParser()
	if parser == nil {
		t.Fatal("expected NewParser to return a non-nil *Parser")
	}
}

func TestParseTransferNotification(t *testing.T) {
	parser := email.NewParser()

	subject := "Uznanie rachunku - Kwota: 123,45 PLN - Nadawca: Jan Kowalski"
	body := "Tytuł: Invoice 123\nAccount number: 12 3456 7890 1234 5678 9012 3456"

	notification, err := parser.ParseTransferNotification(subject, body)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if notification == nil {
		t.Fatal("expected a non-nil notification")
	}

	if notification.Amount != "123,45" {
		t.Errorf("expected amount '123,45', got '%s'", notification.Amount)
	}

	if notification.Sender != "Jan Kowalski" {
		t.Errorf("expected sender 'Jan Kowalski', got '%s'", notification.Sender)
	}

	if notification.AccountNumber != "12345678901234567890123456" {
		t.Errorf("expected account number '12345678901234567890123456', got '%s'", notification.AccountNumber)
	}

	if notification.Title != "Invoice 123" {
		t.Errorf("expected title 'Invoice 123', got '%s'", notification.Title)
	}
}
