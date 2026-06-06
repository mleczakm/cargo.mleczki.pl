package domain

import "time"

// Transfer Aggregate.
type Transfer struct {
	ID        string
	Date      string
	Sender    string
	Title     string
	Amount    int
	Status    string // "unmatched", "matched"
	OrderID   *string
	CreatedAt time.Time
	Version   int
}

// Transfer Commands.
type RegisterTransferCommand struct {
	TransferID string
	Date       string
	Sender     string
	Title      string
	Amount     int
}

type LinkTransferToOrderCommand struct {
	TransferID string
	OrderID    string
}

// Transfer Events.
type TransferReceivedEvent struct {
	TransferID string    `json:"transferId"`
	Date       string    `json:"date"`
	Sender     string    `json:"sender"`
	Title      string    `json:"title"`
	Amount     int       `json:"amount"`
	Timestamp  time.Time `json:"timestamp"`
}

func (e *TransferReceivedEvent) EventType() string {
	return "TransferReceived"
}

type TransferLinkedEvent struct {
	TransferID string    `json:"transferId"`
	OrderID    string    `json:"orderId"`
	Timestamp  time.Time `json:"timestamp"`
}

func (e *TransferLinkedEvent) EventType() string {
	return "TransferLinked"
}
