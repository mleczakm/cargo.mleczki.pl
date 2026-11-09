package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"time"

	"github.com/SherClockHolmes/webpush-go"

	"cargo.mleczki.pl/internal/eventstore"
	"cargo.mleczki.pl/internal/middleware"
	"cargo.mleczki.pl/internal/notifications"
)

const (
	maxPushBodyBytes = 8 << 10
	// A push about an event this old is no news: it only happens when events are replayed.
	maxPushEventAge = 30 * time.Minute
)

// requireSignedInAPI answers 401 (instead of redirecting to the login page) for API calls.
func requireSignedInAPI(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !middleware.IsAuthenticated(r) {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// handleServiceWorker serves the service worker from the site root so that its scope is the
// whole site. Browsers must always revalidate it to pick up new versions.
func (s *Server) handleServiceWorker(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Service-Worker-Allowed", "/")
	http.ServeFile(w, r, "web/static/sw.js")
}

// handleWebPushVAPIDKey returns the VAPID public key for web push subscriptions.
func (s *Server) handleWebPushVAPIDKey(w http.ResponseWriter, r *http.Request) {
	if s.webPushNotifier == nil {
		http.Error(w, "Web push not configured", http.StatusServiceUnavailable)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	writeJSON(w, map[string]string{
		"publicKey": s.webPushNotifier.GetVAPIDPublicKey(),
	})
}

// handleWebPushSubscribe registers the caller's device for web push.
func (s *Server) handleWebPushSubscribe(w http.ResponseWriter, r *http.Request) {
	if s.webPushNotifier == nil {
		http.Error(w, "Web push not configured", http.StatusServiceUnavailable)
		return
	}

	var sub webpush.Subscription
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxPushBodyBytes)).Decode(&sub); err != nil {
		http.Error(w, "Invalid subscription", http.StatusBadRequest)
		return
	}

	err := s.webPushNotifier.Subscribe(r.Context(), middleware.GetUserID(r), sub, r.UserAgent())
	switch {
	case errors.Is(err, notifications.ErrInvalidEndpoint):
		http.Error(w, "Unsupported push endpoint", http.StatusBadRequest)
		return
	case errors.Is(err, notifications.ErrTooManySubscriptions):
		http.Error(w, "Too many devices", http.StatusConflict)
		return
	case err != nil:
		// Missing keys and similar client mistakes are not distinguishable from storage errors here.
		log.Printf("Failed to save push subscription: %v", err)
		http.Error(w, "Invalid subscription", http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	writeJSON(w, map[string]string{"status": "subscribed"})
}

// handleWebPushUnsubscribe removes the caller's device from web push.
func (s *Server) handleWebPushUnsubscribe(w http.ResponseWriter, r *http.Request) {
	if s.webPushNotifier == nil {
		http.Error(w, "Web push not configured", http.StatusServiceUnavailable)
		return
	}

	var body struct {
		Endpoint string `json:"endpoint"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxPushBodyBytes)).Decode(&body); err != nil || body.Endpoint == "" {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	if err := s.webPushNotifier.Unsubscribe(r.Context(), middleware.GetUserID(r), body.Endpoint); err != nil {
		log.Printf("Failed to remove push subscription: %v", err)
		http.Error(w, "Failed to unsubscribe", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	writeJSON(w, map[string]string{"status": "unsubscribed"})
}

// observeProjectedEvent turns order status changes into push notifications. It is called by
// the projector, so the read models already reflect the event; the sending happens
// in the background.
func (s *Server) observeProjectedEvent(ctx context.Context, event *eventstore.Event) {
	if s.webPushNotifier == nil || !isOrderEvent(event.EventType) || eventIsStale(event, time.Now()) {
		return
	}
	go s.pushOrderEvent(context.WithoutCancel(ctx), event)
}

func isOrderEvent(eventType string) bool {
	switch eventType {
	case "OrderPlaced", "OrderPaid", "OrderConfirmed", "OrderRealized", "OrderCancelled":
		return true
	}
	return false
}

func eventIsStale(event *eventstore.Event, now time.Time) bool {
	created, err := time.Parse(time.RFC3339, event.CreatedAt)
	return err == nil && now.Sub(created) > maxPushEventAge
}

func (s *Server) pushOrderEvent(ctx context.Context, event *eventstore.Event) {
	var payload struct {
		OrderID       string `json:"orderId"`
		Method        string `json:"method"`
		PaymentMethod string `json:"paymentMethod"`
	}
	if err := json.Unmarshal(event.Payload, &payload); err != nil || payload.OrderID == "" {
		return
	}

	var userID, customer, paymentMethod string
	err := s.readModels.GetDB().QueryRowContext(ctx, `
		SELECT o.user_id, COALESCE(u.name, ''), o.payment_method
		FROM orders o LEFT JOIN users u ON u.id = o.user_id WHERE o.id = ?`, payload.OrderID).Scan(&userID, &customer, &paymentMethod)
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			log.Printf("Failed to load order %s for push: %v", payload.OrderID, err)
		}
		return
	}

	toUser, toAdmins := orderPushMessages(event.EventType, payload.OrderID, payload.Method, paymentMethod, customer)
	if toUser != nil && userID != "" {
		if err := s.webPushNotifier.SendToUser(ctx, userID, *toUser); err != nil {
			log.Printf("Failed to push %s to the customer of order %s: %v", event.EventType, payload.OrderID, err)
		}
	}
	if toAdmins != nil {
		if err := s.webPushNotifier.SendToAdmins(ctx, *toAdmins); err != nil {
			log.Printf("Failed to push %s to admins for order %s: %v", event.EventType, payload.OrderID, err)
		}
	}
}

// orderPushMessages decides who hears about an order event and what they are told:
// the customer about their order's progress, the admins about what needs their attention.
// Pickup orders are announced to the admins at checkout already, and a payment the
// recipient recorded themselves needs no notification.
func orderPushMessages(eventType, orderID, payMethod, orderPaymentMethod, customer string) (toUser, toAdmins *notifications.Message) {
	userMsg := func(title, body string) *notifications.Message {
		return &notifications.Message{Title: title, Body: body, URL: "/user/order/" + url.PathEscape(orderID), Tag: "order-" + orderID}
	}
	adminMsg := func(title, body string) *notifications.Message {
		return &notifications.Message{Title: title, Body: body, URL: "/admin/order/" + url.PathEscape(orderID), Tag: "order-" + orderID}
	}
	who := customer
	if who == "" {
		who = "klienta"
	}

	switch eventType {
	case "OrderPlaced":
		if !isPickupPaymentMethod(orderPaymentMethod) {
			toAdmins = adminMsg("Nowe zamówienie", fmt.Sprintf("Zamówienie %s od %s czeka na płatność.", orderID, who))
		}
	case "OrderPaid":
		if payMethod != "cash" {
			toUser = userMsg("Płatność przyjęta", fmt.Sprintf("Zamówienie %s zostało opłacone. Dziękujemy!", orderID))
		}
		if payMethod == "transfer" {
			toAdmins = adminMsg("Przelew zaksięgowany", fmt.Sprintf("Zamówienie %s od %s zostało opłacone przelewem.", orderID, who))
		}
	case "OrderConfirmed":
		toUser = userMsg("Zamówienie potwierdzone", fmt.Sprintf("Zamówienie %s zostało potwierdzone.", orderID))
	case "OrderRealized":
		toUser = userMsg("Zamówienie zrealizowane", fmt.Sprintf("Zamówienie %s zostało zrealizowane. Dziękujemy!", orderID))
	case "OrderCancelled":
		toUser = userMsg("Zamówienie anulowane", fmt.Sprintf("Zamówienie %s zostało anulowane.", orderID))
	}
	return toUser, toAdmins
}
