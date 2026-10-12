package shop_test

import (
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"cargo.mleczki.pl/internal/domain"
	"cargo.mleczki.pl/internal/shop"
)

func newTestStore(t *testing.T) (*shop.Store, *sql.DB) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "rm.db") + "?_pragma=busy_timeout(5000)"
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.Exec(shop.Schema); err != nil {
		t.Fatalf("schema: %v", err)
	}
	if _, err := db.Exec(`CREATE TABLE orders (id TEXT PRIMARY KEY, status TEXT, created_at TEXT)`); err != nil {
		t.Fatal(err)
	}
	return shop.NewStore(db), db
}

func stockedItem(id string, initial int) domain.ShopItem {
	return domain.ShopItem{
		ID: id, Type: domain.ShopItemDirect, Price: domain.Zloty(10), InitialStock: initial,
		Stock: domain.StockPolicy{Mode: domain.StockTracked},
	}
}

func mustLevel(t *testing.T, store *shop.Store, sku string) shop.Level {
	t.Helper()
	levels, err := store.Levels(t.Context(), []string{sku})
	if err != nil {
		t.Fatal(err)
	}
	return levels[sku]
}

func TestSyncCatalogSeedsInitialStockOnlyOnce(t *testing.T) {
	ctx := t.Context()
	store, _ := newTestStore(t)

	withVariants := stockedItem("kask", 0)
	withVariants.Variants = []domain.ShopVariant{{SKU: "kask-s", InitialStock: 3}, {SKU: "kask-l", InitialStock: 0}}
	unlimited := domain.ShopItem{ID: "naklejka", Type: domain.ShopItemDirect, Price: domain.Zloty(1), Stock: domain.StockPolicy{Mode: domain.StockUnlimited}}
	affiliate := domain.ShopItem{ID: "lampka", Type: domain.ShopItemAffiliate}
	items := []domain.ShopItem{stockedItem("dzwonek", 12), withVariants, unlimited, affiliate}

	if err := store.SyncCatalog(ctx, items); err != nil {
		t.Fatal(err)
	}
	if got := mustLevel(t, store, "dzwonek").OnHand; got != 12 {
		t.Errorf("dzwonek on hand = %d, want 12", got)
	}
	if got := mustLevel(t, store, "kask-s").OnHand; got != 3 {
		t.Errorf("kask-s on hand = %d, want 3", got)
	}
	all, _ := store.AllLevels(ctx)
	if len(all) != 3 {
		t.Errorf("want rows for dzwonek, kask-s, kask-l only, got %d: %+v", len(all), all)
	}

	// Selling a unit and syncing again (a restart) must not re-seed.
	if err := store.Reserve(ctx, "ORD-1", []shop.ReserveLine{{SKU: "dzwonek", ItemID: "dzwonek", Quantity: 2}}); err != nil {
		t.Fatal(err)
	}
	if err := store.FulfilOrder(ctx, "ORD-1"); err != nil {
		t.Fatal(err)
	}
	if err := store.SyncCatalog(ctx, items); err != nil {
		t.Fatal(err)
	}
	if l := mustLevel(t, store, "dzwonek"); l.OnHand != 10 || l.Reserved != 0 {
		t.Errorf("restart changed stock: %+v", l)
	}

	moves, _ := store.Movements(ctx, "dzwonek", 10)
	if len(moves) != 3 || moves[len(moves)-1].Kind != shop.KindInitial {
		t.Errorf("ledger should hold initial, reserve, fulfil: %+v", moves)
	}
}

func TestReserveIsAllOrNothing(t *testing.T) {
	ctx := t.Context()
	store, _ := newTestStore(t)
	_ = store.SyncCatalog(ctx, []domain.ShopItem{stockedItem("a", 5), stockedItem("b", 1)})

	err := store.Reserve(ctx, "ORD-1", []shop.ReserveLine{
		{SKU: "a", ItemID: "a", Quantity: 3},
		{SKU: "b", ItemID: "b", Quantity: 2},
	})
	var short *shop.InsufficientStockError
	if !errors.As(err, &short) || short.SKU != "b" || short.Available != 1 || short.Requested != 2 {
		t.Fatalf("want shop.InsufficientStockError for b, got %v", err)
	}
	if got := mustLevel(t, store, "a"); got.Reserved != 0 {
		t.Errorf("a must be untouched after a failed reservation, got %+v", got)
	}

	if err := store.Reserve(ctx, "ORD-2", []shop.ReserveLine{{SKU: "a", ItemID: "a", Quantity: 3}, {SKU: "b", ItemID: "b", Quantity: 1}}); err != nil {
		t.Fatal(err)
	}
	if got := mustLevel(t, store, "a"); got.Available() != 2 {
		t.Errorf("a available = %d, want 2", got.Available())
	}
}

func TestReserveMergesDuplicateLines(t *testing.T) {
	ctx := t.Context()
	store, _ := newTestStore(t)
	_ = store.SyncCatalog(ctx, []domain.ShopItem{stockedItem("a", 5)})

	if err := store.Reserve(ctx, "ORD-1", []shop.ReserveLine{{SKU: "a", ItemID: "a", Quantity: 3}, {SKU: "a", ItemID: "a", Quantity: 3}}); err == nil {
		t.Fatal("3+3 must not fit in 5")
	}
	if err := store.Reserve(ctx, "ORD-1", []shop.ReserveLine{{SKU: "a", ItemID: "a", Quantity: 2}, {SKU: "a", ItemID: "a", Quantity: 3}}); err != nil {
		t.Fatalf("2+3 fits in 5: %v", err)
	}
}

func TestBackorderLinesMayExceedStock(t *testing.T) {
	ctx := t.Context()
	store, _ := newTestStore(t)

	if err := store.Reserve(ctx, "ORD-1", []shop.ReserveLine{{SKU: "x", ItemID: "x", Quantity: 4, Backorder: true}}); err != nil {
		t.Fatalf("backorder on a SKU without stock: %v", err)
	}
	if l := mustLevel(t, store, "x"); l.OnHand != 0 || l.Reserved != 4 {
		t.Errorf("got %+v", l)
	}
	if err := store.Reserve(ctx, "ORD-2", []shop.ReserveLine{{SKU: "x", ItemID: "x", Quantity: 1}}); err == nil {
		t.Error("a non-backorder line must not take units that are already promised")
	}
}

func TestConcurrentReservationsNeverOversell(t *testing.T) {
	ctx := t.Context()
	store, _ := newTestStore(t)
	_ = store.SyncCatalog(ctx, []domain.ShopItem{stockedItem("last", 5)})

	var wg sync.WaitGroup
	var won, lost atomic.Int32
	for i := range 25 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := store.Reserve(ctx, fmt.Sprintf("ORD-%d", i), []shop.ReserveLine{{SKU: "last", ItemID: "last", Quantity: 1}})
			var short *shop.InsufficientStockError
			switch {
			case err == nil:
				won.Add(1)
			case errors.As(err, &short):
				lost.Add(1)
			default:
				t.Errorf("unexpected error: %v", err)
			}
		}()
	}
	wg.Wait()

	if won.Load() != 5 || lost.Load() != 20 {
		t.Errorf("won=%d lost=%d, want 5 and 20", won.Load(), lost.Load())
	}
	if l := mustLevel(t, store, "last"); l.Reserved != 5 || l.Available() != 0 {
		t.Errorf("level = %+v", l)
	}
}

func TestReleaseAndFulfilAreIdempotent(t *testing.T) {
	ctx := t.Context()
	store, _ := newTestStore(t)
	_ = store.SyncCatalog(ctx, []domain.ShopItem{stockedItem("a", 10)})

	_ = store.Reserve(ctx, "ORD-cancel", []shop.ReserveLine{{SKU: "a", ItemID: "a", Quantity: 4}})
	_ = store.Reserve(ctx, "ORD-sold", []shop.ReserveLine{{SKU: "a", ItemID: "a", Quantity: 3}})

	for range 2 {
		if err := store.ReleaseOrder(ctx, "ORD-cancel"); err != nil {
			t.Fatal(err)
		}
		if err := store.FulfilOrder(ctx, "ORD-sold"); err != nil {
			t.Fatal(err)
		}
	}
	if l := mustLevel(t, store, "a"); l.OnHand != 7 || l.Reserved != 0 {
		t.Errorf("level = %+v, want on hand 7, reserved 0", l)
	}

	// A fulfilled order can no longer be released (and vice versa).
	_ = store.ReleaseOrder(ctx, "ORD-sold")
	_ = store.FulfilOrder(ctx, "ORD-cancel")
	if l := mustLevel(t, store, "a"); l.OnHand != 7 || l.Reserved != 0 {
		t.Errorf("settled reservations must stay settled, got %+v", l)
	}
	if held, _ := store.HoldsStock(ctx, "ORD-sold"); held {
		t.Error("fulfilled order should not hold stock")
	}
}

func TestFulfilClampsWhenGoodsWereNeverReceived(t *testing.T) {
	ctx := t.Context()
	store, _ := newTestStore(t)

	_ = store.Reserve(ctx, "ORD-1", []shop.ReserveLine{{SKU: "pre", ItemID: "pre", Quantity: 3, Backorder: true}})
	if err := store.CanFulfil(ctx, "ORD-1"); !errors.Is(err, shop.ErrNotEnoughOnHand) {
		t.Fatalf("CanFulfil = %v, want shop.ErrNotEnoughOnHand", err)
	}
	if err := store.FulfilOrder(ctx, "ORD-1"); err != nil {
		t.Fatalf("projector-side fulfilment must never fail: %v", err)
	}
	l := mustLevel(t, store, "pre")
	if l.OnHand != 0 || l.Reserved != 0 {
		t.Errorf("level = %+v", l)
	}
	moves, _ := store.Movements(ctx, "pre", 1)
	if moves[0].Note == "" {
		t.Error("the shortfall should be recorded in the ledger")
	}
}

func TestAdjust(t *testing.T) {
	ctx := t.Context()
	store, _ := newTestStore(t)
	_ = store.SyncCatalog(ctx, []domain.ShopItem{stockedItem("a", 10)})
	_ = store.Reserve(ctx, "ORD-1", []shop.ReserveLine{{SKU: "a", ItemID: "a", Quantity: 6}})

	adjust := func(kind string, qty int) error {
		return store.Adjust(ctx, shop.AdjustInput{SKU: "a", ItemID: "a", Kind: kind, Quantity: qty, Note: "test", Actor: "admin"})
	}

	if err := adjust(shop.KindReceive, 5); err != nil {
		t.Fatal(err)
	}
	if got := mustLevel(t, store, "a").OnHand; got != 15 {
		t.Errorf("after receive: %d", got)
	}
	if err := adjust(shop.KindLoss, 9); err != nil {
		t.Fatal(err)
	}
	if err := adjust(shop.KindLoss, 1); err == nil {
		t.Error("losing units held by an open order must be refused (6 on hand, 6 reserved)")
	}
	if err := adjust(shop.KindCount, 5); err == nil {
		t.Error("counting below the reserved quantity must be refused")
	}
	if err := adjust(shop.KindCount, 8); err != nil {
		t.Fatal(err)
	}
	if got := mustLevel(t, store, "a").OnHand; got != 8 {
		t.Errorf("after stocktake: %d", got)
	}
	if err := adjust(shop.KindCount, 8); err != nil {
		t.Errorf("recounting the same value is a no-op, got %v", err)
	}
	if err := adjust(shop.KindReceive, 0); err == nil {
		t.Error("receiving zero must be refused")
	}
	if err := adjust("bogus", 1); err == nil {
		t.Error("unknown kinds must be refused")
	}

	moves, _ := store.Movements(ctx, "a", 20)
	last := moves[0]
	if last.Kind != shop.KindCount || last.DeltaOnHand != 8-6 || last.Actor != "admin" || last.Note != "test" {
		t.Errorf("last movement = %+v", last)
	}
}

func TestStaleReservations(t *testing.T) {
	ctx := t.Context()
	store, db := newTestStore(t)
	_ = store.SyncCatalog(ctx, []domain.ShopItem{stockedItem("a", 10)})

	old := time.Now().UTC().Add(-72 * time.Hour).Format("2006-01-02 15:04:05")
	fresh := time.Now().UTC().Format("2006-01-02 15:04:05")
	for id, row := range map[string][2]string{
		"ORD-old":   {"awaiting_payment", old},
		"ORD-fresh": {"awaiting_payment", fresh},
		"ORD-paid":  {"paid", old},
	} {
		if _, err := db.Exec(`INSERT INTO orders (id, status, created_at) VALUES (?, ?, ?)`, id, row[0], row[1]); err != nil {
			t.Fatal(err)
		}
		if err := store.Reserve(ctx, id, []shop.ReserveLine{{SKU: "a", ItemID: "a", Quantity: 1}}); err != nil {
			t.Fatal(err)
		}
	}

	ids, err := store.StaleReservations(ctx, time.Now().Add(-48*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 1 || ids[0] != "ORD-old" {
		t.Errorf("stale = %v, want only ORD-old", ids)
	}
}

func TestAffiliateClicks(t *testing.T) {
	ctx := t.Context()
	store, db := newTestStore(t)

	for range 3 {
		if err := store.RecordClick(ctx, "lampka"); err != nil {
			t.Fatal(err)
		}
	}
	_ = store.RecordClick(ctx, "kask")
	if _, err := db.Exec(`INSERT INTO shop_affiliate_clicks (item_id, day, clicks) VALUES ('lampka', '2020-01-01', 7)`); err != nil {
		t.Fatal(err)
	}

	stats, err := store.Clicks(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if stats["lampka"].Total != 10 || stats["lampka"].Last30Days != 3 || stats["kask"].Total != 1 {
		t.Errorf("stats = %+v", stats)
	}
}
