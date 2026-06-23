package email

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"time"

	"cargo.mleczki.pl/internal/domain"
	"cargo.mleczki.pl/internal/eventstore"
)

// Importer handles importing transfers from email notifications.
type Importer struct {
	imapClient *IMAPClient
	parser     *Parser
	eventStore eventstore.EventStore
	db         *sql.DB
}

// HasPendingPayments checks if there are pending BLIK payments waiting to be matched.
func (i *Importer) HasPendingPayments() (bool, error) {
	var count int
	query := `
	SELECT COUNT(*) FROM orders
	WHERE status = 'awaiting_payment' AND payment_method = 'blik'
	`
	err := i.db.QueryRow(query).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// NewImporter creates a new transfer importer.
func NewImporter(imapClient *IMAPClient, parser *Parser, eventStore eventstore.EventStore, db *sql.DB) *Importer {
	return &Importer{
		imapClient: imapClient,
		parser:     parser,
		eventStore: eventStore,
		db:         db,
	}
}

// ImportTransfers fetches unread transfer notification emails and creates transfer records.
// Returns the number of transfers imported and any error.
func (i *Importer) ImportTransfers(ctx context.Context) (int, error) {
	// Fetch unread transfer notification emails
	emails, err := i.imapClient.FetchUnreadTransferNotifications()
	if err != nil {
		return 0, fmt.Errorf("failed to fetch emails: %w", err)
	}

	if len(emails) == 0 {
		return 0, nil
	}

	importedCount := 0
	for _, email := range emails {
		// Parse email to extract transfer details
		notification, err := i.parser.ParseTransferNotification(email.Subject, email.Body)
		if err != nil {
			log.Printf("Failed to parse email %d: %v", email.UID, err)
			continue
		}

		// Convert amount to grosz
		amountGrosz, err := i.parser.ParseAmountToGrosz(notification.Amount)
		if err != nil {
			log.Printf("Failed to parse amount from email %d: %v", email.UID, err)
			continue
		}

		// Check if transfer already exists (by title + amount + date)
		exists, err := i.transferExists(notification.Title, amountGrosz, email.Date)
		if err != nil {
			log.Printf("Failed to check if transfer exists: %v", err)
			continue
		}
		if exists {
			log.Printf("Transfer already exists, skipping: %s", notification.Title)
			continue
		}

		// Create TransferReceivedEvent
		transferID := fmt.Sprintf("TRF-%d", time.Now().UnixNano())
		eventData := &domain.TransferReceivedEvent{
			TransferID: transferID,
			Date:       email.Date,
			Sender:     notification.Sender,
			Title:      notification.Title,
			Amount:     amountGrosz,
			Timestamp:  time.Now(),
		}

		// Convert to Event and save to event store
		event, err := eventstore.ToEvent(transferID, "transfer", eventData, 1)
		if err != nil {
			log.Printf("Failed to convert event: %v", err)
			continue
		}
		err = i.eventStore.Save(ctx, event)
		if err != nil {
			log.Printf("Failed to emit TransferReceivedEvent: %v", err)
			continue
		}

		// Also insert directly into read models for immediate availability
		err = i.insertTransferToReadModels(transferID, notification, amountGrosz, email.Date, email.Body)
		if err != nil {
			log.Printf("Failed to insert transfer to read models: %v", err)
		}

		importedCount++
		log.Printf("Imported transfer: %s - %s PLN from %s", notification.Title, notification.Amount, notification.Sender)
	}

	// Update last import metadata if any transfers were imported
	if importedCount > 0 {
		_, err = i.db.Exec("INSERT INTO email_import_metadata (id, last_import_at, last_import_count, updated_at) VALUES (1, ?, ?, ?) ON CONFLICT(id) DO UPDATE SET last_import_at = excluded.last_import_at, last_import_count = excluded.last_import_count, updated_at = excluded.updated_at", time.Now().UTC().Format(time.RFC3339), importedCount, time.Now().UTC().Format(time.RFC3339))
		if err != nil {
			log.Printf("Failed to update last import metadata: %v", err)
		}
	}

	return importedCount, nil
}

// transferExists checks if a transfer with the given details already exists.
func (i *Importer) transferExists(title string, amount int, date string) (bool, error) {
	var count int
	query := `SELECT COUNT(*) FROM transfers WHERE title = ? AND amount = ? AND date = ?`
	err := i.db.QueryRow(query, title, amount, date).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// insertTransferToReadModels inserts a transfer directly into the read models database.
func (i *Importer) insertTransferToReadModels(transferID string, notification *TransferNotification, amount int, date, rawBody string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	query := `
	INSERT INTO transfers (id, date, sender, title, amount, status, created_at, raw_email_body)
	VALUES (?, ?, ?, ?, ?, 'unmatched', ?, ?)
	ON CONFLICT(id) DO UPDATE SET
		date = excluded.date,
		sender = excluded.sender,
		title = excluded.title,
		amount = excluded.amount,
		status = excluded.status,
		created_at = excluded.created_at,
		raw_email_body = excluded.raw_email_body
	`

	_, err := i.db.Exec(query,
		transferID,
		date,
		notification.Sender,
		notification.Title,
		amount,
		now,
		rawBody,
	)
	return err
}
