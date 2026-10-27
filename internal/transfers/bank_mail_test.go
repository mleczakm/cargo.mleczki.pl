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
		`CREATE TABLE bank_mail_tests (reference TEXT PRIMARY KEY, created_at TEXT NOT NULL, received_at TEXT, message_id TEXT)`,
		`CREATE TABLE bank_mail_test_deliveries (message_id TEXT PRIMARY KEY, reference TEXT NOT NULL)`,
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

func TestDiagnosticTransferNeverCreatesTransferOrPaysOrder(t *testing.T) {
	handler, db := newTestHandler(t)
	if err := CreateBankTest(t.Context(), db); err != nil {
		t.Fatal(err)
	}
	test, err := LatestBankTest(t.Context(), db)
	if err != nil || test == nil || test.ReceivedAt.Valid {
		t.Fatalf("pending test = %+v, %v", test, err)
	}
	notification := payload(strings.Repeat("e", 64))
	notification.Body = strings.Replace(notification.Body, "JLN4 wynajem", "JLN4 "+test.Reference+" dopisek", 1)
	diagnosticBody := notification.Body
	for i, id := range []string{notification.ID, notification.ID, strings.Repeat("f", 64)} {
		notification.ID = id
		if i == 1 {
			notification.ID = strings.ToUpper(id)
			notification.Body = payload(id).Body
		} else {
			notification.Body = diagnosticBody
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, signedRequest(t, notification, testSecret, time.Now()))
		if w.Code != http.StatusNoContent {
			t.Fatalf("diagnostic delivery=%d %s", w.Code, w.Body.String())
		}
	}
	test, err = LatestBankTest(t.Context(), db)
	if err != nil || !test.ReceivedAt.Valid {
		t.Fatalf("test did not succeed: %+v %v", test, err)
	}
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM transfers`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("transfers=%d err=%v", count, err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM bank_mail_messages`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("bank messages=%d err=%v", count, err)
	}
	events, err := handler.EventStore.GetEvents(t.Context(), "ORD-1")
	if err != nil || len(events) != 0 {
		t.Fatalf("test paid an order: %v %v", events, err)
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

func TestBankMailMatchesAmountsWithGroszeExactly(t *testing.T) {
	cases := map[string]struct {
		amount string
		paid   bool
	}{
		"exact":         {"49,90", true},
		"one grosz off": {"49,89", false},
		"whole złoty":   {"50,00", false},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			handler, db := newTestHandler(t)
			for _, stmt := range []string{
				`INSERT INTO orders VALUES ('ORD-2', 49.9, 'awaiting_payment', 'blik')`,
				`INSERT INTO payment_codes VALUES ('pc2', 'KLM7', 'ORD-2')`,
			} {
				if _, err := db.Exec(stmt); err != nil {
					t.Fatal(err)
				}
			}
			p := payload(strings.Repeat("d", 64))
			p.Subject = "Uznanie rachunku 43...2755 kwotą " + tc.amount + " PLN"
			p.Body = strings.NewReplacer("150,00", tc.amount, "JLN4", "KLM7").Replace(p.Body)
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, signedRequest(t, p, testSecret, time.Now()))
			if w.Code != http.StatusNoContent {
				t.Fatalf("got %d", w.Code)
			}
			events, _ := handler.EventStore.GetEvents(t.Context(), "ORD-2")
			if paid := len(events) == 1; paid != tc.paid {
				t.Errorf("paid = %v, want %v (events %v)", paid, tc.paid, events)
			}
		})
	}
}
