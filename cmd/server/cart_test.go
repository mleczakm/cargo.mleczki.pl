package main

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"cargo.mleczki.pl/internal/domain"
)

func (app *shopHarness) addRental(client *http.Client, form url.Values) (int, string) {
	app.t.Helper()
	form.Set("product_id", form.Get("product_id"))
	status, header := app.post(client, "/cart/add", form, false)
	return status, header.Get("Location")
}

func (app *shopHarness) forgeCart(client *http.Client, lines []CartItem) {
	app.t.Helper()
	data, err := json.Marshal(lines)
	if err != nil {
		app.t.Fatal(err)
	}
	client.Jar.SetCookies(app.base, []*http.Cookie{{Name: "cart", Value: url.QueryEscape(string(data)), Path: "/"}})
}

func (app *shopHarness) bookings(orderID string) map[string][]string {
	app.t.Helper()
	rows, err := app.rm.GetDB().Query(`SELECT product_id, booked_date FROM product_bookings WHERE order_id = ? ORDER BY booked_date`, orderID)
	if err != nil {
		app.t.Fatal(err)
	}
	defer rows.Close()
	booked := map[string][]string{}
	for rows.Next() {
		var product, date string
		if err := rows.Scan(&product, &date); err != nil {
			app.t.Fatal(err)
		}
		booked[product] = append(booked[product], date)
	}
	if err := rows.Err(); err != nil {
		app.t.Fatal(err)
	}
	return booked
}

func (app *shopHarness) orderItems(orderID string) []domain.OrderItem {
	app.t.Helper()
	_, itemsJSON, _ := app.orderRow(orderID)
	var items []domain.OrderItem
	if err := json.Unmarshal([]byte(itemsJSON), &items); err != nil {
		app.t.Fatal(err)
	}
	return items
}

func TestRentalIsPricedFromTheCatalogWithAddonsAndGrosze(t *testing.T) {
	app := newShopHarness(t)
	shopper := app.visitor()

	// The visitor may claim anything about days and prices; only dates and add-ons count.
	status, location := app.addRental(shopper, url.Values{
		"product_id": {"cargo"}, "start_date": {"2099-06-10"}, "end_date": {"2099-06-11"},
		"rental_days": {"99"}, "addons": {"daszek", "kaucja", "daszek", "wymyslony"},
	})
	if status != http.StatusBadRequest {
		t.Fatalf("an unknown add-on must be refused, got %d %s", status, location)
	}
	status, location = app.addRental(shopper, url.Values{
		"product_id": {"cargo"}, "start_date": {"2099-06-10"}, "end_date": {"2099-06-11"},
		"rental_days": {"99"}, "addons": {"daszek", "kaucja", "daszek"},
	})
	if status != http.StatusFound || location != "/checkout" {
		t.Fatalf("add: %d %s", status, location)
	}

	line := getCartFromJar(t, app, shopper)[0]
	// (150 + 15.50) × 2 days + 50 one-time deposit
	if line.RentalDays != 2 || line.Total != domain.Zloty(381) || len(line.Addons) != 2 {
		t.Errorf("line = %+v", line)
	}

	_, page, _ := app.get(shopper, "/checkout")
	assertContains(t, page, "381 zł", "Opcje dodatkowe", "Daszek (15,50 zł / dobę)", "Kaucja (50 zł jednorazowo)", "<span>381 zł</span>")

	_, _, orderID := app.checkout(shopper, "cash_pickup")
	if orderID == "" {
		t.Fatal("checkout failed")
	}
	if _, _, total := app.orderRow(orderID); total != 381 {
		t.Errorf("the add-ons must be charged: total = %v, want 381", total)
	}
	items := app.orderItems(orderID)
	if len(items) != 1 || items[0].Total() != domain.Zloty(381) || items[0].StartDate != "2099-06-10" || len(items[0].SelectedAddons) != 2 {
		t.Errorf("items = %+v", items)
	}
	if items[0].SelectedAddons[0].Price != domain.PLN(1550) {
		t.Errorf("add-on price = %s", items[0].SelectedAddons[0].Price)
	}
}

func TestForgedRentalCookieIsRepriced(t *testing.T) {
	app := newShopHarness(t)
	shopper := app.visitor()
	app.forgeCart(shopper, []CartItem{{
		CartID: "1", ProductID: "cargo", ProductName: "Gratis", BasePrice: domain.Zloty(1),
		StartDate: "2099-07-01", EndDate: "2099-07-05", RentalDays: 1, Total: domain.Zloty(1),
	}})

	_, page, _ := app.get(shopper, "/checkout")
	assertContains(t, page, "Rower cargo", "750 zł", "5 dni")
	assertNotContains(t, page, "Gratis")

	_, _, orderID := app.checkout(shopper, "cash_pickup")
	if _, _, total := app.orderRow(orderID); total != 750 {
		t.Errorf("5 days at 150 zł, got %v", total)
	}
	if items := app.orderItems(orderID); items[0].ProductName != "Rower cargo" || items[0].RentalDays != 5 {
		t.Errorf("items = %+v", items)
	}
	if got := len(app.bookings(orderID)["cargo"]); got != 5 {
		t.Errorf("booked %d days, want 5", got)
	}
}

func TestEachRentalLineKeepsItsOwnDates(t *testing.T) {
	app := newShopHarness(t)
	shopper := app.visitor()
	for _, form := range []url.Values{
		{"product_id": {"cargo"}, "start_date": {"2099-08-01"}, "end_date": {"2099-08-02"}},
		{"product_id": {"tandem"}, "start_date": {"2099-09-10"}, "end_date": {"2099-09-12"}},
		{"product_id": {"cargo"}, "start_date": {"2099-10-01"}, "end_date": {""}}, // a single day: end defaults to start
	} {
		if status, location := app.addRental(shopper, form); status != http.StatusFound {
			t.Fatalf("add %v: %d %s", form, status, location)
		}
	}

	_, _, orderID := app.checkout(shopper, "cash_pickup")
	if orderID == "" {
		t.Fatal("checkout failed")
	}

	got := app.bookings(orderID)
	want := map[string][]string{
		"cargo":  {"2099-08-01", "2099-08-02", "2099-10-01"},
		"tandem": {"2099-09-10", "2099-09-11", "2099-09-12"},
	}
	for product, dates := range want {
		if strings.Join(got[product], ",") != strings.Join(dates, ",") {
			t.Errorf("%s booked %v, want %v", product, got[product], dates)
		}
	}

	// 2 + 1 days of cargo at 150, 3 days of tandem at 80.50.
	if _, _, total := app.orderRow(orderID); total != 450+241.5 {
		t.Errorf("total = %v", total)
	}
	var start, end string
	if err := app.rm.GetDB().QueryRow(`SELECT start_date, end_date FROM orders WHERE id = ?`, orderID).Scan(&start, &end); err != nil {
		t.Fatal(err)
	}
	if start != "2099-08-01" || end != "2099-10-01" {
		t.Errorf("order span = %s..%s", start, end)
	}
}

func TestInvalidRentalsAreRefusedWhenAdded(t *testing.T) {
	app := newShopHarness(t)
	cases := map[string]url.Values{
		"in the past":        {"product_id": {"cargo"}, "start_date": {"2000-01-01"}, "end_date": {"2000-01-02"}},
		"return before pick": {"product_id": {"cargo"}, "start_date": {"2099-01-05"}, "end_date": {"2099-01-01"}},
		"no dates":           {"product_id": {"cargo"}},
		"garbage dates":      {"product_id": {"cargo"}, "start_date": {"jutro"}, "end_date": {"pojutrze"}},
		"too long":           {"product_id": {"cargo"}, "start_date": {"2099-01-01"}, "end_date": {"2099-06-01"}},
		"unknown add-on":     {"product_id": {"cargo"}, "start_date": {"2099-01-01"}, "end_date": {"2099-01-02"}, "addons": {"nie-ma"}},
	}
	for name, form := range cases {
		t.Run(name, func(t *testing.T) {
			shopper := app.visitor()
			if status, _ := app.addRental(shopper, form); status != http.StatusBadRequest {
				t.Errorf("status %d, want 400", status)
			}
			if cart := getCartFromJar(t, app, shopper); len(cart) != 0 {
				t.Errorf("nothing may land in the cart: %+v", cart)
			}
		})
	}
	if status, _ := app.addRental(app.visitor(), url.Values{"product_id": {"nie-ma"}, "start_date": {"2099-01-01"}}); status != http.StatusNotFound {
		t.Errorf("unknown product: %d", status)
	}
}

func TestCartLinesThatClashOrGoStaleAreFlagged(t *testing.T) {
	app := newShopHarness(t)
	shopper := app.visitor()
	app.addRental(shopper, url.Values{"product_id": {"cargo"}, "start_date": {"2099-06-10"}, "end_date": {"2099-06-12"}})
	app.addRental(shopper, url.Values{"product_id": {"cargo"}, "start_date": {"2099-06-12"}, "end_date": {"2099-06-13"}})
	app.addRental(shopper, url.Values{"product_id": {"cargo"}, "start_date": {"2099-07-01"}, "end_date": {"2099-07-02"}})

	_, page, _ := app.get(shopper, "/checkout")
	assertContains(t, page, "nakładającym się terminie")
	if strings.Count(page, "nakładającym się terminie") != 1 {
		t.Error("only the second, clashing line may be flagged")
	}
	status, location, orderID := app.checkout(shopper, "cash_pickup")
	if status != http.StatusConflict || location != "/checkout?notice=cart" || orderID != "" {
		t.Errorf("a flagged cart must not be ordered: %d %q %q", status, location, orderID)
	}

	// Someone else books the free line's days in the meantime.
	other := app.visitor()
	app.forgeCart(other, []CartItem{{CartID: "2", ProductID: "cargo", StartDate: "2099-07-01", EndDate: "2099-07-02"}})
	if _, err := app.rm.GetDB().Exec(`INSERT INTO product_bookings (product_id, order_id, booked_date) VALUES ('cargo', 'ADMIN-BLOCK-cargo', '2099-07-02')`); err != nil {
		t.Fatal(err)
	}
	_, page, _ = app.get(other, "/checkout")
	assertContains(t, page, "Wybrane terminy nie są już dostępne")
}

func TestShopPricesWithGroszeAreSummedExactly(t *testing.T) {
	app := newShopHarness(t)
	shopper := app.visitor()
	if status, _ := app.addToCart(shopper, "smar", "", "3"); status != http.StatusSeeOther {
		t.Fatalf("add: %d", status)
	}

	_, shopPage, _ := app.get(shopper, "/shop/smar")
	assertContains(t, shopPage, "12,34 zł")
	_, page, _ := app.get(shopper, "/checkout")
	assertContains(t, page, "12,34 zł / szt.", "37,02 zł", "<span>37,02 zł</span>")

	_, _, orderID := app.checkout(shopper, "blik")
	if _, _, total := app.orderRow(orderID); total != 37.02 {
		t.Errorf("3 × 12.34 = 37.02, got %v", total)
	}
	_, payment, _ := app.get(shopper, "/payment/"+orderID+"?method=blik")
	assertContains(t, payment, "37,02 zł")

	events, err := app.es.GetEvents(t.Context(), orderID)
	if err != nil || len(events) == 0 {
		t.Fatalf("events: %v %v", events, err)
	}
	if !strings.Contains(string(events[0].Payload), `"totalAmount":"37.02"`) || !strings.Contains(string(events[0].Payload), `"basePrice":"12.34"`) {
		t.Errorf("amounts must be stored as exact decimal strings: %s", events[0].Payload)
	}
}

func TestManualReservationAcceptsDecimalPricesAndRejectsBadOnes(t *testing.T) {
	app := newShopHarness(t)
	admin := app.asAdmin()
	if _, err := app.rm.GetDB().Exec(`INSERT INTO users (id, email, name) VALUES ('u1', 'jan@example.com', 'Jan')`); err != nil {
		t.Fatal(err)
	}

	form := func(price string) url.Values {
		return url.Values{"user_id": {"u1"}, "product_id": {"cargo"}, "start_date": {"2099-03-01"}, "end_date": {"2099-03-02"}, "price": {price}}
	}
	for _, bad := range []string{"abc", "-5", "1.999"} {
		if status, _ := app.post(admin, "/admin/reservation/create", form(bad), false); status != http.StatusBadRequest {
			t.Errorf("price %q: status %d, want 400", bad, status)
		}
	}
	if status, _ := app.post(admin, "/admin/reservation/create", form("99,50"), false); status != http.StatusOK {
		t.Fatalf("a valid manual reservation: %d", status)
	}
	var total float64
	if err := app.rm.GetDB().QueryRow(`SELECT total_amount FROM orders WHERE id LIKE 'MANUAL-%'`).Scan(&total); err != nil || total != 99.5 {
		t.Errorf("total = %v, %v", total, err)
	}
}
