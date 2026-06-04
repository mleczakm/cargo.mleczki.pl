package projections

import (
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite"
)

// ReadModelsDB handles the read models database
type ReadModelsDB struct {
	db *sql.DB
}

// NewReadModelsDB creates a new read models database connection
func NewReadModelsDB(dbPath string) (*ReadModelsDB, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Enable WAL mode for better concurrency
	if _, err := db.Exec("PRAGMA journal_mode=WAL;"); err != nil {
		return nil, fmt.Errorf("failed to enable WAL mode: %w", err)
	}

	rm := &ReadModelsDB{db: db}
	if err := rm.initSchema(); err != nil {
		return nil, fmt.Errorf("failed to initialize schema: %w", err)
	}

	return rm, nil
}

// initSchema creates the read model tables
func (rm *ReadModelsDB) initSchema() error {
	// Users table
	usersTable := `
	CREATE TABLE IF NOT EXISTS users (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		email TEXT NOT NULL UNIQUE,
		phone TEXT,
		address TEXT,
		password_hash TEXT,
		created_at TEXT NOT NULL,
		updated_at TEXT NOT NULL,
		deletion_requested_at TEXT,
		is_deleted INTEGER DEFAULT 0
	);
	CREATE INDEX IF NOT EXISTS idx_users_email ON users(email);
	`

	// Orders table
	ordersTable := `
	CREATE TABLE IF NOT EXISTS orders (
		id TEXT PRIMARY KEY,
		user_id TEXT,
		items_json TEXT NOT NULL,
		total_amount INTEGER NOT NULL,
		status TEXT NOT NULL,
		payment_method TEXT NOT NULL,
		start_date TEXT NOT NULL,
		end_date TEXT NOT NULL,
		rental_days INTEGER NOT NULL,
		created_at TEXT NOT NULL,
		updated_at TEXT NOT NULL,
		FOREIGN KEY (user_id) REFERENCES users(id)
	);
	CREATE INDEX IF NOT EXISTS idx_orders_user_id ON orders(user_id);
	CREATE INDEX IF NOT EXISTS idx_orders_status ON orders(status);
	CREATE INDEX IF NOT EXISTS idx_orders_dates ON orders(start_date, end_date);
	`

	// Transfers table
	transfersTable := `
	CREATE TABLE IF NOT EXISTS transfers (
		id TEXT PRIMARY KEY,
		date TEXT NOT NULL,
		sender TEXT NOT NULL,
		title TEXT NOT NULL,
		amount INTEGER NOT NULL,
		status TEXT NOT NULL,
		order_id TEXT,
		created_at TEXT NOT NULL,
		FOREIGN KEY (order_id) REFERENCES orders(id)
	);
	CREATE INDEX IF NOT EXISTS idx_transfers_status ON transfers(status);
	CREATE INDEX IF NOT EXISTS idx_transfers_order_id ON transfers(order_id);
	`

	// Product availability table
	productAvailabilityTable := `
	CREATE TABLE IF NOT EXISTS product_availability (
		product_id TEXT NOT NULL,
		date TEXT NOT NULL,
		is_booked INTEGER DEFAULT 0,
		PRIMARY KEY (product_id, date)
	);
	CREATE INDEX IF NOT EXISTS idx_product_availability_date ON product_availability(date);
	`

	// Projection checkpoint table
	checkpointTable := `
	CREATE TABLE IF NOT EXISTS projection_checkpoint (
		projection_name TEXT PRIMARY KEY,
		last_event_version INTEGER NOT NULL
	);
	`

	schemas := []string{usersTable, ordersTable, transfersTable, productAvailabilityTable, checkpointTable}
	for _, schema := range schemas {
		if _, err := rm.db.Exec(schema); err != nil {
			return err
		}
	}

	return nil
}

// GetDB returns the underlying database connection
func (rm *ReadModelsDB) GetDB() *sql.DB {
	return rm.db
}

// Close closes the database connection
func (rm *ReadModelsDB) Close() error {
	return rm.db.Close()
}

// GetCheckpoint retrieves the last processed event version for a projection
func (rm *ReadModelsDB) GetCheckpoint(projectionName string) (int, error) {
	var version int
	err := rm.db.QueryRow("SELECT last_event_version FROM projection_checkpoint WHERE projection_name = ?", projectionName).Scan(&version)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	return version, err
}

// SaveCheckpoint saves the last processed event version for a projection
func (rm *ReadModelsDB) SaveCheckpoint(projectionName string, version int) error {
	query := `
	INSERT INTO projection_checkpoint (projection_name, last_event_version)
	VALUES (?, ?)
	ON CONFLICT(projection_name) DO UPDATE SET last_event_version = excluded.last_event_version
	`
	_, err := rm.db.Exec(query, projectionName, version)
	return err
}
