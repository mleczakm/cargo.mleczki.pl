package notifications

import (
	"testing"

	"github.com/SherClockHolmes/webpush-go"
)

func TestNewWebPushNotifier(t *testing.T) {
	// Test with environment variables not set - should generate keys
	notifier, err := NewWebPushNotifier()
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if notifier == nil {
		t.Fatal("Expected notifier to be created, got nil")
	}

	if notifier.vapidPrivateKey == "" {
		t.Error("Expected vapidPrivateKey to be set")
	}

	if notifier.vapidPublicKey == "" {
		t.Error("Expected vapidPublicKey to be set")
	}

	if notifier.vapidSubject == "" {
		t.Error("Expected vapidSubject to be set")
	}
}

func TestWebPushNotifier_AddSubscription(t *testing.T) {
	notifier := &WebPushNotifier{
		vapidPrivateKey: "test-key",
		vapidPublicKey:  "test-public-key",
		vapidSubject:    "mailto:test@example.com",
		subscriptions:   make(map[string]webpush.Subscription),
	}

	sub := webpush.Subscription{
		Endpoint: "https://example.com/push",
		Keys: webpush.Keys{
			Auth:  "test-auth",
			P256dh: "test-p256dh",
		},
	}

	notifier.AddSubscription("user1", sub)

	if len(notifier.subscriptions) != 1 {
		t.Fatalf("Expected 1 subscription, got %d", len(notifier.subscriptions))
	}

	if _, exists := notifier.subscriptions["user1"]; !exists {
		t.Error("Expected user1 subscription to exist")
	}
}

func TestWebPushNotifier_RemoveSubscription(t *testing.T) {
	notifier := &WebPushNotifier{
		vapidPrivateKey: "test-key",
		vapidPublicKey:  "test-public-key",
		vapidSubject:    "mailto:test@example.com",
		subscriptions:   make(map[string]webpush.Subscription),
	}

	sub := webpush.Subscription{
		Endpoint: "https://example.com/push",
		Keys: webpush.Keys{
			Auth:   "test-auth",
			P256dh: "test-p256dh",
		},
	}

	notifier.AddSubscription("user1", sub)
	notifier.RemoveSubscription("user1")

	if len(notifier.subscriptions) != 0 {
		t.Fatalf("Expected 0 subscriptions, got %d", len(notifier.subscriptions))
	}
}

func TestWebPushNotifier_GetVAPIDPublicKey(t *testing.T) {
	notifier := &WebPushNotifier{
		vapidPrivateKey: "test-key",
		vapidPublicKey:  "test-public-key",
		vapidSubject:    "mailto:test@example.com",
		subscriptions:   make(map[string]webpush.Subscription),
	}

	publicKey := notifier.GetVAPIDPublicKey()
	if publicKey != "test-public-key" {
		t.Errorf("Expected test-public-key, got %s", publicKey)
	}
}

func TestWebPushNotifier_NotifyOrderRequiringConfirmation_NoSubscriptions(t *testing.T) {
	notifier := &WebPushNotifier{
		vapidPrivateKey: "test-key",
		vapidPublicKey:  "test-public-key",
		vapidSubject:    "mailto:test@example.com",
		subscriptions:   make(map[string]webpush.Subscription),
	}

	// This should not error when there are no subscriptions
	err := notifier.NotifyOrderRequiringConfirmation(nil, "ORD-123", "John Doe", "cash_pickup", 100.0)
	if err != nil {
		t.Fatalf("Expected no error when no subscriptions, got %v", err)
	}
}
