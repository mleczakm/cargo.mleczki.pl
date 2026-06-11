package email

import (
	"crypto/tls"
	"fmt"
	"log"
	"strings"

	"github.com/emersion/go-imap"
	"github.com/emersion/go-imap/client"
)

// IMAPClient handles fetching emails from IMAP servers (e.g., Gmail).
type IMAPClient struct {
	server   string
	username string
	password string
	mailbox  string
}

// NewIMAPClient creates a new IMAP client.
func NewIMAPClient(server, username, password, mailbox string) *IMAPClient {
	return &IMAPClient{
		server:   server,
		username: username,
		password: password,
		mailbox:  mailbox,
	}
}

// Email represents a fetched email message.
type Email struct {
	Subject string
	Body    string
	From    string
	Date    string
	UID     uint32
}

// FetchUnreadTransferNotifications fetches unread emails that are likely transfer notifications.
// It filters emails by subject starting with "Uznanie rachunku" (Polish for "Account credit").
// Only processes emails with the "cargo" flag to prevent duplicate processing.
func (c *IMAPClient) FetchUnreadTransferNotifications() ([]Email, error) {
	// Connect to IMAP server
	// #nosec G402 - InsecureSkipVerify is needed for self-signed certificates in development
	cl, err := client.DialTLS(c.server, &tls.Config{InsecureSkipVerify: true})
	if err != nil {
		return nil, fmt.Errorf("failed to connect to IMAP server: %w", err)
	}
	defer func() {
		_ = cl.Logout()
	}()

	// Login
	if err := cl.Login(c.username, c.password); err != nil {
		return nil, fmt.Errorf("failed to login: %w", err)
	}

	// Select mailbox
	_, err = cl.Select(c.mailbox, false)
	if err != nil {
		return nil, fmt.Errorf("failed to select mailbox: %w", err)
	}

	// Search for messages with "cargo" flag and subject starting with "Uznanie rachunku"
	criteria := imap.NewSearchCriteria()
	criteria.WithFlags = []string{"cargo"}
	criteria.Header.Add("Subject", "Uzzan*") // IMAP wildcard for "Uznanie"

	uids, err := cl.Search(criteria)
	if err != nil {
		return nil, fmt.Errorf("failed to search: %w", err)
	}

	if len(uids) == 0 {
		return []Email{}, nil
	}

	// Fetch messages
	seqset := new(imap.SeqSet)
	seqset.AddNum(uids...)

	messages := make(chan *imap.Message, len(uids))
	err = cl.Fetch(seqset, []imap.FetchItem{imap.FetchEnvelope, imap.FetchBody}, messages)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch messages: %w", err)
	}

	var emails []Email
	for msg := range messages {
		email := c.parseMessage(msg)
		if email != nil && strings.HasPrefix(email.Subject, "Uznanie rachunku") {
			emails = append(emails, *email)
		}
	}

	// Remove "cargo" flag from processed messages
	for _, uid := range uids {
		seqset := new(imap.SeqSet)
		seqset.AddNum(uid)
		err := cl.Store(seqset, imap.RemoveFlags, []interface{}{"cargo"}, nil)
		if err != nil {
			log.Printf("Failed to remove cargo flag from message %d: %v", uid, err)
		}
	}

	log.Printf("Fetched %d transfer notification emails with cargo flag from %d total messages", len(emails), len(uids))
	return emails, nil
}

// MarkEmailsWithCargoFlag marks unread emails with the "cargo" flag.
// This is meant to be called by an external process (e.g., Gmail filter) to flag emails for processing.
func (c *IMAPClient) MarkEmailsWithCargoFlag() error {
	// Connect to IMAP server
	// #nosec G402 - InsecureSkipVerify is needed for self-signed certificates in development
	cl, err := client.DialTLS(c.server, &tls.Config{InsecureSkipVerify: true})
	if err != nil {
		return fmt.Errorf("failed to connect to IMAP server: %w", err)
	}
	defer func() {
		_ = cl.Logout()
	}()

	// Login
	if err := cl.Login(c.username, c.password); err != nil {
		return fmt.Errorf("failed to login: %w", err)
	}

	// Select mailbox
	_, err = cl.Select(c.mailbox, false)
	if err != nil {
		return fmt.Errorf("failed to select mailbox: %w", err)
	}

	// Search for unread messages with subject starting with "Uznanie rachunku"
	criteria := imap.NewSearchCriteria()
	criteria.WithoutFlags = []string{imap.SeenFlag, "cargo"}
	criteria.Header.Add("Subject", "Uzzan*") // IMAP wildcard for "Uznanie"

	uids, err := cl.Search(criteria)
	if err != nil {
		return fmt.Errorf("failed to search: %w", err)
	}

	if len(uids) == 0 {
		return nil
	}

	// Mark messages with cargo flag
	seqset := new(imap.SeqSet)
	seqset.AddNum(uids...)
	err = cl.Store(seqset, imap.AddFlags, []interface{}{"cargo"}, nil)
	if err != nil {
		return fmt.Errorf("failed to mark messages with cargo flag: %w", err)
	}

	log.Printf("Marked %d emails with cargo flag", len(uids))
	return nil
}

// parseMessage parses an IMAP message into an Email struct.
func (c *IMAPClient) parseMessage(msg *imap.Message) *Email {
	if msg.Envelope == nil {
		return nil
	}

	email := &Email{
		Subject: msg.Envelope.Subject,
		From:    msg.Envelope.From[0].Address(),
		UID:     msg.Uid,
		Date:    msg.Envelope.Date.Format("2006-01-02 15:04:05"),
	}

	// Extract body
	if msg.Body != nil {
		for _, section := range msg.Body {
			if section == nil {
				continue
			}

			r := section
			body := make([]byte, 1024*1024) // 1MB limit
			n, err := r.Read(body)
			if err != nil {
				log.Printf("Failed to read message body: %v", err)
				continue
			}
			email.Body = string(body[:n])
			break
		}
	}

	return email
}
