package shop

import (
	"fmt"

	"cargo.mleczki.pl/internal/domain"
)

// State is the storefront view of how obtainable a stocked unit is.
type State string

const (
	StateInStock   State = "in_stock"
	StateLow       State = "low"
	StateBackorder State = "backorder"
	StateOut       State = "out"
)

// Availability tells what a visitor may do with a SKU or an item.
type Availability struct {
	State       State
	Available   int  // units free to order; meaningless when Unlimited
	Unlimited   bool // stock is not tracked
	CanBuy      bool
	MaxQuantity int // upper bound for the quantity picker
	Label       string
	LeadTime    string
}

// AvailabilityFor evaluates one SKU of a direct item against its stock level.
// A SKU without a ledger row has no stock.
func AvailabilityFor(item domain.ShopItem, level Level) Availability {
	if !item.Tracked() {
		return Availability{State: StateInStock, Unlimited: true, CanBuy: true, MaxQuantity: item.MaxQuantity(), Label: "Dostępny"}
	}

	free := max(level.Available(), 0)
	switch {
	case free > 0:
		result := Availability{State: StateInStock, Available: free, CanBuy: true, MaxQuantity: min(free, item.MaxQuantity()), Label: "Dostępny"}
		if item.Stock.Backorder {
			result.MaxQuantity = item.MaxQuantity()
		}
		if free <= item.LowThreshold() {
			result.State = StateLow
			result.Label = fmt.Sprintf("Ostatnie sztuki: %d", free)
		}
		return result
	case item.Stock.Backorder:
		label := "Na zamówienie"
		if item.Stock.LeadTime != "" {
			label += " – " + item.Stock.LeadTime
		}
		return Availability{State: StateBackorder, CanBuy: true, MaxQuantity: item.MaxQuantity(), Label: label, LeadTime: item.Stock.LeadTime}
	default:
		return Availability{State: StateOut, Label: "Brak w magazynie"}
	}
}

// ItemAvailability summarises an item: the best state among its SKUs, so a
// product with one size left is shown as obtainable.
func ItemAvailability(item domain.ShopItem, levels map[string]Level) Availability {
	best := Availability{State: StateOut, Label: "Brak w magazynie"}
	bestRank := -1
	for _, sku := range item.SKUs() {
		a := AvailabilityFor(item, levels[sku])
		if rank := stateRank(a.State); rank > bestRank {
			best, bestRank = a, rank
		}
	}
	return best
}

func stateRank(s State) int {
	switch s {
	case StateInStock:
		return 3
	case StateLow:
		return 2
	case StateBackorder:
		return 1
	case StateOut:
		return 0
	default:
		return 0
	}
}
