package email

// SMTPHostFromEnv exposes SMTP host resolution for tests.
func SMTPHostFromEnv() string {
	return smtpHost()
}

// BuildHTMLMessageForTest exposes HTML message construction for tests.
func BuildHTMLMessageForTest(from, to, subject, htmlContent string) []byte {
	return buildHTMLMessage(from, to, subject, htmlContent)
}
