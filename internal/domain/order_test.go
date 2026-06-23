package domain_test

import (
	"testing"
	"time"

	"cargo.mleczki.pl/internal/domain"
)

func TestOrderPlacedEvent_EventType(t *testing.T) {
	event := &domain.OrderPlacedEvent{
		OrderID:       "ORD-123",
		UserID:        "user-123",
		Items:         []domain.OrderItem{},
		TotalAmount:   100,
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
