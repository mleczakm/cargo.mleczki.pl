package email

import (
	"context"
	"fmt"
	"net/smtp"
	"strings"
)

// SMTPClient sends email via plain SMTP (e.g. local Mailpit).
type SMTPClient struct {
	host string
	port string
}

// NewSMTPClient creates an SMTP mailer for the given host and port.
func NewSMTPClient(host, port string) *SMTPClient {
	return &SMTPClient{host: host, port: port}
}

// SendEmail delivers an HTML message through SMTP.
func (c *SMTPClient) SendEmail(_ context.Context, sender *EmailSender, to []EmailRecipient, subject string, htmlContent string) error {
	if sender == nil || sender.Email == "" {
		return fmt.Errorf("sender email is required")
	}
	if len(to) == 0 {
		return fmt.Errorf("at least one recipient is required")
	}

	from := FormatAddress(sender.Name, sender.Email)
	recipients := make([]string, 0, len(to))
	var toHeader strings.Builder
	for i, recipient := range to {
		if recipient.Email == "" {
			continue
		}
		recipients = append(recipients, recipient.Email)
		if i > 0 {
			toHeader.WriteString(", ")
		}
		toHeader.WriteString(FormatAddress(recipient.Name, recipient.Email))
	}
	if len(recipients) == 0 {
		return fmt.Errorf("at least one recipient email is required")
	}

	message := buildHTMLMessage(from, toHeader.String(), subject, htmlContent)
	addr := fmt.Sprintf("%s:%s", c.host, c.port)
	return smtp.SendMail(addr, nil, sender.Email, recipients, message)
}

func buildHTMLMessage(from, to, subject, htmlContent string) []byte {
	var msg strings.Builder
	msg.WriteString("From: ")
	msg.WriteString(from)
	msg.WriteString("\r\nTo: ")
	msg.WriteString(to)
	msg.WriteString("\r\nSubject: ")
	msg.WriteString(subject)
	msg.WriteString("\r\nMIME-Version: 1.0\r\nContent-Type: text/html; charset=UTF-8\r\n\r\n")
	msg.WriteString(htmlContent)
	return []byte(msg.String())
}
