package domain

// Product represents a rental item.
type Product struct {
	ID          string
	Name        string
	Description string
	BasePrice   int
	Image       string
	Images      []string
	Icon        string
	Addons      []ProductAddon
	BookedDates []string
}

// ProductAddon represents an optional add-on for a product.
type ProductAddon struct {
	ID    string
	Name  string
	Price int
	Icon  string
}

// ProductAvailability represents availability for a specific date.
type ProductAvailability struct {
	ProductID string
	Date      string
	IsBooked  bool
}
