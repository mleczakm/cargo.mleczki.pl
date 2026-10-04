package transfers

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"strings"
	"time"

	sharedpayments "github.com/mleczakm/blik-phone-payments-go"

	"cargo.mleczki.pl/internal/domain"
	"cargo.mleczki.pl/internal/eventstore"
)

const (
	bankMailBodyLimit   = 128 << 10
	bankMailMaxSkew     = 5 * time.Minute
	timestampHeader     = "X-Cargo-Bank-Timestamp"
	signatureHeader     = "X-Cargo-Bank-Signature"
	minWebhookSecretLen = 32
)

type bankMailPayload struct {
	ID         string `json:"id"`
	ReceivedAt string `json:"received_at"` //nolint:tagliatelle // wire format of the Cloudflare Worker
	Recipient  string `json:"recipient"`
	MailFrom   string `json:"mail_from"` //nolint:tagliatelle // wire format of the Cloudflare Worker
	Subject    string `json:"subject"`
	Body       string `json:"body"`
}

// BankMailHandler receives bank notifications forwarded by the Cloudflare Email Routing Worker.
type BankMailHandler struct {
	DB         *sql.DB
	EventStore eventstore.EventStore
	Address    string
	Secret     string
	Matcher    *Matcher
}

// Configured reports whether the webhook can accept requests.
func (h *BankMailHandler) Configured() bool {
	return h != nil && len(h.Secret) >= minWebhookSecretLen && h.Address != ""
}

// ServeHTTP accepts only requests signed by the Worker.
func (h *BankMailHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !h.Configured() {
		http.Error(w, "bank mail is not configured", http.StatusServiceUnavailable)
		return
	}
	payload, received, ok := h.readPayload(w, r)
	if !ok {
		return
	}

	notification, parsed := (sharedpayments.ChainParser{sharedpayments.AliorParser{}, sharedpayments.GenericParser{}}).Parse(payload.Subject, payload.Body)
	transferID, inserted, err := h.save(r.Context(), payload, received, notification, parsed)
	if err != nil {
		log.Printf("bank mail: save failed: %v", err)
		http.Error(w, "storage error", http.StatusInternalServerError)
		return
	}
	if inserted && !parsed {
		log.Printf("bank mail: unrecognized message %s (%q)", payload.ID, payload.Subject)
	}
	if transferID != "" {
		h.autoMatch(r.Context(), transferID, notification)
	}
	w.WriteHeader(http.StatusNoContent)
}

// readPayload verifies the signature and decodes the request; on failure it writes the response.
func (h *BankMailHandler) readPayload(w http.ResponseWriter, r *http.Request) (bankMailPayload, time.Time, bool) {
	var payload bankMailPayload
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, bankMailBodyLimit))
	if err != nil {
		http.Error(w, "request too large", http.StatusRequestEntityTooLarge)
		return payload, time.Time{}, false
	}
	if !h.validSignature(r.Header.Get(timestampHeader), r.Header.Get(signatureHeader), body) {
		http.Error(w, "invalid signature", http.StatusUnauthorized)
		return payload, time.Time{}, false
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		http.Error(w, "invalid payload", http.StatusBadRequest)
		return payload, time.Time{}, false
	}
	if len(payload.ID) != 64 || !isHex(payload.ID) || !strings.EqualFold(payload.Recipient, h.Address) ||
		len(payload.Subject) > 2000 || len(payload.Body) > 100000 || payload.MailFrom == "" {
		http.Error(w, "invalid payload", http.StatusBadRequest)
		return payload, time.Time{}, false
	}
	received, err := time.Parse(time.RFC3339, payload.ReceivedAt)
	if err != nil {
		http.Error(w, "invalid timestamp", http.StatusBadRequest)
		return payload, time.Time{}, false
	}
	return payload, received, true
}

func (h *BankMailHandler) validSignature(ts, sig string, body []byte) bool {
	t, err := time.Parse(time.RFC3339, ts)
	if err != nil || time.Since(t) > bankMailMaxSkew || time.Until(t) > bankMailMaxSkew {
		return false
	}
	provided, err := hex.DecodeString(sig)
	if err != nil {
		return false
	}
	mac := hmac.New(sha256.New, []byte(h.Secret))
	_, _ = mac.Write([]byte(ts + "\n"))
	_, _ = mac.Write(body)
	return hmac.Equal(provided, mac.Sum(nil))
}

// save stores the raw message and, when it parsed, the transfer in one transaction.
// A repeated delivery of the same message is a no-op (inserted=false, transferID="").
func (h *BankMailHandler) save(ctx context.Context, payload bankMailPayload, received time.Time, notification sharedpayments.Notification, parsed bool) (string, bool, error) {
	tx, err := h.DB.BeginTx(ctx, nil)
	if err != nil {
		return "", false, err
	}
	defer func() { _ = tx.Rollback() }()

	res, err := tx.ExecContext(ctx, `
		INSERT OR IGNORE INTO bank_mail_messages (id, received_at, mail_from, subject, body, parsed)
		VALUES (?, ?, ?, ?, ?, ?)`,
		payload.ID, received.UTC().Format(time.RFC3339), payload.MailFrom, payload.Subject, payload.Body, parsed)
	if err != nil {
		return "", false, err
	}
	if affected, _ := res.RowsAffected(); affected == 0 {
		return "", false, nil
	}

	transferID := ""
	if parsed {
		transferID = "TRF-" + payload.ID[:16]
		_, err = tx.ExecContext(ctx, `
			INSERT INTO transfers (id, sender_name, amount, order_title, status, received_at, raw_email_body)
			VALUES (?, ?, ?, ?, 'unmatched', ?, ?)`,
			transferID, notification.Sender, float64(notification.Amount)/100, notification.Title, received.UTC().Format(time.RFC3339), payload.Body)
		if err != nil {
			return "", false, err
		}
		now := time.Now().UTC().Format(time.RFC3339)
		_, err = tx.ExecContext(ctx, `
			INSERT INTO email_import_metadata (id, last_import_at, last_import_count, updated_at)
			VALUES (1, ?, 1, ?)
			ON CONFLICT(id) DO UPDATE SET last_import_at = excluded.last_import_at,
				last_import_count = last_import_count + 1, updated_at = excluded.updated_at`, now, now)
		if err != nil {
			return "", false, err
		}
	}
	if err := tx.Commit(); err != nil {
		return "", false, err
	}
	return transferID, true, nil
}

// autoMatch links the transfer to an awaiting BLIK order whose payment code is in the title
// and whose total equals the transferred amount. Anything else stays unmatched for manual review.
func (h *BankMailHandler) autoMatch(ctx context.Context, transferID string, notification sharedpayments.Notification) {
	rows, err := h.DB.QueryContext(ctx, `
		SELECT pc.code, pc.order_id, o.total_amount
		FROM payment_codes pc JOIN orders o ON o.id = pc.order_id
		WHERE o.status = 'awaiting_payment' AND o.payment_method = 'blik'`)
	if err != nil {
		log.Printf("bank mail: load payment codes: %v", err)
		return
	}
	defer rows.Close()
	var codes []string
	orders := map[string]struct {
		id    string
		gross int64
	}{}
	for rows.Next() {
		var code, orderID string
		var total float64
		if err := rows.Scan(&code, &orderID, &total); err != nil {
			log.Printf("bank mail: scan payment code: %v", err)
			return
		}
		codes = append(codes, code)
		orders[code] = struct {
			id    string
			gross int64
		}{orderID, int64(math.Round(total * 100))}
	}
	if err := rows.Err(); err != nil {
		log.Printf("bank mail: iterate payment codes: %v", err)
		return
	}
	matched := h.Matcher.MatchByPaymentCode(notification.Title, codes)
	if matched == "" {
		return
	}
	order := orders[matched]
	if order.gross != notification.Amount {
		log.Printf("bank mail: code %s matched order %s but amount %d != %d grosz; left for manual review", matched, order.id, notification.Amount, order.gross)
		return
	}
	if err := h.emit(ctx, transferID, order.id); err != nil {
		log.Printf("bank mail: auto-match %s -> %s: %v", transferID, order.id, err)
		return
	}
	log.Printf("bank mail: transfer %s paid order %s", transferID, order.id)
}

func (h *BankMailHandler) emit(ctx context.Context, transferID, orderID string) error {
	now := time.Now().UTC()
	linked, err := eventstore.ToEvent(transferID, "transfer", &domain.TransferLinkedEvent{TransferID: transferID, OrderID: orderID, Timestamp: now}, 0)
	if err != nil {
		return err
	}
	paid, err := eventstore.ToEvent(orderID, "order", &domain.OrderPaidEvent{OrderID: orderID, Method: "transfer", Timestamp: now}, 0)
	if err != nil {
		return err
	}
	if err := h.EventStore.Save(ctx, linked); err != nil {
		return fmt.Errorf("save TransferLinked: %w", err)
	}
	if err := h.EventStore.Save(ctx, paid); err != nil {
		return fmt.Errorf("save OrderPaid: %w", err)
	}
	return nil
}

func isHex(s string) bool {
	_, err := hex.DecodeString(s)
	return err == nil
}
