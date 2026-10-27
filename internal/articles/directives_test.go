package articles_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"cargo.mleczki.pl/internal/articles"
)

// storyDir writes an article next to an images directory holding the given photos.
func storyDir(t *testing.T, photos ...string) string {
	t.Helper()
	root := t.TempDir()
	articlesDir := filepath.Join(root, "articles")
	images := filepath.Join(root, "images", "articles")
	for _, d := range []string{articlesDir, images} {
		if err := os.MkdirAll(d, 0o750); err != nil {
			t.Fatal(err)
		}
	}
	for _, photo := range photos {
		if err := os.WriteFile(filepath.Join(images, photo), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return articlesDir
}

func load(t *testing.T, dir, body string) (string, []string) {
	t.Helper()
	writeArticle(t, dir, "a.md", "---\nid: a\ntitle: A\n---\n"+body)
	article, err := articles.NewParser(dir).LoadArticle("a.md")
	if err != nil {
		t.Fatal(err)
	}
	return article.Content, article.MissingImages
}

func TestSceneKeepsOnePhotoUnderManyCards(t *testing.T) {
	dir := storyDir(t, "hel.jpg")
	html, missing := load(t, dir, "Przed.\n\n::: scene image=hel.jpg focus=\"40% 20%\" tone=light align=right alt=\"Półwysep\" credit=\"Foto: *ja*\"\nPierwsza karta.\n\nDruga w tej samej karcie.\n+++\nTrzecia karta z [[shop:rzepy]].\n:::\n\nPo.")
	if len(missing) != 0 {
		t.Errorf("hel.jpg exists but was reported missing: %v", missing)
	}
	for _, want := range []string{
		`<section class="scene scene--light scene--right" style="--focus:40% 20%">`,
		`<img class="scene__bg" src="/data/images/articles/hel.jpg" alt="Półwysep" loading="lazy" decoding="async">`,
		`<div class="scene__card"><p>Pierwsza karta.</p>`, "<p>Druga w tej samej karcie.</p>",
		`<div class="scene__card"><p>Trzecia karta z [[shop:rzepy]].</p>`,
		`<p class="scene__credit">Foto: <em>ja</em></p>`,
		"<p>Przed.</p>", "<p>Po.</p>",
	} {
		if !strings.Contains(html, want) {
			t.Errorf("missing %q in:\n%s", want, html)
		}
	}
	if strings.Count(html, `class="scene__card"`) != 2 {
		t.Errorf("want two cards:\n%s", html)
	}
}

func TestMissingPhotosAreReportedAndReplacedByAPlaceholder(t *testing.T) {
	dir := storyDir(t)
	html, missing := load(t, dir, "::: scene image=nie-ma.jpg\nTekst\n:::\n\n::: figure image=tez-nie.jpg caption=\"Podpis\"\n:::\n\n::: gallery images=a.jpg,b.jpg\n:::")
	if len(missing) != 4 {
		t.Errorf("missing = %v, want the four photos", missing)
	}
	if !strings.Contains(html, `scene__bg scene__bg--placeholder`) || strings.Contains(html, "<img") {
		t.Errorf("a missing photo must not render a broken <img>:\n%s", html)
	}
}

func TestFigureGalleryStatsAndNote(t *testing.T) {
	dir := storyDir(t, "a.jpg", "b.jpg", "c.jpg")
	html, _ := load(t, dir, `::: figure image=a.jpg size=tall
Podpis z **pogrubieniem**
:::

::: gallery images="a.jpg, b.jpg, c.jpg"
:::

::: stats
70 km | pierwszy dzień
3 | dzieci <na pokładzie>
bez kreski
:::

::: note title="Uwaga"
Warto *wiedzieć*.
:::`)
	for _, want := range []string{
		`<figure class="figure figure--tall" style="--focus:50% 50%">`, `<figcaption>Podpis z <strong>pogrubieniem</strong></figcaption>`,
		`gallery gallery--3`,
		`<li><strong>70 km</strong><span>pierwszy dzień</span></li>`, `<span>dzieci &lt;na pokładzie&gt;</span>`,
		`<aside class="note"><p class="note__title">Uwaga</p>`, `Warto <em>wiedzieć</em>`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("missing %q in:\n%s", want, html)
		}
	}
	if strings.Contains(html, "bez kreski") {
		t.Error("a stats line without | must be ignored")
	}
}

func TestDirectiveAttributesCannotInjectMarkup(t *testing.T) {
	dir := storyDir(t)
	html, _ := load(t, dir, "::: scene image=\"x.jpg\" focus=\"0 0;background:url(evil)\" tone=\"dark\\\" onload=\\\"x\" alt=\"a\\\"><script>alert(1)</script>\"\nOK\n:::\n\n::: figure image=\"../../etc/passwd\"\n:::\n\n::: figure image=\"https://example.com/a.jpg\" caption=\"<script>x</script>\"\n:::")
	if strings.Contains(html, "<script") || strings.Contains(html, "onload") || strings.Contains(html, "evil") || strings.Contains(html, "passwd") {
		t.Errorf("unsafe output:\n%s", html)
	}
	if !strings.Contains(html, `style="--focus:50% 50%"`) {
		t.Errorf("an invalid focus must fall back to the centre:\n%s", html)
	}
	if !strings.Contains(html, `src="https://example.com/a.jpg"`) {
		t.Errorf("remote photos are allowed:\n%s", html)
	}
}

func TestDirectiveErrorsNameTheProblem(t *testing.T) {
	dir := storyDir(t)
	for name, body := range map[string]string{
		"not closed":    "::: scene image=a.jpg\ntekst",
		"unknown":       "::: carousel\n:::",
		"empty gallery": "::: gallery\n:::",
	} {
		writeArticle(t, dir, "bad.md", "---\nid: bad\ntitle: B\n---\n"+body)
		if _, err := articles.NewParser(dir).LoadArticle("bad.md"); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}

func TestFenceMarkersInsideCodeBlocksAreLeftAlone(t *testing.T) {
	dir := storyDir(t)
	html, _ := load(t, dir, "```\n::: scene image=a.jpg\n```\n")
	if strings.Contains(html, "scene__bg") || !strings.Contains(html, "::: scene image=a.jpg") {
		t.Errorf("a directive inside a code block must be shown as text:\n%s", html)
	}
}

func TestArticleMetadataDraftSEOMenuAndReadingTime(t *testing.T) {
	dir := storyDir(t)
	writeArticle(t, dir, "a.md", `---
id: a
title: Szkic
subtitle: Podtytuł
status: draft
menu: [nav, Footer, nav]
menuLabel: Blog
menuOrder: 5
seo:
  title: Tytuł SEO
  description: Opis SEO
  noindex: true
---
`+strings.Repeat("słowo ", 450))
	item, err := articles.NewParser(dir).LoadArticle("a.md")
	if err != nil {
		t.Fatal(err)
	}
	if !item.Draft || item.Subtitle != "Podtytuł" || item.SEO.Title != "Tytuł SEO" || item.SEO.Description != "Opis SEO" || !item.SEO.NoIndex {
		t.Errorf("metadata: %+v", item)
	}
	if len(item.Menu) != 2 || !item.InMenu("nav") || !item.InMenu("footer") || item.MenuText() != "Blog" || item.MenuOrder != 5 {
		t.Errorf("menu: %+v", item.Menu)
	}
	if item.ReadingMinutes != 3 {
		t.Errorf("450 words should take 3 minutes, got %d", item.ReadingMinutes)
	}
	if !strings.HasSuffix(item.Excerpt, "…") || len([]rune(item.Excerpt)) > 200 {
		t.Errorf("excerpt = %q", item.Excerpt)
	}

	for name, front := range map[string]string{"status": "status: later", "menu": "menu: [sidebar]"} {
		writeArticle(t, dir, "b.md", "---\nid: b\ntitle: B\n"+front+"\n---\nx")
		if _, err := articles.NewParser(dir).LoadArticle("b.md"); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}

func TestParsedArticlesAreCachedUntilAFileChanges(t *testing.T) {
	dir := storyDir(t)
	writeArticle(t, dir, "a.md", "---\nid: a\ntitle: Pierwszy\n---\nx")
	parser := articles.NewParser(dir)
	first, _ := parser.LoadAllArticles()
	if len(first) != 1 || first[0].Title != "Pierwszy" {
		t.Fatalf("first load: %+v", first)
	}

	writeArticle(t, dir, "a.md", "---\nid: a\ntitle: Drugi, dłuższy tytuł\n---\nx")
	second, _ := parser.LoadAllArticles()
	if second[0].Title != "Drugi, dłuższy tytuł" {
		t.Errorf("a changed file must be reloaded, got %q", second[0].Title)
	}

	// A photo that appears later turns a missing-image warning into a picture.
	writeArticle(t, dir, "a.md", "---\nid: a\ntitle: Foto\nimage: hero.jpg\n---\nx")
	if a, _ := parser.LoadAllArticles(); !a[0].ImageMissing {
		t.Fatal("hero.jpg does not exist yet")
	}
	if err := os.WriteFile(filepath.Join(filepath.Dir(dir), "images", "articles", "hero.jpg"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if a, _ := parser.LoadAllArticles(); a[0].ImageMissing {
		t.Error("adding the photo must clear the warning")
	}
}
