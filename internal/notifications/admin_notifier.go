package notifications

import (
	"context"
	"database/sql"
	"fmt"
	"log"

	"cargo.mleczki.pl/internal/email"
)

// AdminNotifier handles sending notifications to administrators.
type AdminNotifier struct {
	mailer *email.Sender
	db     *sql.DB
}

// NewAdminNotifier creates a new admin notifier.
func NewAdminNotifier(db *sql.DB) (*AdminNotifier, error) {
	mailer, err := email.NewMailer()
	if err != nil {
		return nil, fmt.Errorf("failed to create mailer: %w", err)
	}

	return &AdminNotifier{
		mailer: mailer,
		db:     db,
	}, nil
}

// GetAdminEmails retrieves all admin email addresses from the database.
func (n *AdminNotifier) GetAdminEmails() ([]string, error) {
	query := `SELECT email FROM users WHERE is_admin = 1`
	rows, err := n.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to query admin emails: %w", err)
	}
	defer rows.Close()

	var emails []string
	for rows.Next() {
		var email string
		if err := rows.Scan(&email); err != nil {
			return nil, fmt.Errorf("failed to scan admin email: %w", err)
		}
		emails = append(emails, email)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating admin emails: %w", err)
	}

	if len(emails) == 0 {
		log.Printf("No admin emails found in database")
	}

	return emails, nil
}

// NotifyOrderRequiringConfirmation sends an email notification to admin about an order requiring manual confirmation.
func (n *AdminNotifier) NotifyOrderRequiringConfirmation(ctx context.Context, orderID, userName, userEmail, paymentMethod string, totalAmount float64) error {
	adminEmails, err := n.GetAdminEmails()
	if err != nil {
		return fmt.Errorf("failed to get admin emails: %w", err)
	}

	if len(adminEmails) == 0 {
		log.Printf("No admin emails found, skipping notification for order %s", orderID)
		return nil
	}

	subject := fmt.Sprintf("Nowa zamówienie wymaga potwierdzenia: %s", orderID)

	htmlContent := fmt.Sprintf(`
		<h2>Zamówienie wymaga ręcznego potwierdzenia</h2>
		<p><strong>ID zamówienia:</strong> %s</p>
		<p><strong>Klient:</strong> %s (%s)</p>
		<p><strong>Metoda płatności:</strong> %s</p>
		<p><strong>Kwota:</strong> %.2f zł</p>
		<p><strong>Typ:</strong> Pierwsze zamówienie klienta - płatność przy odbiorze</p>
		<p><a href="https://cargo.mleczki.pl/admin">Przejdź do panelu administratora</a></p>
	`, orderID, userName, userEmail, paymentMethod, totalAmount)

	sender := &email.EmailSender{
		Name:  "Cargo Mleczki",
		Email: email.DefaultSenderEmail(),
	}

	recipients := make([]email.EmailRecipient, 0, len(adminEmails))
	for _, adminEmail := range adminEmails {
		recipients = append(recipients, email.EmailRecipient{
			Name:  "Administrator",
			Email: adminEmail,
		})
	}

	if n.mailer == nil || !n.mailer.Configured() {
		log.Printf("Mailer not configured, skipping admin notification for order %s", orderID)
		return nil
	}

	err = n.mailer.SendEmail(ctx, sender, recipients, subject, htmlContent)
	if err != nil {
		return fmt.Errorf("failed to send admin notification email: %w", err)
	}

	log.Printf("Sent admin notification email for order %s to %d admins", orderID, len(adminEmails))
	return nil
}
