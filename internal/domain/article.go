package domain

import "time"

// ArticleCategory represents the category of an article.
type ArticleCategory string

const (
	CategoryPorady     ArticleCategory = "porady"     // Tips/advice
	CategoryRecenzje   ArticleCategory = "recenzje"   // Reviews
	CategoryPrzewodnik ArticleCategory = "przewodnik" // Guides
)

// Article represents a blog article or review.
type Article struct {
	ID              string
	Title           string
	Category        ArticleCategory
	Author          string
	Summary         string
	Content         string // Markdown content
	Image           string
	PublishedAt     time.Time
	RelatedProducts []string // Product IDs this article relates to
	Tags            []string
	Visibility      Visibility
	Priority        int // Higher priority = shown first
	UpdatedAt       time.Time
}
