package eventstore

import (
	"testing"
	"time"
)

type testEvent struct {
	Timestamp time.Time
}

func (e *testEvent) EventType() string {
	return "TestEvent"
}

func TestToEventSetsUniqueID(t *testing.T) {
	event, err := ToEvent("ORD-123", "order", &testEvent{Timestamp: time.Now().UTC()}, 1)
	if err != nil {
		t.Fatalf("ToEvent: %v", err)
	}
	if event.ID != "ORD-123-TestEvent-v1" {
		t.Fatalf("expected ORD-123-TestEvent-v1, got %q", event.ID)
	}
}

func TestSQLiteEventStoreMigrateEmptyEventIDs(t *testing.T) {
	store, err := NewSQLiteEventStore(":memory:")
	if err != nil {
		t.Fatalf("NewSQLiteEventStore: %v", err)
	}
	defer store.Close()

	_, err = store.db.Exec(`
		INSERT INTO events (id, aggregate_id, aggregate_type, event_type, payload, version, created_at)
		VALUES ('', 'ORD-legacy', 'order', 'OrderPlaced', '{}', 1, '2026-01-01T00:00:00Z')
	`)
	if err != nil {
		t.Fatalf("insert legacy event: %v", err)
	}

	if err := store.migrateEmptyEventIDs(); err != nil {
		t.Fatalf("migrateEmptyEventIDs: %v", err)
	}

	var id string
	err = store.db.QueryRow(`SELECT id FROM events WHERE aggregate_id = 'ORD-legacy'`).Scan(&id)
	if err != nil {
		t.Fatalf("query migrated event: %v", err)
	}
	if id != "ORD-legacy-OrderPlaced-v1" {
		t.Fatalf("expected migrated id ORD-legacy-OrderPlaced-v1, got %q", id)
	}
}
