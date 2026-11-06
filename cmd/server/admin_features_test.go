package main

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func TestStoreClosuresBlockRentalsAndCanBeReopened(t *testing.T) {
	app := newShopHarness(t)
	admin := app.asAdmin()
	rental := func(day string) url.Values {
		return url.Values{"product_id": {"cargo"}, "start_date": {day}, "end_date": {day}}
	}

	// Close a three-day range; the panel lists the days with their weekdays.
	status, header := app.post(admin, "/admin/global-closure/add", url.Values{"date": {"2099-07-01"}, "end_date": {"2099-07-03"}}, false)
	if status != http.StatusSeeOther || !strings.Contains(header.Get("Location"), "notice=") {
		t.Fatalf("closing failed: %d %s", status, header.Get("Location"))
	}
	_, panel, _ := app.get(admin, "/admin")
	assertContains(t, panel, "2099-07-01", "2099-07-02", "2099-07-03", "Nadchodzące zamknięcia (3)", "Otwórz ponownie")

	// Closing the same days again is harmless.
	if _, header := app.post(admin, "/admin/global-closure/add", url.Values{"date": {"2099-07-02"}}, false); strings.Contains(header.Get("Location"), "error=") {
		t.Fatalf("closing an already closed day failed: %s", header.Get("Location"))
	}

	// Customers cannot rent on closed days, but can on open ones.
	shopper := app.visitor()
	app.addRental(shopper, rental("2099-07-02"))
	if status, _, id := app.checkout(shopper, "bank_transfer"); id != "" {
		t.Fatalf("a rental on a closed day was ordered: %d %q", status, id)
	}
	other := app.visitor()
	if status, location := app.addRental(other, rental("2099-07-10")); status != http.StatusFound {
		t.Fatalf("an open day was refused: %d %s", status, location)
	}

	// Reopen one day.
	app.post(admin, "/admin/global-closure/remove", url.Values{"date": {"2099-07-02"}}, false)
	_, panel, _ = app.get(admin, "/admin")
	assertContains(t, panel, "Nadchodzące zamknięcia (2)")

	// Bad input is refused with a message, never a server error.
	for name, form := range map[string]url.Values{
		"missing date": {},
		"garbage":      {"date": {"jutro"}},
		"reversed":     {"date": {"2099-07-05"}, "end_date": {"2099-07-01"}},
		"too long":     {"date": {"2099-01-01"}, "end_date": {"2101-01-01"}},
		"in the past":  {"date": {"2001-01-01"}},
	} {
		status, header := app.post(admin, "/admin/global-closure/add", form, false)
		if status != http.StatusSeeOther || !strings.Contains(header.Get("Location"), "error=") {
			t.Errorf("%s: %d %s", name, status, header.Get("Location"))
		}
	}
	if status, _ := app.post(app.visitor(), "/admin/global-closure/add", url.Values{"date": {"2099-08-01"}}, false); status != http.StatusFound {
		t.Errorf("visitors must not close the store, got %d", status)
	}
}

func TestAdminCanBlockAndUnblockEquipmentByHand(t *testing.T) {
	app := newShopHarness(t)
	admin := app.asAdmin()
	_, header := app.post(admin, "/admin/product/cargo/block-date", url.Values{"date": {"2099-09-01"}, "end_date": {"2099-09-02"}}, false)
	if !strings.Contains(header.Get("Location"), "notice=") {
		t.Fatalf("blocking failed: %s", header.Get("Location"))
	}
	_, panel, _ := app.get(admin, "/admin")
	assertContains(t, panel, "2099-09-01", "2099-09-02", "Zablokowane ręcznie (2)")

	// Blocked days are not rentable; days that are already taken are reported.
	shopper := app.visitor()
	app.addRental(shopper, url.Values{"product_id": {"cargo"}, "start_date": {"2099-09-01"}, "end_date": {"2099-09-01"}})
	if _, _, id := app.checkout(shopper, "bank_transfer"); id != "" {
		t.Fatalf("a manually blocked day was ordered: %q", id)
	}
	_, header = app.post(admin, "/admin/product/cargo/block-date", url.Values{"date": {"2099-09-01"}}, false)
	if !strings.Contains(header.Get("Location"), "error=") {
		t.Errorf("blocking a taken day should be reported: %s", header.Get("Location"))
	}

	_, header = app.post(admin, "/admin/product/cargo/unblock-date", url.Values{"date": {"2099-09-01"}, "end_date": {"2099-09-02"}}, false)
	if !strings.Contains(header.Get("Location"), "notice=") {
		t.Fatalf("unblocking failed: %s", header.Get("Location"))
	}
	_, panel, _ = app.get(admin, "/admin")
	assertContains(t, panel, "Zablokowane ręcznie (0)")

	for name, path := range map[string]string{"unknown product": "/admin/product/nope/block-date"} {
		if _, header := app.post(admin, path, url.Values{"date": {"2099-09-05"}}, false); !strings.Contains(header.Get("Location"), "error=") {
			t.Errorf("%s: %s", name, header.Get("Location"))
		}
	}
	if _, header := app.post(admin, "/admin/product/cargo/block-date", url.Values{"date": {"2001-01-01"}}, false); !strings.Contains(header.Get("Location"), "error=") {
		t.Errorf("blocking the past: %s", header.Get("Location"))
	}
}

func TestAdminPanelActionsComeBackToTheRightTabAndCustomerPageIsReal(t *testing.T) {
	app := newShopHarness(t)
	admin := app.asAdmin()
	shopper := app.visitor()
	app.addToCart(shopper, "dzwonek", "", "1")
	_, _, id := app.checkout(shopper, "bank_transfer")
	if id == "" {
		t.Fatal("checkout failed")
	}

	// Orders show Polish labels and a plain-form action, not raw codes or HTMX swaps.
	_, panel, _ := app.get(admin, "/admin")
	assertContains(t, panel, id, "Oczekuje na płatność", "Przelew tradycyjny", `action="/admin/order/`+id+`/mark-paid"`)
	assertNotContains(t, panel, "hx-post", "OrdersRequiringConfirmation")

	status, header := app.post(admin, "/admin/order/"+id+"/mark-paid", nil, false)
	if status != http.StatusSeeOther || !strings.HasSuffix(header.Get("Location"), "#orders") {
		t.Fatalf("mark-paid: %d %s", status, header.Get("Location"))
	}

	// The customer page shows the real customer and keeps admin rights on save.
	var userID, name string
	if err := app.rm.GetDB().QueryRow(`SELECT id, name FROM users WHERE email = 'jan@example.com'`).Scan(&userID, &name); err != nil {
		t.Fatal(err)
	}
	_, page, _ := app.get(admin, "/admin/user/"+userID)
	assertContains(t, page, name, "jan@example.com", id)
	assertNotContains(t, page, "Jan Kowalski", "hx-post")
	if status, _, _ := app.get(admin, "/admin/user/nope"); status != http.StatusNotFound {
		t.Errorf("unknown customer: %d", status)
	}
	status, header = app.post(admin, "/admin/user/"+userID, url.Values{"name": {"Jan Nowy"}, "email": {"jan@example.com"}, "phone": {"500"}, "address": {"x"}}, false)
	if status != http.StatusSeeOther || !strings.Contains(header.Get("Location"), "/admin/user/"+userID) {
		t.Fatalf("saving the customer: %d %s", status, header.Get("Location"))
	}
	_, page, _ = app.get(admin, header.Get("Location"))
	assertContains(t, page, "Jan Nowy", "Zapisano dane klienta")
}

func TestAdminPanelInlineScriptsAreWellFormed(t *testing.T) {
	app := newShopHarness(t)
	_, panel, _ := app.get(app.asAdmin(), "/admin")
	if opens, closes := strings.Count(panel, "<script"), strings.Count(panel, "</script>"); opens != closes {
		t.Fatalf("unbalanced <script> tags in the admin panel: %d opened, %d closed", opens, closes)
	}
	assertContains(t, panel, `id="content-orders"`, `id="content-users"`, `id="content-fleet"`, `id="content-shop"`, `id="content-settings"`, `id="closures"`, `id="fulfillment"`)
}

func TestCheckoutRedirectsHTMXClientsWithoutSwappingPages(t *testing.T) {
	app := newShopHarness(t)
	shopper := app.visitor()
	app.addToCart(shopper, "dzwonek", "", "1")
	status, header := app.post(shopper, "/checkout/submit", url.Values{
		"payment_method": {"bank_transfer"}, "name": {"Jan Test"}, "email": {"jan@example.com"}, "phone": {"500100200"},
		"address": {"Radzymin"}, "is_adult": {"on"}, "accept_tos": {"on"}, "password": {"haslo12345"},
	}, true)
	if status != http.StatusOK || !strings.HasPrefix(header.Get("HX-Redirect"), "/payment/") || header.Get("Location") != "" {
		t.Fatalf("HTMX checkout: status %d, HX-Redirect %q, Location %q", status, header.Get("HX-Redirect"), header.Get("Location"))
	}
}

func TestOrdersAwaitingConfirmationReachTheAdminAndCanBeConfirmed(t *testing.T) {
	app := newShopHarness(t)
	admin := app.asAdmin()
	shopper := app.visitor()
	app.addToCart(shopper, "dzwonek", "", "1")
	_, _, id := app.checkout(shopper, "cash_pickup")
	if id == "" {
		t.Fatal("checkout failed")
	}

	_, panel, _ := app.get(admin, "/admin")
	assertContains(t, panel, "Zamówienia wymagające ręcznego potwierdzenia", id, "jan@example.com", "Wymaga potwierdzenia przez wypożyczalnię", `action="/admin/order/`+id+`/confirm"`)
	assertNotContains(t, panel, `[{"productId"`) // readable summary, not raw JSON

	status, header := app.post(admin, "/admin/order/"+id+"/confirm", nil, false)
	if status != http.StatusSeeOther || !strings.HasSuffix(header.Get("Location"), "#orders") {
		t.Fatalf("confirm: %d %s", status, header.Get("Location"))
	}
	// No manual projection: the action itself brings the read models up to date.
	if st, _, _ := app.orderRow(id); st != "confirmed" {
		t.Fatalf("status right after confirming: %s", st)
	}
	_, panel, _ = app.get(admin, "/admin")
	assertNotContains(t, panel, "Zamówienia wymagające ręcznego potwierdzenia")
}
