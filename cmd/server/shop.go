package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"net/url"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"

	"cargo.mleczki.pl/internal/crosslinks"
	"cargo.mleczki.pl/internal/domain"
	"cargo.mleczki.pl/internal/eventstore"
	"cargo.mleczki.pl/internal/middleware"
	"cargo.mleczki.pl/internal/shop"
)

// maxCartCookieBytes keeps the cart cookie safely below the ~4 KB browsers accept.
const maxCartCookieBytes = 3500

// ---------------------------------------------------------------------------
// Catalog access
// ---------------------------------------------------------------------------

var (
	shopProblemsMu   sync.Mutex
	shopProblemsSeen string
)

// shopCatalog returns every valid shop item. Invalid files are skipped; the
// problem is logged once per change instead of on every request.
func (s *Server) shopCatalog() []domain.ShopItem {
	if s.shopParser == nil {
		return nil
	}
	items, err := s.shopParser.LoadAll()
	if err != nil {
		shopProblemsMu.Lock()
		if msg := err.Error(); msg != shopProblemsSeen {
			shopProblemsSeen = msg
			log.Printf("Shop catalog problems (affected items are hidden):\n%s", msg)
		}
		shopProblemsMu.Unlock()
	}
	return items
}

func (s *Server) visibleShopItems(r *http.Request) []domain.ShopItem {
	var visible []domain.ShopItem
	for _, item := range s.shopCatalog() {
		if isItemVisible(item.Visibility, r) {
			visible = append(visible, item)
		}
	}
	return visible
}

func (s *Server) shopHasVisibleItems(r *http.Request) bool {
	return len(s.visibleShopItems(r)) > 0
}

func (s *Server) shopItemByID(r *http.Request, id string) (domain.ShopItem, bool) {
	for _, item := range s.visibleShopItems(r) {
		if item.ID == id {
			return item, true
		}
	}
	return domain.ShopItem{}, false
}

func (s *Server) shopLevels(ctx context.Context, skus []string) map[string]shop.Level {
	if s.readModels == nil || len(skus) == 0 {
		return map[string]shop.Level{}
	}
	levels, err := s.readModels.Stock().Levels(ctx, skus)
	if err != nil {
		log.Printf("Failed to read stock levels: %v", err)
		return map[string]shop.Level{}
	}
	return levels
}

// ---------------------------------------------------------------------------
// View models
// ---------------------------------------------------------------------------

type shopVariantView struct {
	domain.ShopVariant
	EffectivePrice domain.Money
	Availability   shop.Availability
}

type shopItemView struct {
	domain.ShopItem
	Availability    shop.Availability
	BadgeClass      string
	DisplayPrice    domain.Money // lowest unit price
	PriceFrom       bool         // prices differ between variants
	VariantViews    []shopVariantView
	DescriptionHTML template.HTML
}

func stockBadgeClass(state shop.State) string {
	switch state {
	case shop.StateInStock:
		return "bg-emerald-100 text-emerald-700"
	case shop.StateLow:
		return "bg-amber-100 text-amber-800"
	case shop.StateBackorder:
		return "bg-blue-100 text-blue-800"
	case shop.StateOut:
		return "bg-gray-200 text-gray-600"
	default:
		return "bg-gray-200 text-gray-600"
	}
}

func newShopItemView(item domain.ShopItem, levels map[string]shop.Level) shopItemView {
	view := shopItemView{ShopItem: item}
	if !item.IsDirect() {
		return view
	}

	view.Availability = shop.ItemAvailability(item, levels)
	view.BadgeClass = stockBadgeClass(view.Availability.State)

	var lowest, highest domain.Money
	for i, sku := range item.SKUs() {
		price := item.UnitPrice(sku)
		if i == 0 || price.Less(lowest) {
			lowest = price
		}
		if highest.Less(price) {
			highest = price
		}
	}
	view.DisplayPrice, view.PriceFrom = lowest, lowest != highest

	for _, v := range item.Variants {
		view.VariantViews = append(view.VariantViews, shopVariantView{
			ShopVariant:    v,
			EffectivePrice: item.UnitPrice(v.SKU),
			Availability:   shop.AvailabilityFor(item, levels[v.SKU]),
		})
	}
	return view
}

func (s *Server) shopViews(r *http.Request, items []domain.ShopItem) []shopItemView {
	var skus []string
	for _, item := range items {
		if item.Tracked() {
			skus = append(skus, item.SKUs()...)
		}
	}
	levels := s.shopLevels(r.Context(), skus)

	views := make([]shopItemView, 0, len(items))
	for _, item := range items {
		views = append(views, newShopItemView(item, levels))
	}
	return views
}

// ---------------------------------------------------------------------------
// Cross links between products, articles and shop items
// ---------------------------------------------------------------------------

// linkIndex lists everything a [[kind:id]] reference can point at. With a
// request, items the visitor may not see are left out, so they cannot be
// discovered through a link; with a nil request nothing is filtered (admin audit).
func (s *Server) linkIndex(r *http.Request) *crosslinks.Index {
	visible := func(v domain.Visibility) bool { return r == nil || isItemVisible(v, r) }
	index := &crosslinks.Index{}

	if s.productParser != nil {
		if products, err := s.productParser.LoadAllProducts(); err == nil {
			for _, p := range products {
				if visible(p.Visibility) {
					index.Add(crosslinks.Target{Kind: crosslinks.KindProduct, ID: p.ID, Title: p.Name, URL: "/product/" + p.ID})
				}
			}
		}
	}
	if s.articleParser != nil {
		if articles, err := s.articleParser.LoadAllArticles(); err == nil {
			for _, a := range articles {
				if visible(a.Visibility) {
					index.Add(crosslinks.Target{Kind: crosslinks.KindArticle, ID: a.ID, Title: a.Title, URL: "/article/" + a.ID})
				}
			}
		}
	}
	for _, item := range s.shopCatalog() {
		if visible(item.Visibility) {
			index.Add(crosslinks.Target{Kind: crosslinks.KindShop, ID: item.ID, Title: item.Name, URL: "/shop/" + item.ID})
		}
	}
	return index
}

// expandLinks turns [[kind:id]] references in rendered markdown into anchors.
func (s *Server) expandLinks(r *http.Request, rendered string) template.HTML {
	if !strings.Contains(rendered, "[[") {
		return template.HTML(rendered) // #nosec G203 -- trusted markdown rendered by goldmark
	}
	out, _ := crosslinks.Expand(rendered, s.linkIndex(r).Lookup)
	return template.HTML(out) // #nosec G203 -- targets and labels are escaped by crosslinks.Expand
}

// plainTextLinks resolves references to plain text, for places inside another link.
func (s *Server) plainTextLinks(r *http.Request, rendered string) string {
	if !strings.Contains(rendered, "[[") {
		return rendered
	}
	return crosslinks.PlainText(rendered, s.linkIndex(r).Lookup)
}

func orderedUnique[T any](explicit []string, all []T, id func(T) string, reverse func(T) bool) []T {
	byID := make(map[string]T, len(all))
	for _, v := range all {
		byID[id(v)] = v
	}
	seen := map[string]bool{}
	var out []T
	for _, wanted := range explicit {
		if v, ok := byID[wanted]; ok && !seen[wanted] {
			seen[wanted] = true
			out = append(out, v)
		}
	}
	for _, v := range all {
		if !seen[id(v)] && reverse(v) {
			seen[id(v)] = true
			out = append(out, v)
		}
	}
	return out
}

// shopItemsForProduct lists shop items related to a rental product, whether the
// product or the shop item declared the relation.
func (s *Server) shopItemsForProduct(r *http.Request, product *domain.Product) []shopItemView {
	items := orderedUnique(product.ShopItems, s.visibleShopItems(r),
		func(i domain.ShopItem) string { return i.ID },
		func(i domain.ShopItem) bool { return slices.Contains(i.RelatedProducts, product.ID) })
	return s.shopViews(r, items)
}

// shopItemsForArticle lists shop items related to an article.
func (s *Server) shopItemsForArticle(r *http.Request, article *domain.Article) []shopItemView {
	items := orderedUnique(article.RelatedShop, s.visibleShopItems(r),
		func(i domain.ShopItem) string { return i.ID },
		func(i domain.ShopItem) bool { return slices.Contains(i.RelatedArticles, article.ID) })
	return s.shopViews(r, items)
}

type shopItemRelations struct {
	Products []domain.Product
	Articles []domain.Article
	Shop     []shopItemView
}

func (s *Server) relationsOfShopItem(r *http.Request, item domain.ShopItem) shopItemRelations {
	var rel shopItemRelations

	if s.productParser != nil {
		if products, err := s.productParser.LoadAllProducts(); err == nil {
			rel.Products = orderedUnique(item.RelatedProducts, filterProductsByVisibility(products, r),
				func(p domain.Product) string { return p.ID },
				func(p domain.Product) bool { return slices.Contains(p.ShopItems, item.ID) })
		}
	}
	if s.articleParser != nil {
		if articles, err := s.articleParser.LoadAllArticles(); err == nil {
			rel.Articles = orderedUnique(item.RelatedArticles, filterArticlesByVisibility(articles, r),
				func(a domain.Article) string { return a.ID },
				func(a domain.Article) bool { return slices.Contains(a.RelatedShop, item.ID) })
		}
	}

	others := slices.DeleteFunc(s.visibleShopItems(r), func(i domain.ShopItem) bool { return i.ID == item.ID })
	related := orderedUnique(item.RelatedShop, others,
		func(i domain.ShopItem) string { return i.ID },
		func(i domain.ShopItem) bool { return slices.Contains(i.RelatedShop, item.ID) })
	rel.Shop = s.shopViews(r, related)
	return rel
}

// ---------------------------------------------------------------------------
// Public pages
// ---------------------------------------------------------------------------

type shopCategory struct {
	Name   string
	Count  int
	Active bool
}

// handleShop renders the shop front page.
func (s *Server) handleShop(w http.ResponseWriter, r *http.Request) {
	items := s.visibleShopItems(r)

	active := strings.TrimSpace(r.URL.Query().Get("category"))
	counts := map[string]int{}
	for _, item := range items {
		if item.Category != "" {
			counts[item.Category]++
		}
	}
	categories := make([]shopCategory, 0, len(counts))
	for name, count := range counts {
		categories = append(categories, shopCategory{Name: name, Count: count, Active: name == active})
	}
	sort.Slice(categories, func(i, j int) bool { return categories[i].Name < categories[j].Name })

	if active != "" {
		items = slices.DeleteFunc(items, func(i domain.ShopItem) bool { return i.Category != active })
	}
	hasAffiliate := slices.ContainsFunc(items, func(i domain.ShopItem) bool { return i.IsAffiliate() })

	s.renderTemplate(w, r, "shop.html", map[string]interface{}{
		"Title":          "Sklep",
		"Items":          s.shopViews(r, items),
		"Categories":     categories,
		"ActiveCategory": active,
		"HasAffiliate":   hasAffiliate,
	})
}

// handleShopItem renders a single shop item.
func (s *Server) handleShopItem(w http.ResponseWriter, r *http.Request) {
	item, ok := s.shopItemByID(r, chi.URLParam(r, "id"))
	if !ok {
		http.NotFound(w, r)
		return
	}

	view := s.shopViews(r, []domain.ShopItem{item})[0]
	view.DescriptionHTML = s.expandLinks(r, item.Description)

	rel := s.relationsOfShopItem(r, item)
	data := map[string]interface{}{
		"Title":           item.Name,
		"Item":            view,
		"RelatedProducts": rel.Products,
		"RelatedArticles": rel.Articles,
		"RelatedShop":     rel.Shop,
		"Added":           r.URL.Query().Get("added") == "1",
		"CartError":       shopCartError(r.URL.Query().Get("error")),
	}
	s.renderTemplate(w, r, "shop_item.html", data)
}

func shopCartError(code string) string {
	switch code {
	case "stock":
		return "Nie mamy tylu sztuk. Zmniejsz ilość albo wybierz inną wersję."
	case "unavailable":
		return "Ten produkt jest chwilowo niedostępny."
	case "variant":
		return "Wybierz wersję produktu."
	case "full":
		return "Koszyk jest pełny. Złóż zamówienie, a resztę dodaj w kolejnym."
	case "":
		return ""
	default:
		return "Nie udało się dodać produktu do koszyka."
	}
}

// handleShopGo counts the click and sends the visitor to the affiliate offer.
func (s *Server) handleShopGo(w http.ResponseWriter, r *http.Request) {
	item, ok := s.shopItemByID(r, chi.URLParam(r, "id"))
	if !ok || !item.IsAffiliate() {
		http.NotFound(w, r)
		return
	}
	if s.readModels != nil {
		if err := s.readModels.Stock().RecordClick(r.Context(), item.ID); err != nil {
			log.Printf("Failed to record affiliate click for %s: %v", item.ID, err)
		}
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Robots-Tag", "noindex, nofollow")
	// The target comes from the catalog file (validated as http(s) when parsed), never from the request.
	http.Redirect(w, r, item.Affiliate.URL, http.StatusFound) // #nosec G710
}

// ---------------------------------------------------------------------------
// Cart
// ---------------------------------------------------------------------------

func cartHasShopLines(cart []CartItem) bool {
	return slices.ContainsFunc(cart, func(c CartItem) bool { return c.IsShop() })
}

func cartHasRentalLines(cart []CartItem) bool {
	return slices.ContainsFunc(cart, func(c CartItem) bool { return !c.IsShop() })
}

func cartFitsCookie(cart []CartItem) bool {
	data, err := json.Marshal(cart)
	return err == nil && len(url.QueryEscape(string(data))) <= maxCartCookieBytes
}

// resolveShopSKU picks the SKU to buy: the chosen variant, or the item itself.
// The second result is an error code for shopCartError ("" when fine).
func resolveShopSKU(item domain.ShopItem, requested string) (string, string) {
	if !item.HasVariants() {
		return item.ID, ""
	}
	requested = strings.TrimSpace(requested)
	switch {
	case requested == "":
		return "", "variant"
	case !item.HasSKU(requested):
		return "", "unavailable"
	default:
		return requested, ""
	}
}

// upsertShopLine adds quantity units of sku to the cart, merging with an existing
// line, and caps the line at the item's per-order limit.
func upsertShopLine(cart []CartItem, item domain.ShopItem, sku string, quantity int) ([]CartItem, CartItem) {
	existing := slices.IndexFunc(cart, func(c CartItem) bool { return c.IsShop() && c.SKU == sku })
	line := CartItem{
		CartID:      fmt.Sprintf("%d", time.Now().UnixNano()),
		Kind:        cartKindShop,
		ProductID:   item.ID,
		SKU:         sku,
		ProductName: item.LineName(sku),
		BasePrice:   item.UnitPrice(sku),
	}
	if existing >= 0 {
		line.CartID = cart[existing].CartID
		quantity += cart[existing].Quantity
	}
	line.Quantity = min(quantity, item.MaxQuantity())
	line.Total = line.BasePrice.Mul(line.Quantity)

	if existing >= 0 {
		updated := slices.Clone(cart)
		updated[existing] = line
		return updated, line
	}
	return append(slices.Clone(cart), line), line
}

// handleCartAddShop puts a quantity of a shop item in the cart.
func (s *Server) handleCartAddShop(w http.ResponseWriter, r *http.Request) {
	if r.Method != methodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	item, ok := s.shopItemByID(r, r.FormValue("item_id"))
	if !ok || !item.IsDirect() {
		http.Error(w, "Product not found", http.StatusNotFound)
		return
	}
	back := func(code string) {
		http.Redirect(w, r, "/shop/"+item.ID+"?error="+code, http.StatusSeeOther)
	}

	sku, problem := resolveShopSKU(item, r.FormValue("sku"))
	if problem != "" {
		back(problem)
		return
	}
	quantity, err := strconv.Atoi(strings.TrimSpace(r.FormValue("quantity")))
	if err != nil || quantity < 1 {
		quantity = 1
	}

	cart, line := upsertShopLine(getCart(r), item, sku, quantity)

	availability := shop.AvailabilityFor(item, s.shopLevels(r.Context(), []string{sku})[sku])
	switch {
	case !availability.CanBuy:
		back("unavailable")
	case item.Tracked() && !item.Stock.Backorder && line.Quantity > availability.Available:
		back("stock")
	case !cartFitsCookie(cart):
		back("full")
	default:
		setCart(w, r, cart)
		http.Redirect(w, r, "/shop/"+item.ID+"?added=1", http.StatusSeeOther)
	}
}

// handleCartUpdateShop changes the quantity of a shop line; zero removes it.
func (s *Server) handleCartUpdateShop(w http.ResponseWriter, r *http.Request) {
	if r.Method != methodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	cartID := chi.URLParam(r, "id")
	quantity, err := strconv.Atoi(strings.TrimSpace(r.FormValue("quantity")))
	if err != nil || quantity < 0 {
		http.Error(w, "Invalid quantity", http.StatusBadRequest)
		return
	}

	cart := getCart(r)
	updated := make([]CartItem, 0, len(cart))
	found := false
	for _, line := range cart {
		if line.CartID != cartID || !line.IsShop() {
			updated = append(updated, line)
			continue
		}
		found = true
		if quantity == 0 {
			continue
		}
		if item, ok := s.shopItemByID(r, line.ProductID); ok {
			quantity = min(quantity, item.MaxQuantity())
		}
		line.Quantity = quantity
		line.Total = line.BasePrice.Mul(quantity)
		updated = append(updated, line)
	}
	if !found {
		http.Error(w, "Item not found in cart", http.StatusNotFound)
		return
	}

	setCart(w, r, updated)
	http.Redirect(w, r, "/checkout", http.StatusSeeOther)
}

// normalizeShopLines re-prices and validates the shop lines against the catalog
// and the stock ledger, updating them in place and recording what is wrong per CartID.
func (s *Server) normalizeShopLines(r *http.Request, cart []CartItem, problems map[string]string) {
	if !cartHasShopLines(cart) {
		return
	}

	catalog := map[string]domain.ShopItem{}
	for _, item := range s.visibleShopItems(r) {
		catalog[item.ID] = item
	}
	var skus []string
	for _, line := range cart {
		if line.IsShop() {
			skus = append(skus, line.SKU)
		}
	}
	levels := s.shopLevels(r.Context(), skus)

	for i, line := range cart {
		if !line.IsShop() {
			continue
		}
		item, ok := catalog[line.ProductID]
		switch {
		case !ok || !item.IsDirect():
			problems[line.CartID] = "Ten produkt nie jest już dostępny w sklepie."
			continue
		case !item.HasSKU(line.SKU):
			problems[line.CartID] = "Ta wersja produktu nie jest już dostępna."
			continue
		}

		cart[i].ProductName = item.LineName(line.SKU)
		cart[i].BasePrice = item.UnitPrice(line.SKU)
		cart[i].tracked = item.Tracked()
		cart[i].backorder = item.Stock.Backorder
		cart[i].Total = cart[i].BasePrice.Mul(line.Quantity)

		if problem := shopLineProblem(item, line, shop.AvailabilityFor(item, levels[line.SKU])); problem != "" {
			problems[line.CartID] = problem
		}
	}
}

// shopLineProblem explains why a cart line cannot be ordered as it is ("" when it can).
func shopLineProblem(item domain.ShopItem, line CartItem, availability shop.Availability) string {
	switch {
	case line.Quantity < 1:
		return "Podaj ilość."
	case line.Quantity > item.MaxQuantity():
		return fmt.Sprintf("Maksymalnie %d szt. w jednym zamówieniu.", item.MaxQuantity())
	case !availability.CanBuy:
		return "Brak w magazynie. Usuń tę pozycję, aby złożyć zamówienie."
	case item.Tracked() && !item.Stock.Backorder && line.Quantity > availability.Available:
		return fmt.Sprintf("Dostępne tylko %d szt.", availability.Available)
	default:
		return ""
	}
}

func checkoutNotice(code string) string {
	switch code {
	case "stock":
		return "Ktoś był szybszy: nie wszystkie produkty są już dostępne w wybranej ilości. Zaktualizuj koszyk i spróbuj ponownie."
	case "cart":
		return "Zmieniły się produkty w koszyku. Sprawdź uwagi przy pozycjach i spróbuj ponownie."
	default:
		return deliveryNotice(code)
	}
}

// redirectToCheckout sends the visitor (or the htmx form) back to the cart with an explanation.
func redirectToCheckout(w http.ResponseWriter, r *http.Request, notice string) {
	target := "/checkout?notice=" + url.QueryEscape(notice)
	if r.Header.Get("HX-Request") != "" {
		w.Header().Set("HX-Redirect", target)
		w.WriteHeader(http.StatusConflict)
		return
	}
	http.Redirect(w, r, target, http.StatusSeeOther)
}

// reserveStockOrRespond reserves the stock of the cart for the order. When that
// fails it answers the request itself (back to the cart if the stock ran out)
// and reports false.
func (s *Server) reserveStockOrRespond(w http.ResponseWriter, r *http.Request, orderID string, cart []CartItem) (func(), bool) {
	release, err := s.reserveShopStock(r.Context(), orderID, cart)
	if err == nil {
		return release, true
	}
	var short *shop.InsufficientStockError
	if errors.As(err, &short) {
		redirectToCheckout(w, r, "stock")
		return nil, false
	}
	log.Printf("Failed to reserve stock for %s: %v", orderID, err)
	http.Error(w, "Failed to reserve stock", http.StatusInternalServerError)
	return nil, false
}

// reserveShopStock holds the stock of every tracked shop line for orderID. The
// returned function undoes the hold if placing the order fails afterwards.
func (s *Server) reserveShopStock(ctx context.Context, orderID string, cart []CartItem) (func(), error) {
	release := func() {}
	var lines []shop.ReserveLine
	for _, line := range cart {
		if line.IsShop() && line.tracked {
			lines = append(lines, shop.ReserveLine{SKU: line.SKU, ItemID: line.ProductID, Quantity: line.Quantity, Backorder: line.backorder})
		}
	}
	if len(lines) == 0 {
		return release, nil
	}

	stock := s.readModels.Stock()
	if err := stock.Reserve(ctx, orderID, lines); err != nil {
		return release, err
	}
	return func() {
		if err := stock.ReleaseOrder(context.WithoutCancel(ctx), orderID); err != nil {
			log.Printf("Failed to release stock of %s after a failed checkout: %v", orderID, err)
		}
	}, nil
}

// ---------------------------------------------------------------------------
// Reservation expiry
// ---------------------------------------------------------------------------

// expireStaleShopOrders cancels orders that hold shop stock but were never
// paid, so abandoned BLIK checkouts do not lock units forever. The cancellation
// goes through the normal OrderCancelled event, which also releases the stock.
func (s *Server) expireStaleShopOrders(ctx context.Context, ttl time.Duration) (int, error) {
	if ttl <= 0 {
		return 0, nil
	}
	// Decide on fresh data: a payment that just arrived must win over the sweep.
	if err := s.projector.Run(ctx); err != nil {
		return 0, fmt.Errorf("project before expiry: %w", err)
	}
	ids, err := s.readModels.Stock().StaleReservations(ctx, time.Now().Add(-ttl))
	if err != nil {
		return 0, err
	}

	cancelled := 0
	for _, id := range ids {
		event, err := eventstore.ToEvent(id, "order", &domain.OrderCancelledEvent{OrderID: id, Timestamp: time.Now().UTC()}, 0)
		if err != nil {
			return cancelled, err
		}
		if err := s.eventStore.Save(ctx, event); err != nil {
			log.Printf("Failed to expire order %s: %v", id, err)
			continue
		}
		cancelled++
		log.Printf("Order %s was not paid within %s: cancelled, shop stock released", id, ttl)
	}
	if cancelled > 0 {
		if err := s.projector.Run(ctx); err != nil {
			return cancelled, fmt.Errorf("project expiry: %w", err)
		}
	}
	return cancelled, nil
}

// RunShopReservationSweeper periodically expires unpaid orders that hold stock.
func (s *Server) RunShopReservationSweeper(ctx context.Context, ttl, every time.Duration) {
	if ttl <= 0 {
		return
	}
	ticker := time.NewTicker(every)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if _, err := s.expireStaleShopOrders(ctx, ttl); err != nil {
				log.Printf("Shop reservation sweep failed: %v", err)
			}
		}
	}
}

// ---------------------------------------------------------------------------
// Admin
// ---------------------------------------------------------------------------

type adminStockRow struct {
	ItemID, ItemName, SKU, VariantName string
	OnHand, Reserved, Available        int
	Threshold                          int
	Backorder                          bool
	State                              string // "ok", "low" or "out"
}

type adminMovementRow struct {
	shop.Movement
	KindLabel string
	ItemName  string
}

type adminAffiliateRow struct {
	ItemID, Name, Merchant, URL string
	Total, Last30Days           int
}

type adminShopView struct {
	Stock      []adminStockRow
	LowCount   int
	OutCount   int
	Movements  []adminMovementRow
	Affiliate  []adminAffiliateRow
	Unlimited  []string
	Problems   []string
	ItemsTotal int
}

// shopAdminView gathers everything the "Sklep" admin tab shows.
func (s *Server) shopAdminView(ctx context.Context) adminShopView {
	var view adminShopView
	if s.shopParser == nil {
		return view
	}

	items, loadErr := s.shopParser.LoadAll()
	view.ItemsTotal = len(items)
	view.Problems = append(catalogProblemLines(loadErr), s.brokenReferences(items)...)

	stock := s.readModels.Stock()
	view.Stock, view.LowCount, view.OutCount = s.adminStockRows(ctx, items)
	view.Movements = adminMovementRows(ctx, stock, items)
	view.Affiliate = adminAffiliateRows(ctx, stock, items)
	for _, item := range items {
		if item.IsDirect() && !item.Tracked() {
			view.Unlimited = append(view.Unlimited, item.Name)
		}
	}
	return view
}

// catalogProblemLines splits the joined parse errors into one line per problem.
func catalogProblemLines(err error) []string {
	if err == nil {
		return nil
	}
	var lines []string
	for _, line := range strings.Split(err.Error(), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			lines = append(lines, line)
		}
	}
	return lines
}

func (s *Server) adminStockRows(ctx context.Context, items []domain.ShopItem) ([]adminStockRow, int, int) {
	var skus []string
	for _, item := range items {
		if item.Tracked() {
			skus = append(skus, item.SKUs()...)
		}
	}
	levels, err := s.readModels.Stock().Levels(ctx, skus)
	if err != nil {
		log.Printf("Failed to read stock for admin: %v", err)
	}

	var rows []adminStockRow
	low, out := 0, 0
	for _, item := range items {
		if !item.Tracked() {
			continue
		}
		for _, sku := range item.SKUs() {
			level := levels[sku]
			row := adminStockRow{
				ItemID: item.ID, ItemName: item.Name, SKU: sku,
				OnHand: level.OnHand, Reserved: level.Reserved, Available: level.Available(),
				Threshold: item.LowThreshold(), Backorder: item.Stock.Backorder, State: "ok",
			}
			if variant, ok := item.Variant(sku); ok {
				row.VariantName = variant.Name
			}
			switch {
			case level.Available() <= 0:
				row.State = "out"
				out++
			case level.Available() <= item.LowThreshold():
				row.State = "low"
				low++
			}
			rows = append(rows, row)
		}
	}
	return rows, low, out
}

func adminMovementRows(ctx context.Context, stock *shop.Store, items []domain.ShopItem) []adminMovementRow {
	names := map[string]string{} // sku -> display name
	for _, item := range items {
		for _, sku := range item.SKUs() {
			names[sku] = item.LineName(sku)
		}
	}
	movements, err := stock.Movements(ctx, "", 40)
	if err != nil {
		log.Printf("Failed to read stock movements for admin: %v", err)
	}
	rows := make([]adminMovementRow, 0, len(movements))
	for _, m := range movements {
		name := names[m.SKU]
		if name == "" {
			name = m.SKU
		}
		rows = append(rows, adminMovementRow{Movement: m, KindLabel: shop.KindLabel(m.Kind), ItemName: name})
	}
	return rows
}

func adminAffiliateRows(ctx context.Context, stock *shop.Store, items []domain.ShopItem) []adminAffiliateRow {
	clicks, err := stock.Clicks(ctx)
	if err != nil {
		log.Printf("Failed to read affiliate clicks for admin: %v", err)
	}
	var rows []adminAffiliateRow
	for _, item := range items {
		if item.IsAffiliate() {
			rows = append(rows, adminAffiliateRow{
				ItemID: item.ID, Name: item.Name, Merchant: item.Affiliate.Merchant, URL: item.Affiliate.URL,
				Total: clicks[item.ID].Total, Last30Days: clicks[item.ID].Last30Days,
			})
		}
	}
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].Last30Days > rows[j].Last30Days })
	return rows
}

// brokenReferences finds [[kind:id]] references and relation lists that point at
// something that does not exist, so a typo does not go unnoticed.
func (s *Server) brokenReferences(items []domain.ShopItem) []string {
	index := s.linkIndex(nil)
	var broken []string

	check := func(owner, where string, kind string, ids []string) {
		for _, id := range ids {
			if _, ok := index.Lookup(kind, id); !ok {
				broken = append(broken, fmt.Sprintf("%s: %s wskazuje na nieistniejący element %s „%s”", owner, where, kind, id))
			}
		}
	}
	scan := func(owner, rendered string) {
		for _, ref := range crosslinks.Find(rendered) {
			if _, ok := index.Lookup(ref.Kind, ref.ID); !ok {
				broken = append(broken, fmt.Sprintf("%s: odnośnik %s nie prowadzi do istniejącego elementu", owner, ref.Raw))
			}
		}
	}

	for _, item := range items {
		owner := "Sklep „" + item.ID + "”"
		scan(owner, item.Description)
		check(owner, "relatedProducts", crosslinks.KindProduct, item.RelatedProducts)
		check(owner, "relatedArticles", crosslinks.KindArticle, item.RelatedArticles)
		check(owner, "relatedShop", crosslinks.KindShop, item.RelatedShop)
	}
	if s.productParser != nil {
		if products, err := s.productParser.LoadAllProducts(); err == nil {
			for _, p := range products {
				owner := "Produkt „" + p.ID + "”"
				scan(owner, p.Description)
				check(owner, "shop", crosslinks.KindShop, p.ShopItems)
			}
		}
	}
	if s.articleParser != nil {
		if articles, err := s.articleParser.LoadAllArticles(); err == nil {
			for _, a := range articles {
				owner := "Artykuł „" + a.ID + "”"
				scan(owner, a.Content)
				check(owner, "relatedShop", crosslinks.KindShop, a.RelatedShop)
			}
		}
	}
	return broken
}

func adminRedirect(w http.ResponseWriter, r *http.Request, kind, message string) {
	http.Redirect(w, r, "/admin?"+kind+"="+url.QueryEscape(message)+"#shop", http.StatusSeeOther)
}

func adminActor(r *http.Request) string {
	if user, ok := middleware.GetUser(r).(*domain.User); ok && user != nil {
		return user.Email
	}
	return "administrator"
}

// handleAdminShopStockAdjust records a delivery, return, loss or stocktake.
func (s *Server) handleAdminShopStockAdjust(w http.ResponseWriter, r *http.Request) {
	if r.Method != methodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	sku := strings.TrimSpace(r.FormValue("sku"))
	var owner *domain.ShopItem
	for _, item := range s.shopCatalog() {
		if item.Tracked() && item.HasSKU(sku) {
			owner = &item
			break
		}
	}
	if owner == nil {
		adminRedirect(w, r, "error", "Nieznany lub nieśledzony SKU: "+sku)
		return
	}

	quantity, err := strconv.Atoi(strings.TrimSpace(r.FormValue("quantity")))
	if err != nil {
		adminRedirect(w, r, "error", "Podaj ilość jako liczbę całkowitą.")
		return
	}
	kind := r.FormValue("kind")
	if !slices.Contains([]string{shop.KindReceive, shop.KindReturn, shop.KindLoss, shop.KindCount}, kind) {
		adminRedirect(w, r, "error", "Nieznany rodzaj operacji.")
		return
	}
	note := strings.TrimSpace(r.FormValue("note"))
	if len([]rune(note)) > 200 {
		note = string([]rune(note)[:200])
	}

	err = s.readModels.Stock().Adjust(r.Context(), shop.AdjustInput{
		SKU: sku, ItemID: owner.ID, Kind: kind, Quantity: quantity, Note: note, Actor: adminActor(r),
	})
	if err != nil {
		adminRedirect(w, r, "error", err.Error())
		return
	}
	adminRedirect(w, r, "notice", fmt.Sprintf("Zapisano: %s – %s (%d).", owner.LineName(sku), shop.KindLabel(kind), quantity))
}

// errOrderNotConfirmed is returned when an order cannot be handed over because the shop has not confirmed it.
var errOrderNotConfirmed = errors.New("order is not confirmed")

// realizeOrder marks a confirmed order as handed over (or sent). For shop lines
// this takes the units off the shelf.
func (s *Server) realizeOrder(ctx context.Context, orderID string) error {
	var status string
	if err := s.readModels.GetDB().QueryRowContext(ctx, `SELECT status FROM orders WHERE id = ?`, orderID).Scan(&status); err != nil {
		return err
	}
	if status != string(domain.StatusConfirmed) {
		return errOrderNotConfirmed
	}
	if err := s.readModels.Stock().CanFulfil(ctx, orderID); err != nil {
		return err
	}

	event, err := eventstore.ToEvent(orderID, "order", &domain.OrderRealizedEvent{OrderID: orderID, Timestamp: time.Now().UTC()}, 0)
	if err != nil {
		return err
	}
	if err := s.eventStore.Save(ctx, event); err != nil {
		return fmt.Errorf("emit OrderRealizedEvent: %w", err)
	}
	if err := s.projector.Run(ctx); err != nil {
		log.Printf("Failed to project order realization: %v", err)
	}
	return nil
}

// handleAdminOrderRealize marks a confirmed order as handed over. For shop
// lines this takes the units off the shelf.
func (s *Server) handleAdminOrderRealize(w http.ResponseWriter, r *http.Request) {
	if r.Method != methodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	orderID := chi.URLParam(r, "id")
	err := s.realizeOrder(r.Context(), orderID)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		http.NotFound(w, r)
	case errors.Is(err, errOrderNotConfirmed):
		adminRedirect(w, r, "error", "Zrealizować można tylko potwierdzone zamówienie ("+orderID+").")
	case errors.Is(err, shop.ErrNotEnoughOnHand):
		adminRedirect(w, r, "error", "Zamówienie "+orderID+": brakuje towaru na stanie ("+err.Error()+"). Przyjmij dostawę i spróbuj ponownie.")
	case err != nil:
		log.Printf("Failed to realize order %s: %v", orderID, err)
		http.Error(w, "Failed to realize order", http.StatusInternalServerError)
	default:
		adminRedirect(w, r, "notice", "Zamówienie "+orderID+" oznaczone jako zrealizowane.")
	}
}
