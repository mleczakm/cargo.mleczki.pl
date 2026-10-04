package domain

import (
	"time"

	sharedpayments "github.com/mleczakm/blik-phone-payments-go"
)

const (
	CodeLength = sharedpayments.CodeLength
	// Characters used for payment codes (excluding I and O for readability).
	CodeChars = sharedpayments.CodeChars
)

// PaymentCode represents a unique payment code for matching transfers.
type PaymentCode struct {
	ID        string
	Code      string
	OrderID   string
	CreatedAt time.Time
	Version   int
}

// PaymentCode Commands.
type GeneratePaymentCodeCommand struct {
	PaymentCodeID string
	OrderID       string
}

// PaymentCode Events.
type PaymentCodeGeneratedEvent struct {
	PaymentCodeID string    `json:"paymentCodeId"`
	Code          string    `json:"code"`
	OrderID       string    `json:"orderId"`
	Timestamp     time.Time `json:"timestamp"`
}

func (e *PaymentCodeGeneratedEvent) EventType() string {
	return "PaymentCodeGenerated"
}

// GeneratePaymentCode generates a random 4-character payment code.
func GeneratePaymentCode() (string, error) {
	return sharedpayments.GenerateCode()
}
