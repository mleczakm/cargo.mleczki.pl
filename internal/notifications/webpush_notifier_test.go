package notifications_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"cargo.mleczki.pl/internal/notifications"
	"cargo.mleczki.pl/internal/projections"

	"github.com/SherClockHolmes/webpush-go"
)

type sentPush struct {
	endpoint string
	message  notifications.Message
}

// pushRecorder stands in for the push service; status is what it answers per endpoint.
type pushRecorder struct {
	mu     sync.Mutex
	sent   []sentPush
	status map[string]int
}

func (p *pushRecorder) send(_ context.Context, payload []byte, sub *webpush.Subscription, _ *webpush.Options) (*http.Response, error) {
	var msg notifications.Message
	if err := json.Unmarshal(payload, &msg); err != nil {
		return nil, err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.sent = append(p.sent, sentPush{endpoint: sub.Endpoint, message: msg})
	code := http.StatusCreated
	if c, ok := p.status[sub.Endpoint]; ok {
		code = c
	}
	return &http.Response{StatusCode: code, Body: io.NopCloser(strings.NewReader(""))}, nil
}

func (p *pushRecorder) endpoints() []string {
	var out []string
	for _, s := range p.sent {
		out = append(out, s.endpoint)
	}
	return out
}

func newNotifier(t *testing.T) (*notifications.WebPushNotifier, *pushRecorder, *projections.ReadModelsDB) {
	t.Helper()
	rm, err := projections.NewReadModelsDB(filepath.Join(t.TempDir(), "rm.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = rm.Close() })
	for _, u := range []struct {
		id    string
		admin int
	}{{"customer", 0}, {"other", 0}, {"admin", 1}} {
		if _, err := rm.GetDB().Exec(`INSERT INTO users (id, email, name, is_admin) VALUES (?, ?, ?, ?)`, u.id, u.id+"@example.com", u.id, u.admin); err != nil {
			t.Fatal(err)
		}
	}
	rec := &pushRecorder{status: map[string]int{}}
	return notifications.NewWebPushNotifierForTest(rm.GetDB(), rec.send), rec, rm
}

func sub(endpoint string) webpush.Subscription {
	return webpush.Subscription{Endpoint: endpoint, Keys: webpush.Keys{Auth: "auth", P256dh: "p256dh"}}
}

func fcm(id string) string { return "https://fcm.googleapis.com/fcm/send/" + id }

func TestNewWebPushNotifierKeepsGeneratedKeysAcrossRestarts(t *testing.T) {
	t.Setenv("VAPID_PRIVATE_KEY", "")
	t.Setenv("VAPID_PUBLIC_KEY", "")
	rm, err := projections.NewReadModelsDB(filepath.Join(t.TempDir(), "rm.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer rm.Close()

	first, err := notifications.NewWebPushNotifier(rm.GetDB())
	if err != nil {
		t.Fatal(err)
	}
	second, err := notifications.NewWebPushNotifier(rm.GetDB())
	if err != nil {
		t.Fatal(err)
	}
	if first.GetVAPIDPublicKey() == "" || first.GetVAPIDPublicKey() != second.GetVAPIDPublicKey() {
		t.Errorf("expected the same stored public key, got %q and %q", first.GetVAPIDPublicKey(), second.GetVAPIDPublicKey())
	}
}

func TestNewWebPushNotifierPrefersEnvironmentKeys(t *testing.T) {
	t.Setenv("VAPID_PRIVATE_KEY", "env-private")
	t.Setenv("VAPID_PUBLIC_KEY", "env-public")
	n, err := notifications.NewWebPushNotifier(nil)
	if err != nil {
		t.Fatal(err)
	}
	if n.GetVAPIDPublicKey() != "env-public" {
		t.Errorf("got %q", n.GetVAPIDPublicKey())
	}
}

func TestSubscribeRejectsUnknownEndpoints(t *testing.T) {
	n, _, _ := newNotifier(t)
	for _, endpoint := range []string{
		"http://fcm.googleapis.com/fcm/send/x",   // not https
		"https://127.0.0.1:8080/x",               // internal address
		"https://evilgoogleapis.com/x",           // lookalike host
		"https://fcm.googleapis.com.evil.test/x", // suffix trick
		"https://user:pw@fcm.googleapis.com/x",   // credentials
		"https://mail.google.com/mail",           // google.com only counts for FCM paths
		"https://google.com/fcm/send/x",          // the bare domain is not a push host
		"",
	} {
		if err := n.Subscribe(t.Context(), "customer", sub(endpoint), "ua"); !errors.Is(err, notifications.ErrInvalidEndpoint) {
			t.Errorf("endpoint %q: expected ErrInvalidEndpoint, got %v", endpoint, err)
		}
	}
	for _, endpoint := range []string{
		"https://jmt17.google.com/fcm/send/fx4POKmgAzU:APA91b", // what current Chrome returns
		fcm("a"),
		"https://updates.push.services.mozilla.com/wpush/v2/abc",
		"https://web.push.apple.com/abc",
		"https://wns2-par02p.notify.windows.com/w/?token=x",
	} {
		if err := n.Subscribe(t.Context(), "customer", sub(endpoint), "ua"); err != nil {
			t.Errorf("endpoint %q: unexpected error %v", endpoint, err)
		}
	}
}

func TestSubscribeRequiresKeysAndUser(t *testing.T) {
	n, _, _ := newNotifier(t)
	if err := n.Subscribe(t.Context(), "customer", webpush.Subscription{Endpoint: fcm("a")}, ""); err == nil {
		t.Error("expected an error without keys")
	}
	if err := n.Subscribe(t.Context(), "", sub(fcm("a")), ""); err == nil {
		t.Error("expected an error without a user")
	}
}

func TestSubscribeLimitsDevicesPerUser(t *testing.T) {
	n, _, _ := newNotifier(t)
	for i := 0; i < 10; i++ {
		if err := n.Subscribe(t.Context(), "customer", sub(fcm(string(rune('a'+i)))), ""); err != nil {
			t.Fatalf("device %d: %v", i, err)
		}
	}
	if err := n.Subscribe(t.Context(), "customer", sub(fcm("z")), ""); !errors.Is(err, notifications.ErrTooManySubscriptions) {
		t.Errorf("expected ErrTooManySubscriptions, got %v", err)
	}
	// Re-registering a known device is not a new device.
	if err := n.Subscribe(t.Context(), "customer", sub(fcm("a")), ""); err != nil {
		t.Errorf("re-subscribe: %v", err)
	}
}

func TestSendToUserReachesOnlyThatUsersDevices(t *testing.T) {
	n, rec, _ := newNotifier(t)
	mustSubscribe(t, n, "customer", fcm("phone"))
	mustSubscribe(t, n, "customer", fcm("laptop"))
	mustSubscribe(t, n, "other", fcm("other-phone"))

	if err := n.SendToUser(t.Context(), "customer", notifications.Message{Title: "Hi", URL: "/user"}); err != nil {
		t.Fatal(err)
	}
	if got := len(rec.sent); got != 2 {
		t.Fatalf("expected 2 pushes, got %v", rec.endpoints())
	}
	for _, s := range rec.sent {
		if s.endpoint == fcm("other-phone") || s.message.Title != "Hi" || s.message.URL != "/user" {
			t.Errorf("unexpected push %+v", s)
		}
	}
}

func TestSendToAdminsReachesOnlyAdmins(t *testing.T) {
	n, rec, _ := newNotifier(t)
	mustSubscribe(t, n, "customer", fcm("customer-phone"))
	mustSubscribe(t, n, "admin", fcm("admin-phone"))

	if err := n.SendToAdmins(t.Context(), notifications.Message{Title: "Order"}); err != nil {
		t.Fatal(err)
	}
	if got := rec.endpoints(); len(got) != 1 || got[0] != fcm("admin-phone") {
		t.Errorf("expected only the admin device, got %v", got)
	}
}

func TestNotifyOrderRequiringConfirmationGoesToAdmins(t *testing.T) {
	n, rec, _ := newNotifier(t)
	mustSubscribe(t, n, "admin", fcm("admin-phone"))

	if err := n.NotifyOrderRequiringConfirmation(t.Context(), "ORD-1", "Jan", "cash_pickup", 100); err != nil {
		t.Fatal(err)
	}
	if len(rec.sent) != 1 || rec.sent[0].message.URL != "/admin/order/ORD-1" || !strings.Contains(rec.sent[0].message.Body, "ORD-1") {
		t.Errorf("unexpected push %+v", rec.sent)
	}
}

func TestNoSubscriptionsIsNotAnError(t *testing.T) {
	n, rec, _ := newNotifier(t)
	if err := n.SendToAdmins(t.Context(), notifications.Message{Title: "x"}); err != nil {
		t.Fatal(err)
	}
	if len(rec.sent) != 0 {
		t.Errorf("unexpected pushes %v", rec.endpoints())
	}
}

func TestExpiredSubscriptionsAreDropped(t *testing.T) {
	n, rec, _ := newNotifier(t)
	mustSubscribe(t, n, "customer", fcm("gone"))
	mustSubscribe(t, n, "customer", fcm("flaky"))
	rec.status[fcm("gone")] = http.StatusGone
	rec.status[fcm("flaky")] = http.StatusInternalServerError

	_ = n.SendToUser(t.Context(), "customer", notifications.Message{Title: "1"})
	rec.sent = nil
	_ = n.SendToUser(t.Context(), "customer", notifications.Message{Title: "2"})

	if got := rec.endpoints(); len(got) != 1 || got[0] != fcm("flaky") {
		t.Errorf("expected only the flaky device to remain, got %v", got)
	}
}

func TestSubscribeRebindsDeviceToNewUser(t *testing.T) {
	n, rec, _ := newNotifier(t)
	mustSubscribe(t, n, "customer", fcm("shared"))
	mustSubscribe(t, n, "admin", fcm("shared")) // someone else signed in on the same device

	_ = n.SendToUser(t.Context(), "customer", notifications.Message{Title: "x"})
	if len(rec.sent) != 0 {
		t.Errorf("previous user must not get pushes any more, got %v", rec.endpoints())
	}
	_ = n.SendToAdmins(t.Context(), notifications.Message{Title: "x"})
	if len(rec.sent) != 1 {
		t.Errorf("new user should get pushes, got %v", rec.endpoints())
	}
}

func TestUnsubscribeOnlyRemovesOwnDevice(t *testing.T) {
	n, rec, _ := newNotifier(t)
	mustSubscribe(t, n, "customer", fcm("phone"))

	if err := n.Unsubscribe(t.Context(), "other", fcm("phone")); err != nil {
		t.Fatal(err)
	}
	_ = n.SendToUser(t.Context(), "customer", notifications.Message{Title: "x"})
	if len(rec.sent) != 1 {
		t.Fatalf("another user must not be able to unsubscribe the device, got %v", rec.endpoints())
	}

	if err := n.Unsubscribe(t.Context(), "customer", fcm("phone")); err != nil {
		t.Fatal(err)
	}
	rec.sent = nil
	_ = n.SendToUser(t.Context(), "customer", notifications.Message{Title: "x"})
	if len(rec.sent) != 0 {
		t.Errorf("device should be gone, got %v", rec.endpoints())
	}
}

func mustSubscribe(t *testing.T, n *notifications.WebPushNotifier, userID, endpoint string) {
	t.Helper()
	if err := n.Subscribe(t.Context(), userID, sub(endpoint), "ua"); err != nil {
		t.Fatal(err)
	}
}
