package projections

import (
	"database/sql"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

func TestRunMigrationsAddsIsFirstOrderColumn(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "legacy.db")

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}

	// Minimal legacy schema without is_first_order.
	_, err = db.Exec(`
		CREATE TABLE orders (
			id TEXT PRIMARY KEY,
			user_id TEXT NOT NULL,
			total_amount REAL NOT NULL,
			status TEXT DEFAULT 'pending',
			items_json TEXT,
			start_date TEXT,
			end_date TEXT,
			rental_days INTEGER,
			created_at TEXT DEFAULT CURRENT_TIMESTAMP,
			updated_at TEXT DEFAULT CURRENT_TIMESTAMP,
			payment_code TEXT
		);
	`)
	if err != nil {
		t.Fatalf("create legacy schema: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close legacy db: %v", err)
	}

	rm, err := NewReadModelsDB(dbPath)
	if err != nil {
		t.Fatalf("NewReadModelsDB: %v", err)
	}
	defer rm.Close()

	var count int
	err = rm.GetDB().QueryRow(
		`SELECT COUNT(*) FROM pragma_table_info('orders') WHERE name = 'is_first_order'`,
	).Scan(&count)
	if err != nil {
		t.Fatalf("query column: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected is_first_order column, got count=%d", count)
	}
}
