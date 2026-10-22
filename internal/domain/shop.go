package domain

// ShopItemType tells how a shop item is sold.
type ShopItemType string

const (
	// ShopItemDirect items are sold (and stocked) by the shop itself.
	ShopItemDirect ShopItemType = "direct"
	// ShopItemAffiliate items only link out to an external marketplace.
	ShopItemAffiliate ShopItemType = "affiliate"
)

// StockMode tells whether the shop keeps count of an item.
type StockMode string

const (
	// StockTracked items have a quantity that is reserved and sold.
	StockTracked StockMode = "tracked"
	// StockUnlimited items can always be ordered (made to order, digital, ...).
	StockUnlimited StockMode = "unlimited"
)

const (
	// DefaultLowStockThreshold is used when an item does not set its own.
	DefaultLowStockThreshold = 3
	// DefaultMaxPerOrder caps a single cart line when an item does not set its own.
	DefaultMaxPerOrder = 10
)

// StockPolicy holds the stock rules of a direct shop item. Quantities are not
// part of the policy: they live in the stock ledger (see internal/shop).
type StockPolicy struct {
	Mode         StockMode
	LowThreshold int
	Backorder    bool   // allow ordering when nothing is available
	LeadTime     string // human readable delivery estimate for backorders
	MaxPerOrder  int
}

// ShopVariant is a sellable variation of an item (size, colour, ...) with its own SKU and stock.
type ShopVariant struct {
	SKU          string
	Name         string
	Price        Money // zero inherits the item price
	Image        string
	InitialStock int
}

// ShopShipping describes how an item travels. The zero value means "an ordinary
// small item": it fits the default box and weighs the default item weight.
type ShopShipping struct {
	WeightG    int
	LengthCm   int
	WidthCm    int
	HeightCm   int
	PickupOnly bool // too big or too awkward to post: only collection in person
}

// AffiliateLink points at the external offer of an affiliate item.
type AffiliateLink struct {
	URL       string
	Merchant  string // normalised key, e.g. "allegro"
	Label     string // button text
	PriceHint string // free text, e.g. "od ok. 59 zł"
}

// ShopItem is a product sold in the shop or recommended through an affiliate link.
type ShopItem struct {
	ID              string
	Name            string
	Type            ShopItemType
	Category        string
	Summary         string
	Description     string // HTML rendered from markdown; may contain [[kind:id]] cross-links
	Price           Money  // direct items only
	Image           string
	Images          []string
	Icon            string
	Tags            []string
	Variants        []ShopVariant
	InitialStock    int // seeded once when the SKU is first seen
	Stock           StockPolicy
	Shipping        ShopShipping
	Affiliate       *AffiliateLink
	RelatedProducts []string // rental product IDs
	RelatedArticles []string
	RelatedShop     []string
	Visibility      Visibility
	Priority        int
}

// IsDirect reports whether the shop sells the item itself.
func (i ShopItem) IsDirect() bool { return i.Type == ShopItemDirect }

// IsAffiliate reports whether the item is an outbound recommendation.
func (i ShopItem) IsAffiliate() bool { return i.Type == ShopItemAffiliate }

// Tracked reports whether the item has stock that must be counted.
func (i ShopItem) Tracked() bool { return i.IsDirect() && i.Stock.Mode == StockTracked }

// HasVariants reports whether the customer has to pick a variant.
func (i ShopItem) HasVariants() bool { return len(i.Variants) > 0 }

// SKUs lists every sellable SKU of the item. An item without variants has a single SKU equal to its ID.
func (i ShopItem) SKUs() []string {
	if !i.HasVariants() {
		return []string{i.ID}
	}
	skus := make([]string, 0, len(i.Variants))
	for _, v := range i.Variants {
		skus = append(skus, v.SKU)
	}
	return skus
}

// Variant finds a variant by SKU.
func (i ShopItem) Variant(sku string) (ShopVariant, bool) {
	for _, v := range i.Variants {
		if v.SKU == sku {
			return v, true
		}
	}
	return ShopVariant{}, false
}

// HasSKU reports whether sku identifies a sellable unit of the item.
func (i ShopItem) HasSKU(sku string) bool {
	if !i.HasVariants() {
		return sku == i.ID
	}
	_, ok := i.Variant(sku)
	return ok
}

// UnitPrice is the price of one unit of sku.
func (i ShopItem) UnitPrice(sku string) Money {
	if v, ok := i.Variant(sku); ok && v.Price.IsPositive() {
		return v.Price
	}
	return i.Price
}

// LineName is the name shown in carts and orders for sku.
func (i ShopItem) LineName(sku string) string {
	if v, ok := i.Variant(sku); ok && v.Name != "" {
		return i.Name + " – " + v.Name
	}
	return i.Name
}

// InitialStockFor is the quantity seeded when sku is first seen.
func (i ShopItem) InitialStockFor(sku string) int {
	if v, ok := i.Variant(sku); ok {
		return v.InitialStock
	}
	return i.InitialStock
}

// MaxQuantity is the largest quantity of the item allowed on a single order.
func (i ShopItem) MaxQuantity() int {
	if i.Stock.MaxPerOrder > 0 {
		return i.Stock.MaxPerOrder
	}
	return DefaultMaxPerOrder
}

// LowThreshold is the available quantity at or below which the item is "low".
func (i ShopItem) LowThreshold() int {
	if i.Stock.LowThreshold > 0 {
		return i.Stock.LowThreshold
	}
	return DefaultLowStockThreshold
}
