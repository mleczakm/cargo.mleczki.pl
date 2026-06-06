package projections_test

import (
	"encoding/json"
	"testing"

	"cargo.mleczki.pl/internal/eventstore"
	"cargo.mleczki.pl/internal/projections"
)

func TestProjector_Run(t *testing.T) {
	// Create in-memory databases
	eventStore, err := eventstore.NewSQLiteEventStore(":memory:")
	if err != nil {
		t.Fatalf("Failed to create event store: %v", err)
	}
	defer eventStore.Close()

	readModels, err := projections.NewReadModelsDB(":memory:")
	if err != nil {
		t.Fatalf("Failed to create read models: %v", err)
	}
	defer readModels.Close()

	// Create projector
	projector := projections.NewProjector(eventStore, readModels, "test")

	// Save a test event
	ctx := t.Context()
	data := map[string]interface{}{"test": "data"}
	payload, _ := json.Marshal(data)

	event := &eventstore.Event{
		ID:            "test-event-1",
		AggregateID:   "aggregate-1",
		AggregateType: "Order",
		EventType:     "OrderPlaced",
		Payload:       payload,
		Version:       1,
	}

	if err := eventStore.Save(ctx, event); err != nil {
		t.Fatalf("Failed to save event: %v", err)
	}

	// Run projector
	if err := projector.Run(ctx); err != nil {
		t.Fatalf("Projector.Run failed: %v", err)
	}

	// Verify checkpoint was updated
	checkpoint, err := readModels.GetCheckpoint("test")
	if err != nil {
		t.Fatalf("Failed to get checkpoint: %v", err)
	}

	if checkpoint == 0 {
		t.Error("Expected checkpoint to be updated, got 0")
	}
}

func TestReadModelsDB_GetCheckpoint(t *testing.T) {
	// Create in-memory database
	readModels, err := projections.NewReadModelsDB(":memory:")
	if err != nil {
		t.Fatalf("Failed to create read models: %v", err)
	}
	defer readModels.Close()

	// Get checkpoint for non-existent projector
	checkpoint, err := readModels.GetCheckpoint("non-existent")
	if err != nil {
		t.Fatalf("GetCheckpoint failed: %v", err)
	}

	if checkpoint != 0 {
		t.Errorf("Expected checkpoint 0 for non-existent projector, got %d", checkpoint)
	}
}

func TestReadModelsDB_SaveCheckpoint(t *testing.T) {
	// Create in-memory database
	readModels, err := projections.NewReadModelsDB(":memory:")
	if err != nil {
		t.Fatalf("Failed to create read models: %v", err)
	}
	defer readModels.Close()

	// Save checkpoint
	if err := readModels.SaveCheckpoint("test", 42); err != nil {
		t.Fatalf("SaveCheckpoint failed: %v", err)
	}

	// Verify checkpoint was saved
	checkpoint, err := readModels.GetCheckpoint("test")
	if err != nil {
		t.Fatalf("GetCheckpoint failed: %v", err)
	}

	if checkpoint != 42 {
		t.Errorf("Expected checkpoint 42, got %d", checkpoint)
	}
}
