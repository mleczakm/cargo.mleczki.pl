package domain

// Visibility represents who can see a product or article.
type Visibility string

const (
	VisibilityPublic   Visibility = "public"    // Visible to everyone
	VisibilityLoggedIn Visibility = "logged-in" // Visible only to logged-in users
	VisibilityAdmin    Visibility = "admin"     // Visible only to admins
	VisibilityHidden   Visibility = "hidden"    // Not visible to anyone
)

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
	Articles    []ArticleReference
	Visibility  Visibility
	Priority    int // Higher priority = shown first
}

// ProductAddon represents an optional add-on for a product.
type ProductAddon struct {
	ID    string
	Name  string
	Price int
	Icon  string
	Image string
}

// ArticleReference represents a reference to an external article linked to a product.
type ArticleReference struct {
	ID          string
	Title       string
	Type        string // "test", "blog", "recommendation"
	URL         string
	PublishedAt string
	Author      string
	Summary     string
}

// ProductAvailability represents availability for a specific date.
type ProductAvailability struct {
	ProductID string
	Date      string
	IsBooked  bool
}
