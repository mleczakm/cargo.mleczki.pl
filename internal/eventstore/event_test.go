package eventstore_test

import (
	"database/sql"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"cargo.mleczki.pl/internal/eventstore"

	_ "modernc.org/sqlite"
)

type testEvent struct {
	Timestamp time.Time
}

func (e *testEvent) EventType() string {
	return "TestEvent"
}

func TestToEventSetsUniqueID(t *testing.T) {
	event, err := eventstore.ToEvent("ORD-123", "order", &testEvent{Timestamp: time.Now().UTC()}, 1)
	if err != nil {
		t.Fatalf("ToEvent: %v", err)
	}
	if event.ID != "ORD-123-TestEvent-v1" {
		t.Fatalf("expected ORD-123-TestEvent-v1, got %q", event.ID)
	}
}

func TestSQLiteEventStoreMigrateEmptyEventIDs(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "legacy.db")

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}

	_, err = db.Exec(`
		CREATE TABLE events (
			id TEXT PRIMARY KEY,
			aggregate_id TEXT NOT NULL,
			aggregate_type TEXT NOT NULL,
			event_type TEXT NOT NULL,
			payload BLOB NOT NULL,
			version INTEGER NOT NULL,
			created_at TEXT NOT NULL
		);
	`)
	if err != nil {
		t.Fatalf("create events table: %v", err)
	}

	_, err = db.Exec(`
		INSERT INTO events (id, aggregate_id, aggregate_type, event_type, payload, version, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, "", "ORD-legacy", "order", "OrderPlaced", []byte("{}"), 1, "2026-01-01T00:00:00Z")
	if err != nil {
		t.Fatalf("seed legacy event: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close legacy db: %v", err)
	}

	store, err := eventstore.NewSQLiteEventStore(dbPath)
	if err != nil {
		t.Fatalf("NewSQLiteEventStore: %v", err)
	}
	defer store.Close()

	events, err := store.GetEvents(t.Context(), "ORD-legacy")
	if err != nil {
		t.Fatalf("GetEvents: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].ID != "ORD-legacy-OrderPlaced-v1" {
		t.Fatalf("expected migrated id ORD-legacy-OrderPlaced-v1, got %q", events[0].ID)
	}
}

func TestGetEventsSinceUsesStreamPosition(t *testing.T) {
	store, err := eventstore.NewSQLiteEventStore(":memory:")
	if err != nil {
		t.Fatalf("NewSQLiteEventStore: %v", err)
	}
	defer store.Close()

	ctx := t.Context()
	payload, err := json.Marshal(map[string]string{"order": "1"})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	for i := 0; i < 3; i++ {
		event := &eventstore.Event{
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
