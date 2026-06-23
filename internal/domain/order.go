package domain

import (
	"time"
)

// OrderStatus represents the state of an order in the state machine.
type OrderStatus string

const (
	StatusPending         OrderStatus = "pending"          // Order created, not yet submitted
	StatusAwaitingPayment OrderStatus = "awaiting_payment" // Order submitted, waiting for payment
	StatusPaid            OrderStatus = "paid"             // Payment received
	StatusConfirmed       OrderStatus = "confirmed"        // Order confirmed (ready for rental)
	StatusRealized        OrderStatus = "realized"         // Rental completed (final state)
	StatusCancelled       OrderStatus = "cancelled"        // Order cancelled
)

// Order Aggregate.
type Order struct {
	ID            string
	UserID        string
	Items         []OrderItem
	TotalAmount   int
	Status        OrderStatus
	PaymentMethod string  // "blik", "cash_pickup"
	PaymentCode   *string // 4-character code for BLIK transfers
	StartDate     string
	EndDate       string
	RentalDays    int
	IsFirstOrder  bool // true if this is the client's first order
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
	IsFirstOrder  bool
}

type ConfirmOrderCommand struct {
	OrderID string
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
	PaymentCode   *string     `json:"paymentCode,omitempty"`
	StartDate     string      `json:"startDate"`
	EndDate       string      `json:"endDate"`
	RentalDays    int         `json:"rentalDays"`
	IsFirstOrder  bool        `json:"isFirstOrder"`
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

type OrderConfirmedEvent struct {
	OrderID   string    `json:"orderId"`
	Timestamp time.Time `json:"timestamp"`
}

func (e *OrderConfirmedEvent) EventType() string {
	return "OrderConfirmed"
}
