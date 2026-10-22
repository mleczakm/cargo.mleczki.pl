package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"slices"
	"strings"
	"sync"
	"time"

	"cargo.mleczki.pl/internal/domain"
	"cargo.mleczki.pl/internal/projections"
	"cargo.mleczki.pl/internal/shipping"
)

// defaultShippingConfigPath is where the delivery options and the sender live.
const defaultShippingConfigPath = "data/shipping.yml"

// newShippingService builds the Polkurier service. Credentials come from the
// environment (POLKURIER_LOGIN, POLKURIER_TOKEN, POLKURIER_SANDBOX); without
// them only collection in person is offered.
func newShippingService(readModels *projections.ReadModelsDB) *shipping.Service {
	path := strings.TrimSpace(os.Getenv("SHIPPING_CONFIG"))
	if path == "" {
		path = defaultShippingConfigPath
	}
	svc := &shipping.Service{Config: &shipping.ConfigFile{Path: path}, Store: readModels.Shipments()}

	sandbox := true // never touch production by accident
	if raw := strings.TrimSpace(os.Getenv("POLKURIER_SANDBOX")); raw != "" {
		sandbox = !strings.EqualFold(raw, "false") && raw != "0"
	}
	svc.Configure(os.Getenv("POLKURIER_LOGIN"), os.Getenv("POLKURIER_TOKEN"), sandbox, os.Getenv("POLKURIER_API_URL"), "1")
	switch {
	case !svc.Configured():
		log.Println("Shipping: Polkurier disabled (set POLKURIER_LOGIN and POLKURIER_TOKEN); only collection in person is offered")
	case sandbox:
		log.Println("Shipping: Polkurier enabled (sandbox)")
	default:
		log.Println("Shipping: Polkurier enabled (production)")
	}
	return svc
}

var (
	loggedMu   sync.Mutex
	loggedSeen = map[string]string{}
)

// logOnce logs msg unless the last message under key was the same, so a broken
// config file does not flood the log on every request.
func logOnce(key, msg string) {
	loggedMu.Lock()
	defer loggedMu.Unlock()
	if loggedSeen[key] != msg {
		loggedSeen[key] = msg
		log.Printf("%s: %s", key, msg)
	}
}

// shippingConfig is the current data/shipping.yml. A broken file is logged and
// the last good version (or the default: collection in person) keeps serving.
func (s *Server) shippingConfig() shipping.Config {
	if s.shipping == nil || s.shipping.Config == nil {
		return shipping.DefaultConfig()
	}
	cfg, err := s.shipping.Config.Load()
	if err != nil {
		logOnce("shipping-config", err.Error())
	}
	return cfg
}

// shipments is the parcel store; it works even when Polkurier is not configured.
func (s *Server) shipments() *shipping.Store { return s.readModels.Shipments() }

func (s *Server) polkurierOn() bool { return s.shipping != nil && s.shipping.Configured() }

// ---------------------------------------------------------------------------
// Checkout
// ---------------------------------------------------------------------------

type deliveryOptionView struct {
	shipping.Option
	Cost       domain.Money
	CostLabel  string // "Bezpłatnie" or the price
	TotalLabel string // what the whole order costs with this option
	Default    bool   // preselected: collection in person
}

type deliveryChoices struct {
	Options []deliveryOptionView
	Note    string // why delivery is not offered, when there is a reason
	Offered bool   // more than collection in person
}

// shopLineItems finds the catalog item behind every shop line of the cart.
func (s *Server) shopLineItems(r *http.Request, cart []CartItem) []domain.ShopItem {
	var items []domain.ShopItem
	for _, line := range cart {
		if !line.IsShop() {
			continue
		}
		if item, ok := s.shopItemByID(r, line.ProductID); ok {
			items = append(items, item)
		}
	}
	return items
}

// deliveryChoices lists how the cart can be delivered. Rentals are always
// collected in person, and so are items marked pickupOnly.
func (s *Server) deliveryChoices(r *http.Request, cart []CartItem) deliveryChoices {
	cfg := s.shippingConfig()
	goods := calculateShopTotal(cart)
	base := calculateFinalTotal(cart)

	options := cfg.Available(s.polkurierOn())
	var note string
	switch {
	case cartHasRentalLines(cart):
		note = "Zamówienie zawiera sprzęt do wypożyczenia, który odbierasz osobiście w Radzyminie. Produkty ze sklepu odbierzesz razem z nim."
		options = onlyPickup(options)
	default:
		for _, item := range s.shopLineItems(r, cart) {
			if item.Shipping.PickupOnly {
				note = "„" + item.Name + "” wysyłamy wyłącznie po wcześniejszym ustaleniu — w tym zamówieniu dostępny jest odbiór osobisty."
				options = onlyPickup(options)
				break
			}
		}
	}

	choices := deliveryChoices{Note: note, Offered: len(options) > 1}
	for _, o := range options {
		cost := o.Cost(goods)
		view := deliveryOptionView{Option: o, Cost: cost, CostLabel: "Bezpłatnie", TotalLabel: base.Add(cost).String()}
		if cost.IsPositive() {
			view.CostLabel = cost.String()
		}
		choices.Options = append(choices.Options, view)
	}
	for i := range choices.Options {
		if choices.Options[i].Kind == shipping.KindPickup {
			choices.Options[i].Default = true
			return choices
		}
	}
	if len(choices.Options) > 0 {
		choices.Options[0].Default = true
	}
	return choices
}

func onlyPickup(options []shipping.Option) []shipping.Option {
	return slices.DeleteFunc(slices.Clone(options), func(o shipping.Option) bool { return o.Kind != shipping.KindPickup })
}

// Notice codes of problems with the delivery part of the checkout form.
const (
	noticeDeliveryMethod  = "delivery_method"
	noticeDeliveryLocker  = "delivery_locker"
	noticeDeliveryUnknown = "delivery_locker_unknown"
	noticeDeliveryAddress = "delivery_address"
	noticeDeliveryPayment = "delivery_payment"
)

func deliveryNotice(code string) string {
	switch code {
	case noticeDeliveryMethod:
		return "Wybrany sposób dostawy nie jest już dostępny. Wybierz go ponownie."
	case noticeDeliveryLocker:
		return "Wybierz paczkomat lub punkt odbioru."
	case noticeDeliveryUnknown:
		return "Nie znaleźliśmy takiego paczkomatu lub punktu. Wyszukaj go po mieście, ulicy albo kodzie."
	case noticeDeliveryAddress:
		return "Uzupełnij adres dostawy: ulicę z numerem, kod pocztowy (00-000) i miasto."
	case noticeDeliveryPayment:
		return "Przesyłkę opłacisz BLIK-iem przed wysyłką. Płatność przy odbiorze dotyczy tylko odbioru osobistego."
	default:
		return ""
	}
}

// resolveDelivery turns the checkout form into the delivery stored with the
// order. It returns a notice code when the form cannot be accepted. A cart
// without shop goods has no delivery at all (rentals are collected).
func (s *Server) resolveDelivery(ctx context.Context, r *http.Request, cart []CartItem, paymentMethod, name, email, phone string) (*domain.Delivery, string) {
	if !cartHasShopLines(cart) {
		return nil, ""
	}
	method := strings.TrimSpace(r.FormValue("shipping_method"))
	if method == "" {
		method = shipping.PickupCode
	}
	var chosen *deliveryOptionView
	choices := s.deliveryChoices(r, cart)
	for i := range choices.Options {
		if choices.Options[i].Code == method {
			chosen = &choices.Options[i]
			break
		}
	}
	if chosen == nil {
		return nil, noticeDeliveryMethod
	}

	d := &domain.Delivery{
		Method: chosen.Code, Name: chosen.Name, Carrier: chosen.Carrier, Cost: chosen.Cost,
		Person: strings.TrimSpace(name), Phone: strings.TrimSpace(phone), Email: strings.TrimSpace(email),
	}
	if chosen.Kind == shipping.KindPickup {
		d.Cost = domain.Money{}
		return d, ""
	}
	if paymentMethod != domain.PaymentMethodBlik {
		return nil, noticeDeliveryPayment
	}

	switch chosen.Kind {
	case shipping.KindLocker:
		point := strings.ToUpper(strings.TrimSpace(r.FormValue("locker")))
		if point == "" {
			return nil, noticeDeliveryLocker
		}
		d.PointID, d.PointLabel = point, point
		lookup, cancel := context.WithTimeout(ctx, 6*time.Second)
		defer cancel()
		found, err := s.shipping.LookupPoint(lookup, chosen.Carrier, point)
		switch {
		case errors.Is(err, shipping.ErrPointNotFound):
			return nil, noticeDeliveryUnknown
		case err != nil:
			// An outage of the courier API never blocks a sale: the admin sees the code on the order.
			log.Printf("Could not verify %s locker %s: %v", chosen.Carrier, point, err)
		default:
			d.PointLabel = found.Label
			d.Street, d.Postcode, d.City = found.Street, found.Postcode, found.City
		}
	case shipping.KindCourier:
		d.Street = strings.TrimSpace(r.FormValue("ship_street"))
		d.Postcode = strings.TrimSpace(r.FormValue("ship_postcode"))
		d.City = strings.TrimSpace(r.FormValue("ship_city"))
		if d.Street == "" || d.City == "" || !shipping.ValidPostcode(d.Postcode) {
			return nil, noticeDeliveryAddress
		}
	}
	return d, ""
}

// handleShippingPoints powers the parcel locker search in the checkout.
func (s *Server) handleShippingPoints(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	empty := []shipping.Point{}

	option, ok := s.shippingConfig().Option(r.URL.Query().Get("method"))
	if !ok || !option.NeedsLocker() || !s.polkurierOn() {
		writeJSON(w, empty)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
	defer cancel()
	points, err := s.shipping.SearchPoints(ctx, option.Carrier, r.URL.Query().Get("q"))
	if err != nil {
		log.Printf("Locker search failed: %v", err)
		points = empty
	}
	if points == nil {
		points = empty
	}
	w.Header().Set("Cache-Control", "public, max-age=300")
	writeJSON(w, points)
}

// ---------------------------------------------------------------------------
// Packing
// ---------------------------------------------------------------------------

// goodsOf describes what the shop lines of an order weigh and measure.
func (s *Server) goodsOf(items []domain.OrderItem) []shipping.Good {
	catalog := map[string]domain.ShopItem{}
	for _, item := range s.shopCatalog() {
		catalog[item.ID] = item
	}
	var goods []shipping.Good
	for _, line := range items {
		if !line.IsShop() {
			continue
		}
		g := shipping.Good{Quantity: line.Quantity}
		if item, ok := catalog[line.ProductID]; ok {
			g.WeightG, g.LengthCm, g.WidthCm, g.HeightCm = item.Shipping.WeightG, item.Shipping.LengthCm, item.Shipping.WidthCm, item.Shipping.HeightCm
		}
		goods = append(goods, g)
	}
	return goods
}

// goodsValue is what the shop lines of an order are worth (for parcel insurance).
func goodsValue(items []domain.OrderItem) domain.Money {
	total := domain.Money{}
	for _, line := range items {
		if line.IsShop() {
			total = total.Add(line.Total())
		}
	}
	return total
}
