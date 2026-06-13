package projections

import (
	"database/sql"
	"fmt"
	"log"
	"time"

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
		payment_code TEXT,
		FOREIGN KEY (user_id) REFERENCES users(id)
	);
	CREATE INDEX IF NOT EXISTS idx_orders_user ON orders(user_id);
	CREATE INDEX IF NOT EXISTS idx_orders_status ON orders(status);
	CREATE INDEX IF NOT EXISTS idx_orders_created ON orders(created_at);
	CREATE INDEX IF NOT EXISTS idx_orders_payment_code ON orders(payment_code);
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

	// Payment codes table
	paymentCodesTable := `
	CREATE TABLE IF NOT EXISTS payment_codes (
		id TEXT PRIMARY KEY,
		code TEXT NOT NULL UNIQUE,
		order_id TEXT NOT NULL,
		created_at TEXT DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (order_id) REFERENCES orders(id)
	);
	CREATE INDEX IF NOT EXISTS idx_payment_codes_code ON payment_codes(code);
	CREATE INDEX IF NOT EXISTS idx_payment_codes_order ON payment_codes(order_id);
	`

	// Email import metadata table
	emailImportTable := `
	CREATE TABLE IF NOT EXISTS email_import_metadata (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		last_import_at TEXT,
		last_import_count INTEGER DEFAULT 0,
		updated_at TEXT DEFAULT CURRENT_TIMESTAMP
	);
	`

	// Password reset tokens table
	// #nosec G101 - This is SQL schema, not hardcoded credentials
	passwordResetTokensTable := `
	CREATE TABLE IF NOT EXISTS password_reset_tokens (
		id TEXT PRIMARY KEY,
		user_id TEXT NOT NULL,
		token TEXT NOT NULL UNIQUE,
		expires_at TEXT NOT NULL,
		used INTEGER DEFAULT 0,
		created_at TEXT DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (user_id) REFERENCES users(id)
	);
	CREATE INDEX IF NOT EXISTS idx_password_reset_tokens_token ON password_reset_tokens(token);
	CREATE INDEX IF NOT EXISTS idx_password_reset_tokens_user ON password_reset_tokens(user_id);
	CREATE INDEX IF NOT EXISTS idx_password_reset_tokens_expires ON password_reset_tokens(expires_at);
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
		paymentCodesTable,
		emailImportTable,
		passwordResetTokensTable,
	}
	for i, schema := range schemas {
		if _, err := rm.db.Exec(schema); err != nil {
			log.Printf("Failed to execute schema %d: %v", i, err)
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

// GetOrdersByUserID retrieves orders for a specific user.
func (rm *ReadModelsDB) GetOrdersByUserID(userID string) ([]map[string]interface{}, error) {
	query := `
	SELECT o.id, o.user_id, o.total_amount, o.status, o.payment_method, o.items_json, o.start_date, o.end_date, o.rental_days, o.created_at
	FROM orders o
	WHERE o.user_id = ?
	ORDER BY o.created_at DESC
	LIMIT 50
	`
	rows, err := rm.db.Query(query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var orders []map[string]interface{}
	for rows.Next() {
		var id, userIDRetrieved, status, paymentMethod, itemsJSON, startDate, endDate, createdAt string
		var totalAmount float64
		var rentalDays int
		if err := rows.Scan(&id, &userIDRetrieved, &totalAmount, &status, &paymentMethod, &itemsJSON, &startDate, &endDate, &rentalDays, &createdAt); err != nil {
			return nil, err
		}
		orders = append(orders, map[string]interface{}{
			"ID":            id,
			"UserID":        userIDRetrieved,
			"TotalAmount":   totalAmount,
			"Status":        status,
			"PaymentMethod": paymentMethod,
			"Items":         itemsJSON,
			"StartDate":     startDate,
			"EndDate":       endDate,
			"RentalDays":    rentalDays,
			"CreatedAt":     createdAt,
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

// CreatePaymentCode creates a new payment code for an order.
func (rm *ReadModelsDB) CreatePaymentCode(paymentCodeID, code, orderID string) error {
	query := `INSERT INTO payment_codes (id, code, order_id) VALUES (?, ?, ?)`
	_, err := rm.db.Exec(query, paymentCodeID, code, orderID)
	return err
}

// GetPaymentCodeByOrderID retrieves the payment code for a specific order.
func (rm *ReadModelsDB) GetPaymentCodeByOrderID(orderID string) (string, error) {
	var code string
	query := `SELECT code FROM payment_codes WHERE order_id = ?`
	err := rm.db.QueryRow(query, orderID).Scan(&code)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return code, err
}

// GetOrderByPaymentCode retrieves an order by its payment code.
func (rm *ReadModelsDB) GetOrderByPaymentCode(code string) (map[string]interface{}, error) {
	query := `
	SELECT o.id, o.user_id, o.total_amount, o.status, o.payment_method, o.items_json, o.created_at
	FROM orders o
	WHERE o.payment_code = ?
	`
	var id, userID, status, paymentMethod, itemsJSON, createdAt string
	var totalAmount float64
	err := rm.db.QueryRow(query, code).Scan(&id, &userID, &totalAmount, &status, &paymentMethod, &itemsJSON, &createdAt)
	if err == sql.ErrNoRows {
		return nil, sql.ErrNoRows
	}
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{
		"ID":            id,
		"UserID":        userID,
		"TotalAmount":   totalAmount,
		"Status":        status,
		"PaymentMethod": paymentMethod,
		"Items":         itemsJSON,
	}, nil
}

// GetLastEmailImport retrieves the last email import metadata.
func (rm *ReadModelsDB) GetLastEmailImport() (map[string]interface{}, error) {
	query := `
	SELECT last_import_at, last_import_count, updated_at
	FROM email_import_metadata
	WHERE id = 1
	`
	var lastImportAt, updatedAt string
	var lastImportCount int
	err := rm.db.QueryRow(query).Scan(&lastImportAt, &lastImportCount, &updatedAt)
	if err == sql.ErrNoRows {
		return nil, sql.ErrNoRows
	}
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{
		"LastImportAt":    lastImportAt,
		"LastImportCount": lastImportCount,
		"UpdatedAt":       updatedAt,
	}, nil
}

// UpdateLastEmailImport updates the last email import metadata.
func (rm *ReadModelsDB) UpdateLastEmailImport(count int) error {
	now := time.Now().UTC().Format(time.RFC3339)
	query := `
	INSERT INTO email_import_metadata (id, last_import_at, last_import_count, updated_at)
	VALUES (1, ?, ?, ?)
	ON CONFLICT(id) DO UPDATE SET
		last_import_at = excluded.last_import_at,
		last_import_count = excluded.last_import_count,
		updated_at = excluded.updated_at
	`
	_, err := rm.db.Exec(query, now, count, now)
	return err
}
