package domain

import "time"

// ArticleCategory represents the category of an article.
type ArticleCategory string

const (
	CategoryHistorie   ArticleCategory = "historie"   // Stories from everyday riding
	CategoryPorady     ArticleCategory = "porady"     // Tips/advice
	CategoryRecenzje   ArticleCategory = "recenzje"   // Reviews
	CategoryPrzewodnik ArticleCategory = "przewodnik" // Guides
)

// Menu slots an article can be linked from.
const (
	MenuNav    = "nav"    // main navigation
	MenuFooter = "footer" // footer links
)

// ArticleSEO overrides what search engines and social networks show.
type ArticleSEO struct {
	Title       string // <title> and the social card title; defaults to the article title
	Description string // meta description; defaults to the summary
	NoIndex     bool   // keep the page out of search engines
}

// Article represents a blog article or review.
type Article struct {
	ID              string
	Title           string
	Subtitle        string
	Category        ArticleCategory
	Author          string
	Summary         string
	Content         string // HTML rendered from markdown and directives; may contain [[kind:id]] cross-links
	Excerpt         string // plain text start of the article, for descriptions
	Image           string
	ImageMissing    bool   // the hero photo is named but not in data/images/articles yet
	HeroFocus       string // CSS object-position of the hero photo
	PublishedAt     time.Time
	ReadingMinutes  int
	RelatedProducts []string // Product IDs this article relates to
	RelatedShop     []string // Shop item IDs this article relates to
	Tags            []string
	Visibility      Visibility
	Draft           bool // drafts are visible to the admin only, never indexed and never linked from menus
	SEO             ArticleSEO
	Menu            []string // MenuNav and/or MenuFooter
	MenuLabel       string   // link text in the menus; defaults to the title
	MenuOrder       int      // lower comes first
	MissingImages   []string // photos named in the article that are not in data/images/articles yet
	Priority        int      // Higher priority = shown first
	UpdatedAt       time.Time
}

// InMenu reports whether the article is linked from the given menu.
func (a Article) InMenu(slot string) bool {
	for _, s := range a.Menu {
		if s == slot {
			return true
		}
	}
	return false
}

// MenuText is the link text used in menus.
func (a Article) MenuText() string {
	if a.MenuLabel != "" {
		return a.MenuLabel
	}
	return a.Title
}
