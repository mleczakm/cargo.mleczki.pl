package projections_test

import (
	"testing"

	"cargo.mleczki.pl/internal/projections"
)

func TestEnsureColumnQuotesIdentifiersAndIsIdempotent(t *testing.T) {
	db, err := projections.NewReadModelsDB(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for range 2 {
		if err := db.EnsureColumnForTest("orders", `note"; DROP TABLE users; --`, "TEXT"); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.EnsureColumnForTest(`orders); DROP TABLE users; --`, "note", "TEXT"); err == nil {
		t.Fatal("a nonexistent hostile table name should fail")
	}
	var count int
	if err := db.GetDB().QueryRow("SELECT COUNT(*) FROM users").Scan(&count); err != nil {
		t.Fatalf("users table was affected: %v", err)
	}
}

func TestProjectorRollsBackEarlierDatesWhenLaterLineFails(t *testing.T) {
	readModels, err := projectOrderPlaced(t, `{
		"orderId":"ORD-1","userId":"u","paymentMethod":"blik","timestamp":"2024-01-01T00:00:00Z","totalAmount":100,
		"items":[
			{"productId":"cargo","startDate":"2099-05-01","endDate":"2099-05-02"},
			{"productId":"broken","startDate":"bad-date","endDate":"2099-05-02"}
		]}`)
	if err == nil {
		t.Fatal("invalid dates should fail projection")
	}
	if got := bookedDays(t, readModels, "cargo"); len(got) != 0 {
		t.Fatalf("earlier calendar writes survived rollback: %v", got)
	}
}
