// Package shop implements the shop: a flat-file catalog (markdown with YAML
// frontmatter, like products and articles) and a stock ledger kept in SQLite.
package shop

import (
	"errors"
	"fmt"
	stdhtml "html"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer/html"
	"gopkg.in/yaml.v3"

	"cargo.mleczki.pl/internal/crosslinks"
	"cargo.mleczki.pl/internal/domain"
)

const summaryMaxRunes = 160

var (
	idPattern  = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)
	tagPattern = regexp.MustCompile(`<[^>]*>`)
)

// Parser reads shop items from markdown files. Results are cached until a file
// in the directory changes, because several lookups happen on every page view.
type Parser struct {
	dir      string
	markdown goldmark.Markdown

	mu       sync.Mutex
	cacheKey string
	items    []domain.ShopItem
	problems error
}

// NewParser creates a parser reading dir (usually data/shop).
func NewParser(dir string) *Parser {
	md := goldmark.New(
		goldmark.WithExtensions(extension.GFM),
		goldmark.WithParserOptions(parser.WithAutoHeadingID()),
		goldmark.WithRendererOptions(html.WithHardWraps()),
	)
	return &Parser{dir: dir, markdown: md}
}

type itemFrontmatter struct {
	ID              string                `yaml:"id"`
	Name            string                `yaml:"name"`
	Type            string                `yaml:"type"`
	Category        string                `yaml:"category"`
	Summary         string                `yaml:"summary"`
	Price           domain.Money          `yaml:"price"`
	Image           string                `yaml:"image"`
	Images          []string              `yaml:"images"`
	Icon            string                `yaml:"icon"`
	Tags            []string              `yaml:"tags"`
	Stock           *stockFrontmatter     `yaml:"stock"`
	Shipping        *shippingFrontmatter  `yaml:"shipping"`
	Variants        []variantFrontmatter  `yaml:"variants"`
	Affiliate       *affiliateFrontmatter `yaml:"affiliate"`
	RelatedProducts []string              `yaml:"relatedProducts"`
	RelatedArticles []string              `yaml:"relatedArticles"`
	RelatedShop     []string              `yaml:"relatedShop"`
	Visibility      string                `yaml:"visibility"`
	Priority        int                   `yaml:"priority"`
}

type stockFrontmatter struct {
	Mode         string `yaml:"mode"`
	Initial      int    `yaml:"initial"`
	LowThreshold int    `yaml:"lowThreshold"`
	Backorder    bool   `yaml:"backorder"`
	LeadTime     string `yaml:"leadTime"`
	MaxPerOrder  int    `yaml:"maxPerOrder"`
}

type shippingFrontmatter struct {
	WeightG    int  `yaml:"weightG"`
	LengthCm   int  `yaml:"lengthCm"`
	WidthCm    int  `yaml:"widthCm"`
	HeightCm   int  `yaml:"heightCm"`
	PickupOnly bool `yaml:"pickupOnly"`
}

type variantFrontmatter struct {
	SKU   string       `yaml:"sku"`
	Name  string       `yaml:"name"`
	Price domain.Money `yaml:"price"`
	Image string       `yaml:"image"`
	Stock int          `yaml:"stock"`
}

type affiliateFrontmatter struct {
	URL       string `yaml:"url"`
	Merchant  string `yaml:"merchant"`
	Label     string `yaml:"label"`
	PriceHint string `yaml:"priceHint"`
}

// LoadAll loads every valid item. Files that fail validation are skipped and
// reported through the returned error (joined, one problem per line), so a typo
// in one file never takes the whole shop down: callers should log the error and
// carry on with the items.
func (p *Parser) LoadAll() ([]domain.ShopItem, error) {
	entries, err := os.ReadDir(p.dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("read shop directory: %w", err)
	}

	key := cacheKey(entries)
	p.mu.Lock()
	defer p.mu.Unlock()
	if key == p.cacheKey && p.cacheKey != "" {
		return slices.Clone(p.items), p.problems
	}
	items, problems := p.loadEntries(entries)
	p.cacheKey, p.items, p.problems = key, items, problems
	return slices.Clone(items), problems
}

// cacheKey changes whenever a markdown file is added, removed or modified.
func cacheKey(entries []os.DirEntry) string {
	var key strings.Builder
	key.WriteString("shop:")
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return "" // never cache what cannot be fingerprinted
		}
		fmt.Fprintf(&key, "%s|%d|%d;", entry.Name(), info.Size(), info.ModTime().UnixNano())
	}
	return key.String()
}

func (p *Parser) loadEntries(entries []os.DirEntry) ([]domain.ShopItem, error) {
	var (
		items    []domain.ShopItem
		problems []error
		ids      = map[string]string{} // id -> file
		skus     = map[string]string{} // sku -> item id
	)
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		item, err := p.LoadFile(entry.Name())
		if err != nil {
			problems = append(problems, fmt.Errorf("%s: %w", entry.Name(), err))
			continue
		}
		if other, dup := ids[item.ID]; dup {
			problems = append(problems, fmt.Errorf("%s: id %q is already used by %s", entry.Name(), item.ID, other))
			continue
		}
		if clash := firstSKUClash(item, skus); clash != "" {
			problems = append(problems, fmt.Errorf("%s: sku %q is already used by item %q", entry.Name(), clash, skus[clash]))
			continue
		}
		ids[item.ID] = entry.Name()
		for _, sku := range item.SKUs() {
			skus[sku] = item.ID
		}
		items = append(items, *item)
	}

	sort.SliceStable(items, func(i, j int) bool {
		if items[i].Priority != items[j].Priority {
			return items[i].Priority > items[j].Priority
		}
		return items[i].Name < items[j].Name
	})
	return items, errors.Join(problems...)
}

// LoadByID finds a valid item by ID.
func (p *Parser) LoadByID(id string) (*domain.ShopItem, error) {
	items, _ := p.LoadAll()
	for i := range items {
		if items[i].ID == id {
			return &items[i], nil
		}
	}
	return nil, fmt.Errorf("shop item not found: %s", id)
}

func firstSKUClash(item *domain.ShopItem, used map[string]string) string {
	for _, sku := range item.SKUs() {
		if _, taken := used[sku]; taken {
			return sku
		}
	}
	return ""
}

// LoadFile parses and validates a single item file.
func (p *Parser) LoadFile(filename string) (*domain.ShopItem, error) {
	raw, err := os.ReadFile(filepath.Join(p.dir, filename)) // #nosec G304 - filename comes from ReadDir of the shop directory
	if err != nil {
		return nil, fmt.Errorf("read file: %w", err)
	}
	front, body, err := splitFrontmatter(string(raw))
	if err != nil {
		return nil, err
	}

	var fm itemFrontmatter
	if err := yaml.Unmarshal([]byte(front), &fm); err != nil {
		return nil, fmt.Errorf("parse frontmatter: %w", err)
	}

	var rendered strings.Builder
	if err := p.markdown.Convert([]byte(body), &rendered); err != nil {
		return nil, fmt.Errorf("convert markdown: %w", err)
	}

	item, err := buildItem(fm, rendered.String())
	if err != nil {
		return nil, err
	}
	return item, nil
}

func splitFrontmatter(content string) (string, string, error) {
	content = strings.TrimPrefix(content, "\ufeff")
	content = strings.ReplaceAll(content, "\r\n", "\n")
	rest, ok := strings.CutPrefix(strings.TrimLeft(content, "\n"), "---\n")
	if !ok {
		return "", "", errors.New("invalid markdown format: missing frontmatter")
	}
	end := strings.Index(rest, "\n---")
	if end < 0 {
		return "", "", errors.New("invalid markdown format: frontmatter is not closed")
	}
	body := strings.TrimPrefix(rest[end+len("\n---"):], "\n")
	return rest[:end], body, nil
}

func buildItem(fm itemFrontmatter, descriptionHTML string) (*domain.ShopItem, error) {
	if !idPattern.MatchString(fm.ID) {
		return nil, fmt.Errorf("id %q must be lowercase letters, digits and dashes", fm.ID)
	}
	if strings.TrimSpace(fm.Name) == "" {
		return nil, errors.New("name is required")
	}

	itemType, err := resolveType(fm)
	if err != nil {
		return nil, err
	}
	visibility, err := resolveVisibility(fm.Visibility)
	if err != nil {
		return nil, err
	}

	item := &domain.ShopItem{
		ID:              fm.ID,
		Name:            strings.TrimSpace(fm.Name),
		Type:            itemType,
		Category:        strings.TrimSpace(fm.Category),
		Description:     descriptionHTML,
		Image:           normalizeImage(fm.Image),
		Icon:            fm.Icon,
		Tags:            fm.Tags,
		RelatedProducts: fm.RelatedProducts,
		RelatedArticles: fm.RelatedArticles,
		RelatedShop:     fm.RelatedShop,
		Visibility:      visibility,
		Priority:        fm.Priority,
	}
	for _, img := range fm.Images {
		item.Images = append(item.Images, normalizeImage(img))
	}
	if item.Image == "" && len(item.Images) > 0 {
		item.Image = item.Images[0]
	}
	item.Summary = strings.TrimSpace(fm.Summary)
	if item.Summary == "" {
		item.Summary = summarize(descriptionHTML)
	}

	if itemType == domain.ShopItemAffiliate {
		if err := fillAffiliate(item, fm); err != nil {
			return nil, err
		}
		return item, nil
	}
	if err := fillDirect(item, fm); err != nil {
		return nil, err
	}
	return item, nil
}

func resolveType(fm itemFrontmatter) (domain.ShopItemType, error) {
	switch strings.ToLower(strings.TrimSpace(fm.Type)) {
	case "":
		if fm.Affiliate != nil {
			return domain.ShopItemAffiliate, nil
		}
		return domain.ShopItemDirect, nil
	case string(domain.ShopItemDirect):
		return domain.ShopItemDirect, nil
	case string(domain.ShopItemAffiliate):
		return domain.ShopItemAffiliate, nil
	default:
		return "", fmt.Errorf("type %q must be %q or %q", fm.Type, domain.ShopItemDirect, domain.ShopItemAffiliate)
	}
}

func resolveVisibility(raw string) (domain.Visibility, error) {
	switch domain.Visibility(raw) {
	case "":
		return domain.VisibilityPublic, nil
	case domain.VisibilityPublic, domain.VisibilityLoggedIn, domain.VisibilityAdmin, domain.VisibilityHidden:
		return domain.Visibility(raw), nil
	default:
		return "", fmt.Errorf("visibility %q must be public, logged-in, admin or hidden", raw)
	}
}

func fillAffiliate(item *domain.ShopItem, fm itemFrontmatter) error {
	switch {
	case fm.Affiliate == nil:
		return errors.New("affiliate items need an affiliate block with a url")
	case !fm.Price.IsZero():
		return errors.New("price is not used by affiliate items (use affiliate.priceHint)")
	case len(fm.Variants) > 0:
		return errors.New("variants are not supported on affiliate items")
	case fm.Stock != nil:
		return errors.New("stock is not used by affiliate items")
	case fm.Shipping != nil:
		return errors.New("shipping is not used by affiliate items (the seller ships them)")
	}

	target, err := url.Parse(strings.TrimSpace(fm.Affiliate.URL))
	if err != nil || (target.Scheme != "http" && target.Scheme != "https") || target.Host == "" {
		return fmt.Errorf("affiliate.url %q must be an absolute http(s) URL", fm.Affiliate.URL)
	}

	merchant := strings.TrimSpace(fm.Affiliate.Merchant)
	if merchant == "" {
		merchant = MerchantName(target.Hostname())
	} else {
		merchant = MerchantName(merchant)
	}
	label := strings.TrimSpace(fm.Affiliate.Label)
	if label == "" {
		label = "Zobacz ofertę w serwisie " + merchant
	}
	item.Affiliate = &domain.AffiliateLink{
		URL:       target.String(),
		Merchant:  merchant,
		Label:     label,
		PriceHint: strings.TrimSpace(fm.Affiliate.PriceHint),
	}
	return nil
}

func fillDirect(item *domain.ShopItem, fm itemFrontmatter) error {
	if fm.Affiliate != nil {
		return errors.New("affiliate block on a direct item: set type: affiliate or remove it")
	}
	if fm.Price.IsNegative() || (fm.Price.IsZero() && len(fm.Variants) == 0) {
		return errors.New("price must be greater than 0 (at most two decimal places)")
	}
	item.Price = fm.Price

	stock := fm.Stock
	if stock == nil {
		stock = &stockFrontmatter{}
	}
	mode := domain.StockMode(strings.ToLower(strings.TrimSpace(stock.Mode)))
	switch mode {
	case "":
		mode = domain.StockTracked
	case domain.StockTracked, domain.StockUnlimited:
	default:
		return fmt.Errorf("stock.mode %q must be %q or %q", stock.Mode, domain.StockTracked, domain.StockUnlimited)
	}
	if stock.Initial < 0 || stock.LowThreshold < 0 || stock.MaxPerOrder < 0 {
		return errors.New("stock numbers must not be negative")
	}
	if mode == domain.StockUnlimited && (stock.Initial != 0 || stock.Backorder) {
		return errors.New("stock.initial and stock.backorder only make sense for tracked items")
	}
	item.Stock = domain.StockPolicy{
		Mode:         mode,
		LowThreshold: stock.LowThreshold,
		Backorder:    stock.Backorder,
		LeadTime:     strings.TrimSpace(stock.LeadTime),
		MaxPerOrder:  stock.MaxPerOrder,
	}
	item.InitialStock = stock.Initial

	if sh := fm.Shipping; sh != nil {
		if sh.WeightG < 0 || sh.LengthCm < 0 || sh.WidthCm < 0 || sh.HeightCm < 0 {
			return errors.New("shipping numbers must not be negative")
		}
		item.Shipping = domain.ShopShipping{WeightG: sh.WeightG, LengthCm: sh.LengthCm, WidthCm: sh.WidthCm, HeightCm: sh.HeightCm, PickupOnly: sh.PickupOnly}
	}

	return fillVariants(item, fm)
}

func fillVariants(item *domain.ShopItem, fm itemFrontmatter) error {
	if len(fm.Variants) == 0 {
		return nil
	}
	if fm.Stock != nil && fm.Stock.Initial != 0 {
		return errors.New("stock.initial cannot be combined with variants: give each variant its own stock")
	}
	seen := map[string]bool{}
	for _, variant := range fm.Variants {
		switch {
		case !idPattern.MatchString(variant.SKU):
			return fmt.Errorf("variant sku %q must be lowercase letters, digits and dashes", variant.SKU)
		case seen[variant.SKU]:
			return fmt.Errorf("variant sku %q is repeated", variant.SKU)
		case strings.TrimSpace(variant.Name) == "":
			return fmt.Errorf("variant %q needs a name", variant.SKU)
		case variant.Stock < 0:
			return fmt.Errorf("variant %q: stock must not be negative", variant.SKU)
		case variant.Stock != 0 && item.Stock.Mode == domain.StockUnlimited:
			return fmt.Errorf("variant %q: stock only makes sense for tracked items", variant.SKU)
		}
		if variant.Price.IsNegative() {
			return fmt.Errorf("variant %q: price must not be negative", variant.SKU)
		}
		if variant.Price.IsZero() && item.Price.IsZero() {
			return fmt.Errorf("variant %q has no price and the item has none to inherit", variant.SKU)
		}
		seen[variant.SKU] = true
		item.Variants = append(item.Variants, domain.ShopVariant{
			SKU:          variant.SKU,
			Name:         strings.TrimSpace(variant.Name),
			Price:        variant.Price,
			Image:        normalizeImage(variant.Image),
			InitialStock: variant.Stock,
		})
	}
	return nil
}

func normalizeImage(path string) string {
	if path == "" || strings.HasPrefix(path, "http") || strings.HasPrefix(path, "/") {
		return path
	}
	return "/data/images/shop/" + path
}

// MerchantName turns a host or merchant key into a display name.
func MerchantName(hostOrName string) string {
	lower := strings.ToLower(strings.TrimSpace(hostOrName))
	switch {
	case strings.Contains(lower, "allegro"):
		return "Allegro"
	case strings.Contains(lower, "aliexpress"):
		return "AliExpress"
	case strings.Contains(lower, "amazon"):
		return "Amazon"
	case strings.Contains(lower, "olx"):
		return "OLX"
	case strings.Contains(lower, "ceneo"):
		return "Ceneo"
	}
	return strings.TrimPrefix(strings.TrimSpace(hostOrName), "www.")
}

// summarize derives a short plain-text summary from rendered HTML.
func summarize(descriptionHTML string) string {
	text := tagPattern.ReplaceAllString(crosslinks.Strip(descriptionHTML), " ")
	text = strings.Join(strings.Fields(stdhtml.UnescapeString(text)), " ")
	if utf8.RuneCountInString(text) <= summaryMaxRunes {
		return text
	}
	runes := []rune(text)
	return strings.TrimSpace(string(runes[:summaryMaxRunes-1])) + "…"
}
