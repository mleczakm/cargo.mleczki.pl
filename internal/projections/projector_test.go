package projections_test

import (
	"encoding/json"
	"testing"

	"cargo.mleczki.pl/internal/domain"
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
	eventData := map[string]interface{}{
		"orderId":       "order-1",
		"userId":        "user-1",
		"items":         []domain.OrderItem{},
		"totalAmount":   100,
		"paymentMethod": "blik",
		"startDate":     "2024-01-01",
		"endDate":       "2024-01-03",
		"rentalDays":    2,
		"timestamp":     "2024-01-01T00:00:00Z",
	}
	payload, _ := json.Marshal(eventData)

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

func projectOrderPlaced(t *testing.T, payload string) (*projections.ReadModelsDB, error) {
	t.Helper()
	eventStore, err := eventstore.NewSQLiteEventStore(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = eventStore.Close() })
	readModels, err := projections.NewReadModelsDB(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = readModels.Close() })

	event := &eventstore.Event{ID: "e1", AggregateID: "ORD-1", AggregateType: "order", EventType: "OrderPlaced", Payload: json.RawMessage(payload), Version: 1}
	if err := eventStore.Save(t.Context(), event); err != nil {
		t.Fatal(err)
	}
	return readModels, projections.NewProjector(eventStore, readModels, "test").Run(t.Context())
}

func bookedDays(t *testing.T, readModels *projections.ReadModelsDB, product string) []string {
	t.Helper()
	days, err := readModels.GetBookedDatesForProduct(product)
	if err != nil {
		t.Fatal(err)
	}
	return days
}

func TestProjector_BooksEveryRentalLineOnItsOwnDates(t *testing.T) {
	readModels, err := projectOrderPlaced(t, `{
		"orderId":"ORD-1","userId":"u","paymentMethod":"blik","timestamp":"2024-01-01T00:00:00Z",
		"startDate":"2099-05-01","endDate":"2099-05-20","rentalDays":20,
		"totalAmount":"381.50",
		"items":[
			{"productId":"cargo","productName":"Cargo","basePrice":"150.00","rentalDays":2,"startDate":"2099-05-01","endDate":"2099-05-02"},
			{"productId":"tandem","productName":"Tandem","basePrice":"80.50","rentalDays":1,"startDate":"2099-05-20","endDate":"2099-05-20"},
			{"kind":"shop","productId":"bell","sku":"bell","basePrice":"9.90","quantity":2}
		]}`)
	if err != nil {
		t.Fatal(err)
	}
	if got := bookedDays(t, readModels, "cargo"); len(got) != 2 || got[0] != "2099-05-01" || got[1] != "2099-05-02" {
		t.Errorf("cargo booked %v", got)
	}
	if got := bookedDays(t, readModels, "tandem"); len(got) != 1 || got[0] != "2099-05-20" {
		t.Errorf("tandem booked %v", got)
	}
	if got := bookedDays(t, readModels, "bell"); len(got) != 0 {
		t.Errorf("shop lines book no days, got %v", got)
	}

	var total float64
	if err := readModels.GetDB().QueryRow(`SELECT total_amount FROM orders WHERE id = 'ORD-1'`).Scan(&total); err != nil || total != 381.5 {
		t.Errorf("total = %v, %v", total, err)
	}
	orders, err := readModels.GetAllOrders()
	if err != nil || len(orders) != 1 || orders[0]["TotalAmount"] != domain.PLN(38150) {
		t.Errorf("orders = %v, %v", orders, err)
	}
}

func TestProjector_OrdersStoredBeforeLineDatesUseTheOrderDates(t *testing.T) {
	// The shape of an event written before decimals and per-line dates existed.
	readModels, err := projectOrderPlaced(t, `{
		"orderId":"ORD-1","userId":"u","paymentMethod":"blik","timestamp":"2024-01-01T00:00:00Z",
		"startDate":"2099-05-01","endDate":"2099-05-03","rentalDays":3,"totalAmount":450,
		"items":[{"productId":"cargo","productName":"Cargo","basePrice":150,"rentalDays":3,"selectedAddons":[]}]}`)
	if err != nil {
		t.Fatal(err)
	}
	if got := bookedDays(t, readModels, "cargo"); len(got) != 3 {
		t.Errorf("cargo booked %v, want the 3 order days", got)
	}
	orders, _ := readModels.GetAllOrders()
	if len(orders) != 1 || orders[0]["TotalAmount"] != domain.Zloty(450) {
		t.Errorf("a legacy whole-złoty total must read as 450 zł: %v", orders)
	}
}
