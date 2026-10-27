package shipping

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// Schema creates the shipments table. Shipments are operational data (like the
// stock ledger), not events: they mirror what Polkurier knows about a parcel.
const Schema = `
CREATE TABLE IF NOT EXISTS shipments (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	order_id TEXT NOT NULL,
	provider TEXT NOT NULL DEFAULT 'polkurier',
	carrier TEXT NOT NULL,
	external_number TEXT NOT NULL,
	waybill TEXT NOT NULL DEFAULT '',
	tracking_url TEXT NOT NULL DEFAULT '',
	price_gross INTEGER NOT NULL DEFAULT 0,
	status TEXT NOT NULL DEFAULT '',
	status_code TEXT NOT NULL DEFAULT '',
	cancelled INTEGER NOT NULL DEFAULT 0,
	created_at TEXT NOT NULL,
	updated_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_shipments_order ON shipments(order_id);
`

// Shipment is a parcel ordered at Polkurier for an order.
type Shipment struct {
	ID             int64
	OrderID        string
	Provider       string
	Carrier        string
	ExternalNumber string
	Waybill        string
	TrackingURL    string
	PriceGross     int64 // grosze, what the shop pays the courier
	Status         string
	StatusCode     string
	Cancelled      bool
	CreatedAt      string
	UpdatedAt      string
}

// Final status codes after which a parcel needs no more tracking.
// They are Polkurier order status codes: D delivered, Z returned, A cancelled.
var finalStatusCodes = map[string]bool{"D": true, "Z": true, "A": true}

// Active reports whether the parcel is still on its way (not cancelled, not finished).
func (s Shipment) Active() bool { return !s.Cancelled && s.StatusCode != "A" }

// Delivered reports whether the customer has the parcel.
func (s Shipment) Delivered() bool { return s.StatusCode == "D" }

// Tracked reports whether its status can still change.
func (s Shipment) Tracked() bool { return s.Active() && !finalStatusCodes[s.StatusCode] }

// ErrShipmentNotFound is returned for unknown shipment IDs.
var ErrShipmentNotFound = errors.New("shipment not found")

// Store keeps shipments in SQLite.
type Store struct{ db *sql.DB }

// NewStore creates a store on a database that already has Schema.
func NewStore(db *sql.DB) *Store { return &Store{db: db} }

func stamp() string { return time.Now().UTC().Format(time.RFC3339) }

const shipmentColumns = `id, order_id, provider, carrier, external_number, waybill, tracking_url, price_gross, status, status_code, cancelled, created_at, updated_at`

func scanShipment(row interface{ Scan(dest ...any) error }) (*Shipment, error) {
	var sh Shipment
	var cancelled int
	err := row.Scan(&sh.ID, &sh.OrderID, &sh.Provider, &sh.Carrier, &sh.ExternalNumber, &sh.Waybill, &sh.TrackingURL,
		&sh.PriceGross, &sh.Status, &sh.StatusCode, &cancelled, &sh.CreatedAt, &sh.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrShipmentNotFound
	}
	if err != nil {
		return nil, err
	}
	sh.Cancelled = cancelled == 1
	return &sh, nil
}

// Insert stores a new shipment and returns it with its ID.
func (s *Store) Insert(ctx context.Context, sh Shipment) (*Shipment, error) {
	now := stamp()
	res, err := s.db.ExecContext(ctx, `
		INSERT INTO shipments (order_id, provider, carrier, external_number, waybill, tracking_url, price_gross, status, status_code, created_at, updated_at)
		VALUES (?, 'polkurier', ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		sh.OrderID, sh.Carrier, sh.ExternalNumber, sh.Waybill, sh.TrackingURL, sh.PriceGross, sh.Status, sh.StatusCode, now, now)
	if err != nil {
		return nil, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}
	return s.ByID(ctx, id)
}

// ByID finds a shipment.
func (s *Store) ByID(ctx context.Context, id int64) (*Shipment, error) {
	return scanShipment(s.db.QueryRowContext(ctx, `SELECT `+shipmentColumns+` FROM shipments WHERE id = ?`, id))
}

// ForOrder lists the shipments of an order, newest first.
func (s *Store) ForOrder(ctx context.Context, orderID string) ([]*Shipment, error) {
	return s.list(ctx, `SELECT `+shipmentColumns+` FROM shipments WHERE order_id = ? ORDER BY id DESC`, orderID)
}

// Trackable lists the parcels whose status can still change.
func (s *Store) Trackable(ctx context.Context) ([]*Shipment, error) {
	all, err := s.list(ctx, `SELECT `+shipmentColumns+` FROM shipments WHERE cancelled = 0 ORDER BY id`)
	if err != nil {
		return nil, err
	}
	var out []*Shipment
	for _, sh := range all {
		if sh.Tracked() {
			out = append(out, sh)
		}
	}
	return out, nil
}

func (s *Store) list(ctx context.Context, query string, args ...any) ([]*Shipment, error) {
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Shipment
	for rows.Next() {
		sh, err := scanShipment(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, sh)
	}
	return out, rows.Err()
}

// UpdateStatus records what Polkurier reports about a parcel.
func (s *Store) UpdateStatus(ctx context.Context, id int64, status, code, trackingURL string) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE shipments SET status = ?, status_code = ?, tracking_url = CASE WHEN ? <> '' THEN ? ELSE tracking_url END, updated_at = ?
		WHERE id = ?`, status, code, trackingURL, trackingURL, stamp(), id)
	return err
}

// Cancel marks a parcel as cancelled.
func (s *Store) Cancel(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, `UPDATE shipments SET cancelled = 1, status = 'Anulowana', status_code = 'A', updated_at = ? WHERE id = ?`, stamp(), id)
	return err
}
