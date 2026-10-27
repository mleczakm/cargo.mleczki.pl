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

func LatestBankTest(ctx context.Context, db *sql.DB) (*BankTest, error) {
	var item BankTest
	err := db.QueryRowContext(ctx, `SELECT reference, received_at FROM bank_mail_tests ORDER BY rowid DESC LIMIT 1`).Scan(&item.Reference, &item.ReceivedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return &item, err
}
