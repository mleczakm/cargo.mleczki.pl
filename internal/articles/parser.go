package articles

import (
	"errors"
	"fmt"
	stdhtml "html"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"cargo.mleczki.pl/internal/crosslinks"
	"cargo.mleczki.pl/internal/domain"
	"cargo.mleczki.pl/internal/markdown"

	"github.com/yuin/goldmark"
	"gopkg.in/yaml.v3"
)

const (
	excerptMaxRunes = 200
	wordsPerMinute  = 200
)

var tagPattern = regexp.MustCompile(`<[^>]*>`)

// Parser handles parsing article markdown files. Parsed articles are cached
// until a file in the directory changes, because menus and lists read them on
// every page view.
type Parser struct {
	articlesDir string
	images      ImageDir
	markdown    goldmark.Markdown

	mu       sync.Mutex
	cacheKey string
	cached   []domain.Article
	problems error
}

// NewParser creates a new article parser. Photos are expected in
// data/images/articles, next to the articles directory.
func NewParser(articlesDir string) *Parser {
	return &Parser{
		articlesDir: articlesDir,
		images: ImageDir{
			Files: filepath.Join(filepath.Dir(articlesDir), "images", "articles"),
			URL:   "/data/images/articles/",
		},
		markdown: goldmark.New(markdown.Options()...),
	}
}

// ArticleFrontmatter represents the YAML frontmatter in article files.
type ArticleFrontmatter struct {
	ID              string   `yaml:"id"`
	Title           string   `yaml:"title"`
	Subtitle        string   `yaml:"subtitle"`
	Category        string   `yaml:"category"`
	Author          string   `yaml:"author"`
	Summary         string   `yaml:"summary"`
	Image           string   `yaml:"image"`
	HeroFocus       string   `yaml:"heroFocus"`
	PublishedAt     string   `yaml:"publishedAt"`
	RelatedProducts []string `yaml:"relatedProducts"`
	RelatedShop     []string `yaml:"relatedShop"`
	Tags            []string `yaml:"tags"`
	Visibility      string   `yaml:"visibility"`
	Status          string   `yaml:"status"` // published (default) or draft
	Priority        int      `yaml:"priority"`
	SEO             seoFront `yaml:"seo"`
	Menu            []string `yaml:"menu"`
	MenuLabel       string   `yaml:"menuLabel"`
	MenuOrder       int      `yaml:"menuOrder"`
}

type seoFront struct {
	Title       string `yaml:"title"`
	Description string `yaml:"description"`
	NoIndex     bool   `yaml:"noindex"`
}

// LoadAllArticles loads every valid article from the articles directory. A file
// that fails validation is left out and reported by Problems, so a typo in one
// draft never takes the whole blog down. The error is only for an unreadable directory.
func (p *Parser) LoadAllArticles() ([]domain.Article, error) {
	files, err := os.ReadDir(p.articlesDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read articles directory: %w", err)
	}

	key := p.fingerprint(files)
	p.mu.Lock()
	defer p.mu.Unlock()
	if key != "" && key == p.cacheKey {
		return slices.Clone(p.cached), nil
	}

	var (
		articles []domain.Article
		problems []error
	)
	for _, file := range files {
		if file.IsDir() || !strings.HasSuffix(file.Name(), ".md") {
			continue
		}
		article, err := p.LoadArticle(file.Name())
		if err != nil {
			problems = append(problems, fmt.Errorf("%s: %w", file.Name(), err))
			continue
		}
		articles = append(articles, *article)
	}

	// Sort by priority (highest first), then by creation date (newest first)
	sort.SliceStable(articles, func(i, j int) bool {
		if articles[i].Priority != articles[j].Priority {
			return articles[i].Priority > articles[j].Priority
		}
		return articles[i].PublishedAt.After(articles[j].PublishedAt)
	})

	p.cacheKey, p.cached, p.problems = key, articles, errors.Join(problems...)
	if p.problems != nil {
		log.Printf("Article problems (affected articles are hidden):\n%v", p.problems)
	}
	return slices.Clone(articles), nil
}

// Problems lists the article files that could not be loaded, one per line, as of the last load.
func (p *Parser) Problems() error {
	if _, err := p.LoadAllArticles(); err != nil {
		return err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.problems
}

// fingerprint changes whenever an article or one of its photos is added,
// removed or modified. An empty string means "do not cache".
func (p *Parser) fingerprint(files []os.DirEntry) string {
	var key strings.Builder
	for _, file := range files {
		if file.IsDir() || !strings.HasSuffix(file.Name(), ".md") {
			continue
		}
		info, err := file.Info()
		if err != nil {
			return ""
		}
		fmt.Fprintf(&key, "%s|%d|%d;", file.Name(), info.Size(), info.ModTime().UnixNano())
	}
	if photos, err := os.ReadDir(p.images.Files); err == nil {
		for _, photo := range photos {
			fmt.Fprintf(&key, "img:%s;", photo.Name())
		}
	}
	return key.String()
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

func splitFrontmatter(content string) (string, string, error) {
	content = strings.ReplaceAll(strings.TrimPrefix(content, "\xef\xbb\xbf"), "\r\n", "\n")
	rest, ok := strings.CutPrefix(strings.TrimLeft(content, "\n"), "---\n")
	if !ok {
		return "", "", errors.New("invalid markdown format: missing frontmatter")
	}
	end := strings.Index(rest, "\n---")
	if end < 0 {
		return "", "", errors.New("invalid markdown format: frontmatter is not closed")
	}
	return rest[:end], strings.TrimPrefix(rest[end+len("\n---"):], "\n"), nil
}

// LoadArticle loads a single article from a markdown file.
func (p *Parser) LoadArticle(filename string) (*domain.Article, error) {
	filePath := filepath.Join(p.articlesDir, filename)
	content, err := os.ReadFile(filePath) // #nosec G304 - filename comes from ReadDir of the articles directory
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	front, body, err := splitFrontmatter(string(content))
	if err != nil {
		return nil, err
	}
	var fm ArticleFrontmatter
	if err := yaml.Unmarshal([]byte(front), &fm); err != nil {
		return nil, fmt.Errorf("failed to parse frontmatter: %w", err)
	}

	rendered, missing, err := RenderBody(p.markdown, body, p.images)
	if err != nil {
		return nil, err
	}

	publishedAt, err := time.Parse("2006-01-02", fm.PublishedAt)
	if err != nil {
		publishedAt = time.Now()
	}

	image, heroMissing := fm.Image, false
	if image != "" && !strings.HasPrefix(image, "http") && !strings.HasPrefix(image, "/") {
		image = p.images.URL + image
		if _, statErr := os.Stat(filepath.Join(p.images.Files, fm.Image)); statErr != nil {
			missing = append(missing, fm.Image)
			heroMissing = true // the hero falls back to a gradient instead of a broken picture
		}
	}

	menu, err := cleanMenu(fm.Menu)
	if err != nil {
		return nil, err
	}
	draft, err := draftStatus(fm.Status)
	if err != nil {
		return nil, err
	}
	text := plainText(rendered)

	article := &domain.Article{
		ID:              fm.ID,
		Title:           fm.Title,
		Subtitle:        strings.TrimSpace(fm.Subtitle),
		Category:        domain.ArticleCategory(fm.Category),
		Author:          fm.Author,
		Summary:         fm.Summary,
		Content:         rendered,
		Excerpt:         truncate(text, excerptMaxRunes),
		Image:           image,
		ImageMissing:    heroMissing,
		HeroFocus:       strings.TrimSpace(fm.HeroFocus),
		PublishedAt:     publishedAt,
		ReadingMinutes:  max(1, (len(strings.Fields(text))+wordsPerMinute-1)/wordsPerMinute),
		RelatedProducts: fm.RelatedProducts,
		RelatedShop:     fm.RelatedShop,
		Tags:            fm.Tags,
		Visibility:      domain.Visibility(fm.Visibility),
		Draft:           draft,
		SEO:             domain.ArticleSEO{Title: strings.TrimSpace(fm.SEO.Title), Description: strings.TrimSpace(fm.SEO.Description), NoIndex: fm.SEO.NoIndex},
		Menu:            menu,
		MenuLabel:       strings.TrimSpace(fm.MenuLabel),
		MenuOrder:       fm.MenuOrder,
		MissingImages:   dedupe(missing),
		Priority:        fm.Priority,
		UpdatedAt:       time.Now(),
	}

	return article, nil
}

func draftStatus(status string) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "", "published":
		return false, nil
	case "draft":
		return true, nil
	default:
		return false, fmt.Errorf("status %q must be published or draft", status)
	}
}

func cleanMenu(slots []string) ([]string, error) {
	var out []string
	for _, slot := range slots {
		slot = strings.ToLower(strings.TrimSpace(slot))
		if slot != domain.MenuNav && slot != domain.MenuFooter {
			return nil, fmt.Errorf("menu %q must be %q or %q", slot, domain.MenuNav, domain.MenuFooter)
		}
		if !slices.Contains(out, slot) {
			out = append(out, slot)
		}
	}
	return out, nil
}

func dedupe(in []string) []string {
	var out []string
	for _, s := range in {
		if !slices.Contains(out, s) {
			out = append(out, s)
		}
	}
	return out
}

// plainText strips markup, cross-link tokens and entities from rendered HTML.
func plainText(rendered string) string {
	text := tagPattern.ReplaceAllString(crosslinks.Strip(rendered), " ")
	return strings.Join(strings.Fields(stdhtml.UnescapeString(text)), " ")
}

func truncate(text string, limit int) string {
	if utf8.RuneCountInString(text) <= limit {
		return text
	}
	runes := []rune(text)
	cut := strings.TrimSpace(string(runes[:limit-1]))
	if i := strings.LastIndex(cut, " "); i > limit/2 {
		cut = cut[:i]
	}
	return cut + "…"
}

// LoadArticleByID loads an article by its ID.
func (p *Parser) LoadArticleByID(id string) (*domain.Article, error) {
	all, err := p.LoadAllArticles()
	if err != nil {
		return nil, err
	}
	for i := range all {
		if all[i].ID == id {
			return &all[i], nil
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
		if slices.Contains(article.RelatedProducts, productID) {
			related = append(related, article)
		}
	}

	return related, nil
}
