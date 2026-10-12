package articles_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"cargo.mleczki.pl/internal/articles"
)

func writeArticle(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestLoadArticleParsesMetadataAndSafeMarkdown(t *testing.T) {
	dir := t.TempDir()
	writeArticle(t, dir, "article.md", "---\nid: test\ntitle: Porada\npublishedAt: 2026-10-12\nimage: bike.jpg\npriority: 3\nrelatedProducts: [cargo]\n---\n# Heading\nFirst line\nsecond line\n\n<script>alert(1)</script>\n[unsafe](javascript:alert(1))\n")
	article, err := articles.NewParser(dir).LoadArticle("article.md")
	if err != nil {
		t.Fatal(err)
	}
	if article.ID != "test" || article.Priority != 3 || article.Image != "/data/images/articles/bike.jpg" || article.PublishedAt.Format("2006-01-02") != "2026-10-12" {
		t.Fatalf("unexpected metadata: %+v", article)
	}
	if len(article.RelatedProducts) != 1 || article.RelatedProducts[0] != "cargo" {
		t.Fatalf("unexpected related products: %v", article.RelatedProducts)
	}
	if !strings.Contains(article.Content, `id="heading"`) || !strings.Contains(article.Content, "<br>") {
		t.Fatalf("shared Markdown options were lost: %s", article.Content)
	}
	if strings.Contains(article.Content, "<script>") || strings.Contains(article.Content, `href="javascript:`) {
		t.Fatalf("unsafe Markdown rendered: %s", article.Content)
	}
}

func TestLoadArticleRejectsInvalidFiles(t *testing.T) {
	dir := t.TempDir()
	writeArticle(t, dir, "missing-frontmatter.md", "just text")
	writeArticle(t, dir, "invalid-yaml.md", "---\ntitle: [\n---\nbody")
	for _, filename := range []string{"absent.md", "missing-frontmatter.md", "invalid-yaml.md"} {
		t.Run(filename, func(t *testing.T) {
			if _, err := articles.NewParser(dir).LoadArticle(filename); err == nil {
				t.Fatal("expected an error")
			}
		})
	}
}

func TestLoadAllArticlesSortsPriorityThenDateAndSkipsNonMarkdown(t *testing.T) {
	dir := t.TempDir()
	for name, metadata := range map[string]string{
		"low.md": "id: low\npriority: 1\npublishedAt: 2026-10-12",
		"old.md": "id: old\npriority: 2\npublishedAt: 2026-10-01",
		"new.md": "id: new\npriority: 2\npublishedAt: 2026-10-12",
	} {
		writeArticle(t, dir, name, "---\n"+metadata+"\n---\nbody")
	}
	writeArticle(t, dir, "ignored.txt", "invalid markdown")
	got, err := articles.NewParser(dir).LoadAllArticles()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 || got[0].ID != "new" || got[1].ID != "old" || got[2].ID != "low" {
		t.Fatalf("unexpected ordering: %+v", got)
	}
	writeArticle(t, dir, "broken.md", "no frontmatter")
	if _, err := articles.NewParser(dir).LoadAllArticles(); err == nil {
		t.Fatal("invalid article should fail the catalog load")
	}
}
