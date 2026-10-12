package shop

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"cargo.mleczki.pl/internal/domain"
)

// Schema creates the shop tables. It is idempotent and executed with the rest
// of the read-model schema.
//
// shop_stock holds the running counters, shop_stock_movements is the append-only
// ledger that explains them, and shop_reservations remembers which order holds
// which units so that cancelling or fulfilling an order is idempotent.
const Schema = `
CREATE TABLE IF NOT EXISTS shop_stock (
	sku TEXT PRIMARY KEY,
	item_id TEXT NOT NULL,
	on_hand INTEGER NOT NULL DEFAULT 0 CHECK (on_hand >= 0),
	reserved INTEGER NOT NULL DEFAULT 0 CHECK (reserved >= 0),
	updated_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_shop_stock_item ON shop_stock(item_id);

CREATE TABLE IF NOT EXISTS shop_stock_movements (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	sku TEXT NOT NULL,
	kind TEXT NOT NULL,
	delta_on_hand INTEGER NOT NULL DEFAULT 0,
	delta_reserved INTEGER NOT NULL DEFAULT 0,
	on_hand_after INTEGER NOT NULL,
	reserved_after INTEGER NOT NULL,
	order_id TEXT,
	note TEXT,
	actor TEXT,
	created_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_shop_movements_sku ON shop_stock_movements(sku, id);

CREATE TABLE IF NOT EXISTS shop_reservations (
	order_id TEXT NOT NULL,
	sku TEXT NOT NULL,
	quantity INTEGER NOT NULL CHECK (quantity > 0),
	state TEXT NOT NULL DEFAULT 'reserved',
	created_at TEXT NOT NULL,
	updated_at TEXT NOT NULL,
	PRIMARY KEY (order_id, sku)
);
CREATE INDEX IF NOT EXISTS idx_shop_reservations_state ON shop_reservations(state);

CREATE TABLE IF NOT EXISTS shop_affiliate_clicks (
	item_id TEXT NOT NULL,
	day TEXT NOT NULL,
	clicks INTEGER NOT NULL DEFAULT 0,
	PRIMARY KEY (item_id, day)
);
`

// Ledger movement kinds.
const (
	KindInitial = "initial" // first sight of a SKU, seeded from the catalog
	KindReceive = "receive" // delivery from a supplier
	KindReturn  = "return"  // goods came back from a customer
	KindLoss    = "loss"    // damaged, lost or given away
	KindCount   = "count"   // stocktake: on hand set to the counted quantity
	KindReserve = "reserve" // an order holds units
	KindRelease = "release" // an order let go of units (cancelled, expired)
	KindFulfil  = "fulfil"  // an order took units out of the shop
)

// KindLabel is the Polish name of a movement kind, for the admin panel.
func KindLabel(kind string) string {
	switch kind {
	case KindInitial:
		return "Stan początkowy"
	case KindReceive:
		return "Dostawa"
	case KindReturn:
		return "Zwrot od klienta"
	case KindLoss:
		return "Strata / uszkodzenie"
	case KindCount:
		return "Inwentaryzacja"
	case KindReserve:
		return "Rezerwacja zamówienia"
	case KindRelease:
		return "Zwolnienie rezerwacji"
	case KindFulfil:
		return "Wydanie z magazynu"
	default:
		return kind
	}
}

// Level is the stock of one SKU.
type Level struct {
	SKU       string
	ItemID    string
	OnHand    int
	Reserved  int
	UpdatedAt string
}

// Available is what can still be promised to new orders.
func (l Level) Available() int { return l.OnHand - l.Reserved }

// Movement is one ledger entry.
type Movement struct {
	ID            int64
	SKU           string
	Kind          string
	DeltaOnHand   int
	DeltaReserved int
	OnHandAfter   int
	ReservedAfter int
	OrderID       string
	Note          string
	Actor         string
	CreatedAt     string
}

// ReserveLine asks to hold Quantity units of a SKU for an order.
type ReserveLine struct {
	SKU       string
	ItemID    string
	Quantity  int
	Backorder bool // allow holding more than is available
}

// InsufficientStockError says which SKU could not be reserved.
type InsufficientStockError struct {
	SKU       string
	Requested int
	Available int
}

func (e *InsufficientStockError) Error() string {
	return fmt.Sprintf("insufficient stock for %s: requested %d, available %d", e.SKU, e.Requested, e.Available)
}

// ErrNotEnoughOnHand is returned when an order cannot be handed over because
// fewer units are physically on hand than the order holds.
var ErrNotEnoughOnHand = errors.New("za mało sztuk na stanie")

// AdjustInput is a manual stock correction made by an administrator.
type AdjustInput struct {
	SKU      string
	ItemID   string
	Kind     string // KindReceive, KindReturn, KindLoss or KindCount
	Quantity int
	Note     string
	Actor    string
}

// Store keeps stock, reservations and affiliate click counters in SQLite.
type Store struct {
	db *sql.DB
	mu sync.Mutex // serialises writers inside this process
}

// NewStore wraps db. The Schema must already be applied.
func NewStore(db *sql.DB) *Store { return &Store{db: db} }

func now() string { return time.Now().UTC().Format(time.RFC3339) }

// conn is the subset of *sql.Conn the store needs.
type conn interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// write runs fn inside a BEGIN IMMEDIATE transaction so that the read-check-write
// sequences below cannot interleave with other writers, in or out of process.
func (s *Store) write(ctx context.Context, fn func(db conn) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	session, err := s.db.Conn(ctx)
	if err != nil {
		return fmt.Errorf("stock connection: %w", err)
	}
	defer session.Close()

	if _, err := session.ExecContext(ctx, "PRAGMA busy_timeout = 5000"); err != nil {
		return fmt.Errorf("stock busy timeout: %w", err)
	}
	if _, err := session.ExecContext(ctx, "BEGIN IMMEDIATE"); err != nil {
		return fmt.Errorf("begin stock transaction: %w", err)
	}
	rollback := func() { _, _ = session.ExecContext(context.WithoutCancel(ctx), "ROLLBACK") }

	if err := fn(session); err != nil {
		rollback()
		return err
	}
	if _, err := session.ExecContext(ctx, "COMMIT"); err != nil {
		rollback()
		return fmt.Errorf("commit stock transaction: %w", err)
	}
	return nil
}

type change struct {
	sku, itemID, kind, orderID, note, actor string
	dOnHand, dReserved                      int
}

// apply updates the counters of a SKU (creating the row if needed) and appends
// the ledger entry. Callers check business rules first.
func apply(ctx context.Context, db conn, ch change) error {
	ts := now()
	if _, err := db.ExecContext(ctx,
		`INSERT OR IGNORE INTO shop_stock (sku, item_id, on_hand, reserved, updated_at) VALUES (?, ?, 0, 0, ?)`,
		ch.sku, ch.itemID, ts); err != nil {
		return fmt.Errorf("create stock row %s: %w", ch.sku, err)
	}
	if _, err := db.ExecContext(ctx,
		`UPDATE shop_stock SET on_hand = on_hand + ?, reserved = reserved + ?, updated_at = ? WHERE sku = ?`,
		ch.dOnHand, ch.dReserved, ts, ch.sku); err != nil {
		return fmt.Errorf("update stock %s: %w", ch.sku, err)
	}
	var onHand, reserved int
	if err := db.QueryRowContext(ctx, `SELECT on_hand, reserved FROM shop_stock WHERE sku = ?`, ch.sku).Scan(&onHand, &reserved); err != nil {
		return fmt.Errorf("read stock %s: %w", ch.sku, err)
	}
	_, err := db.ExecContext(ctx, `
		INSERT INTO shop_stock_movements
			(sku, kind, delta_on_hand, delta_reserved, on_hand_after, reserved_after, order_id, note, actor, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		ch.sku, ch.kind, ch.dOnHand, ch.dReserved, onHand, reserved,
		nullable(ch.orderID), nullable(ch.note), nullable(ch.actor), ts)
	if err != nil {
		return fmt.Errorf("record movement %s: %w", ch.sku, err)
	}
	return nil
}

func nullable(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func readLevel(ctx context.Context, db conn, sku string) (Level, error) {
	level := Level{SKU: sku}
	err := db.QueryRowContext(ctx, `SELECT item_id, on_hand, reserved, updated_at FROM shop_stock WHERE sku = ?`, sku).
		Scan(&level.ItemID, &level.OnHand, &level.Reserved, &level.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return level, nil
	}
	return level, err
}

// SyncCatalog makes sure every stocked SKU of the catalog has a ledger row. A
// SKU seen for the first time starts with the initial stock from its file; after
// that the file no longer influences the quantity.
func (s *Store) SyncCatalog(ctx context.Context, items []domain.ShopItem) error {
	return s.write(ctx, func(db conn) error {
		for _, item := range items {
			if !item.Tracked() {
				continue
			}
			for _, sku := range item.SKUs() {
				res, err := db.ExecContext(ctx,
					`INSERT OR IGNORE INTO shop_stock (sku, item_id, on_hand, reserved, updated_at) VALUES (?, ?, 0, 0, ?)`,
					sku, item.ID, now())
				if err != nil {
					return fmt.Errorf("sync %s: %w", sku, err)
				}
				created, _ := res.RowsAffected()
				initial := item.InitialStockFor(sku)
				if created == 0 || initial == 0 {
					continue
				}
				if err := apply(ctx, db, change{sku: sku, itemID: item.ID, kind: KindInitial, dOnHand: initial, note: "Stan początkowy z pliku katalogu", actor: "system"}); err != nil {
					return err
				}
			}
		}
		return nil
	})
}

// Levels returns the stock of the given SKUs. Unknown SKUs are absent from the map.
func (s *Store) Levels(ctx context.Context, skus []string) (map[string]Level, error) {
	levels := make(map[string]Level, len(skus))
	if len(skus) == 0 {
		return levels, nil
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(skus)), ",")
	args := make([]any, len(skus))
	for i, sku := range skus {
		args[i] = sku
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT sku, item_id, on_hand, reserved, updated_at FROM shop_stock WHERE sku IN (`+placeholders+`)`, args...) // #nosec G202 - placeholders only
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var l Level
		if err := rows.Scan(&l.SKU, &l.ItemID, &l.OnHand, &l.Reserved, &l.UpdatedAt); err != nil {
			return nil, err
		}
		levels[l.SKU] = l
	}
	return levels, rows.Err()
}

// Reserve atomically holds units for an order: either every line is reserved or
// none is, and a concurrent order can never take the same units.
func (s *Store) Reserve(ctx context.Context, orderID string, lines []ReserveLine) error {
	merged := mergeLines(lines)
	if orderID == "" || len(merged) == 0 {
		return nil
	}
	return s.write(ctx, func(db conn) error {
		for _, line := range merged {
			if line.Quantity <= 0 {
				return fmt.Errorf("invalid quantity %d for %s", line.Quantity, line.SKU)
			}
			level, err := readLevel(ctx, db, line.SKU)
			if err != nil {
				return err
			}
			if !line.Backorder && level.Available() < line.Quantity {
				return &InsufficientStockError{SKU: line.SKU, Requested: line.Quantity, Available: max(level.Available(), 0)}
			}
			if err := apply(ctx, db, change{sku: line.SKU, itemID: line.ItemID, kind: KindReserve, dReserved: line.Quantity, orderID: orderID}); err != nil {
				return err
			}
			ts := now()
			if _, err := db.ExecContext(ctx, `
				INSERT INTO shop_reservations (order_id, sku, quantity, state, created_at, updated_at)
				VALUES (?, ?, ?, 'reserved', ?, ?)`, orderID, line.SKU, line.Quantity, ts, ts); err != nil {
				return fmt.Errorf("record reservation %s/%s: %w", orderID, line.SKU, err)
			}
		}
		return nil
	})
}

func mergeLines(lines []ReserveLine) []ReserveLine {
	bySKU := map[string]*ReserveLine{}
	var order []string
	for _, line := range lines {
		if existing, ok := bySKU[line.SKU]; ok {
			existing.Quantity += line.Quantity
			existing.Backorder = existing.Backorder || line.Backorder
			continue
		}
		copied := line
		bySKU[line.SKU] = &copied
		order = append(order, line.SKU)
	}
	sort.Strings(order)
	merged := make([]ReserveLine, 0, len(order))
	for _, sku := range order {
		merged = append(merged, *bySKU[sku])
	}
	return merged
}

type heldUnits struct {
	sku      string
	quantity int
}

func heldBy(ctx context.Context, db conn, orderID string) ([]heldUnits, error) {
	rows, err := db.QueryContext(ctx, `SELECT sku, quantity FROM shop_reservations WHERE order_id = ? AND state = 'reserved' ORDER BY sku`, orderID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var held []heldUnits
	for rows.Next() {
		var unit heldUnits
		if err := rows.Scan(&unit.sku, &unit.quantity); err != nil {
			return nil, err
		}
		held = append(held, unit)
	}
	return held, rows.Err()
}

func setReservationState(ctx context.Context, db conn, orderID, sku, state string) error {
	_, err := db.ExecContext(ctx, `UPDATE shop_reservations SET state = ?, updated_at = ? WHERE order_id = ? AND sku = ?`, state, now(), orderID, sku)
	return err
}

// ReleaseOrder gives back the units an order holds. It does nothing for orders
// without a live reservation, so it is safe to call repeatedly.
func (s *Store) ReleaseOrder(ctx context.Context, orderID string) error {
	return s.write(ctx, func(db conn) error {
		held, err := heldBy(ctx, db, orderID)
		if err != nil {
			return err
		}
		for _, unit := range held {
			level, err := readLevel(ctx, db, unit.sku)
			if err != nil {
				return err
			}
			if err := apply(ctx, db, change{sku: unit.sku, itemID: level.ItemID, kind: KindRelease, dReserved: -unit.quantity, orderID: orderID}); err != nil {
				return err
			}
			if err := setReservationState(ctx, db, orderID, unit.sku, "released"); err != nil {
				return err
			}
		}
		return nil
	})
}

// CanFulfil checks that every unit an order holds is physically on hand.
func (s *Store) CanFulfil(ctx context.Context, orderID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	rows, err := s.db.QueryContext(ctx, `
		SELECT r.sku, r.quantity, COALESCE(st.on_hand, 0)
		FROM shop_reservations r LEFT JOIN shop_stock st ON st.sku = r.sku
		WHERE r.order_id = ? AND r.state = 'reserved'`, orderID)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var sku string
		var quantity, onHand int
		if err := rows.Scan(&sku, &quantity, &onHand); err != nil {
			return err
		}
		if onHand < quantity {
			return fmt.Errorf("%w: %s – potrzeba %d, na stanie %d", ErrNotEnoughOnHand, sku, quantity, onHand)
		}
	}
	return rows.Err()
}

// FulfilOrder takes the units an order holds out of the shop (they leave both
// the reserved and the on-hand count). It never fails on business grounds: if
// fewer units are on hand than the order holds, the shortfall is recorded in the
// ledger note and on-hand is clamped at zero.
func (s *Store) FulfilOrder(ctx context.Context, orderID string) error {
	return s.write(ctx, func(db conn) error {
		held, err := heldBy(ctx, db, orderID)
		if err != nil {
			return err
		}
		for _, unit := range held {
			level, err := readLevel(ctx, db, unit.sku)
			if err != nil {
				return err
			}
			taken := min(unit.quantity, level.OnHand)
			note := ""
			if taken < unit.quantity {
				note = fmt.Sprintf("Brakowało %d szt. na stanie fizycznym", unit.quantity-taken)
			}
			if err := apply(ctx, db, change{sku: unit.sku, itemID: level.ItemID, kind: KindFulfil, dOnHand: -taken, dReserved: -unit.quantity, orderID: orderID, note: note}); err != nil {
				return err
			}
			if err := setReservationState(ctx, db, orderID, unit.sku, "fulfilled"); err != nil {
				return err
			}
		}
		return nil
	})
}

// HoldsStock reports whether an order has live reservations.
func (s *Store) HoldsStock(ctx context.Context, orderID string) (bool, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM shop_reservations WHERE order_id = ? AND state = 'reserved'`, orderID).Scan(&n)
	return n > 0, err
}

// Adjust applies a manual correction. Stock can never drop below what open
// orders hold, so a correction that would strand an order is refused.
func (s *Store) Adjust(ctx context.Context, in AdjustInput) error {
	if in.SKU == "" || in.ItemID == "" {
		return errors.New("brak SKU lub produktu")
	}
	return s.write(ctx, func(db conn) error {
		level, err := readLevel(ctx, db, in.SKU)
		if err != nil {
			return err
		}
		ch := change{sku: in.SKU, itemID: in.ItemID, kind: in.Kind, note: in.Note, actor: in.Actor}
		switch in.Kind {
		case KindReceive, KindReturn:
			if in.Quantity < 1 {
				return errors.New("ilość musi wynosić co najmniej 1")
			}
			ch.dOnHand = in.Quantity
		case KindLoss:
			if in.Quantity < 1 {
				return errors.New("ilość musi wynosić co najmniej 1")
			}
			if level.OnHand-in.Quantity < level.Reserved {
				return fmt.Errorf("nie można zdjąć %d szt.: wolnych jest %d (reszta jest zarezerwowana w zamówieniach)", in.Quantity, max(level.Available(), 0))
			}
			ch.dOnHand = -in.Quantity
		case KindCount:
			if in.Quantity < 0 {
				return errors.New("policzona ilość nie może być ujemna")
			}
			if in.Quantity < level.Reserved {
				return fmt.Errorf("policzone %d szt. to mniej niż %d zarezerwowane w otwartych zamówieniach", in.Quantity, level.Reserved)
			}
			ch.dOnHand = in.Quantity - level.OnHand
		default:
			return fmt.Errorf("nieznany rodzaj operacji %q", in.Kind)
		}
		if ch.dOnHand == 0 {
			return nil
		}
		return apply(ctx, db, ch)
	})
}

// AllLevels lists every ledger row.
func (s *Store) AllLevels(ctx context.Context) ([]Level, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT sku, item_id, on_hand, reserved, updated_at FROM shop_stock ORDER BY item_id, sku`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var levels []Level
	for rows.Next() {
		var l Level
		if err := rows.Scan(&l.SKU, &l.ItemID, &l.OnHand, &l.Reserved, &l.UpdatedAt); err != nil {
			return nil, err
		}
		levels = append(levels, l)
	}
	return levels, rows.Err()
}

// Movements returns the newest ledger entries, optionally for a single SKU.
func (s *Store) Movements(ctx context.Context, sku string, limit int) ([]Movement, error) {
	query := `SELECT id, sku, kind, delta_on_hand, delta_reserved, on_hand_after, reserved_after,
		COALESCE(order_id, ''), COALESCE(note, ''), COALESCE(actor, ''), created_at FROM shop_stock_movements`
	var args []any
	if sku != "" {
		query += ` WHERE sku = ?`
		args = append(args, sku)
	}
	query += ` ORDER BY id DESC LIMIT ?`
	args = append(args, limit)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var movements []Movement
	for rows.Next() {
		var m Movement
		if err := rows.Scan(&m.ID, &m.SKU, &m.Kind, &m.DeltaOnHand, &m.DeltaReserved, &m.OnHandAfter, &m.ReservedAfter, &m.OrderID, &m.Note, &m.Actor, &m.CreatedAt); err != nil {
			return nil, err
		}
		movements = append(movements, m)
	}
	return movements, rows.Err()
}

// StaleReservations lists orders that still await payment although they hold
// stock since before cutoff.
func (s *Store) StaleReservations(ctx context.Context, cutoff time.Time) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT DISTINCT o.id
		FROM orders o JOIN shop_reservations r ON r.order_id = o.id
		WHERE r.state = 'reserved' AND o.status = 'awaiting_payment' AND o.created_at < ?
		ORDER BY o.created_at`, cutoff.UTC().Format("2006-01-02 15:04:05"))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// RecordClick counts an outbound click on an affiliate item.
func (s *Store) RecordClick(ctx context.Context, itemID string) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO shop_affiliate_clicks (item_id, day, clicks) VALUES (?, ?, 1)
		ON CONFLICT(item_id, day) DO UPDATE SET clicks = clicks + 1`,
		itemID, time.Now().UTC().Format("2006-01-02"))
	return err
}

// ClickStats are affiliate click totals for one item.
type ClickStats struct {
	Total      int
	Last30Days int
}

// Clicks returns click totals per item.
func (s *Store) Clicks(ctx context.Context) (map[string]ClickStats, error) {
	since := time.Now().UTC().AddDate(0, 0, -30).Format("2006-01-02")
	rows, err := s.db.QueryContext(ctx, `
		SELECT item_id, SUM(clicks), SUM(CASE WHEN day >= ? THEN clicks ELSE 0 END)
		FROM shop_affiliate_clicks GROUP BY item_id`, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	stats := map[string]ClickStats{}
	for rows.Next() {
		var id string
		var cs ClickStats
		if err := rows.Scan(&id, &cs.Total, &cs.Last30Days); err != nil {
			return nil, err
		}
		stats[id] = cs
	}
	return stats, rows.Err()
}
