package email

import "github.com/getbrevo/brevo-go/lib"

// SMTPHostFromEnv exposes SMTP host resolution for tests.
func SMTPHostFromEnv() string {
	return smtpHost()
}

// BuildHTMLMessageForTest exposes HTML message construction for tests.
func BuildHTMLMessageForTest(from, to, replyTo, subject, htmlContent string) []byte {
	return buildHTMLMessage(from, to, replyTo, subject, htmlContent)
}

// NewBrevoClientForTest constructs a client with a controlled transport.
func NewBrevoClientForTest(client *lib.APIClient) *BrevoClient {
	return &BrevoClient{client: client}
}
