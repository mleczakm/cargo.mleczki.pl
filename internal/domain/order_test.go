package domain_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"cargo.mleczki.pl/internal/domain"
)

func TestOrderPlacedEvent_EventType(t *testing.T) {
	event := &domain.OrderPlacedEvent{
		OrderID:       "ORD-123",
		UserID:        "user-123",
		Items:         []domain.OrderItem{},
		TotalAmount:   domain.Zloty(100),
		PaymentMethod: domain.PaymentMethodCashPickup,
		PaymentCode:   nil,
		StartDate:     "2024-01-01",
		EndDate:       "2024-01-02",
		RentalDays:    1,
		IsFirstOrder:  true,
		Timestamp:     time.Now().UTC(),
	}

	eventType := event.EventType()
	if eventType != "OrderPlaced" {
		t.Errorf("Expected OrderPlaced, got %s", eventType)
	}
}

func TestOrderPaidEvent_EventType(t *testing.T) {
	event := &domain.OrderPaidEvent{
		OrderID:   "ORD-123",
		Method:    "admin_manual",
		Timestamp: time.Now().UTC(),
	}

	eventType := event.EventType()
	if eventType != "OrderPaid" {
		t.Errorf("Expected OrderPaid, got %s", eventType)
	}
}

func TestOrderCancelledEvent_EventType(t *testing.T) {
	event := &domain.OrderCancelledEvent{
		OrderID:   "ORD-123",
		Timestamp: time.Now().UTC(),
	}

	eventType := event.EventType()
	if eventType != "OrderCancelled" {
		t.Errorf("Expected OrderCancelled, got %s", eventType)
	}
}

func TestOrderConfirmedEvent_EventType(t *testing.T) {
	event := &domain.OrderConfirmedEvent{
		OrderID:   "ORD-123",
		Timestamp: time.Now().UTC(),
	}

	eventType := event.EventType()
	if eventType != "OrderConfirmed" {
		t.Errorf("Expected OrderConfirmed, got %s", eventType)
	}
}

func TestOrderStatusConstants(t *testing.T) {
	tests := []struct {
		status   domain.OrderStatus
		expected string
	}{
		{domain.StatusPending, "pending"},
		{domain.StatusAwaitingPayment, "awaiting_payment"},
		{domain.StatusPaid, "paid"},
		{domain.StatusConfirmed, "confirmed"},
		{domain.StatusRealized, "realized"},
		{domain.StatusCancelled, "cancelled"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			if string(tt.status) != tt.expected {
				t.Errorf("Expected %s, got %s", tt.expected, tt.status)
			}
		})
	}
}

func TestOrderPlacedEventReadsEventsStoredBeforeDecimals(t *testing.T) {
	legacy := `{"orderId":"ORD-1","items":[{"productId":"cargo","productName":"Cargo","basePrice":150,
		"selectedAddons":[{"id":"daszek","name":"Daszek","price":15,"paymentType":"daily"}],"rentalDays":2}],"totalAmount":330}`

	var event domain.OrderPlacedEvent
	if err := json.Unmarshal([]byte(legacy), &event); err != nil {
		t.Fatal(err)
	}
	if event.TotalAmount != domain.Zloty(330) || event.Items[0].BasePrice != domain.Zloty(150) || event.Items[0].SelectedAddons[0].Price != domain.Zloty(15) {
		t.Errorf("legacy amounts were misread: %+v", event)
	}
	if got := event.Items[0].Total(); got != domain.Zloty(330) {
		t.Errorf("(150 + 15) × 2 = 330, got %s", got)
	}

	out, err := json.Marshal(event)
	if err != nil || !strings.Contains(string(out), `"totalAmount":"330.00"`) {
		t.Errorf("new events store decimal strings: %s %v", out, err)
	}
}

func TestOrderItemTotals(t *testing.T) {
	rental := domain.OrderItem{
		BasePrice: domain.PLN(8050), RentalDays: 3,
		SelectedAddons: []domain.Addon{
			{Price: domain.PLN(1550), PaymentType: domain.AddonPaidDaily},
			{Price: domain.PLN(5000), PaymentType: domain.AddonPaidOneTime},
			{Price: domain.PLN(500)}, // no type: daily
		},
	}
	// (80.50 + 15.50 + 5) × 3 + 50
	if got := rental.Total(); got != domain.PLN(35300) {
		t.Errorf("rental total = %s", got)
	}
	shopLine := domain.OrderItem{Kind: domain.OrderItemKindShop, BasePrice: domain.PLN(1234), Quantity: 3}
	if got := shopLine.Total(); got != domain.PLN(3702) {
		t.Errorf("shop total = %s", got)
	}
}
