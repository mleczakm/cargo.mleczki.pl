package eventstore

import (
	"context"
	"encoding/json"
	"testing"
)

func TestSQLiteEventStore_Save(t *testing.T) {
	// Create in-memory database for testing
	store, err := NewSQLiteEventStore(":memory:")
	if err != nil {
		t.Fatalf("Failed to create event store: %v", err)
	}
	defer store.Close()
	
	// Create test event
	data := map[string]interface{}{"test": "data"}
	payload, _ := json.Marshal(data)
	
	event := &Event{
		ID:            "test-event-1",
		AggregateID:   "aggregate-1",
		AggregateType: "Order",
		EventType:     "OrderPlaced",
		Payload:       payload,
		Version:       1,
	}
	
	// Save event
	ctx := context.Background()
	if err := store.Save(ctx, event); err != nil {
		t.Fatalf("Save failed: %v", err)
	}
	
	// Verify event was saved
	events, err := store.GetEvents(ctx, "aggregate-1")
	if err != nil {
		t.Fatalf("GetEvents failed: %v", err)
	}
	
	if len(events) != 1 {
		t.Errorf("Expected 1 event, got %d", len(events))
	}
	
	if events[0].ID != "test-event-1" {
		t.Errorf("Expected event ID test-event-1, got %s", events[0].ID)
	}
}

func TestSQLiteEventStore_GetEvents(t *testing.T) {
	// Create in-memory database for testing
	store, err := NewSQLiteEventStore(":memory:")
	if err != nil {
		t.Fatalf("Failed to create event store: %v", err)
	}
	defer store.Close()
	
	// Create and save multiple events for same aggregate
	aggregateID := "test-aggregate"
	ctx := context.Background()
	
	for i := 0; i < 3; i++ {
		data := map[string]interface{}{"index": i}
		payload, _ := json.Marshal(data)
		
		event := &Event{
			ID:            "test-event-" + string(rune('0'+i)),
			AggregateID:   aggregateID,
			AggregateType: "Order",
			EventType:     "OrderPlaced",
			Payload:       payload,
			Version:       i + 1,
		}
		if err := store.Save(ctx, event); err != nil {
			t.Fatalf("Save failed: %v", err)
		}
	}
	
	// Get events by aggregate
	events, err := store.GetEvents(ctx, aggregateID)
	if err != nil {
		t.Fatalf("GetEvents failed: %v", err)
	}
	
	if len(events) != 3 {
		t.Errorf("Expected 3 events, got %d", len(events))
	}
}

func TestSQLiteEventStore_GetEventsByType(t *testing.T) {
	// Create in-memory database for testing
	store, err := NewSQLiteEventStore(":memory:")
	if err != nil {
		t.Fatalf("Failed to create event store: %v", err)
	}
	defer store.Close()
	
	// Save events of different types
	eventTypes := []string{"OrderPlaced", "OrderPaid", "OrderCancelled"}
	ctx := context.Background()
	
	for i, eventType := range eventTypes {
		data := map[string]interface{}{"type": eventType}
		payload, _ := json.Marshal(data)
		
		event := &Event{
			ID:            "test-event-" + string(rune('0'+i)),
			AggregateID:   "aggregate-" + string(rune('0'+i)),
			AggregateType: "Order",
			EventType:     eventType,
			Payload:       payload,
			Version:       1,
		}
		if err := store.Save(ctx, event); err != nil {
			t.Fatalf("Save failed: %v", err)
		}
	}
	
	// Get events by type
	events, err := store.GetEventsByType(ctx, "OrderPlaced")
	if err != nil {
		t.Fatalf("GetEventsByType failed: %v", err)
	}
	
	if len(events) != 1 {
		t.Errorf("Expected 1 event of type OrderPlaced, got %d", len(events))
	}
	
	if events[0].EventType != "OrderPlaced" {
		t.Errorf("Expected event type OrderPlaced, got %s", events[0].EventType)
	}
}
