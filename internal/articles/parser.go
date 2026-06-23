package articles

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"cargo.mleczki.pl/internal/domain"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer/html"
	"gopkg.in/yaml.v3"
)

// Parser handles parsing article markdown files.
type Parser struct {
	articlesDir string
	markdown    goldmark.Markdown
}

// NewParser creates a new article parser.
func NewParser(articlesDir string) *Parser {
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
		articlesDir: articlesDir,
		markdown:    md,
	}
}

// ArticleFrontmatter represents the YAML frontmatter in article files.
type ArticleFrontmatter struct {
	ID              string   `yaml:"id"`
	Title           string   `yaml:"title"`
	Category        string   `yaml:"category"`
	Author          string   `yaml:"author"`
	Summary         string   `yaml:"summary"`
	Image           string   `yaml:"image"`
	PublishedAt     string   `yaml:"publishedAt"`
	RelatedProducts []string `yaml:"relatedProducts"`
	Tags            []string `yaml:"tags"`
	Visibility      string   `yaml:"visibility"`
	Priority        int      `yaml:"priority"`
}

// LoadAllArticles loads all articles from the articles directory.
func (p *Parser) LoadAllArticles() ([]domain.Article, error) {
	var articles []domain.Article

	files, err := os.ReadDir(p.articlesDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read articles directory: %w", err)
	}

	for _, file := range files {
		if file.IsDir() || !strings.HasSuffix(file.Name(), ".md") {
			continue
		}

		article, err := p.LoadArticle(file.Name())
		if err != nil {
			return nil, fmt.Errorf("failed to load article %s: %w", file.Name(), err)
		}

		articles = append(articles, *article)
	}

	// Sort by priority (highest first), then by creation date (newest first)
	sort.Slice(articles, func(i, j int) bool {
		if articles[i].Priority != articles[j].Priority {
			return articles[i].Priority > articles[j].Priority
		}
		return articles[i].PublishedAt.After(articles[j].PublishedAt)
	})

	return articles, nil
}

// LoadArticlesByCategory loads articles filtered by category.
func (p *Parser) LoadArticlesByCategory(category domain.ArticleCategory) ([]domain.Article, error) {
	allArticles, err := p.LoadAllArticles()
	if err != nil {
		return nil, err
	}

	var filtered []domain.Article
	for _, article := range allArticles {
		if article.Category == category {
			filtered = append(filtered, article)
		}
	}

	return filtered, nil
}

// LoadArticle loads a single article from a markdown file.
func (p *Parser) LoadArticle(filename string) (*domain.Article, error) {
	filePath := filepath.Join(p.articlesDir, filename)
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
	var frontmatter ArticleFrontmatter
	if err := yaml.Unmarshal([]byte(parts[1]), &frontmatter); err != nil {
		return nil, fmt.Errorf("failed to parse frontmatter: %w", err)
	}

	// Convert markdown to HTML
	var htmlBuilder strings.Builder
	if err := p.markdown.Convert([]byte(parts[2]), &htmlBuilder); err != nil {
		return nil, fmt.Errorf("failed to convert markdown: %w", err)
	}

	// Parse published date
	publishedAt, err := time.Parse("2006-01-02", frontmatter.PublishedAt)
	if err != nil {
		publishedAt = time.Now()
	}

	// Ensure image has correct path
	image := frontmatter.Image
	if image != "" && !strings.HasPrefix(image, "http") && !strings.HasPrefix(image, "/") {
		image = "/data/images/articles/" + image
	}

	article := &domain.Article{
		ID:              frontmatter.ID,
		Title:           frontmatter.Title,
		Category:        domain.ArticleCategory(frontmatter.Category),
		Author:          frontmatter.Author,
		Summary:         frontmatter.Summary,
		Content:         htmlBuilder.String(),
		Image:           image,
		PublishedAt:     publishedAt,
		RelatedProducts: frontmatter.RelatedProducts,
		Tags:            frontmatter.Tags,
		Visibility:      domain.Visibility(frontmatter.Visibility),
		Priority:        frontmatter.Priority,
		UpdatedAt:       time.Now(),
	}

	return article, nil
}

// LoadArticleByID loads an article by its ID.
func (p *Parser) LoadArticleByID(id string) (*domain.Article, error) {
	files, err := os.ReadDir(p.articlesDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read articles directory: %w", err)
	}

	for _, file := range files {
		if file.IsDir() || !strings.HasSuffix(file.Name(), ".md") {
			continue
		}

		article, err := p.LoadArticle(file.Name())
		if err != nil {
			continue
		}

		if article.ID == id {
			return article, nil
		}
	}

	return nil, fmt.Errorf("article not found: %s", id)
}

// LoadArticlesByProductID loads articles related to a specific product.
func (p *Parser) LoadArticlesByProductID(productID string) ([]domain.Article, error) {
	allArticles, err := p.LoadAllArticles()
	if err != nil {
		return nil, err
	}

	var related []domain.Article
	for _, article := range allArticles {
		for _, relatedID := range article.RelatedProducts {
			if relatedID == productID {
				related = append(related, article)
				break
			}
		}
	}

	return related, nil
}
