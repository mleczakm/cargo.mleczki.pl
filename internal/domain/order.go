package domain

import (
	"time"
)

// Order Aggregate
type Order struct {
	ID            string
	UserID        string
	Items         []OrderItem
	TotalAmount   int
	Status        string // "pending_payment", "paid", "cancelled", "accepted"
	PaymentMethod string // "blik", "cash"
	StartDate     string
	EndDate       string
	RentalDays    int
	CreatedAt     time.Time
	UpdatedAt     time.Time
	Version       int
}

type OrderItem struct {
	ProductID      string
	ProductName    string
	BasePrice      int
	SelectedAddons []Addon
	RentalDays     int
}

type Addon struct {
	ID    string
	Name  string
	Price int
}

// Order Commands
type PlaceOrderCommand struct {
	OrderID       string
	UserID        string
	Items         []OrderItem
	TotalAmount   int
	PaymentMethod string
	StartDate     string
	EndDate       string
	RentalDays    int
}

type MarkAsPaidCommand struct {
	OrderID string
	Method  string
}

type CancelOrderCommand struct {
	OrderID string
}

// Order Events
type OrderPlacedEvent struct {
	OrderID       string      `json:"order_id"`
	UserID        string      `json:"user_id"`
	Items         []OrderItem `json:"items"`
	TotalAmount   int         `json:"total_amount"`
	PaymentMethod string      `json:"payment_method"`
	StartDate     string      `json:"start_date"`
	EndDate       string      `json:"end_date"`
	RentalDays    int         `json:"rental_days"`
	Timestamp     time.Time   `json:"timestamp"`
}

func (e *OrderPlacedEvent) EventType() string {
	return "OrderPlaced"
}

type OrderPaidEvent struct {
	OrderID   string    `json:"order_id"`
	Method    string    `json:"method"`
	Timestamp time.Time `json:"timestamp"`
}

func (e *OrderPaidEvent) EventType() string {
	return "OrderPaid"
}

type OrderCancelledEvent struct {
	OrderID   string    `json:"order_id"`
	Timestamp time.Time `json:"timestamp"`
}

func (e *OrderCancelledEvent) EventType() string {
	return "OrderCancelled"
}
