package domain

import (
	"crypto/rand"
	"math/big"
	"time"
)

const (
	CodeLength = 4
	// Characters used for payment codes (excluding I and O for readability).
	CodeChars = "0123456789ABCDEFGHJKLMNPQRSTUVWXYZ"
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
func GeneratePaymentCode() string {
	maxVal := big.NewInt(int64(len(CodeChars)))
	code := ""
	for i := 0; i < CodeLength; i++ {
		n, _ := rand.Int(rand.Reader, maxVal)
		code += string(CodeChars[n.Int64()])
	}
	return code
}
