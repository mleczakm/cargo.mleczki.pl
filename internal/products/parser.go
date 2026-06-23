package products

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"cargo.mleczki.pl/internal/domain"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer/html"
	"gopkg.in/yaml.v3"
)

// Parser handles parsing product markdown files.
type Parser struct {
	productsDir string
	markdown    goldmark.Markdown
}

// NewParser creates a new product parser.
func NewParser(productsDir string) *Parser {
	md := goldmark.New(
		goldmark.WithExtensions(extension.GFM),
		goldmark.WithParserOptions(
			parser.WithAutoHeadingID(),
		),
		goldmark.WithRendererOptions(
			html.WithHardWraps(),
		),
	)

	return &Parser{
		productsDir: productsDir,
		markdown:    md,
	}
}

// ProductFrontmatter represents the YAML frontmatter in product files.
type ProductFrontmatter struct {
	ID          string               `yaml:"id"`
	Name        string               `yaml:"name"`
	BasePrice   int                  `yaml:"basePrice"`
	Image       string               `yaml:"image"`
	Images      []string             `yaml:"images"`
	Icon        string               `yaml:"icon"`
	BookedDates []string             `yaml:"bookedDates"`
	Addons      []AddonFrontmatter   `yaml:"addons"`
	Articles    []ArticleFrontmatter `yaml:"articles"`
	Visibility  string               `yaml:"visibility"`
	Priority    int                  `yaml:"priority"`
}

type AddonFrontmatter struct {
	ID    string `yaml:"id"`
	Name  string `yaml:"name"`
	Price int    `yaml:"price"`
	Icon  string `yaml:"icon"`
	Image string `yaml:"image"`
}

type ArticleFrontmatter struct {
	ID          string `yaml:"id"`
	Title       string `yaml:"title"`
	Type        string `yaml:"type"`
	URL         string `yaml:"url"`
	PublishedAt string `yaml:"publishedAt"`
	Author      string `yaml:"author"`
	Summary     string `yaml:"summary"`
}

// LoadAllProducts loads all products from the products directory.
func (p *Parser) LoadAllProducts() ([]domain.Product, error) {
	var products []domain.Product

	files, err := os.ReadDir(p.productsDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read products directory: %w", err)
	}

	for _, file := range files {
		if file.IsDir() || !strings.HasSuffix(file.Name(), ".md") {
			continue
		}

		product, err := p.LoadProduct(file.Name())
		if err != nil {
			return nil, fmt.Errorf("failed to load product %s: %w", file.Name(), err)
		}

		products = append(products, *product)
	}

	// Sort by priority (highest first), then by native order (file order)
	sort.SliceStable(products, func(i, j int) bool {
		return products[i].Priority > products[j].Priority
	})

	return products, nil
}

// LoadProduct loads a single product from a markdown file.
func (p *Parser) LoadProduct(filename string) (*domain.Product, error) {
	filePath := filepath.Join(p.productsDir, filename)
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	// Split frontmatter and content
	parts := strings.SplitN(string(content), "---", 3)
	if len(parts) < 3 {
		return nil, fmt.Errorf("invalid markdown format: missing frontmatter")
	}

	// Parse YAML frontmatter
	var frontmatter ProductFrontmatter
	if err := yaml.Unmarshal([]byte(parts[1]), &frontmatter); err != nil {
		return nil, fmt.Errorf("failed to parse frontmatter: %w", err)
	}

	// Convert markdown to HTML
	var htmlBuilder strings.Builder
	if err := p.markdown.Convert([]byte(parts[2]), &htmlBuilder); err != nil {
		return nil, fmt.Errorf("failed to convert markdown: %w", err)
	}

	// Ensure images have correct paths
	if frontmatter.Image != "" && !strings.HasPrefix(frontmatter.Image, "http") && !strings.HasPrefix(frontmatter.Image, "/") {
		frontmatter.Image = "/data/images/products/" + frontmatter.Image
	}
	for i, img := range frontmatter.Images {
		if img != "" && !strings.HasPrefix(img, "http") && !strings.HasPrefix(img, "/") {
			frontmatter.Images[i] = "/data/images/products/" + img
		}
	}

	// Convert addons
	addons := make([]domain.ProductAddon, len(frontmatter.Addons))
	for i, addon := range frontmatter.Addons {
		addonImage := addon.Image
		if addonImage != "" && !strings.HasPrefix(addonImage, "http") && !strings.HasPrefix(addonImage, "/") {
			addonImage = "/data/images/addons/" + addonImage
		}
		addons[i] = domain.ProductAddon{
			ID:    addon.ID,
			Name:  addon.Name,
			Price: addon.Price,
			Icon:  addon.Icon,
			Image: addonImage,
		}
	}

	// Convert articles
	articles := make([]domain.ArticleReference, len(frontmatter.Articles))
	for i, article := range frontmatter.Articles {
		articles[i] = domain.ArticleReference{
			ID:          article.ID,
			Title:       article.Title,
			Type:        article.Type,
			URL:         article.URL,
			PublishedAt: article.PublishedAt,
			Author:      article.Author,
			Summary:     article.Summary,
		}
	}

	product := &domain.Product{
		ID:          frontmatter.ID,
		Name:        frontmatter.Name,
		Description: htmlBuilder.String(),
		BasePrice:   frontmatter.BasePrice,
		Image:       frontmatter.Image,
		Images:      frontmatter.Images,
		Icon:        frontmatter.Icon,
		Addons:      addons,
		BookedDates: frontmatter.BookedDates,
		Articles:    articles,
		Visibility:  domain.Visibility(frontmatter.Visibility),
		Priority:    frontmatter.Priority,
	}

	return product, nil
}

// LoadProductByID loads a product by its ID.
func (p *Parser) LoadProductByID(id string) (*domain.Product, error) {
	files, err := os.ReadDir(p.productsDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read products directory: %w", err)
	}

	for _, file := range files {
		if file.IsDir() || !strings.HasSuffix(file.Name(), ".md") {
			continue
		}

		product, err := p.LoadProduct(file.Name())
		if err != nil {
			continue
		}

		if product.ID == id {
			return product, nil
		}
	}

	return nil, fmt.Errorf("product not found: %s", id)
}
