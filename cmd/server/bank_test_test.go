package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAdminBankDiagnosticAccessAndStatus(t *testing.T) {
	app := newShopHarness(t)
	app.server.bankMail.Address = "bank@example.test"
	app.server.bankMail.Secret = strings.Repeat("a", 32)
	if status, _ := app.post(app.visitor(), "/admin/bank-test", nil, false); status != http.StatusFound {
		t.Fatalf("visitor create: %d", status)
	}
	admin := app.asAdmin()
	req, _ := http.NewRequest(http.MethodPost, app.http.URL+"/admin/bank-test", nil)
	req.Header.Set("Origin", "https://attacker.example")
	res, err := admin.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = res.Body.Close()
	if res.StatusCode != http.StatusForbidden {
		t.Fatalf("cross-origin create: %d", res.StatusCode)
	}
	status, headers := app.post(admin, "/admin/bank-test", nil, false)
	if status != http.StatusSeeOther || headers.Get("Location") != "/admin#settings" {
		t.Fatalf("admin create: %d %s", status, headers.Get("Location"))
	}
	status, page, _ := app.get(admin, "/admin")
	if status != http.StatusOK || !strings.Contains(page, "Oczekiwanie na powiadomienie bankowe") || !strings.Contains(page, "bank-test-reference") {
		t.Fatalf("missing diagnostic in settings: %d", status)
	}
}

func TestBankDiagnosticRequiresConfiguration(t *testing.T) {
	app := newShopHarness(t)
	w := httptest.NewRecorder()
	app.server.handleAdminBankTestCreate(w, httptest.NewRequest(http.MethodPost, "/admin/bank-test", nil))
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("unconfigured = %d", w.Code)
	}
}
