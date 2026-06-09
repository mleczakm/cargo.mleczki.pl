package projections

import (
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite"
)

// ReadModelsDB handles the read models database.
type ReadModelsDB struct {
	db *sql.DB
}

// NewReadModelsDB creates a new read models database connection.
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

// initSchema creates the read model tables.
func (rm *ReadModelsDB) initSchema() error {
	// Users table
	usersTable := `
	CREATE TABLE IF NOT EXISTS users (
		id TEXT PRIMARY KEY,
		email TEXT UNIQUE NOT NULL,
		password_hash TEXT,
		name TEXT NOT NULL,
		phone TEXT,
		address TEXT,
		is_adult INTEGER DEFAULT 0,
		accepted_tos INTEGER DEFAULT 0,
		is_admin INTEGER DEFAULT 0,
		deletion_requested INTEGER DEFAULT 0,
		deletion_requested_at TEXT,
		created_at TEXT DEFAULT CURRENT_TIMESTAMP,
		updated_at TEXT DEFAULT CURRENT_TIMESTAMP
	);
	CREATE INDEX IF NOT EXISTS idx_users_email ON users(email);
	`

	// Orders table
	ordersTable := `
	CREATE TABLE IF NOT EXISTS orders (
		id TEXT PRIMARY KEY,
		user_id TEXT NOT NULL,
		total_amount REAL NOT NULL,
		equipment_total REAL,
		addons_total REAL,
		status TEXT DEFAULT 'pending',
		payment_method TEXT,
		rental_items TEXT,
		items_json TEXT,
		start_date TEXT,
		end_date TEXT,
		rental_days INTEGER,
		created_at TEXT DEFAULT CURRENT_TIMESTAMP,
		updated_at TEXT DEFAULT CURRENT_TIMESTAMP,
		paid_at TEXT,
		FOREIGN KEY (user_id) REFERENCES users(id)
	);
	CREATE INDEX IF NOT EXISTS idx_orders_user ON orders(user_id);
	CREATE INDEX IF NOT EXISTS idx_orders_status ON orders(status);
	CREATE INDEX IF NOT EXISTS idx_orders_created ON orders(created_at);
	`

	// Order items table
	orderItemsTable := `
	CREATE TABLE IF NOT EXISTS order_items (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		order_id TEXT NOT NULL,
		product_id TEXT NOT NULL,
		product_name TEXT NOT NULL,
		base_price REAL NOT NULL,
		quantity_days INTEGER NOT NULL,
		selected_addons TEXT,
		item_total REAL,
		FOREIGN KEY (order_id) REFERENCES orders(id)
	);
	CREATE INDEX IF NOT EXISTS idx_order_items_order ON order_items(order_id);
	`

	// Transfers table
	transfersTable := `
	CREATE TABLE IF NOT EXISTS transfers (
		id TEXT PRIMARY KEY,
		sender_name TEXT,
		sender_email TEXT,
		amount REAL NOT NULL,
		order_title TEXT,
		order_id TEXT,
		status TEXT DEFAULT 'unmatched',
		received_at TEXT NOT NULL,
		linked_at TEXT,
		raw_email_body TEXT,
		FOREIGN KEY (order_id) REFERENCES orders(id)
	);
	CREATE INDEX IF NOT EXISTS idx_transfers_status ON transfers(status);
	CREATE INDEX IF NOT EXISTS idx_transfers_order ON transfers(order_id);
	CREATE INDEX IF NOT EXISTS idx_transfers_received ON transfers(received_at);
	`

	// Product bookings table (renamed from product_availability)
	productBookingsTable := `
	CREATE TABLE IF NOT EXISTS product_bookings (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		product_id TEXT NOT NULL,
		order_id TEXT NOT NULL,
		booked_date TEXT NOT NULL,
		UNIQUE(product_id, booked_date),
		FOREIGN KEY (order_id) REFERENCES orders(id)
	);
	CREATE INDEX IF NOT EXISTS idx_product_bookings_product ON product_bookings(product_id);
	CREATE INDEX IF NOT EXISTS idx_product_bookings_date ON product_bookings(booked_date);
	`

	// User sessions table
	userSessionsTable := `
	CREATE TABLE IF NOT EXISTS user_sessions (
		id TEXT PRIMARY KEY,
		user_id TEXT,
		ip_address TEXT,
		user_agent TEXT,
		is_admin INTEGER DEFAULT 0,
		created_at TEXT DEFAULT CURRENT_TIMESTAMP,
		expires_at TEXT,
		last_activity TEXT
	);
	CREATE INDEX IF NOT EXISTS idx_user_sessions_user ON user_sessions(user_id);
	CREATE INDEX IF NOT EXISTS idx_user_sessions_expires ON user_sessions(expires_at);
	`

	// Shopping carts table
	shoppingCartsTable := `
	CREATE TABLE IF NOT EXISTS shopping_carts (
		id TEXT PRIMARY KEY,
		user_id TEXT,
		items TEXT NOT NULL,
		created_at TEXT DEFAULT CURRENT_TIMESTAMP,
		expires_at TEXT
	);
	CREATE INDEX IF NOT EXISTS idx_shopping_carts_user ON shopping_carts(user_id);
	CREATE INDEX IF NOT EXISTS idx_shopping_carts_expires ON shopping_carts(expires_at);
	`

	// Projection checkpoint table
	checkpointTable := `
	CREATE TABLE IF NOT EXISTS projection_checkpoint (
		projection_name TEXT PRIMARY KEY,
		last_event_version INTEGER NOT NULL
	);
	`

	// Global blocked dates table
	globalBlockedDatesTable := `
	CREATE TABLE IF NOT EXISTS global_blocked_dates (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		date TEXT NOT NULL UNIQUE,
		created_at TEXT DEFAULT CURRENT_TIMESTAMP,
		created_by TEXT
	);
	CREATE INDEX IF NOT EXISTS idx_global_blocked_dates_date ON global_blocked_dates(date);
	`

	schemas := []string{
		usersTable,
		ordersTable,
		orderItemsTable,
		transfersTable,
		productBookingsTable,
		userSessionsTable,
		shoppingCartsTable,
		checkpointTable,
		globalBlockedDatesTable,
	}
	for _, schema := range schemas {
		if _, err := rm.db.Exec(schema); err != nil {
			return err
		}
	}

	return nil
}

// GetDB returns the underlying database connection.
func (rm *ReadModelsDB) GetDB() *sql.DB {
	return rm.db
}

// Close closes the database connection.
func (rm *ReadModelsDB) Close() error {
	return rm.db.Close()
}

// GetCheckpoint retrieves the last processed event version for a projection.
func (rm *ReadModelsDB) GetCheckpoint(projectionName string) (int, error) {
	var version int
	err := rm.db.QueryRow("SELECT last_event_version FROM projection_checkpoint WHERE projection_name = ?", projectionName).Scan(&version)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	return version, err
}

// SaveCheckpoint saves the last processed event version for a projection.
func (rm *ReadModelsDB) SaveCheckpoint(projectionName string, version int) error {
	query := `
	INSERT INTO projection_checkpoint (projection_name, last_event_version)
	VALUES (?, ?)
	ON CONFLICT(projection_name) DO UPDATE SET last_event_version = excluded.last_event_version
	`
	_, err := rm.db.Exec(query, projectionName, version)
	return err
}

// GetAllUsers retrieves all users from the database.
func (rm *ReadModelsDB) GetAllUsers() ([]map[string]interface{}, error) {
	query := `
	SELECT id, email, name, phone, address, is_admin, created_at
	FROM users
	WHERE deletion_requested = 0
	ORDER BY created_at DESC
	`
	rows, err := rm.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []map[string]interface{}
	for rows.Next() {
		var id, email, name, phone, address, createdAt string
		var isAdmin int
		if err := rows.Scan(&id, &email, &name, &phone, &address, &isAdmin, &createdAt); err != nil {
			return nil, err
		}
		users = append(users, map[string]interface{}{
			"ID":      id,
			"Email":   email,
			"Name":    name,
			"Phone":   phone,
			"Address": address,
			"IsAdmin": isAdmin == 1,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return users, nil
}

// GetAllOrders retrieves all orders from the database.
func (rm *ReadModelsDB) GetAllOrders() ([]map[string]interface{}, error) {
	query := `
	SELECT o.id, o.user_id, u.name as user_name, o.total_amount, o.status, o.payment_method, o.items_json, o.created_at
	FROM orders o
	LEFT JOIN users u ON o.user_id = u.id
	ORDER BY o.created_at DESC
	LIMIT 50
	`
	rows, err := rm.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var orders []map[string]interface{}
	for rows.Next() {
		var id, userID, userName, status, paymentMethod, itemsJSON, createdAt string
		var totalAmount float64
		if err := rows.Scan(&id, &userID, &userName, &totalAmount, &status, &paymentMethod, &itemsJSON, &createdAt); err != nil {
			return nil, err
		}
		orders = append(orders, map[string]interface{}{
			"ID":            id,
			"UserID":        userID,
			"UserName":      userName,
			"TotalAmount":   totalAmount,
			"Status":        status,
			"PaymentMethod": paymentMethod,
			"Items":         itemsJSON,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return orders, nil
}

// GetGlobalBlockedDates retrieves all globally blocked dates.
func (rm *ReadModelsDB) GetGlobalBlockedDates() ([]string, error) {
	query := `SELECT date FROM global_blocked_dates ORDER BY date ASC`
	rows, err := rm.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var dates []string
	for rows.Next() {
		var date string
		if err := rows.Scan(&date); err != nil {
			return nil, err
		}
		dates = append(dates, date)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return dates, nil
}

// AddGlobalBlockedDate adds a date to the global blocked dates.
func (rm *ReadModelsDB) AddGlobalBlockedDate(date string, createdBy string) error {
	query := `INSERT INTO global_blocked_dates (date, created_by) VALUES (?, ?)`
	_, err := rm.db.Exec(query, date, createdBy)
	return err
}

// RemoveGlobalBlockedDate removes a date from the global blocked dates.
func (rm *ReadModelsDB) RemoveGlobalBlockedDate(date string) error {
	query := `DELETE FROM global_blocked_dates WHERE date = ?`
	_, err := rm.db.Exec(query, date)
	return err
}

// IsDateGloballyBlocked checks if a date is globally blocked.
func (rm *ReadModelsDB) IsDateGloballyBlocked(date string) (bool, error) {
	var count int
	query := `SELECT COUNT(*) FROM global_blocked_dates WHERE date = ?`
	err := rm.db.QueryRow(query, date).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}
