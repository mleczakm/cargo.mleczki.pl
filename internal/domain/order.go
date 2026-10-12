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

const (
	PaymentMethodBlik       = "blik"
	PaymentMethodCashPickup = "cash_pickup"
	PaymentMethodBlikPickup = "blik_pickup"
)

// Order Aggregate.
type Order struct {
	ID            string
	UserID        string
	Items         []OrderItem
	TotalAmount   Money
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

// OrderItemKindShop marks an order line that sells a shop item. Lines without a
// kind are rentals, which is how every order placed before the shop was stored.
const OrderItemKindShop = "shop"

// OrderItem is a single order line: either a rental or a shop purchase.
// A rental has its own StartDate/EndDate (orders placed before multi-line dates
// existed only carry them on the order). For shop lines ProductID holds the shop
// item ID, BasePrice the unit price and RentalDays is zero.
type OrderItem struct {
	ProductID      string  `json:"productId"`
	ProductName    string  `json:"productName"`
	BasePrice      Money   `json:"basePrice"`
	SelectedAddons []Addon `json:"selectedAddons"`
	RentalDays     int     `json:"rentalDays"`
	StartDate      string  `json:"startDate,omitempty"`
	EndDate        string  `json:"endDate,omitempty"`
	Kind           string  `json:"kind,omitempty"`
	SKU            string  `json:"sku,omitempty"`
	Quantity       int     `json:"quantity,omitempty"`
}

// IsShop reports whether the line is a shop purchase rather than a rental.
func (i OrderItem) IsShop() bool { return i.Kind == OrderItemKindShop }

// Total is what the line costs.
func (i OrderItem) Total() Money {
	if i.IsShop() {
		return i.BasePrice.Mul(i.Quantity)
	}
	return RentalPrice(i.BasePrice, i.SelectedAddons, i.RentalDays)
}

// Payment types of rental add-ons.
const (
	AddonPaidDaily   = "daily"    // charged for every rental day
	AddonPaidOneTime = "one-time" // charged once per rental
)

// RentalPrice prices a rental: the base price and the per-day add-ons are
// multiplied by the number of days, one-time add-ons are added once.
func RentalPrice(base Money, addons []Addon, days int) Money {
	daily, once := base, Money{}
	for _, addon := range addons {
		if addon.PaymentType == AddonPaidOneTime {
			once = once.Add(addon.Price)
		} else {
			daily = daily.Add(addon.Price) // daily is the default, as in the product files
		}
	}
	return daily.Mul(days).Add(once)
}

type Addon struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Price       Money  `json:"price"`
	PaymentType string `json:"paymentType"`
}

// Order Commands.
type PlaceOrderCommand struct {
	OrderID       string
	UserID        string
	Items         []OrderItem
	TotalAmount   Money
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
	TotalAmount   Money       `json:"totalAmount"`
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

// OrderRealizedEvent marks the order as handed over / completed.
type OrderRealizedEvent struct {
	OrderID   string    `json:"orderId"`
	Timestamp time.Time `json:"timestamp"`
}

func (e *OrderRealizedEvent) EventType() string {
	return "OrderRealized"
}
