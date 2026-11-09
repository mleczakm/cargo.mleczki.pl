package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"cargo.mleczki.pl/internal/eventstore"
)

func TestOrderPushMessages(t *testing.T) {
	tests := []struct {
		name         string
		event        string
		payMethod    string
		orderPayment string
		wantUser     string // title, "" when nobody is notified
		wantAdmin    string
		wantUserURL  string
		wantAdminURL string
	}{
		{name: "online order waits for payment", event: "OrderPlaced", orderPayment: "bank_transfer", wantAdmin: "Nowe zamówienie", wantAdminURL: "/admin/order/ORD-1"},
		{name: "pickup order is announced at checkout instead", event: "OrderPlaced", orderPayment: "cash_pickup"},
		{name: "bank transfer matched", event: "OrderPaid", payMethod: "transfer", wantUser: "Płatność przyjęta", wantAdmin: "Przelew zaksięgowany", wantUserURL: "/user/order/ORD-1", wantAdminURL: "/admin/order/ORD-1"},
		{name: "admin marks order paid", event: "OrderPaid", payMethod: "admin_manual", wantUser: "Płatność przyjęta", wantUserURL: "/user/order/ORD-1"},
		{name: "customer confirms cash themselves", event: "OrderPaid", payMethod: "cash"},
		{name: "confirmed", event: "OrderConfirmed", wantUser: "Zamówienie potwierdzone", wantUserURL: "/user/order/ORD-1"},
		{name: "realized", event: "OrderRealized", wantUser: "Zamówienie zrealizowane", wantUserURL: "/user/order/ORD-1"},
		{name: "cancelled", event: "OrderCancelled", wantUser: "Zamówienie anulowane", wantUserURL: "/user/order/ORD-1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			user, admin := orderPushMessages(tt.event, "ORD-1", tt.payMethod, tt.orderPayment, "Jan")
			if (user == nil) != (tt.wantUser == "") || (user != nil && (user.Title != tt.wantUser || user.URL != tt.wantUserURL)) {
				t.Errorf("customer message = %+v, want title %q url %q", user, tt.wantUser, tt.wantUserURL)
			}
			if (admin == nil) != (tt.wantAdmin == "") || (admin != nil && (admin.Title != tt.wantAdmin || admin.URL != tt.wantAdminURL)) {
				t.Errorf("admin message = %+v, want title %q url %q", admin, tt.wantAdmin, tt.wantAdminURL)
			}
		})
	}
}

func TestEventIsStale(t *testing.T) {
	now := time.Now()
	fresh := &eventstore.Event{CreatedAt: now.Add(-time.Minute).UTC().Format(time.RFC3339)}
	old := &eventstore.Event{CreatedAt: now.Add(-2 * time.Hour).UTC().Format(time.RFC3339)}
	unknown := &eventstore.Event{CreatedAt: "not a date"}
	if eventIsStale(fresh, now) || !eventIsStale(old, now) || eventIsStale(unknown, now) {
		t.Error("only events older than the limit are stale")
	}
}

func TestIsOrderEvent(t *testing.T) {
	if !isOrderEvent("OrderPaid") || isOrderEvent("UserRegistered") || isOrderEvent("TransferReceived") {
		t.Error("only order events produce pushes")
	}
}

func TestPushAPIRequiresSignIn(t *testing.T) {
	handler := requireSignedInAPI(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }))
	for _, path := range []string{"/api/webpush/vapid-key", "/api/webpush/subscribe", "/api/webpush/unsubscribe"} {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, path, strings.NewReader("{}")))
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("%s: status %d, want 401", path, rec.Code)
		}
	}
}

func TestPushHandlersWithoutNotifier(t *testing.T) {
	s := &Server{}
	for name, h := range map[string]http.HandlerFunc{
		"vapid-key":   s.handleWebPushVAPIDKey,
		"subscribe":   s.handleWebPushSubscribe,
		"unsubscribe": s.handleWebPushUnsubscribe,
	} {
		rec := httptest.NewRecorder()
		h(rec, httptest.NewRequest(http.MethodPost, "/", strings.NewReader("{}")))
		if rec.Code != http.StatusServiceUnavailable {
			t.Errorf("%s: status %d, want 503", name, rec.Code)
		}
	}
}

func TestServiceWorkerIsServedFromRoot(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/sw.js", nil)
	// Tests run inside cmd/server; the handler reads the path relative to the repository root.
	t.Chdir("../..")
	(&Server{}).handleServiceWorker(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/javascript") {
		t.Errorf("content type %q", ct)
	}
	if rec.Header().Get("Cache-Control") != "no-cache" || rec.Header().Get("Service-Worker-Allowed") != "/" {
		t.Errorf("headers %v", rec.Header())
	}
	if !strings.Contains(rec.Body.String(), "addEventListener('push'") {
		t.Error("expected the push handler in the worker")
	}
}
