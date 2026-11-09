package notifications

import (
	"database/sql"
)

// NewAdminNotifierForTest creates an admin notifier backed by the given database.
func NewAdminNotifierForTest(db *sql.DB) *AdminNotifier {
	return &AdminNotifier{db: db}
}

// NewWebPushNotifierForTest creates a web push notifier on db that hands payloads to send
// instead of a push service.
func NewWebPushNotifierForTest(db *sql.DB, send SendFunc) *WebPushNotifier {
	return &WebPushNotifier{
		db:              db,
		vapidPrivateKey: "test-private",
		vapidPublicKey:  "test-public",
		vapidSubject:    "mailto:test@example.com",
		send:            send,
		allowEndpoint:   validateEndpoint,
	}
}

// SendFunc is the exported name of sendFunc for tests.
type SendFunc = sendFunc
