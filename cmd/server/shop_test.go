package main

import (
	"encoding/json"
	"html/template"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"cargo.mleczki.pl/internal/articles"
	"cargo.mleczki.pl/internal/auth"
	"cargo.mleczki.pl/internal/domain"
	"cargo.mleczki.pl/internal/eventstore"
	"cargo.mleczki.pl/internal/products"
	"cargo.mleczki.pl/internal/projections"
	"cargo.mleczki.pl/internal/shop"
	"cargo.mleczki.pl/internal/transfers"
)

// shopFixtures is a small catalog exercising every kind of shop item.
var shopFixtures = map[string]map[string]string{
	"products": {
		"cargo.md": `---
id: cargo
name: Rower cargo
basePrice: 150
shop: [dzwonek]
addons:
  - id: daszek
    name: Daszek
    price: 15.50
  - id: kaucja
    name: Kaucja
    price: 50
    paymentType: one-time
---
Mocny rower. Dobrany do niego [[shop:kask]] jest w sklepie.
`,
	},
	"products-extra": {
		"tandem.md": `---
id: tandem
name: Rower tandem
basePrice: 80.50
---
Dla dwojga.
`,
	},
	"articles": {
		"guide.md": `---
id: guide
title: Jak wybrać rower
category: przewodnik
publishedAt: 2024-03-15
relatedShop: [kask]
---
Zobacz [[shop:lampka|polecaną lampkę]].
`,
	},
	"shop": {
		"dzwonek.md": `---
id: dzwonek
name: Dzwonek
category: akcesoria
price: 25
relatedProducts: [cargo]
stock:
  initial: 3
  maxPerOrder: 5
---
Głośny. Pasuje do [[product:cargo]], przeczytaj [[article:guide]] i zobacz [[shop:tajny]] oraz [[shop:nie-ma]].
`,
		"kask.md": `---
id: kask
name: Kask
category: akcesoria
price: 120
variants:
  - sku: kask-s
    name: Rozmiar S
    stock: 2
  - sku: kask-l
    name: Rozmiar L
    price: 130
---
Lekki kask.
`,
		"lampka.md": `---
id: lampka
name: Lampka
category: oświetlenie
affiliate:
  url: https://allegro.pl/oferta/lampka-123
  priceHint: od ok. 59 zł
---
Dobra lampka.
`,
		"smar.md": `---
id: smar
name: Smar do łańcucha
price: 12.34
stock: {mode: unlimited}
---
Smaruje.
`,
		"tajny.md": `---
id: tajny
name: Tajny produkt
price: 10
visibility: admin
stock: {mode: unlimited}
---
Tylko dla admina.
`,
		"na-zamowienie.md": `---
id: na-zamowienie
name: Sakwy na zamówienie
price: 300
stock:
  backorder: true
  leadTime: 7 dni
---
Sprowadzamy.
`,
	},
}

type shopHarness struct {
	t       *testing.T
	server  *Server
	rm      *projections.ReadModelsDB
	es      *eventstore.SQLiteEventStore
	http    *httptest.Server
	client  *http.Client
	base    *url.URL
	adminPW string
}

func newShopHarness(t *testing.T) *shopHarness {
	t.Helper()
	dir := t.TempDir()
	for sub, files := range shopFixtures {
		sub = strings.TrimSuffix(sub, "-extra") // "products-extra" lands in the products directory
		if err := os.MkdirAll(filepath.Join(dir, sub), 0o750); err != nil {
			t.Fatal(err)
		}
		for name, content := range files {
			if err := os.WriteFile(filepath.Join(dir, sub, name), []byte(content), 0o600); err != nil {
				t.Fatal(err)
			}
		}
	}

	es, err := eventstore.NewSQLiteEventStore(filepath.Join(dir, "events.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = es.Close() })
	rm, err := projections.NewReadModelsDB(filepath.Join(dir, "rm.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = rm.Close() })

	am := auth.NewAuthManager(rm.GetDB(), es)
	adminPW, err := am.EnsureAdminUser(t.Context())
	if err != nil {
		t.Fatal(err)
	}

	shopParser := shop.NewParser(filepath.Join(dir, "shop"))
	items, _ := shopParser.LoadAll()
	if err := rm.Stock().SyncCatalog(t.Context(), items); err != nil {
		t.Fatal(err)
	}

	tmpl := template.Must(template.New("main").Funcs(templateFuncs()).ParseGlob(filepath.Join(getTemplatePath(), "*.html")))
	server := &Server{
		eventStore:    es,
		readModels:    rm,
		projector:     projections.NewProjector(es, rm, "main"),
		productParser: products.NewParser(filepath.Join(dir, "products")),
		articleParser: articles.NewParser(filepath.Join(dir, "articles")),
		shopParser:    shopParser,
		authManager:   am,
		templates:     tmpl,
		bankMail:      &transfers.BankMailHandler{},
	}
	router := chi.NewRouter()
	server.RegisterRoutes(router)
	ts := httptest.NewServer(router)
	t.Cleanup(ts.Close)

	jar, _ := cookiejar.New(nil)
	base, _ := url.Parse(ts.URL)
	return &shopHarness{
		t: t, server: server, rm: rm, es: es, http: ts, base: base, adminPW: adminPW,
		client: &http.Client{Jar: jar, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }},
	}
}

// visitor returns a fresh client without cookies.
func (app *shopHarness) visitor() *http.Client {
	jar, _ := cookiejar.New(nil)
	return &http.Client{Jar: jar, CheckRedirect: app.client.CheckRedirect}
}

// asAdmin returns a client holding an admin session.
func (app *shopHarness) asAdmin() *http.Client {
	app.t.Helper()
	token, _, err := app.server.authManager.Login(app.t.Context(), "admin@example.com", app.adminPW)
	if err != nil {
		app.t.Fatalf("admin login: %v", err)
	}
	c := app.visitor()
	c.Jar.SetCookies(app.base, []*http.Cookie{{Name: "session_token", Value: token, Path: "/"}})
	return c
}

func (app *shopHarness) get(c *http.Client, path string) (int, string, http.Header) {
	app.t.Helper()
	resp, err := c.Get(app.http.URL + path)
	if err != nil {
		app.t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(body), resp.Header
}

func (app *shopHarness) post(client *http.Client, path string, form url.Values, hx bool) (int, http.Header) {
	app.t.Helper()
	req, _ := http.NewRequest(http.MethodPost, app.http.URL+path, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if hx {
		req.Header.Set("HX-Request", "true")
	}
	resp, err := client.Do(req)
	if err != nil {
		app.t.Fatal(err)
	}
	defer resp.Body.Close()
	return resp.StatusCode, resp.Header
}

func (app *shopHarness) addToCart(c *http.Client, itemID, sku string, qty string) (int, http.Header) {
	app.t.Helper()
	status, header := app.post(c, "/cart/add-shop", url.Values{"item_id": {itemID}, "sku": {sku}, "quantity": {qty}}, false)
	return status, header
}

// checkout submits the checkout form and returns the status, the Location (or HX-Redirect) and the order ID if one was created.
func (app *shopHarness) checkout(c *http.Client, method string) (int, string, string) {
	app.t.Helper()
	status, header := app.post(c, "/checkout/submit", url.Values{
		"payment_method": {method}, "name": {"Jan Test"}, "email": {"jan@example.com"}, "phone": {"500100200"},
		"address": {"Radzymin"}, "is_adult": {"on"}, "accept_tos": {"on"}, "password": {"haslo12345"},
	}, true)
	location := header.Get("Location")
	if location == "" {
		location = header.Get("HX-Redirect")
	}
	orderID := ""
	for _, prefix := range []string{"/payment/", "/user/order/"} {
		if rest, ok := strings.CutPrefix(location, prefix); ok {
			orderID, _, _ = strings.Cut(rest, "?")
		}
	}
	return status, location, orderID
}

func (app *shopHarness) level(sku string) shop.Level {
	app.t.Helper()
	levels, err := app.rm.Stock().Levels(app.t.Context(), []string{sku})
	if err != nil {
		app.t.Fatal(err)
	}
	return levels[sku]
}

func (app *shopHarness) emit(orderID string, event eventstore.EventData) {
	app.t.Helper()
	e, err := eventstore.ToEvent(orderID, "order", event, 0)
	if err != nil {
		app.t.Fatal(err)
	}
	if err := app.es.Save(app.t.Context(), e); err != nil {
		app.t.Fatal(err)
	}
	if err := app.server.projector.Run(app.t.Context()); err != nil {
		app.t.Fatal(err)
	}
}

func (app *shopHarness) orderRow(orderID string) (string, string, float64) {
	app.t.Helper()
	var status, itemsJSON string
	var total float64
	err := app.rm.GetDB().QueryRow(`SELECT status, items_json, total_amount FROM orders WHERE id = ?`, orderID).Scan(&status, &itemsJSON, &total)
	if err != nil {
		app.t.Fatalf("order %s: %v", orderID, err)
	}
	return status, itemsJSON, total
}

func (app *shopHarness) bookedProducts(orderID string) []string {
	app.t.Helper()
	rows, err := app.rm.GetDB().Query(`SELECT DISTINCT product_id FROM product_bookings WHERE order_id = ?`, orderID)
	if err != nil {
		app.t.Fatal(err)
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			app.t.Fatal(err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		app.t.Fatal(err)
	}
	return ids
}

func assertContains(t *testing.T, body string, wants ...string) {
	t.Helper()
	for _, want := range wants {
		if !strings.Contains(body, want) {
			t.Errorf("response is missing %q", want)
		}
	}
}

func assertNotContains(t *testing.T, body string, unwanted ...string) {
	t.Helper()
	for _, bad := range unwanted {
		if strings.Contains(body, bad) {
			t.Errorf("response must not contain %q", bad)
		}
	}
}

func TestShopListShowsVisibleItemsAndHidesAdminOnes(t *testing.T) {
	app := newShopHarness(t)

	status, body, _ := app.get(app.visitor(), "/shop")
	if status != http.StatusOK {
		t.Fatalf("status %d", status)
	}
	assertContains(t, body, "Dzwonek", "Kask", "Lampka", "Sakwy na zamówienie",
		"od 120 zł",             // variants with different prices
		"Ostatnie sztuki: 3",    // dzwonek: 3 in stock, default threshold 3
		"Na zamówienie – 7 dni", // backorder
		"Polecamy · Allegro", "od ok. 59 zł",
		"linki afiliacyjne", `href="/shop"`) // disclosure + nav link
	assertNotContains(t, body, "Tajny produkt")

	_, adminBody, _ := app.get(app.asAdmin(), "/shop")
	assertContains(t, adminBody, "Tajny produkt")

	_, filtered, _ := app.get(app.visitor(), "/shop?category=akcesoria")
	assertContains(t, filtered, "Dzwonek")
	assertNotContains(t, filtered, "Lampka")
}

func TestShopItemPage(t *testing.T) {
	app := newShopHarness(t)
	shopper := app.visitor()

	_, body, _ := app.get(shopper, "/shop/kask")
	assertContains(t, body, `name="sku" value="kask-s"`, `name="sku" value="kask-l"`, "130 zł", "Ostatnie sztuki: 2", "Brak w magazynie")
	if !strings.Contains(body, `value="kask-l" required class="w-5 h-5 text-emerald-600" disabled`) {
		t.Error("the sold-out size must not be selectable")
	}

	_, affiliate, _ := app.get(shopper, "/shop/lampka")
	assertContains(t, affiliate, `href="/shop/lampka/go"`, `rel="sponsored nofollow noopener noreferrer"`, "Zobacz ofertę w serwisie Allegro", "link afiliacyjny")
	assertNotContains(t, affiliate, "https://allegro.pl/oferta/lampka-123", "/cart/add-shop")

	if status, _, _ := app.get(shopper, "/shop/tajny"); status != http.StatusNotFound {
		t.Errorf("admin-only item must be a 404 for visitors, got %d", status)
	}
	if status, _, _ := app.get(shopper, "/shop/nie-ma"); status != http.StatusNotFound {
		t.Errorf("unknown item must be a 404, got %d", status)
	}
}

func TestCrossLinksBetweenShopProductsAndArticles(t *testing.T) {
	app := newShopHarness(t)
	shopper := app.visitor()

	// Shop description -> rental product, article; unknown and admin-only targets degrade to text.
	_, body, _ := app.get(shopper, "/shop/dzwonek")
	assertContains(t, body,
		`<a href="/product/cargo"`, ">Rower cargo</a>",
		`<a href="/article/guide"`, ">Jak wybrać rower</a>")
	assertNotContains(t, body, "[[", `href="/shop/tajny"`)
	assertContains(t, body, "tajny oraz nie-ma") // plain text fallbacks

	// Reverse relations declared on the other side.
	assertContains(t, body, "Pasuje do sprzętu z wypożyczalni", `href="/product/cargo"`)

	// Product page: description link to a shop item + the accessories strip, from both directions.
	_, product, _ := app.get(shopper, "/product/cargo")
	assertContains(t, product, `<a href="/shop/kask"`, "Akcesoria do tego sprzętu", `href="/shop/dzwonek"`)

	// Article page: custom label, and kask is listed because the article declares relatedShop.
	_, article, _ := app.get(shopper, "/article/guide")
	assertContains(t, article, `<a href="/shop/lampka"`, ">polecaną lampkę</a>", "Akcesoria i polecane produkty", `href="/shop/kask"`)

	// The home page cards show plain text, never raw [[...]] tokens or nested links.
	_, home, _ := app.get(shopper, "/")
	assertNotContains(t, home, "[[")
	assertContains(t, home, "Dobrany do niego Kask jest w sklepie")
}

func TestAdminSeesTheAdminOnlyItemThroughLinks(t *testing.T) {
	app := newShopHarness(t)
	_, body, _ := app.get(app.asAdmin(), "/shop/dzwonek")
	assertContains(t, body, `<a href="/shop/tajny"`, ">Tajny produkt</a>")
}

func TestCheckoutReservesStockAndStopsOverselling(t *testing.T) {
	app := newShopHarness(t)

	// Two shoppers fill their carts with the last two bells each.
	first, second := app.visitor(), app.visitor()
	if status, header := app.addToCart(first, "dzwonek", "", "2"); status != http.StatusSeeOther || header.Get("Location") != "/shop/dzwonek?added=1" {
		t.Fatalf("add to cart: %d %s", status, header.Get("Location"))
	}
	app.addToCart(second, "dzwonek", "", "2")

	status, location, orderID := app.checkout(first, "blik")
	if orderID == "" || !strings.HasPrefix(location, "/payment/") {
		t.Fatalf("first checkout should succeed, got %d %q", status, location)
	}
	if l := app.level("dzwonek"); l.OnHand != 3 || l.Reserved != 2 {
		t.Errorf("after the first order: %+v", l)
	}

	// The second shopper is told to review the cart; no order, no extra reservation.
	status, location, orderID2 := app.checkout(second, "blik")
	if status != http.StatusConflict || location != "/checkout?notice=cart" {
		t.Fatalf("second checkout should bounce back to the cart, got %d %q", status, location)
	}
	if orderID2 != "" {
		t.Errorf("no order may be created, got %s", orderID2)
	}
	if l := app.level("dzwonek"); l.Reserved != 2 {
		t.Errorf("a refused checkout must not reserve anything: %+v", l)
	}

	// The cart page explains what is wrong.
	_, cartPage, _ := app.get(second, "/checkout?notice=cart")
	assertContains(t, cartPage, "Dostępne tylko 1 szt.", "Popraw pozycje oznaczone na czerwono")

	// The order contains a shop line and blocks no calendar days.
	_, items, total := app.orderRow(orderID)
	var lines []domain.OrderItem
	if err := json.Unmarshal([]byte(items), &lines); err != nil {
		t.Fatal(err)
	}
	if len(lines) != 1 || !lines[0].IsShop() || lines[0].Quantity != 2 || lines[0].SKU != "dzwonek" || lines[0].BasePrice != domain.Zloty(25) {
		t.Errorf("order lines = %+v", lines)
	}
	if total != 50 {
		t.Errorf("total = %v, want 50", total)
	}
	var bookings int
	_ = app.rm.GetDB().QueryRow(`SELECT COUNT(*) FROM product_bookings`).Scan(&bookings)
	if bookings != 0 {
		t.Errorf("shop purchases must not book rental days, got %d", bookings)
	}
}

func TestSimultaneousCheckoutsOfTheLastUnitHaveOneWinner(t *testing.T) {
	app := newShopHarness(t)

	const shoppers = 6
	clients := make([]*http.Client, shoppers)
	for i := range clients {
		clients[i] = app.visitor()
		app.addToCart(clients[i], "kask", "kask-l", "1") // kask-l has no stock: cannot even be added
	}
	if l := app.level("kask-l"); l.Reserved != 0 {
		t.Fatalf("setup: %+v", l)
	}

	// Stock exactly one unit and race the checkouts.
	if err := app.rm.Stock().Adjust(t.Context(), shop.AdjustInput{SKU: "kask-l", ItemID: "kask", Kind: shop.KindReceive, Quantity: 1, Actor: "test"}); err != nil {
		t.Fatal(err)
	}
	for _, c := range clients {
		if status, _ := app.addToCart(c, "kask", "kask-l", "1"); status != http.StatusSeeOther {
			t.Fatalf("add: %d", status)
		}
	}

	type result struct{ orderID string }
	results := make(chan result, shoppers)
	for _, c := range clients {
		go func() {
			_, _, id := app.checkout(c, "blik")
			results <- result{id}
		}()
	}
	won := 0
	for range clients {
		if (<-results).orderID != "" {
			won++
		}
	}
	if won != 1 {
		t.Errorf("exactly one shopper may get the last unit, %d did", won)
	}
	if l := app.level("kask-l"); l.OnHand != 1 || l.Reserved != 1 {
		t.Errorf("level = %+v", l)
	}
}

func TestCartPricesComeFromTheCatalogNotTheCookie(t *testing.T) {
	app := newShopHarness(t)
	shopper := app.visitor()

	forged, _ := json.Marshal([]CartItem{{CartID: "1", Kind: cartKindShop, ProductID: "dzwonek", SKU: "dzwonek", ProductName: "Gratis", BasePrice: domain.Zloty(1), Quantity: 2, Total: domain.Zloty(2)}})
	shopper.Jar.SetCookies(app.base, []*http.Cookie{{Name: "cart", Value: url.QueryEscape(string(forged)), Path: "/"}})

	_, page, _ := app.get(shopper, "/checkout")
	assertContains(t, page, "Dzwonek", "25 zł / szt.", "50 zł")
	assertContains(t, page, "<span>50 zł</span>") // the header badge counts shop lines too
	assertNotContains(t, page, "Gratis")

	_, _, orderID := app.checkout(shopper, "cash_pickup")
	if orderID == "" {
		t.Fatal("checkout failed")
	}
	if _, _, total := app.orderRow(orderID); total != 50 {
		t.Errorf("the order must be charged the catalog price, got %v", total)
	}
}

func TestCartRejectsUnavailableAndInvalidShopLines(t *testing.T) {
	app := newShopHarness(t)
	shopper := app.visitor()

	cases := []struct {
		name          string
		item, sku, qt string
		wantError     string
	}{
		{"affiliate items cannot be bought", "lampka", "", "1", ""},
		{"admin-only items are invisible", "tajny", "", "1", ""},
		{"unknown item", "nie-ma", "", "1", ""},
		{"missing variant", "kask", "", "1", "variant"},
		{"unknown variant", "kask", "kask-xxl", "1", "unavailable"},
		{"sold out variant", "kask", "kask-l", "1", "unavailable"},
		{"more than in stock", "dzwonek", "", "4", "stock"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			status, header := app.addToCart(shopper, tc.item, tc.sku, tc.qt)
			if tc.wantError == "" {
				if status != http.StatusNotFound {
					t.Errorf("status %d, want 404", status)
				}
				return
			}
			if want := "/shop/" + tc.item + "?error=" + tc.wantError; header.Get("Location") != want {
				t.Errorf("redirect %q, want %q", header.Get("Location"), want)
			}
		})
	}
	if cart := getCartFromJar(t, app, shopper); len(cart) != 0 {
		t.Errorf("nothing may be added to the cart, got %+v", cart)
	}

	// Adding the same SKU twice merges lines and respects the per-order cap.
	app.addToCart(shopper, "dzwonek", "", "2")
	app.addToCart(shopper, "dzwonek", "", "1")
	cart := getCartFromJar(t, app, shopper)
	if len(cart) != 1 || cart[0].Quantity != 3 || cart[0].Total != domain.Zloty(75) {
		t.Errorf("cart = %+v", cart)
	}
}

func getCartFromJar(t *testing.T, app *shopHarness, c *http.Client) []CartItem {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/checkout", nil)
	for _, cookie := range c.Jar.Cookies(app.base) {
		req.AddCookie(cookie)
	}
	return getCart(req)
}

func TestUpdatingQuantityInTheCart(t *testing.T) {
	app := newShopHarness(t)
	shopper := app.visitor()
	app.addToCart(shopper, "dzwonek", "", "1")
	cartID := getCartFromJar(t, app, shopper)[0].CartID

	if status, header := app.post(shopper, "/cart/update/"+cartID, url.Values{"quantity": {"2"}}, false); status != http.StatusSeeOther || header.Get("Location") != "/checkout" {
		t.Fatalf("update: %d %s", status, header.Get("Location"))
	}
	if cart := getCartFromJar(t, app, shopper); cart[0].Quantity != 2 || cart[0].Total != domain.Zloty(50) {
		t.Errorf("cart = %+v", cart)
	}
	app.post(shopper, "/cart/update/"+cartID, url.Values{"quantity": {"99"}}, false)
	if cart := getCartFromJar(t, app, shopper); cart[0].Quantity != 5 {
		t.Errorf("quantity must be capped at maxPerOrder, got %d", cart[0].Quantity)
	}
	app.post(shopper, "/cart/update/"+cartID, url.Values{"quantity": {"0"}}, false)
	if cart := getCartFromJar(t, app, shopper); len(cart) != 0 {
		t.Errorf("zero removes the line, got %+v", cart)
	}
	if status, _ := app.post(shopper, "/cart/update/nope", url.Values{"quantity": {"1"}}, false); status != http.StatusNotFound {
		t.Errorf("unknown line: %d", status)
	}
}

func TestMixedOrderBooksTheRentalButOnlyHoldsShopStock(t *testing.T) {
	app := newShopHarness(t)
	shopper := app.visitor()

	app.addToCart(shopper, "dzwonek", "", "1")
	rental := []CartItem{{CartID: "r1", ProductID: "cargo", ProductName: "Rower cargo", BasePrice: domain.Zloty(150), StartDate: "2099-06-10", EndDate: "2099-06-11", RentalDays: 2, Total: domain.Zloty(300)}}
	cart := append(getCartFromJar(t, app, shopper), rental...)
	data, _ := json.Marshal(cart)
	shopper.Jar.SetCookies(app.base, []*http.Cookie{{Name: "cart", Value: url.QueryEscape(string(data)), Path: "/"}})

	_, _, orderID := app.checkout(shopper, "blik")
	if orderID == "" {
		t.Fatal("checkout failed")
	}

	var start, end string
	var days int
	_ = app.rm.GetDB().QueryRow(`SELECT start_date, end_date, rental_days FROM orders WHERE id = ?`, orderID).Scan(&start, &end, &days)
	if start != "2099-06-10" || end != "2099-06-11" || days != 2 {
		t.Errorf("rental dates must come from the rental line: %s %s %d", start, end, days)
	}
	bookedProducts := app.bookedProducts(orderID)
	if len(bookedProducts) != 1 || bookedProducts[0] != "cargo" {
		t.Errorf("only the rental may book days, got %v", bookedProducts)
	}
	if _, _, total := app.orderRow(orderID); total != 25+300 {
		t.Errorf("total = %v", total)
	}
	if l := app.level("dzwonek"); l.Reserved != 1 {
		t.Errorf("level = %+v", l)
	}
}

func TestCancellingAnOrderReleasesItsStock(t *testing.T) {
	app := newShopHarness(t)
	shopper := app.visitor()
	app.addToCart(shopper, "dzwonek", "", "3")
	_, _, orderID := app.checkout(shopper, "blik")
	if l := app.level("dzwonek"); l.Available() != 0 {
		t.Fatalf("setup: %+v", l)
	}

	app.emit(orderID, &domain.OrderCancelledEvent{OrderID: orderID, Timestamp: time.Now().UTC()})

	if l := app.level("dzwonek"); l.OnHand != 3 || l.Reserved != 0 {
		t.Errorf("stock must be back on the shelf: %+v", l)
	}
	// ...and somebody else can now buy it.
	other := app.visitor()
	app.addToCart(other, "dzwonek", "", "3")
	if _, _, id := app.checkout(other, "blik"); id == "" {
		t.Error("released stock should be purchasable")
	}
}

func TestRealizingAnOrderTakesStockOffTheShelf(t *testing.T) {
	app := newShopHarness(t)
	admin := app.asAdmin()
	shopper := app.visitor()
	app.addToCart(shopper, "dzwonek", "", "2")
	_, _, orderID := app.checkout(shopper, "cash_pickup")

	// Pickup orders wait for the shop's confirmation; realizing too early is refused.
	_, header := app.post(admin, "/admin/order/"+orderID+"/realize", nil, false)
	if loc := header.Get("Location"); !strings.Contains(loc, "error=") {
		t.Errorf("an unconfirmed order cannot be realized, got %q", loc)
	}

	app.emit(orderID, &domain.OrderConfirmedEvent{OrderID: orderID, Timestamp: time.Now().UTC()})
	status, header := app.post(admin, "/admin/order/"+orderID+"/realize", nil, false)
	if status != http.StatusSeeOther || !strings.Contains(header.Get("Location"), "notice=") {
		t.Fatalf("realize: %d %q", status, header.Get("Location"))
	}

	if s, _, _ := app.orderRow(orderID); s != "realized" {
		t.Errorf("status = %s", s)
	}
	if l := app.level("dzwonek"); l.OnHand != 1 || l.Reserved != 0 {
		t.Errorf("level = %+v, want on hand 1", l)
	}
	if status, _ := app.post(app.visitor(), "/admin/order/"+orderID+"/realize", nil, false); status != http.StatusFound {
		t.Errorf("visitors must be sent to log in, got %d", status)
	}
}

func TestBackorderLetsYouOrderWhatIsNotOnTheShelf(t *testing.T) {
	app := newShopHarness(t)
	shopper := app.visitor()
	if status, _ := app.addToCart(shopper, "na-zamowienie", "", "2"); status != http.StatusSeeOther {
		t.Fatalf("add: %d", status)
	}
	_, _, orderID := app.checkout(shopper, "cash_pickup")
	if orderID == "" {
		t.Fatal("backorders must be accepted")
	}
	if l := app.level("na-zamowienie"); l.OnHand != 0 || l.Reserved != 2 {
		t.Errorf("level = %+v", l)
	}

	// The goods have not arrived, so the order cannot be handed over yet.
	app.emit(orderID, &domain.OrderConfirmedEvent{OrderID: orderID, Timestamp: time.Now().UTC()})
	_, header := app.post(app.asAdmin(), "/admin/order/"+orderID+"/realize", nil, false)
	if loc := header.Get("Location"); !strings.Contains(loc, "error=") || !strings.Contains(loc, "Przyjmij+dostaw") {
		t.Errorf("realizing without goods must be refused with advice, got %q", loc)
	}
}

func TestAffiliateClickRedirectsAndIsCounted(t *testing.T) {
	app := newShopHarness(t)
	shopper := app.visitor()

	for range 2 {
		status, _, header := app.get(shopper, "/shop/lampka/go")
		if status != http.StatusFound || header.Get("Location") != "https://allegro.pl/oferta/lampka-123" {
			t.Fatalf("redirect: %d %q", status, header.Get("Location"))
		}
		if header.Get("X-Robots-Tag") == "" {
			t.Error("the redirect must not be indexed")
		}
	}
	stats, _ := app.rm.Stock().Clicks(t.Context())
	if stats["lampka"].Total != 2 {
		t.Errorf("clicks = %+v", stats["lampka"])
	}
	if status, _, _ := app.get(shopper, "/shop/dzwonek/go"); status != http.StatusNotFound {
		t.Errorf("only affiliate items redirect, got %d", status)
	}
	if stats, _ := app.rm.Stock().Clicks(t.Context()); stats["dzwonek"].Total != 0 {
		t.Error("no click may be counted for a non-affiliate item")
	}
}

func TestUnpaidOrdersExpireAndFreeTheirStock(t *testing.T) {
	app := newShopHarness(t)
	ctx := t.Context()

	waiting, paid := app.visitor(), app.visitor()
	app.addToCart(waiting, "dzwonek", "", "1")
	_, _, unpaidID := app.checkout(waiting, "blik")
	app.addToCart(paid, "dzwonek", "", "1")
	_, _, pickupID := app.checkout(paid, "cash_pickup")

	old := time.Now().UTC().Add(-72 * time.Hour).Format("2006-01-02 15:04:05")
	if _, err := app.rm.GetDB().Exec(`UPDATE orders SET created_at = ?`, old); err != nil {
		t.Fatal(err)
	}

	n, err := app.server.expireStaleShopOrders(ctx, 48*time.Hour)
	if err != nil || n != 1 {
		t.Fatalf("expired %d orders, err %v", n, err)
	}
	if s, _, _ := app.orderRow(unpaidID); s != "cancelled" {
		t.Errorf("the unpaid order should be cancelled, is %s", s)
	}
	if s, _, _ := app.orderRow(pickupID); s != "paid" {
		t.Errorf("the pickup order must be left alone, is %s", s)
	}
	if l := app.level("dzwonek"); l.Reserved != 1 {
		t.Errorf("only the unpaid reservation is released: %+v", l)
	}
	if n, _ := app.server.expireStaleShopOrders(ctx, 48*time.Hour); n != 0 {
		t.Errorf("a second sweep must find nothing, got %d", n)
	}
	if n, _ := app.server.expireStaleShopOrders(ctx, 0); n != 0 {
		t.Error("a zero TTL disables expiry")
	}
}

func TestAdminShopTabAndStockAdjustment(t *testing.T) {
	app := newShopHarness(t)
	admin := app.asAdmin()

	_, panel, _ := app.get(admin, "/admin")
	assertContains(t, panel, `id="content-shop"`, "Stany magazynowe", "Rozmiar S", "kask-l", "Linki afiliacyjne", "Lampka", "allegro.pl/oferta/lampka-123")
	// Reference problems in the fixtures are reported: [[shop:nie-ma]] does not exist.
	assertContains(t, panel, "Do poprawienia w plikach katalogu", "[[shop:nie-ma]]")

	status, header := app.post(admin, "/admin/shop/stock/adjust", url.Values{"sku": {"kask-l"}, "kind": {"receive"}, "quantity": {"4"}, "note": {"dostawa FV 12/10"}}, false)
	if status != http.StatusSeeOther || !strings.Contains(header.Get("Location"), "notice=") {
		t.Fatalf("adjust: %d %q", status, header.Get("Location"))
	}
	if l := app.level("kask-l"); l.OnHand != 4 {
		t.Errorf("level = %+v", l)
	}
	_, panel, _ = app.get(admin, "/admin?notice=Zapisano")
	assertContains(t, panel, "Zapisano", "dostawa FV 12/10", "Dostawa")

	for name, form := range map[string]url.Values{
		"unknown sku":           {"sku": {"nope"}, "kind": {"receive"}, "quantity": {"1"}},
		"untracked item":        {"sku": {"tajny"}, "kind": {"receive"}, "quantity": {"1"}},
		"bad kind":              {"sku": {"kask-l"}, "kind": {"reserve"}, "quantity": {"1"}},
		"not a number":          {"sku": {"kask-l"}, "kind": {"receive"}, "quantity": {"x"}},
		"more than is on shelf": {"sku": {"kask-l"}, "kind": {"loss"}, "quantity": {"99"}},
	} {
		_, header := app.post(admin, "/admin/shop/stock/adjust", form, false)
		if !strings.Contains(header.Get("Location"), "error=") {
			t.Errorf("%s: expected an error redirect, got %q", name, header.Get("Location"))
		}
	}
	if l := app.level("kask-l"); l.OnHand != 4 {
		t.Errorf("rejected adjustments must not change stock: %+v", l)
	}

	if status, _ := app.post(app.visitor(), "/admin/shop/stock/adjust", url.Values{"sku": {"kask-l"}, "kind": {"receive"}, "quantity": {"1"}}, false); status != http.StatusFound {
		t.Errorf("visitors must not reach the admin endpoint, got %d", status)
	}
}

func TestShopIsAbsentWhenThereIsNothingToShow(t *testing.T) {
	app := newShopHarness(t)
	app.server.shopParser = shop.NewParser(filepath.Join(t.TempDir(), "empty"))

	_, body, _ := app.get(app.visitor(), "/")
	assertNotContains(t, body, `href="/shop"`)
	_, shopPage, _ := app.get(app.visitor(), "/shop")
	assertContains(t, shopPage, "Sklep jest w przygotowaniu")
}
