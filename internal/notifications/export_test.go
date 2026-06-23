package notifications

import (
	"database/sql"

	"github.com/SherClockHolmes/webpush-go"
)

// NewAdminNotifierForTest creates an admin notifier backed by the given database.
func NewAdminNotifierForTest(db *sql.DB) *AdminNotifier {
	return &AdminNotifier{db: db}
}

// NewWebPushNotifierForTest creates a web push notifier with fixed VAPID settings.
func NewWebPushNotifierForTest(privateKey, publicKey, subject string) *WebPushNotifier {
	return &WebPushNotifier{
		vapidPrivateKey: privateKey,
		vapidPublicKey:  publicKey,
		vapidSubject:    subject,
		subscriptions:   make(map[string]webpush.Subscription),
	}
}
