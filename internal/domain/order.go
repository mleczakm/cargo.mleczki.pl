package domain

import (
	"time"
)

// Order Aggregate.
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
	ProductID      string  `json:"productId"`
	ProductName    string  `json:"productName"`
	BasePrice      int     `json:"basePrice"`
	SelectedAddons []Addon `json:"selectedAddons"`
	RentalDays     int     `json:"rentalDays"`
}

type Addon struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Price int    `json:"price"`
}

// Order Commands.
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

// Order Events.
type OrderPlacedEvent struct {
	OrderID       string      `json:"orderId"`
	UserID        string      `json:"userId"`
	Items         []OrderItem `json:"items"`
	TotalAmount   int         `json:"totalAmount"`
	PaymentMethod string      `json:"paymentMethod"`
	StartDate     string      `json:"startDate"`
	EndDate       string      `json:"endDate"`
	RentalDays    int         `json:"rentalDays"`
	Timestamp     time.Time   `json:"timestamp"`
}

func (e *OrderPlacedEvent) EventType() string {
	return "OrderPlaced"
}

type OrderPaidEvent struct {
	OrderID   string    `json:"orderId"`
	Method    string    `json:"method"`
	Timestamp time.Time `json:"timestamp"`
}

func (e *OrderPaidEvent) EventType() string {
	return "OrderPaid"
}

type OrderCancelledEvent struct {
	OrderID   string    `json:"orderId"`
	Timestamp time.Time `json:"timestamp"`
}

func (e *OrderCancelledEvent) EventType() string {
	return "OrderCancelled"
}
