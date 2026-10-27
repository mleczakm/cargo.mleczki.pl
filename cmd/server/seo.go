package main

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"

	"cargo.mleczki.pl/internal/domain"
	"cargo.mleczki.pl/internal/og"
	"cargo.mleczki.pl/internal/shop"
)

const (
	siteName        = "cargo.mleczki.pl"
	defaultSiteURL  = "https://cargo.mleczki.pl"
	siteDescription = "Prywatna wypożyczalnia rowerów cargo, przyczepek i sprzętu rowerowego w Radzyminie oraz sklep z akcesoriami sprawdzonymi na co dzień."
	maxDescription  = 160
)

// pageSEO is what <head> says about a page: for search engines, and for the
// previews social networks and messengers draw when the link is shared.
type pageSEO struct {
	Title       string // <title>; the site name is appended when it is missing
	Description string
	Canonical   string
	Image       string // absolute URL of the preview image
	ImageAlt    string
	Type        string // og:type: website, article, product
	NoIndex     bool
	Published   string // RFC 3339, articles only
	JSONLD      template.JS
}

// siteURL is the public address used in canonical links and previews. Behind a
// proxy the Host header cannot be trusted for this, so production uses
// SITE_URL (or the default); local development uses the request.
func siteURL(r *http.Request) string {
	if configured := strings.TrimRight(strings.TrimSpace(os.Getenv("SITE_URL")), "/"); configured != "" {
		return configured
	}
	host := r.Host
	if strings.HasPrefix(host, "localhost") || strings.HasPrefix(host, "127.0.0.1") || strings.HasPrefix(host, "[::1]") {
		return getBaseURL(r)
	}
	return defaultSiteURL
}

// privatePrefixes are pages that must not show up in search results.
var privatePrefixes = []string{"/admin", "/user", "/checkout", "/payment", "/success", "/login", "/forgot-password", "/reset-password", "/cart", "/api"}

func isPrivatePath(path string) bool {
	for _, prefix := range privatePrefixes {
		if path == prefix || strings.HasPrefix(path, prefix+"/") {
			return true
		}
	}
	return false
}

func trimDescription(text string) string {
	text = strings.Join(strings.Fields(text), " ")
	if len([]rune(text)) <= maxDescription {
		return text
	}
	cut := strings.TrimSpace(string([]rune(text)[:maxDescription-1]))
	if i := strings.LastIndex(cut, " "); i > maxDescription/2 {
		cut = cut[:i]
	}
	return cut + "…"
}

// finishSEO fills in what a page did not set itself: title, description,
// canonical address, the site-wide preview image, and noindex for private pages.
func (s *Server) finishSEO(r *http.Request, data map[string]interface{}) {
	seo, _ := data["SEO"].(*pageSEO)
	if seo == nil {
		seo = &pageSEO{}
	}
	title, _ := data["Title"].(string)
	if seo.Title == "" {
		seo.Title = title
	}
	if seo.Description == "" {
		seo.Description = siteDescription
	}
	seo.Description = trimDescription(seo.Description)
	base := siteURL(r)
	if seo.Canonical == "" {
		seo.Canonical = base + r.URL.Path
	}
	if seo.Type == "" {
		seo.Type = "website"
	}
	if seo.Image == "" {
		if spec, _, ok := s.ogSpec(r, "site", ""); ok {
			seo.Image = ogImageURL(base, "site", "", spec)
			seo.ImageAlt = spec.Title
		}
	}
	if isPrivatePath(r.URL.Path) {
		seo.NoIndex = true
	}
	data["SEO"] = seo
	data["PageTitle"] = seo.Title
	if !strings.Contains(seo.Title, siteName) {
		data["PageTitle"] = seo.Title + " - " + siteName
	}
}

func ogImageURL(base, kind, id string, spec og.Spec) string {
	path := "/og/site.jpg"
	if kind != "site" {
		path = "/og/" + kind + "/" + url.PathEscape(id) + ".jpg"
	}
	return base + path + "?v=" + spec.Hash()
}

// ---------------------------------------------------------------------------
// Preview images
// ---------------------------------------------------------------------------

// diskImage maps an image URL of the site to its file; remote images are not used.
func diskImage(imageURL string) string {
	rest, ok := strings.CutPrefix(imageURL, "/data/images/")
	if !ok {
		return ""
	}
	clean := filepath.Clean(filepath.FromSlash(rest))
	if clean == "." || strings.HasPrefix(clean, "..") || filepath.IsAbs(clean) {
		return ""
	}
	return filepath.Join("data", "images", clean)
}

func photoFiles(urls ...string) []string {
	var files []string
	for _, u := range urls {
		if path := diskImage(u); path != "" {
			if _, err := os.Stat(path); err == nil {
				files = append(files, path)
			}
		}
	}
	return files
}

var articleBadges = map[domain.ArticleCategory]string{
	domain.CategoryHistorie:   "Historia",
	domain.CategoryPorady:     "Porady",
	domain.CategoryRecenzje:   "Recenzje",
	domain.CategoryPrzewodnik: "Przewodnik",
}

// ogSpec describes the preview image of a page. The second result tells
// whether anybody may see it (the image of an admin-only page is not cached
// publicly); the third is false when the visitor may not see the page at all.
func (s *Server) ogSpec(r *http.Request, kind, id string) (og.Spec, bool, bool) {
	switch kind {
	case "site":
		return s.ogSpecSite()
	case "product":
		return s.ogSpecProduct(r, id)
	case "shop":
		return s.ogSpecShop(r, id)
	case "article":
		return s.ogSpecArticle(r, id)
	case "shop-list":
		return s.ogSpecShopList(r)
	case "blog":
		return s.ogSpecBlog()
	}
	return og.Spec{}, false, false
}

func (s *Server) ogSpecSite() (og.Spec, bool, bool) {
	spec := og.Spec{
		Layout: og.LayoutProduct, Badge: "Radzymin", Title: "Wypożyczalnia rowerów cargo i sprzętu rowerowego",
		Subtitle: "Rowery cargo, przyczepki i bagażniki do wynajęcia. A do tego sklep z akcesoriami sprawdzonymi na co dzień.",
		CTA:      "Zarezerwuj online",
	}
	for _, p := range s.publicProducts() {
		spec.Photos = append(spec.Photos, photoFiles(p.Image)...)
		if len(spec.Photos) >= 3 {
			break
		}
	}
	return spec, true, true
}

func (s *Server) ogSpecProduct(r *http.Request, id string) (og.Spec, bool, bool) {
	if s.productParser == nil {
		return og.Spec{}, false, false
	}
	product, err := s.productParser.LoadProductByID(id)
	if err != nil || !isItemVisible(product.Visibility, r) {
		return og.Spec{}, false, false
	}
	return og.Spec{
		Layout: og.LayoutProduct, Badge: "Do wynajęcia", Title: product.Name, Subtitle: plainSummary(product.Description, 130),
		Price: product.BasePrice.String() + " / dobę", CTA: "Wypróbuj mnie!", Photos: photoFiles(append([]string{product.Image}, product.Images...)...),
	}, isPublic(product.Visibility), true
}

func (s *Server) ogSpecShop(r *http.Request, id string) (og.Spec, bool, bool) {
	item, ok := s.shopItemByID(r, id)
	if !ok {
		return og.Spec{}, false, false
	}
	spec := og.Spec{Layout: og.LayoutProduct, Title: item.Name, Subtitle: plainSummary(item.Summary, 130), Photos: photoFiles(append([]string{item.Image}, item.Images...)...)}
	if item.IsAffiliate() {
		spec.Badge, spec.CTA, spec.Price = "Polecam · "+item.Affiliate.Merchant, "Zobacz ofertę", item.Affiliate.PriceHint
	} else {
		view := s.shopViews(r, []domain.ShopItem{item})[0]
		spec.Badge, spec.CTA, spec.Price = "Sklep", "Do koszyka", view.DisplayPrice.String()
		if view.PriceFrom {
			spec.Price = "od " + spec.Price
		}
		if !view.Availability.CanBuy {
			spec.CTA = "Wkrótce"
		}
	}
	return spec, isPublic(item.Visibility), true
}

func (s *Server) ogSpecArticle(r *http.Request, id string) (og.Spec, bool, bool) {
	if s.articleParser == nil {
		return og.Spec{}, false, false
	}
	article, err := s.articleParser.LoadArticleByID(id)
	if err != nil || !articleVisible(*article, r) {
		return og.Spec{}, false, false
	}
	spec := og.Spec{
		Layout: og.LayoutCover, Badge: articleBadges[article.Category], Title: article.Title,
		Subtitle: firstNonEmpty(article.Subtitle, article.Summary), CTA: fmt.Sprintf("%d min czytania", article.ReadingMinutes),
	}
	if article.Draft {
		spec.Badge = "Szkic"
	}
	if !article.ImageMissing {
		spec.Photos = photoFiles(article.Image)
	}
	return spec, !article.Draft && isPublic(article.Visibility), true
}

func (s *Server) ogSpecShopList(r *http.Request) (og.Spec, bool, bool) {
	spec := og.Spec{Layout: og.LayoutProduct, Badge: "Sklep", Title: "Sklep z akcesoriami rowerowymi", Subtitle: "Rzeczy sprawdzone w codziennej jeździe: kupisz je u mnie albo zobaczysz polecane oferty."}
	for _, item := range s.visibleShopItems(r) {
		if isPublic(item.Visibility) {
			spec.Photos = append(spec.Photos, photoFiles(item.Image)...)
		}
		if len(spec.Photos) >= 3 {
			break
		}
	}
	return spec, true, true
}

func (s *Server) ogSpecBlog() (og.Spec, bool, bool) {
	spec := og.Spec{Layout: og.LayoutProduct, Badge: "Blog", Title: "Blog o życiu na rowerze", Subtitle: "Historie z codziennej jazdy rowerem cargo, porady dla rodzin i sprzęt, który się sprawdził."}
	if s.articleParser != nil {
		if all, err := s.articleParser.LoadAllArticles(); err == nil {
			for _, a := range all {
				if !a.Draft && isPublic(a.Visibility) && !a.ImageMissing {
					spec.Photos = append(spec.Photos, photoFiles(a.Image)...)
				}
				if len(spec.Photos) >= 3 {
					break
				}
			}
		}
	}
	return spec, true, true
}

// publicProducts lists the rentals anybody can see.
func (s *Server) publicProducts() []domain.Product {
	if s.productParser == nil {
		return nil
	}
	all, err := s.productParser.LoadAllProducts()
	if err != nil {
		return nil
	}
	var out []domain.Product
	for _, p := range all {
		if isPublic(p.Visibility) {
			out = append(out, p)
		}
	}
	return out
}

// isPublic reports whether anybody may see content with this visibility (the default is public).
func isPublic(v domain.Visibility) bool { return v == "" || v == domain.VisibilityPublic }

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

// ogCache keeps rendered previews: drawing one takes a moment, and crawlers ask often.
type ogCache struct {
	mu      sync.Mutex
	entries map[string][]byte
}

const maxOGEntries = 256

func (c *ogCache) get(key string) ([]byte, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	data, ok := c.entries[key]
	return data, ok
}

func (c *ogCache) put(key string, data []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.entries == nil || len(c.entries) >= maxOGEntries {
		c.entries = map[string][]byte{}
	}
	c.entries[key] = data
}

func (s *Server) handleOGSite(w http.ResponseWriter, r *http.Request) { s.serveOG(w, r, "site", "") }

func (s *Server) handleOG(w http.ResponseWriter, r *http.Request) {
	id, ok := strings.CutSuffix(chi.URLParam(r, "file"), ".jpg")
	if !ok || id == "" {
		http.NotFound(w, r)
		return
	}
	s.serveOG(w, r, chi.URLParam(r, "kind"), id)
}

func (s *Server) serveOG(w http.ResponseWriter, r *http.Request, kind, id string) {
	spec, public, ok := s.ogSpec(r, kind, id)
	if !ok {
		http.NotFound(w, r)
		return
	}
	etag := `"` + spec.Hash() + `"`
	w.Header().Set("ETag", etag)
	if public {
		w.Header().Set("Cache-Control", "public, max-age=86400")
	} else {
		w.Header().Set("Cache-Control", "private, no-store")
		w.Header().Set("X-Robots-Tag", "noindex")
	}
	if public && r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}

	data, cached := s.ogCache.get(spec.Hash())
	if !cached {
		var err error
		if data, err = og.Render(spec); err != nil {
			log.Printf("Failed to draw the preview of %s/%s: %v", kind, id, err)
			http.Error(w, "Preview unavailable", http.StatusInternalServerError)
			return
		}
		s.ogCache.put(spec.Hash(), data)
	}
	w.Header().Set("Content-Type", "image/jpeg")
	_, _ = w.Write(data)
}

// ---------------------------------------------------------------------------
// Structured data
// ---------------------------------------------------------------------------

func jsonLD(graph ...map[string]any) template.JS {
	raw, err := json.Marshal(map[string]any{"@context": "https://schema.org", "@graph": graph})
	if err != nil {
		return ""
	}
	return template.JS(raw) // #nosec G203 -- json.Marshal escapes <, > and & so it cannot close the script element
}

func breadcrumbs(base string, crumbs ...[2]string) map[string]any {
	items := make([]map[string]any, 0, len(crumbs))
	for i, c := range crumbs {
		items = append(items, map[string]any{"@type": "ListItem", "position": i + 1, "name": c[0], "item": base + c[1]})
	}
	return map[string]any{"@type": "BreadcrumbList", "itemListElement": items}
}

func money(m domain.Money) string { return m.Plain() }

func (s *Server) homeJSONLD(base string) template.JS {
	return jsonLD(map[string]any{
		"@type": "BikeStore", "name": siteName, "url": base + "/", "description": siteDescription,
		"areaServed": "Radzymin", "address": map[string]any{"@type": "PostalAddress", "addressLocality": "Radzymin", "addressCountry": "PL"},
	})
}

func (s *Server) productJSONLD(base string, rental *domain.Product, image string) template.JS {
	product := map[string]any{
		"@type": "Product", "name": rental.Name, "description": plainSummary(rental.Description, 300), "url": base + "/product/" + rental.ID,
		"offers": map[string]any{
			"@type": "Offer", "price": money(rental.BasePrice), "priceCurrency": "PLN", "availability": "https://schema.org/InStock", "url": base + "/product/" + rental.ID,
			"priceSpecification": map[string]any{"@type": "UnitPriceSpecification", "price": money(rental.BasePrice), "priceCurrency": "PLN", "unitCode": "DAY"},
		},
	}
	if image != "" {
		product["image"] = image
	}
	return jsonLD(product, breadcrumbs(base, [2]string{"Oferta", "/"}, [2]string{rental.Name, "/product/" + rental.ID}))
}

func (s *Server) shopItemJSONLD(base string, view shopItemView, image string) template.JS {
	product := map[string]any{"@type": "Product", "name": view.Name, "description": plainSummary(view.Summary, 300), "url": base + "/shop/" + view.ID}
	if image != "" {
		product["image"] = image
	}
	if view.IsDirect() {
		availability := "https://schema.org/InStock"
		switch view.Availability.State {
		case shop.StateInStock, shop.StateLow:
			availability = "https://schema.org/InStock"
		case shop.StateOut:
			availability = "https://schema.org/OutOfStock"
		case shop.StateBackorder:
			availability = "https://schema.org/BackOrder"
		}
		product["offers"] = map[string]any{
			"@type": "Offer", "price": money(view.DisplayPrice), "priceCurrency": "PLN", "availability": availability, "url": base + "/shop/" + view.ID,
		}
	}
	return jsonLD(product, breadcrumbs(base, [2]string{"Sklep", "/shop"}, [2]string{view.Name, "/shop/" + view.ID}))
}

func (s *Server) articleJSONLD(base string, article *domain.Article, image string) template.JS {
	post := map[string]any{
		"@type": "BlogPosting", "headline": article.Title, "description": firstNonEmpty(article.SEO.Description, article.Summary, article.Excerpt),
		"datePublished": article.PublishedAt.Format("2006-01-02"), "mainEntityOfPage": base + "/article/" + article.ID,
		"publisher": map[string]any{"@type": "Organization", "name": siteName, "url": base + "/"},
	}
	if article.Author != "" {
		post["author"] = map[string]any{"@type": "Person", "name": article.Author}
	}
	if image != "" {
		post["image"] = image
	}
	return jsonLD(post, breadcrumbs(base, [2]string{"Blog", "/articles"}, [2]string{article.Title, "/article/" + article.ID}))
}

// ---------------------------------------------------------------------------
// sitemap.xml and robots.txt
// ---------------------------------------------------------------------------

type sitemapURL struct {
	Loc     string `xml:"loc"`
	LastMod string `xml:"lastmod,omitempty"`
}

type sitemapDoc struct {
	XMLName xml.Name     `xml:"urlset"`
	XMLNS   string       `xml:"xmlns,attr"`
	URLs    []sitemapURL `xml:"url"`
}

// anonymous is a request without a session: the view search engines get.
func anonymous(r *http.Request) *http.Request {
	clean := r.Clone(context.WithoutCancel(context.Background()))
	clean.Header = http.Header{}
	return clean
}

//nolint:contextcheck // The anonymous view intentionally removes session context.
func (s *Server) handleSitemap(w http.ResponseWriter, r *http.Request) {
	base := siteURL(r)
	visitor := anonymous(r)
	doc := sitemapDoc{XMLNS: "http://www.sitemaps.org/schemas/sitemap/0.9"}
	add := func(path string, modified time.Time) {
		u := sitemapURL{Loc: base + path}
		if !modified.IsZero() {
			u.LastMod = modified.Format("2006-01-02")
		}
		doc.URLs = append(doc.URLs, u)
	}

	add("/", time.Time{})
	for _, p := range s.publicProducts() {
		add("/product/"+p.ID, time.Time{})
	}
	if shopItems := s.visibleShopItems(visitor); len(shopItems) > 0 {
		add("/shop", time.Time{})
		for _, item := range shopItems {
			add("/shop/"+item.ID, time.Time{})
		}
	}
	if s.articleParser != nil {
		if all, err := s.articleParser.LoadAllArticles(); err == nil {
			var listed []domain.Article
			for _, a := range all {
				if articleVisible(a, visitor) && !a.SEO.NoIndex {
					listed = append(listed, a)
				}
			}
			if len(listed) > 0 {
				add("/articles", time.Time{})
			}
			sort.SliceStable(listed, func(i, j int) bool { return listed[i].PublishedAt.After(listed[j].PublishedAt) })
			for _, a := range listed {
				add("/article/"+a.ID, a.PublishedAt)
			}
		}
	}

	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	_, _ = w.Write([]byte(xml.Header))
	enc := xml.NewEncoder(w)
	enc.Indent("", " ")
	if err := enc.Encode(doc); err != nil {
		log.Printf("Failed to write the sitemap: %v", err)
	}
}

func (s *Server) handleRobots(w http.ResponseWriter, r *http.Request) {
	var builder strings.Builder
	builder.WriteString("User-agent: *\n")
	for _, prefix := range privatePrefixes {
		builder.WriteString("Disallow: " + prefix + "\n")
	}
	builder.WriteString("Disallow: /shop/*/go\n")
	builder.WriteString("Disallow: /shipping/\n")
	builder.WriteString("\nSitemap: " + siteURL(r) + "/sitemap.xml\n")
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	_, _ = w.Write([]byte(builder.String()))
}

// ---------------------------------------------------------------------------
// Menus fed by articles
// ---------------------------------------------------------------------------

type menuLink struct {
	Label string
	URL   string
}

// menuLinks lists the articles linked from a menu slot, for this visitor.
// Drafts never appear in menus, even for the admin.
func (s *Server) menuLinks(r *http.Request, slot string) []menuLink {
	if s.articleParser == nil {
		return nil
	}
	all, err := s.articleParser.LoadAllArticles()
	if err != nil {
		return nil
	}
	var picked []domain.Article
	for _, a := range all {
		if a.InMenu(slot) && !a.Draft && isItemVisible(a.Visibility, r) {
			picked = append(picked, a)
		}
	}
	sort.SliceStable(picked, func(i, j int) bool {
		if picked[i].MenuOrder != picked[j].MenuOrder {
			return picked[i].MenuOrder < picked[j].MenuOrder
		}
		return picked[i].Title < picked[j].Title
	})
	links := make([]menuLink, 0, len(picked))
	for _, a := range picked {
		links = append(links, menuLink{Label: a.MenuText(), URL: "/article/" + a.ID})
	}
	return links
}

// ogRef is the absolute URL of a page's preview image and its alt text.
func (s *Server) ogRef(r *http.Request, kind, id string) (string, string) {
	spec, _, ok := s.ogSpec(r, kind, id)
	if !ok {
		return "", ""
	}
	return ogImageURL(siteURL(r), kind, id, spec), spec.Title
}

// setListSEO describes a list page (shop, blog). Pages narrowed by a filter are not indexed twice.
func (s *Server) setListSEO(r *http.Request, data map[string]interface{}, kind, title, description string) {
	image, alt := s.ogRef(r, kind, "")
	seo := &pageSEO{Title: title, Description: description, Image: image, ImageAlt: alt}
	if r.URL.Query().Get("category") != "" {
		seo.NoIndex = true
	}
	data["SEO"] = seo
}

func (s *Server) productSEO(r *http.Request, rental *domain.Product) *pageSEO {
	base := siteURL(r)
	image, alt := s.ogRef(r, "product", rental.ID)
	return &pageSEO{
		Title:       "Wynajem: " + rental.Name,
		Description: fmt.Sprintf("Wynajem %s w Radzyminie: %s za dobę. %s", rental.Name, rental.BasePrice, plainSummary(rental.Description, 110)),
		Canonical:   base + "/product/" + rental.ID,
		Image:       image, ImageAlt: alt, Type: "product",
		NoIndex: !isPublic(rental.Visibility),
		JSONLD:  s.productJSONLD(base, rental, image),
	}
}

func (s *Server) shopItemSEO(r *http.Request, item domain.ShopItem, view shopItemView) *pageSEO {
	base := siteURL(r)
	image, alt := s.ogRef(r, "shop", item.ID)
	description := plainSummary(item.Summary, 150)
	if item.IsDirect() {
		description = fmt.Sprintf("%s: %s. %s", item.Name, view.DisplayPrice, description)
	}
	return &pageSEO{
		Title: item.Name, Description: description, Canonical: base + "/shop/" + item.ID, Image: image, ImageAlt: alt, Type: "product",
		NoIndex: !isPublic(item.Visibility),
		JSONLD:  s.shopItemJSONLD(base, view, image),
	}
}

func (s *Server) articleSEO(r *http.Request, article *domain.Article) *pageSEO {
	base := siteURL(r)
	image, alt := s.ogRef(r, "article", article.ID)
	return &pageSEO{
		Title:       firstNonEmpty(article.SEO.Title, article.Title),
		Description: firstNonEmpty(article.SEO.Description, article.Summary, article.Subtitle, article.Excerpt),
		Canonical:   base + "/article/" + article.ID,
		Image:       image, ImageAlt: alt, Type: "article", Published: article.PublishedAt.Format(time.RFC3339),
		NoIndex: article.Draft || article.SEO.NoIndex || !isPublic(article.Visibility),
		JSONLD:  s.articleJSONLD(base, article, image),
	}
}
