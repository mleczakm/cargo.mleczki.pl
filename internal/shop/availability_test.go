package shop_test

import (
	"testing"

	"cargo.mleczki.pl/internal/domain"
	"cargo.mleczki.pl/internal/shop"
)

func trackedItem(policy domain.StockPolicy) domain.ShopItem {
	policy.Mode = domain.StockTracked
	return domain.ShopItem{ID: "item", Type: domain.ShopItemDirect, Price: domain.Zloty(10), Stock: policy}
}

func TestAvailabilityFor(t *testing.T) {
	cases := []struct {
		name      string
		item      domain.ShopItem
		level     shop.Level
		state     shop.State
		canBuy    bool
		maxQty    int
		available int
	}{
		{"plenty", trackedItem(domain.StockPolicy{}), shop.Level{OnHand: 20}, shop.StateInStock, true, 10, 20},
		{"capped by max per order", trackedItem(domain.StockPolicy{MaxPerOrder: 4}), shop.Level{OnHand: 20}, shop.StateInStock, true, 4, 20},
		{"last pieces", trackedItem(domain.StockPolicy{}), shop.Level{OnHand: 3}, shop.StateLow, true, 3, 3},
		{"custom threshold", trackedItem(domain.StockPolicy{LowThreshold: 8}), shop.Level{OnHand: 8}, shop.StateLow, true, 8, 8},
		{"reserved units do not count", trackedItem(domain.StockPolicy{}), shop.Level{OnHand: 5, Reserved: 5}, shop.StateOut, false, 0, 0},
		{"partly reserved", trackedItem(domain.StockPolicy{}), shop.Level{OnHand: 5, Reserved: 4}, shop.StateLow, true, 1, 1},
		{"no ledger row", trackedItem(domain.StockPolicy{}), shop.Level{}, shop.StateOut, false, 0, 0},
		{"backorder when empty", trackedItem(domain.StockPolicy{Backorder: true, LeadTime: "5 dni"}), shop.Level{}, shop.StateBackorder, true, 10, 0},
		{"backorder may exceed stock", trackedItem(domain.StockPolicy{Backorder: true}), shop.Level{OnHand: 2}, shop.StateLow, true, 10, 2},
		{"oversold backorder is not negative", trackedItem(domain.StockPolicy{Backorder: true}), shop.Level{OnHand: 1, Reserved: 4}, shop.StateBackorder, true, 10, 0},
		{"unlimited", domain.ShopItem{ID: "u", Type: domain.ShopItemDirect, Stock: domain.StockPolicy{Mode: domain.StockUnlimited}}, shop.Level{}, shop.StateInStock, true, 10, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := shop.AvailabilityFor(tc.item, tc.level)
			if got.State != tc.state || got.CanBuy != tc.canBuy || got.MaxQuantity != tc.maxQty || got.Available != tc.available {
				t.Errorf("got %+v, want state=%s canBuy=%v max=%d available=%d", got, tc.state, tc.canBuy, tc.maxQty, tc.available)
			}
		})
	}
}

func TestBackorderLabelMentionsLeadTime(t *testing.T) {
	got := shop.AvailabilityFor(trackedItem(domain.StockPolicy{Backorder: true, LeadTime: "5-7 dni"}), shop.Level{})
	if got.Label != "Na zamówienie – 5-7 dni" {
		t.Errorf("label = %q", got.Label)
	}
}

func TestItemAvailabilityPicksTheBestVariant(t *testing.T) {
	item := trackedItem(domain.StockPolicy{})
	item.Variants = []domain.ShopVariant{{SKU: "s", Name: "S"}, {SKU: "m", Name: "M"}, {SKU: "l", Name: "L"}}

	levels := map[string]shop.Level{"s": {OnHand: 0}, "m": {OnHand: 2}, "l": {OnHand: 9}}
	if got := shop.ItemAvailability(item, levels); got.State != shop.StateInStock {
		t.Errorf("expected in stock thanks to size L, got %+v", got)
	}

	levels = map[string]shop.Level{"m": {OnHand: 1}}
	if got := shop.ItemAvailability(item, levels); got.State != shop.StateLow || !got.CanBuy {
		t.Errorf("expected low stock thanks to size M, got %+v", got)
	}

	if got := shop.ItemAvailability(item, nil); got.State != shop.StateOut || got.CanBuy {
		t.Errorf("expected out of stock, got %+v", got)
	}
}
