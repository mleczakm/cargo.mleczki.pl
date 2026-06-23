package notifications

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"

	"github.com/SherClockHolmes/webpush-go"
)

// WebPushNotifier handles sending web push notifications.
type WebPushNotifier struct {
	vapidPrivateKey string
	vapidPublicKey  string
	vapidSubject    string
	subscriptions   map[string]webpush.Subscription // In-memory storage (consider using DB for production)
}

// NewWebPushNotifier creates a new web push notifier.
func NewWebPushNotifier() (*WebPushNotifier, error) {
	vapidPrivateKey := os.Getenv("VAPID_PRIVATE_KEY")
	vapidPublicKey := os.Getenv("VAPID_PUBLIC_KEY")
	vapidSubject := os.Getenv("VAPID_SUBJECT")

	// Generate VAPID keys if not provided
	if vapidPrivateKey == "" || vapidPublicKey == "" {
		privateKey, publicKey, err := generateVAPIDKeys()
		if err != nil {
			return nil, fmt.Errorf("failed to generate VAPID keys: %w", err)
		}
		vapidPrivateKey = privateKey
		vapidPublicKey = publicKey
		log.Printf("Generated VAPID keys - please set these environment variables:")
		log.Printf("VAPID_PRIVATE_KEY=%s", vapidPrivateKey)
		log.Printf("VAPID_PUBLIC_KEY=%s", vapidPublicKey)
	}

	if vapidSubject == "" {
		vapidSubject = "mailto:admin@cargo.mleczki.pl"
	}

	return &WebPushNotifier{
		vapidPrivateKey: vapidPrivateKey,
		vapidPublicKey:  vapidPublicKey,
		vapidSubject:    vapidSubject,
		subscriptions:   make(map[string]webpush.Subscription),
	}, nil
}

// generateVAPIDKeys generates VAPID keys for web push.
func generateVAPIDKeys() (string, string, error) {
	privateKey, publicKey, err := webpush.GenerateVAPIDKeys()
	if err != nil {
		return "", "", err
	}
	return privateKey, publicKey, nil
}

// AddSubscription adds a new push subscription.
func (n *WebPushNotifier) AddSubscription(userID string, sub webpush.Subscription) {
	n.subscriptions[userID] = sub
	log.Printf("Added web push subscription for user %s", userID)
}

// RemoveSubscription removes a push subscription.
func (n *WebPushNotifier) RemoveSubscription(userID string) {
	delete(n.subscriptions, userID)
	log.Printf("Removed web push subscription for user %s", userID)
}

// NotifyOrderRequiringConfirmation sends a web push notification about an order requiring confirmation.
func (n *WebPushNotifier) NotifyOrderRequiringConfirmation(ctx context.Context, orderID, userName, paymentMethod string, totalAmount float64) error {
	if len(n.subscriptions) == 0 {
		log.Printf("No web push subscriptions, skipping notification for order %s", orderID)
		return nil
	}

	payload := map[string]interface{}{
		"title": "Nowe zamówienie wymaga potwierdzenia",
		"body":  fmt.Sprintf("Zamówienie %s od %s (%.2f zł) wymaga ręcznego potwierdzenia", orderID, userName, totalAmount),
		"data": map[string]string{
			"orderID":        orderID,
			"paymentMethod":  paymentMethod,
			"action":         "confirm_order",
			"url":            fmt.Sprintf("/admin?highlight=%s", orderID),
		},
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %w", err)
	}

	// Send to all admin subscriptions
	for userID, sub := range n.subscriptions {
		resp, err := webpush.SendNotification(payloadBytes, &sub, &webpush.Options{
			Subscriber:      n.vapidSubject,
			VAPIDPrivateKey: n.vapidPrivateKey,
			TTL:             3600,
		})
		if err != nil {
			log.Printf("Failed to send web push to user %s: %v", userID, err)
			continue
		}
		defer resp.Body.Close()
		log.Printf("Sent web push notification to user %s for order %s", userID, orderID)
	}

	return nil
}

// GetVAPIDPublicKey returns the VAPID public key for client-side subscription.
func (n *WebPushNotifier) GetVAPIDPublicKey() string {
	return n.vapidPublicKey
}
