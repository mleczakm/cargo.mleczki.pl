package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"cargo.mleczki.pl/internal/domain"
	"cargo.mleczki.pl/internal/shipping"
)

const deliveryConfig = `
sender:
  person: Michał Mleczko
  street: Przykładowa
  houseNumber: "1"
  postcode: 05-250
  city: Radzymin
  email: sklep@example.com
  phone: "500100200"
options:
  - code: pickup
    name: Odbiór osobisty w Radzyminie
  - code: inpost
    name: Paczkomat InPost
    kind: locker
    carrier: INPOST_PACZKOMAT
    price: 14.99
    freeFrom: 200
  - code: kurier
    name: Kurier DPD
    kind: courier
    carrier: DPD
    price: 19.90
`

const heavyItem = `---
id: drabina
name: Drabina
price: 400
shipping: {pickupOnly: true}
stock: {initial: 2}
---
Za duża na paczkę.
`

// fakePolkurier records calls and answers like the real API.
type fakePolkurier struct {
	mu     sync.Mutex
	calls  []string
	status string
}

func (f *fakePolkurier) handler(t *testing.T) http.Handler {
	t.Helper()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		body := string(raw)
		f.mu.Lock()
		defer f.mu.Unlock()
		method := ""
		for _, m := range []string{"available_carriers", "order_valuation_v2", "get_courier_point", "create_order", "get_status", "cancel_order", "get_label", "get_courier_pickup_time"} {
			if strings.Contains(body, `"apimethod":"`+m+`"`) {
				method = m
			}
		}
		f.calls = append(f.calls, method)
		switch method {
		case "available_carriers":
			_, _ = io.WriteString(w, `{"status":"success","response":[{"servicecode":"INPOST_PACZKOMAT","name":"InPost Paczkomaty"},{"servicecode":"DPD","name":"DPD"}]}`)
		case "order_valuation_v2":
			_, _ = io.WriteString(w, `{"status":"success","response":[{"servicecode":"INPOST_PACZKOMAT","serviceName":"InPost","grossprice":11.07,"available":true},{"servicecode":"DPD","serviceName":"DPD","grossprice":18.45,"available":true}]}`)
		case "get_courier_point":
			if strings.Contains(body, `"id":"RDZ01M"`) {
				_, _ = io.WriteString(w, `{"status":"success","response":[{"id":"RDZ01M","city":"Radzymin","zip":"05-250","street":"Konstytucji 3 Maja 2","available":true}]}`)
				return
			}
			if strings.Contains(body, `"searchquery"`) {
				_, _ = io.WriteString(w, `{"status":"success","response":[{"id":"RDZ01M","city":"Radzymin","zip":"05-250","street":"Konstytucji 3 Maja 2","available":true}]}`)
				return
			}
			_, _ = io.WriteString(w, `{"status":"success","response":[]}`)
		case "create_order":
			_, _ = io.WriteString(w, `{"status":"success","response":{"order_number":"PK-1","label":["WB1"],"price_gross":"11.07","url_tracktrace":"https://track/1","is_paid":true}}`)
		case "get_status":
			status := f.status
			if status == "" {
				status = "P"
			}
			_, _ = io.WriteString(w, `{"status":"success","response":{"status":"Status `+status+`","status_code":"`+status+`","url":"https://track/1"}}`)
		case "cancel_order":
			_, _ = io.WriteString(w, `{"status":"success","response":{"cancellation":true}}`)
		case "get_label":
			_, _ = io.WriteString(w, `{"status":"success","response":{"file":"JVBERi0xLjQK"}}`)
		case "get_courier_pickup_time":
			_, _ = io.WriteString(w, `{"status":"success","response":[]}`)
		default:
			t.Errorf("unexpected Polkurier request: %s", body)
			_, _ = io.WriteString(w, `{"status":"error","response":"unexpected"}`)
		}
	})
}

func (f *fakePolkurier) called(method string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	count := 0
	for _, c := range f.calls {
		if c == method {
			count++
		}
	}
	return count
}

// withDelivery gives the harness a shipping config and (optionally) a Polkurier account.
func withDelivery(t *testing.T, app *shopHarness, polkurier bool) *fakePolkurier {
	t.Helper()
	path := filepath.Join(app.dir, "shipping.yml")
	if err := os.WriteFile(path, []byte(deliveryConfig), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(app.dir, "shop", "drabina.md"), []byte(heavyItem), 0o600); err != nil {
		t.Fatal(err)
	}
	items, _ := app.server.shopParser.LoadAll()
	if err := app.rm.Stock().SyncCatalog(t.Context(), items); err != nil {
		t.Fatal(err)
	}
	fake := &fakePolkurier{}
	api := httptest.NewServer(fake.handler(t))
	t.Cleanup(api.Close)

	app.server.shipping = &shipping.Service{Config: &shipping.ConfigFile{Path: path}, Store: app.rm.Shipments()}
	if polkurier {
		app.server.shipping.Configure("acct", "token", true, api.URL, "test")
	}
	return fake
}

func (app *shopHarness) checkoutWith(client *http.Client, form url.Values) (int, string, string) {
	app.t.Helper()
	base := url.Values{
		"payment_method": {"blik"}, "name": {"Jan Test"}, "email": {"jan@example.com"}, "phone": {"500100200"},
		"address": {"Radzymin"}, "is_adult": {"on"}, "accept_tos": {"on"}, "password": {"haslo12345"},
	}
	for k, v := range form {
		base[k] = v
	}
	status, header := app.post(client, "/checkout/submit", base, true)
	location := header.Get("Location")
	if location == "" {
		location = header.Get("HX-Redirect")
	}
	orderID := ""
	if rest, ok := strings.CutPrefix(location, "/payment/"); ok {
		orderID, _, _ = strings.Cut(rest, "?")
	}
	return status, location, orderID
}

func TestWithoutPolkurierOnlyPickupIsOffered(t *testing.T) {
	app := newShopHarness(t)
	withDelivery(t, app, false)
	shopper := app.visitor()
	app.addToCart(shopper, "dzwonek", "", "1")

	_, body, _ := app.get(shopper, "/checkout")
	assertContains(t, body, "Dostawa", "Odbiór osobisty w Radzyminie")
	assertNotContains(t, body, "Paczkomat InPost", "Kurier DPD")

	_, location, orderID := app.checkoutWith(shopper, url.Values{"shipping_method": {"inpost"}, "locker": {"RDZ01M"}})
	if orderID != "" || !strings.Contains(location, "notice=delivery_method") {
		t.Errorf("a delivery that is not offered must be refused: %q", location)
	}
}

func TestCheckoutOffersDeliveryAndChargesForIt(t *testing.T) {
	app := newShopHarness(t)
	fake := withDelivery(t, app, true)
	shopper := app.visitor()
	app.addToCart(shopper, "dzwonek", "", "2") // 2 × 25 zł

	_, body, _ := app.get(shopper, "/checkout")
	assertContains(t, body, "Paczkomat InPost", "14,99 zł", "Kurier DPD", "19,90 zł", "Bezpłatnie od 200 zł", `id="final-total"`)

	status, location, orderID := app.checkoutWith(shopper, url.Values{"shipping_method": {"inpost"}, "locker": {"rdz01m"}})
	if status >= 400 || orderID == "" {
		t.Fatalf("checkout: %d %q", status, location)
	}
	if got := fake.called("get_courier_point"); got != 1 {
		t.Errorf("the locker should be verified once, got %d lookups", got)
	}

	_, _, total := app.orderRow(orderID)
	if total != 64.99 { // 50 + 14.99
		t.Errorf("total = %v, want 64.99", total)
	}
	order, err := app.rm.GetOrderDetail(orderID)
	if err != nil {
		t.Fatal(err)
	}
	d := order.Delivery
	if d == nil || !d.IsShipped() || d.PointID != "RDZ01M" || d.Carrier != "INPOST_PACZKOMAT" || d.Cost != domain.PLN(1499) || d.Postcode != "05-250" {
		t.Fatalf("delivery = %+v", d)
	}
	if !strings.Contains(d.Where(), "RDZ01M") {
		t.Errorf("Where() = %q", d.Where())
	}
}

func TestDeliveryIsFreeAboveTheThreshold(t *testing.T) {
	app := newShopHarness(t)
	withDelivery(t, app, true)
	shopper := app.visitor()
	app.addToCart(shopper, "na-zamowienie", "", "1") // 300 zł ≥ 200 zł

	_, _, orderID := app.checkoutWith(shopper, url.Values{"shipping_method": {"inpost"}, "locker": {"RDZ01M"}})
	if _, _, total := app.orderRow(orderID); total != 300 {
		t.Errorf("total = %v, want 300 (free delivery)", total)
	}
}

func TestDeliveryRulesAreEnforcedOnTheServer(t *testing.T) {
	app := newShopHarness(t)
	withDelivery(t, app, true)

	cases := []struct {
		name   string
		form   url.Values
		notice string
	}{
		{"cash on a parcel", url.Values{"payment_method": {"cash_pickup"}, "shipping_method": {"inpost"}, "locker": {"RDZ01M"}}, "delivery_payment"},
		{"no locker", url.Values{"shipping_method": {"inpost"}}, "delivery_locker"},
		{"unknown locker", url.Values{"shipping_method": {"inpost"}, "locker": {"NIEMA1"}}, "delivery_locker_unknown"},
		{"courier without address", url.Values{"shipping_method": {"kurier"}}, "delivery_address"},
		{"courier with bad postcode", url.Values{"shipping_method": {"kurier"}, "ship_street": {"Długa 1"}, "ship_postcode": {"0525"}, "ship_city": {"Radzymin"}}, "delivery_address"},
		{"made-up option", url.Values{"shipping_method": {"dron"}}, "delivery_method"},
	}
	for _, tc := range cases {
		shopper := app.visitor()
		app.addToCart(shopper, "dzwonek", "", "1")
		_, location, orderID := app.checkoutWith(shopper, tc.form)
		if orderID != "" || !strings.Contains(location, "notice="+tc.notice) {
			t.Errorf("%s: got %q, want notice %s", tc.name, location, tc.notice)
		}
		if l := app.level("dzwonek"); l.Reserved != 0 {
			t.Errorf("%s: a refused checkout must not hold stock, reserved = %d", tc.name, l.Reserved)
		}
	}

	shopper := app.visitor()
	app.addToCart(shopper, "dzwonek", "", "1")
	_, location, orderID := app.checkoutWith(shopper, url.Values{"shipping_method": {"kurier"}, "ship_street": {"Długa 1/2"}, "ship_postcode": {"05-250"}, "ship_city": {"Radzymin"}})
	if orderID == "" {
		t.Fatalf("a valid courier order was refused: %q", location)
	}
	order, _ := app.rm.GetOrderDetail(orderID)
	if order.Delivery.Where() != "Długa 1/2, 05-250 Radzymin" || order.Delivery.Cost != domain.PLN(1990) {
		t.Errorf("delivery = %+v (%s)", order.Delivery, order.Delivery.Where())
	}
}

func TestRentalsAndHeavyGoodsAreCollectedInPerson(t *testing.T) {
	app := newShopHarness(t)
	withDelivery(t, app, true)

	renter := app.visitor()
	app.addToCart(renter, "dzwonek", "", "1")
	app.post(renter, "/cart/add", url.Values{"product_id": {"tandem"}, "start_date": {"2030-06-01"}, "end_date": {"2030-06-02"}}, false)
	_, body, _ := app.get(renter, "/checkout")
	assertContains(t, body, "odbierasz osobiście")
	assertNotContains(t, body, "Paczkomat InPost")

	heavy := app.visitor()
	app.addToCart(heavy, "drabina", "", "1")
	_, body, _ = app.get(heavy, "/checkout")
	assertContains(t, body, "Drabina", "odbiór osobisty")
	assertNotContains(t, body, "Kurier DPD")
	_, location, orderID := app.checkoutWith(heavy, url.Values{"shipping_method": {"kurier"}, "ship_street": {"Długa 1"}, "ship_postcode": {"05-250"}, "ship_city": {"Radzymin"}})
	if orderID != "" || !strings.Contains(location, "delivery_method") {
		t.Errorf("a pickup-only item cannot be posted: %q", location)
	}
}

func TestLockerSearchEndpoint(t *testing.T) {
	app := newShopHarness(t)
	fake := withDelivery(t, app, true)
	value := app.visitor()

	_, body, header := app.get(value, "/shipping/points?method=inpost&q=Radzymin")
	assertContains(t, body, "RDZ01M")
	if !strings.Contains(header.Get("Content-Type"), "json") {
		t.Errorf("content type = %q", header.Get("Content-Type"))
	}
	for _, path := range []string{"/shipping/points?method=pickup&q=Radzymin", "/shipping/points?method=dron&q=Radzymin", "/shipping/points?method=kurier&q=Radzymin", "/shipping/points?method=inpost&q=R"} {
		if _, body, _ := app.get(value, path); strings.TrimSpace(body) != "[]" {
			t.Errorf("%s = %q, want an empty list", path, body)
		}
	}
	if fake.called("get_courier_point") != 1 {
		t.Errorf("only the valid search should reach Polkurier, got %d calls", fake.called("get_courier_point"))
	}

	off := newShopHarness(t)
	withDelivery(t, off, false)
	if _, body, _ := off.get(off.visitor(), "/shipping/points?method=inpost&q=Radzymin"); strings.TrimSpace(body) != "[]" {
		t.Errorf("without Polkurier the search must be empty, got %q", body)
	}
}

func TestAdminShipsAnOrderThroughPolkurier(t *testing.T) {
	app := newShopHarness(t)
	fake := withDelivery(t, app, true)
	admin := app.asAdmin()

	shopper := app.visitor()
	app.addToCart(shopper, "dzwonek", "", "2")
	_, _, orderID := app.checkoutWith(shopper, url.Values{"shipping_method": {"inpost"}, "locker": {"RDZ01M"}})

	// Unpaid: the order page is there, but nothing can be shipped yet.
	_, body, _ := app.get(admin, "/admin/order/"+orderID)
	assertContains(t, body, "Paczkomat InPost", "RDZ01M")
	assertNotContains(t, body, "Nadaj przesyłkę")
	if status, _ := app.post(admin, "/admin/order/"+orderID+"/shipment", url.Values{"carrier": {"INPOST_PACZKOMAT"}}, false); status != http.StatusSeeOther {
		t.Errorf("shipping an unpaid order should redirect with an error, got %d", status)
	}
	if fake.called("create_order") != 0 {
		t.Fatal("an unpaid order must not be shipped")
	}

	app.emit(orderID, &domain.OrderPaidEvent{OrderID: orderID, Method: "blik", Timestamp: time.Now().UTC()})
	_, body, _ = app.get(admin, "/admin")
	assertContains(t, body, "Do wysłania", orderID)

	_, body, _ = app.get(admin, "/admin/order/"+orderID)
	assertContains(t, body, "Nadaj przesyłkę", `name="carrier"`, "RDZ01M", "11,07 zł", "Wycena u przewoźników")

	form := url.Values{
		"carrier": {"INPOST_PACZKOMAT"}, "length": {"30"}, "width": {"20"}, "height": {"10"}, "weight": {"1,3"}, "description": {"Akcesoria rowerowe"},
		"drop_off": {"1"}, "mark_shipped": {"1"},
		"r_person": {"Jan Test"}, "r_email": {"jan@example.com"}, "r_phone": {"500100200"}, "r_point": {"RDZ01M"},
		"r_street": {"Konstytucji 3 Maja"}, "r_house": {"2"}, "r_postcode": {"05-250"}, "r_city": {"Radzymin"},
	}
	status, header := app.post(admin, "/admin/order/"+orderID+"/shipment", form, false)
	if status != http.StatusSeeOther || !strings.Contains(header.Get("Location"), "notice=") {
		t.Fatalf("create shipment: %d %q", status, header.Get("Location"))
	}
	if fake.called("create_order") != 1 {
		t.Errorf("create_order calls = %d", fake.called("create_order"))
	}
	if s, _, _ := app.orderRow(orderID); s != "realized" {
		t.Errorf("order status = %s, want realized (sent)", s)
	}
	if l := app.level("dzwonek"); l.OnHand != 1 || l.Reserved != 0 {
		t.Errorf("stock after shipping = %+v, want 1 on hand", l)
	}

	list, _ := app.rm.Shipments().ForOrder(t.Context(), orderID)
	if len(list) != 1 || list[0].Waybill != "WB1" || !list[0].Active() {
		t.Fatalf("shipments = %+v", list)
	}
	_, body, _ = app.get(admin, "/admin/order/"+orderID)
	assertContains(t, body, "PK-1", "WB1", "Etykieta (PDF)")
	assertNotContains(t, body, "Nadaj przesyłkę")

	// The label is a PDF.
	resp, err := admin.Get(app.http.URL + "/admin/shipment/" + "1" + "/label")
	if err != nil {
		t.Fatal(err)
	}
	pdf, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 || resp.Header.Get("Content-Type") != "application/pdf" || !strings.HasPrefix(string(pdf), "%PDF") {
		t.Errorf("label: %d %q %q", resp.StatusCode, resp.Header.Get("Content-Type"), pdf)
	}

	// The customer sees the delivery and the tracking link.
	customer := app.visitor()
	app.loginAs(customer, "jan@example.com", "haslo12345")
	_, body, _ = app.get(customer, "/user/order/"+orderID)
	assertContains(t, body, "Dostawa", "Paczkomat InPost", "Śledź przesyłkę", "WB1")

	// Cancelling frees the order for a new parcel.
	app.post(admin, "/admin/shipment/"+"1"+"/cancel", nil, false)
	if got, _ := app.rm.Shipments().ByID(t.Context(), 1); got.Active() {
		t.Error("the parcel should be cancelled")
	}
}

func (app *shopHarness) loginAs(client *http.Client, email, password string) {
	app.t.Helper()
	token, _, err := app.server.authManager.Login(app.t.Context(), email, password)
	if err != nil {
		app.t.Fatalf("login %s: %v", email, err)
	}
	client.Jar.SetCookies(app.base, []*http.Cookie{{Name: "session_token", Value: token, Path: "/"}})
}

func TestShipmentSyncMovesTheOrderAlong(t *testing.T) {
	app := newShopHarness(t)
	fake := withDelivery(t, app, true)
	admin := app.asAdmin()

	shopper := app.visitor()
	app.addToCart(shopper, "dzwonek", "", "1")
	_, _, orderID := app.checkoutWith(shopper, url.Values{"shipping_method": {"kurier"}, "ship_street": {"Długa 1"}, "ship_postcode": {"05-250"}, "ship_city": {"Radzymin"}})
	app.emit(orderID, &domain.OrderPaidEvent{OrderID: orderID, Method: "blik", Timestamp: time.Now().UTC()})

	form := url.Values{
		"carrier": {"DPD"}, "length": {"30"}, "width": {"20"}, "height": {"10"}, "weight": {"1"},
		"r_person": {"Jan Test"}, "r_email": {"jan@example.com"}, "r_phone": {"500100200"},
		"r_street": {"Długa"}, "r_house": {"1"}, "r_postcode": {"05-250"}, "r_city": {"Radzymin"},
	}
	app.post(admin, "/admin/order/"+orderID+"/shipment", form, false) // no mark_shipped
	if s, _, _ := app.orderRow(orderID); s != "confirmed" {
		t.Fatalf("status = %s, want confirmed until the courier takes the parcel", s)
	}

	app.server.shipping.SyncOnce(t.Context(), app.server.onShipmentChanged)
	if s, _, _ := app.orderRow(orderID); s != "confirmed" {
		t.Errorf("a parcel waiting for the courier must not move the order, status = %s", s)
	}

	fake.mu.Lock()
	fake.status = "WP"
	fake.mu.Unlock()
	app.server.shipping.SyncOnce(t.Context(), app.server.onShipmentChanged)
	if s, _, _ := app.orderRow(orderID); s != "realized" {
		t.Errorf("a parcel on its way means the order is sent, status = %s", s)
	}
	if l := app.level("dzwonek"); l.OnHand != 2 || l.Reserved != 0 {
		t.Errorf("stock = %+v, want 2 on hand (3 − 1 sent)", l)
	}
}

func TestBrokenShippingConfigFallsBackToPickup(t *testing.T) {
	app := newShopHarness(t)
	withDelivery(t, app, true)
	if err := os.WriteFile(filepath.Join(app.dir, "shipping.yml"), []byte("options: ["), 0o600); err != nil {
		t.Fatal(err)
	}
	future := time.Now().Add(time.Hour)
	_ = os.Chtimes(filepath.Join(app.dir, "shipping.yml"), future, future)

	shopper := app.visitor()
	app.addToCart(shopper, "dzwonek", "", "1")
	status, body, _ := app.get(shopper, "/checkout")
	if status != 200 {
		t.Fatalf("checkout with a broken config: %d", status)
	}
	assertContains(t, body, "Odbiór osobisty")
}
