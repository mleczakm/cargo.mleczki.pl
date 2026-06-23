package email

import "context"

// Mailer sends transactional HTML emails.
type Mailer interface {
	SendEmail(ctx context.Context, sender *EmailSender, to []EmailRecipient, subject string, htmlContent string) error
}

// EmailSender identifies the message sender.
type EmailSender struct {
	Name  string
	Email string
}

// EmailRecipient identifies a message recipient.
type EmailRecipient struct {
	Name  string
	Email string
}
