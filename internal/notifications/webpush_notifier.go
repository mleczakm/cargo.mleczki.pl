package notifications

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/SherClockHolmes/webpush-go"
)

const (
	// maxSubscriptionsPerUser bounds the rows one account can create (phone, laptop, tablet...).
	maxSubscriptionsPerUser = 10
	sendTimeout             = 10 * time.Second
	pushTTLSeconds          = 24 * 60 * 60
)

// pushServiceSuffixes are the push services browsers hand out endpoints for. The server
// POSTs to whatever endpoint a client registers, so anything else is refused (SSRF).
var pushServiceSuffixes = []string{
	"googleapis.com",            // Chrome / Android (fcm.googleapis.com)
	"push.services.mozilla.com", // Firefox
	"push.apple.com",            // Safari / iOS (web.push.apple.com)
	"notify.windows.com",        // Edge / Windows
}

// Chrome also hands out endpoints like https://jmt17.google.com/fcm/send/..., so google.com
// is accepted only for the FCM paths, not for the rest of Google's sites.
const googleHostSuffix, googlePushPathPrefix = "google.com", "/fcm/"

// ErrInvalidEndpoint is returned for subscriptions that do not point at a known push service.
var ErrInvalidEndpoint = errors.New("push endpoint is not an https URL of a known push service")

// ErrTooManySubscriptions is returned when an account already has the maximum number of devices.
var ErrTooManySubscriptions = errors.New("too many push subscriptions for this account")

// Message is what the service worker shows as a notification.
type Message struct {
	Title string `json:"title"`
	Body  string `json:"body"`
	// URL is opened when the notification is clicked (a path on this site).
	URL string `json:"url,omitempty"`
	// Tag makes a newer notification replace an older one with the same tag.
	Tag string `json:"tag,omitempty"`
}

// sendFunc delivers one encrypted payload; replaced in tests.
type sendFunc func(ctx context.Context, payload []byte, sub *webpush.Subscription, opts *webpush.Options) (*http.Response, error)

// WebPushNotifier stores browser push subscriptions and sends web push notifications.
type WebPushNotifier struct {
	db              *sql.DB
	vapidPrivateKey string
	vapidPublicKey  string
	vapidSubject    string
	send            sendFunc
	allowEndpoint   func(endpoint string) error
}

// NewWebPushNotifier creates a notifier whose subscriptions live in db (table push_subscriptions).
//
// VAPID keys come from VAPID_PRIVATE_KEY / VAPID_PUBLIC_KEY. Without them a key pair is
// generated once and kept in the database, so existing subscriptions survive restarts.
func NewWebPushNotifier(db *sql.DB) (*WebPushNotifier, error) {
	privateKey := strings.TrimSpace(os.Getenv("VAPID_PRIVATE_KEY"))
	publicKey := strings.TrimSpace(os.Getenv("VAPID_PUBLIC_KEY"))
	subject := strings.TrimSpace(os.Getenv("VAPID_SUBJECT"))

	if privateKey == "" || publicKey == "" {
		var err error
		privateKey, publicKey, err = storedVAPIDKeys(db)
		if err != nil {
			return nil, fmt.Errorf("failed to prepare VAPID keys: %w", err)
		}
		log.Printf("VAPID_PRIVATE_KEY/VAPID_PUBLIC_KEY not set: using a key pair kept in the database (set them in production)")
	}
	if subject == "" {
		subject = "mailto:admin@cargo.mleczki.pl"
	}

	return &WebPushNotifier{
		db:              db,
		vapidPrivateKey: privateKey,
		vapidPublicKey:  publicKey,
		vapidSubject:    subject,
		send:            webpush.SendNotificationWithContext,
		allowEndpoint:   validateEndpoint,
	}, nil
}

// storedVAPIDKeys returns the key pair kept in business_settings, creating it on first use.
func storedVAPIDKeys(db *sql.DB) (string, string, error) {
	if db == nil {
		return generateVAPIDKeys()
	}
	var privateKey, publicKey string
	_ = db.QueryRow(`SELECT value FROM business_settings WHERE key = 'vapid_private_key'`).Scan(&privateKey)
	_ = db.QueryRow(`SELECT value FROM business_settings WHERE key = 'vapid_public_key'`).Scan(&publicKey)
	if privateKey != "" && publicKey != "" {
		return privateKey, publicKey, nil
	}

	privateKey, publicKey, err := generateVAPIDKeys()
	if err != nil {
		return "", "", err
	}
	tx, err := db.Begin()
	if err != nil {
		return "", "", err
	}
	defer func() { _ = tx.Rollback() }()
	for key, value := range map[string]string{"vapid_private_key": privateKey, "vapid_public_key": publicKey} {
		if _, err := tx.Exec(`INSERT INTO business_settings(key,value) VALUES (?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, key, value); err != nil {
			return "", "", err
		}
	}
	if err := tx.Commit(); err != nil {
		return "", "", err
	}
	return privateKey, publicKey, nil
}

// generateVAPIDKeys generates VAPID keys for web push.
func generateVAPIDKeys() (string, string, error) {
	privateKey, publicKey, err := webpush.GenerateVAPIDKeys()
	if err != nil {
		return "", "", err
	}
	return privateKey, publicKey, nil
}

// GetVAPIDPublicKey returns the VAPID public key for client-side subscription.
func (n *WebPushNotifier) GetVAPIDPublicKey() string {
	return n.vapidPublicKey
}

// validateEndpoint accepts only https endpoints on known push services.
func validateEndpoint(endpoint string) error {
	u, err := url.Parse(endpoint)
	if err != nil || u.Scheme != "https" || u.User != nil || u.Hostname() == "" {
		return ErrInvalidEndpoint
	}
	host := strings.ToLower(u.Hostname())
	if strings.HasSuffix(host, "."+googleHostSuffix) && strings.HasPrefix(u.Path, googlePushPathPrefix) {
		return nil
	}
	for _, suffix := range pushServiceSuffixes {
		if host == suffix || strings.HasSuffix(host, "."+suffix) {
			return nil
		}
	}
	return ErrInvalidEndpoint
}

// Subscribe stores (or re-binds to userID) a device subscription.
func (n *WebPushNotifier) Subscribe(ctx context.Context, userID string, sub webpush.Subscription, userAgent string) error {
	if userID == "" {
		return errors.New("user id is required")
	}
	if sub.Keys.Auth == "" || sub.Keys.P256dh == "" {
		return errors.New("subscription keys are required")
	}
	if err := n.allowEndpoint(sub.Endpoint); err != nil {
		return err
	}
	if len(userAgent) > 300 {
		userAgent = userAgent[:300]
	}

	var count int
	var owner string
	err := n.db.QueryRowContext(ctx, `SELECT user_id FROM push_subscriptions WHERE endpoint = ?`, sub.Endpoint).Scan(&owner)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		if err := n.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM push_subscriptions WHERE user_id = ?`, userID).Scan(&count); err != nil {
			return err
		}
		if count >= maxSubscriptionsPerUser {
			return ErrTooManySubscriptions
		}
	case err != nil:
		return err
	}

	_, err = n.db.ExecContext(ctx, `
		INSERT INTO push_subscriptions (endpoint, user_id, p256dh, auth, user_agent)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(endpoint) DO UPDATE SET
			user_id = excluded.user_id, p256dh = excluded.p256dh, auth = excluded.auth, user_agent = excluded.user_agent`,
		sub.Endpoint, userID, sub.Keys.P256dh, sub.Keys.Auth, userAgent)
	return err
}

// Unsubscribe removes the device subscription of userID.
func (n *WebPushNotifier) Unsubscribe(ctx context.Context, userID, endpoint string) error {
	_, err := n.db.ExecContext(ctx, `DELETE FROM push_subscriptions WHERE endpoint = ? AND user_id = ?`, endpoint, userID)
	return err
}

// SendToUser notifies every device of one user.
func (n *WebPushNotifier) SendToUser(ctx context.Context, userID string, msg Message) error {
	return n.deliver(ctx, `SELECT endpoint, p256dh, auth FROM push_subscriptions WHERE user_id = ?`, []any{userID}, msg)
}

// SendToAdmins notifies every device of every administrator.
func (n *WebPushNotifier) SendToAdmins(ctx context.Context, msg Message) error {
	return n.deliver(ctx, `
		SELECT s.endpoint, s.p256dh, s.auth FROM push_subscriptions s
		JOIN users u ON u.id = s.user_id WHERE u.is_admin = 1`, nil, msg)
}

// NotifyOrderRequiringConfirmation tells the admins about an order waiting for manual confirmation.
func (n *WebPushNotifier) NotifyOrderRequiringConfirmation(ctx context.Context, orderID, userName, paymentMethod string, totalAmount float64) error {
	_ = paymentMethod
	return n.SendToAdmins(ctx, Message{
		Title: "Nowe zamówienie wymaga potwierdzenia",
		Body:  fmt.Sprintf("Zamówienie %s od %s (%.2f zł) wymaga ręcznego potwierdzenia", orderID, userName, totalAmount),
		URL:   "/admin/order/" + url.PathEscape(orderID),
		Tag:   "order-" + orderID,
	})
}

type subscriptionRow struct {
	endpoint, p256dh, auth string
}

func (n *WebPushNotifier) deliver(ctx context.Context, query string, args []any, msg Message) error {
	rows, err := n.db.QueryContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("failed to load push subscriptions: %w", err)
	}
	var subs []subscriptionRow
	for rows.Next() {
		var row subscriptionRow
		if err := rows.Scan(&row.endpoint, &row.p256dh, &row.auth); err != nil {
			_ = rows.Close()
			return fmt.Errorf("failed to read push subscription: %w", err)
		}
		subs = append(subs, row)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return err
	}
	_ = rows.Close()
	if len(subs) == 0 {
		return nil
	}

	payload, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %w", err)
	}

	for _, row := range subs {
		n.deliverOne(ctx, payload, row)
	}
	return nil
}

func (n *WebPushNotifier) deliverOne(ctx context.Context, payload []byte, row subscriptionRow) {
	sendCtx, cancel := context.WithTimeout(ctx, sendTimeout)
	defer cancel()

	resp, err := n.send(sendCtx, payload, &webpush.Subscription{
		Endpoint: row.endpoint,
		Keys:     webpush.Keys{P256dh: row.p256dh, Auth: row.auth},
	}, &webpush.Options{
		Subscriber:      n.vapidSubject,
		VAPIDPublicKey:  n.vapidPublicKey,
		VAPIDPrivateKey: n.vapidPrivateKey,
		TTL:             pushTTLSeconds,
		Urgency:         webpush.UrgencyHigh,
	})
	if err != nil {
		log.Printf("Failed to send web push: %v", err)
		return
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))

	switch {
	case resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusGone:
		// The device unsubscribed or the subscription expired.
		if _, err := n.db.ExecContext(ctx, `DELETE FROM push_subscriptions WHERE endpoint = ?`, row.endpoint); err != nil {
			log.Printf("Failed to drop expired push subscription: %v", err)
		}
	case resp.StatusCode >= http.StatusBadRequest:
		log.Printf("Web push service answered %d", resp.StatusCode)
	}
}
