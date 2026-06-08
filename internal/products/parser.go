package products

import (
	"fmt"
	"os"
	"path/filepath"
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
	ID          string             `yaml:"id"`
	Name        string             `yaml:"name"`
	BasePrice   int                `yaml:"basePrice"`
	Image       string             `yaml:"image"`
	Images      []string           `yaml:"images"`
	Icon        string             `yaml:"icon"`
	BookedDates []string           `yaml:"bookedDates"`
	Addons      []AddonFrontmatter `yaml:"addons"`
}

type AddonFrontmatter struct {
	ID    string `yaml:"id"`
	Name  string `yaml:"name"`
	Price int    `yaml:"price"`
	Icon  string `yaml:"icon"`
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
		addons[i] = domain.ProductAddon{
			ID:    addon.ID,
			Name:  addon.Name,
			Price: addon.Price,
			Icon:  addon.Icon,
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
