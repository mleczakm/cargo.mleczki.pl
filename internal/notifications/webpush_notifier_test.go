package notifications_test

import (
	"testing"

	"cargo.mleczki.pl/internal/notifications"

	"github.com/SherClockHolmes/webpush-go"
)

func TestNewWebPushNotifier(t *testing.T) {
	notifier, err := notifications.NewWebPushNotifier()
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if notifier == nil {
		t.Fatal("Expected notifier to be created, got nil")
	}

	if notifier.GetVAPIDPublicKey() == "" {
		t.Error("Expected vapid public key to be set")
	}
}

func TestWebPushNotifier_AddSubscription(t *testing.T) {
	notifier := notifications.NewWebPushNotifierForTest(
		"test-key",
		"test-public-key",
		"mailto:test@example.com",
	)

	sub := webpush.Subscription{
		Endpoint: "https://example.com/push",
		Keys: webpush.Keys{
			Auth:   "test-auth",
			P256dh: "test-p256dh",
		},
	}

	notifier.AddSubscription("user1", sub)
	notifier.RemoveSubscription("user1")
}

func TestWebPushNotifier_GetVAPIDPublicKey(t *testing.T) {
	notifier := notifications.NewWebPushNotifierForTest(
		"test-key",
		"test-public-key",
		"mailto:test@example.com",
	)

	publicKey := notifier.GetVAPIDPublicKey()
	if publicKey != "test-public-key" {
		t.Errorf("Expected test-public-key, got %s", publicKey)
	}
}

func TestWebPushNotifier_NotifyOrderRequiringConfirmation_NoSubscriptions(t *testing.T) {
	notifier := notifications.NewWebPushNotifierForTest(
		"test-key",
		"test-public-key",
		"mailto:test@example.com",
	)

	err := notifier.NotifyOrderRequiringConfirmation(t.Context(), "ORD-123", "John Doe", "cash_pickup", 100.0)
	if err != nil {
		t.Fatalf("Expected no error when no subscriptions, got %v", err)
	}
}
