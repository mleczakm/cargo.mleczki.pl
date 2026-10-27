package transfers

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"regexp"
	"strings"
	"time"
)

var bankTestReference = regexp.MustCompile(`(?i)TEST-[0-9a-f]{24}`)

// BankTest is a diagnostic transfer, never an order or a recorded payment.
type BankTest struct {
	Reference  string
	ReceivedAt sql.NullString
}

func CreateBankTest(ctx context.Context, db *sql.DB) error {
	var random [12]byte
	if _, err := rand.Read(random[:]); err != nil {
		return err
	}
	_, err := db.ExecContext(ctx, `INSERT INTO bank_mail_tests(reference, created_at) VALUES (?, ?)`,
		"TEST-"+strings.ToUpper(hex.EncodeToString(random[:])), time.Now().UTC().Format(time.RFC3339))
	return err
}

// LatestBankTest returns nil without error if no diagnostic has been started.
//
//nolint:nilnil // Absence is a valid state for the settings page.
func LatestBankTest(ctx context.Context, db *sql.DB) (*BankTest, error) {
	var item BankTest
	err := db.QueryRowContext(ctx, `SELECT reference, received_at FROM bank_mail_tests ORDER BY rowid DESC LIMIT 1`).Scan(&item.Reference, &item.ReceivedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return &item, err
}

func matchBankTest(ctx context.Context, tx *sql.Tx, id string, received time.Time, title string, parsed bool) (bool, error) {
	id = strings.ToLower(id)
	var count int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM bank_mail_test_deliveries WHERE message_id = ?`, id).Scan(&count); err != nil {
		return false, err
	}
	if count > 0 {
		return true, nil
	}
	if !parsed {
		return false, nil
	}
	for _, ref := range bankTestReference.FindAllString(title, -1) {
		ref = strings.ToUpper(ref)
		result, err := tx.ExecContext(ctx, `UPDATE bank_mail_tests SET received_at = COALESCE(received_at, ?), message_id = COALESCE(message_id, ?) WHERE reference = ?`, received.UTC().Format(time.RFC3339), id, ref)
		if err != nil {
			return false, err
		}
		count, err := result.RowsAffected()
		if err != nil {
			return false, err
		}
		if count > 0 {
			_, err := tx.ExecContext(ctx, `INSERT INTO bank_mail_test_deliveries(message_id, reference) VALUES (?, ?)`, id, ref)
			return true, err
		}
	}
	return false, nil
}
