package eventstore

import (
	"encoding/json"
	"testing"
)

func TestGetEventsSinceUsesStreamPosition(t *testing.T) {
	store, err := NewSQLiteEventStore(":memory:")
	if err != nil {
		t.Fatalf("NewSQLiteEventStore: %v", err)
	}
	defer store.Close()

	ctx := t.Context()
	payload, _ := json.Marshal(map[string]string{"order": "1"})

	for i := 0; i < 3; i++ {
		event := &Event{
			ID:            "event-" + string(rune('a'+i)),
			AggregateID:   "ORD-" + string(rune('a'+i)),
			AggregateType: "order",
			EventType:     "OrderPlaced",
			Payload:       payload,
			Version:       1,
		}
		if err := store.Save(ctx, event); err != nil {
			t.Fatalf("Save event %d: %v", i, err)
		}
	}

	events, err := store.GetEventsSince(ctx, 1)
	if err != nil {
		t.Fatalf("GetEventsSince: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("expected 2 events after position 1, got %d", len(events))
	}
	if events[0].StreamPosition != 2 || events[1].StreamPosition != 3 {
		t.Fatalf("unexpected stream positions: %d, %d", events[0].StreamPosition, events[1].StreamPosition)
	}
}
