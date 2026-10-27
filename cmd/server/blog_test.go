package main

import (
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeFile(t *testing.T, app *shopHarness, sub, name, content string) {
	t.Helper()
	dir := filepath.Join(app.dir, sub)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func blogHarness(t *testing.T) *shopHarness {
	t.Helper()
	app := newShopHarness(t)
	writeFile(t, app, "articles", "story.md", `---
id: story
title: Historia
subtitle: Podtytuł
category: historie
publishedAt: 2026-01-02
menu: [nav, footer]
menuLabel: Moja historia
relatedShop: [kask]
seo:
  description: Opis do wyszukiwarki
---
Tekst ze [[shop:dzwonek]], [[shop:lampka|lampką]] i [[product:cargo]].

[[rent:cargo]]

::: stats
70 km | dzień
:::
`)
	writeFile(t, app, "articles", "draft.md", "---\nid: draft\ntitle: Szkic\nstatus: draft\nmenu: [nav]\n---\nTajne.")
	writeFile(t, app, "articles", "broken.md", "---\nid: [\n---\nx")
	return app
}

func TestPreviewCardsAndRentEmbedInArticles(t *testing.T) {
	app := blogHarness(t)
	_, body, _ := app.get(app.visitor(), "/article/story")
	assertContains(t, body,
		`class="hc"`, `class="hc-card"`, `formaction="/cart/add-shop?card=1"`, `name="item_id" value="dzwonek"`, "25 zł", // direct item: cart button
		`href="/shop/lampka/go"`, `target="_blank"`, `hc-ext`, "Allegro", // affiliate: leaves the site
		"Wypróbuj mnie!", `class="rent-embed"`, `href="/product/cargo"`, // rental embed
		`<ul class="stats">`, "Historia", "min czytania")
	assertNotContains(t, body, "[[", "<p><aside")
}

func TestHoverCardCartButtonAddsAndReturnsToTheArticle(t *testing.T) {
	app := blogHarness(t)
	visitorClient := app.visitor()
	req, _ := http.NewRequest(http.MethodPost, app.http.URL+"/cart/add-shop?card=1", strings.NewReader(url.Values{"item_id": {"dzwonek"}}.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Referer", app.http.URL+"/article/story")
	resp, err := visitorClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if loc := resp.Header.Get("Location"); resp.StatusCode != http.StatusSeeOther || loc != "/article/story?cart_added=dzwonek" {
		t.Fatalf("redirect = %d %q", resp.StatusCode, loc)
	}
	if l := app.level("dzwonek"); l.OnHand != 3 {
		t.Errorf("adding to the cart must not touch stock: %+v", l)
	}
	_, body, _ := app.get(visitorClient, "/article/story?cart_added=dzwonek")
	assertContains(t, body, "Dodano do koszyka", "Dzwonek", `href="/checkout"`)
}

func TestDraftsAreAdminOnlyAndOutOfEveryIndex(t *testing.T) {
	app := blogHarness(t)
	visitor2 := app.visitor()
	if status, _, _ := app.get(visitor2, "/article/draft"); status != http.StatusNotFound {
		t.Errorf("a draft must be a 404 for visitors, got %d", status)
	}
	if status, _, _ := app.get(visitor2, "/og/article/draft.jpg"); status != http.StatusNotFound {
		t.Errorf("a draft's preview must be a 404 for visitors, got %d", status)
	}
	_, list, _ := app.get(visitor2, "/articles")
	assertNotContains(t, list, "Szkic")
	_, sitemap, _ := app.get(visitor2, "/sitemap.xml")
	assertNotContains(t, sitemap, "/article/draft")
	_, home, _ := app.get(visitor2, "/")
	assertNotContains(t, home, "/article/draft")

	admin := app.asAdmin()
	status, body, _ := app.get(admin, "/article/draft")
	if status != 200 {
		t.Fatalf("admin must see the draft, got %d", status)
	}
	assertContains(t, body, "To szkic", `content="noindex, nofollow"`)
	_, list, _ = app.get(admin, "/articles")
	assertContains(t, list, "SZKIC")
	_, home, _ = app.get(admin, "/")
	assertNotContains(t, home, "/article/draft") // drafts are never linked from menus
}

func TestMenusAreFedByArticles(t *testing.T) {
	app := blogHarness(t)
	_, body, _ := app.get(app.visitor(), "/")
	assertContains(t, body, `href="/article/story"`, "Moja historia", "Czytaj")
}

func TestOneBrokenArticleDoesNotTakeTheBlogDown(t *testing.T) {
	app := blogHarness(t)
	status, list, _ := app.get(app.visitor(), "/articles")
	if status != 200 {
		t.Fatalf("/articles = %d", status)
	}
	assertContains(t, list, "Historia")
	_, admin, _ := app.get(app.asAdmin(), "/admin")
	assertContains(t, admin, "broken.md")
}

func TestSEOMetaTagsAndStructuredData(t *testing.T) {
	app := blogHarness(t)
	value := app.visitor()

	_, body, _ := app.get(value, "/article/story")
	assertContains(t, body,
		`<meta name="description" content="Opis do wyszukiwarki">`, `<link rel="canonical" href="`, `/article/story">`,
		`property="og:type" content="article"`, `/og/article/story.jpg?v=`, `name="twitter:card" content="summary_large_image"`,
		`application/ld+json`, `BlogPosting`, `content="index, follow, max-image-preview:large"`)

	_, shopPage, _ := app.get(value, "/shop/dzwonek")
	assertContains(t, shopPage, `"@type":"Product"`, `"priceCurrency":"PLN"`, `"price":"25.00"`, `https://schema.org/InStock`, `/og/shop/dzwonek.jpg`)

	_, product, _ := app.get(value, "/product/cargo")
	assertContains(t, product, `"unitCode":"DAY"`, `/og/product/cargo.jpg`)

	_, home, _ := app.get(value, "/")
	assertContains(t, home, "BikeStore", `/og/site.jpg?v=`)

	_, checkout, _ := app.get(value, "/checkout")
	assertContains(t, checkout, `content="noindex, nofollow"`)
}

func TestPreviewImages(t *testing.T) {
	app := blogHarness(t)
	visitor3 := app.visitor()
	for _, path := range []string{"/og/site.jpg", "/og/product/cargo.jpg", "/og/shop/dzwonek.jpg", "/og/shop/lampka.jpg", "/og/article/story.jpg", "/og/shop-list/x.jpg", "/og/blog/x.jpg"} {
		status, body, header := app.get(visitor3, path)
		if status != 200 || header.Get("Content-Type") != "image/jpeg" || !strings.HasPrefix(body, "\xff\xd8") {
			t.Errorf("%s: %d %q", path, status, header.Get("Content-Type"))
		}
		if cc := header.Get("Cache-Control"); !strings.Contains(cc, "public") {
			t.Errorf("%s: public previews must be cacheable, got %q", path, cc)
		}
	}
	for _, path := range []string{"/og/shop/tajny.jpg", "/og/product/nie-ma.jpg", "/og/dziwne/x.jpg", "/og/shop/dzwonek.png"} {
		if status, _, _ := app.get(visitor3, path); status != http.StatusNotFound {
			t.Errorf("%s = %d, want 404", path, status)
		}
	}

	// A conditional request is answered without drawing again.
	_, _, header := app.get(visitor3, "/og/shop/dzwonek.jpg")
	req, _ := http.NewRequest(http.MethodGet, app.http.URL+"/og/shop/dzwonek.jpg", nil)
	req.Header.Set("If-None-Match", header.Get("ETag"))
	resp, err := visitor3.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotModified {
		t.Errorf("If-None-Match: %d", resp.StatusCode)
	}

	// An admin-only item has a preview for the admin, and it is never cached publicly.
	status, _, header := app.get(app.asAdmin(), "/og/shop/tajny.jpg")
	if status != 200 || strings.Contains(header.Get("Cache-Control"), "public") {
		t.Errorf("admin preview: %d %q", status, header.Get("Cache-Control"))
	}
}

func TestSitemapAndRobots(t *testing.T) {
	app := blogHarness(t)
	visitorClient := app.visitor()
	_, sitemap, header := app.get(visitorClient, "/sitemap.xml")
	if !strings.Contains(header.Get("Content-Type"), "xml") {
		t.Errorf("content type = %q", header.Get("Content-Type"))
	}
	assertContains(t, sitemap, "/product/cargo", "/shop/dzwonek", "/shop/kask", "/article/story", "<lastmod>2026-01-02</lastmod>", "/articles")
	assertNotContains(t, sitemap, "/shop/tajny", "/article/draft", "/admin")

	_, robots, _ := app.get(visitorClient, "/robots.txt")
	assertContains(t, robots, "Disallow: /admin", "Disallow: /checkout", "Sitemap: ", "/sitemap.xml")
}
