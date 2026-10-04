//nolint:testpackage // exercises the unexported webhook payload and headers
package transfers

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"cargo.mleczki.pl/internal/eventstore"
)

const testSecret = "0123456789abcdef0123456789abcdef0123456789abcdef"
const testAddress = "aabbccddeeff00112233445566778899@cargo.mleczki.pl"

func newTestHandler(t *testing.T) (*BankMailHandler, *sql.DB) {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	for _, stmt := range []string{
		`CREATE TABLE orders (id TEXT PRIMARY KEY, total_amount REAL, status TEXT, payment_method TEXT)`,
		`CREATE TABLE payment_codes (id TEXT PRIMARY KEY, code TEXT UNIQUE, order_id TEXT)`,
		`CREATE TABLE transfers (id TEXT PRIMARY KEY, sender_name TEXT, sender_email TEXT, amount REAL NOT NULL, order_title TEXT, order_id TEXT, status TEXT DEFAULT 'unmatched', received_at TEXT NOT NULL, linked_at TEXT, raw_email_body TEXT)`,
		`CREATE TABLE email_import_metadata (id INTEGER PRIMARY KEY AUTOINCREMENT, last_import_at TEXT, last_import_count INTEGER DEFAULT 0, updated_at TEXT)`,
		`CREATE TABLE bank_mail_messages (id TEXT PRIMARY KEY, received_at TEXT NOT NULL, mail_from TEXT NOT NULL, subject TEXT NOT NULL, body TEXT NOT NULL, parsed INTEGER NOT NULL DEFAULT 0, created_at TEXT DEFAULT CURRENT_TIMESTAMP)`,
		`INSERT INTO orders VALUES ('ORD-1', 150, 'awaiting_payment', 'blik')`,
		`INSERT INTO payment_codes VALUES ('pc1', 'JLN4', 'ORD-1')`,
	} {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatal(err)
		}
	}
	store, err := eventstore.NewSQLiteEventStore(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	return &BankMailHandler{DB: db, EventStore: store, Address: testAddress, Secret: testSecret, Matcher: NewMatcher()}, db
}

func signedRequest(t *testing.T, p bankMailPayload, secret string, ts time.Time) *http.Request {
	t.Helper()
	body, _ := json.Marshal(p)
	stamp := ts.UTC().Format(time.RFC3339)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(stamp + "\n"))
	mac.Write(body)
	r := httptest.NewRequest(http.MethodPost, "/api/bank-mail", bytes.NewReader(body))
	r.Header.Set(timestampHeader, stamp)
	r.Header.Set(signatureHeader, hex.EncodeToString(mac.Sum(nil)))
	return r
}

func payload(id string) bankMailPayload {
	return bankMailPayload{
		ID: id, ReceivedAt: time.Now().UTC().Format(time.RFC3339), Recipient: testAddress,
		MailFrom: "powiadomienia@alior.pl", Subject: "Uznanie rachunku 43...2755 kwotą 150,00 PLN",
		Body: "Uprzejmie informujemy, że rachunek 43...2755 został uznany kwotą 150,00 PLN.<br/>\nNadawca: JAN KOWALSKI<br/>\nTytuł zlecenia: JLN4 wynajem<br/>\nSaldo rachunku po operacji: 150,00 PLN<br/>\n",
	}
}

func TestBankMailRejectsBadSignature(t *testing.T) {
	handler, _ := newTestHandler(t)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, signedRequest(t, payload(strings.Repeat("a", 64)), "wrong-secret-wrong-secret-wrong-secret", time.Now()))
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("got %d", w.Code)
	}
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, signedRequest(t, payload(strings.Repeat("a", 64)), testSecret, time.Now().Add(-time.Hour)))
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("stale timestamp got %d", w.Code)
	}
}

func TestBankMailStoresTransferMatchesOrderAndIsIdempotent(t *testing.T) {
	handler, db := newTestHandler(t)
	id := strings.Repeat("b", 64)
	for i := 0; i < 2; i++ {
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, signedRequest(t, payload(id), testSecret, time.Now()))
		if w.Code != http.StatusNoContent {
			t.Fatalf("delivery %d: got %d: %s", i, w.Code, w.Body.String())
		}
	}
	var count int
	var amount float64
	var title string
	if err := db.QueryRow(`SELECT COUNT(*), MAX(amount), MAX(order_title) FROM transfers`).Scan(&count, &amount, &title); err != nil {
		t.Fatal(err)
	}
	if count != 1 || amount != 150 || !strings.Contains(title, "JLN4") {
		t.Fatalf("transfers=%d amount=%v title=%q", count, amount, title)
	}
	events, err := handler.EventStore.GetEvents(t.Context(), "ORD-1")
	if err != nil || len(events) != 1 || events[0].EventType != "OrderPaid" {
		t.Fatalf("expected one OrderPaid event, got %v err=%v", events, err)
	}
}

func TestBankMailAmountMismatchStaysUnmatched(t *testing.T) {
	handler, _ := newTestHandler(t)
	p := payload(strings.Repeat("c", 64))
	p.Subject = "Uznanie rachunku 43...2755 kwotą 10,00 PLN"
	p.Body = strings.ReplaceAll(p.Body, "150,00", "10,00")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, signedRequest(t, p, testSecret, time.Now()))
	if w.Code != http.StatusNoContent {
		t.Fatalf("got %d", w.Code)
	}
	events, _ := handler.EventStore.GetEvents(t.Context(), "ORD-1")
	if len(events) != 0 {
		t.Fatalf("order must not be paid on amount mismatch: %v", events)
	}
}
