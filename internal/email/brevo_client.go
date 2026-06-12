package email

import (
	"context"
	"fmt"
	"os"

	"github.com/getbrevo/brevo-go/lib"
)

type BrevoClient struct {
	client *lib.APIClient
}

func NewBrevoClient() (*BrevoClient, error) {
	apiKey := os.Getenv("BREVO_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("BREVO_API_KEY environment variable not set")
	}

	cfg := lib.NewConfiguration()
	cfg.AddDefaultHeader("api-key", apiKey)
	client := lib.NewAPIClient(cfg)

	return &BrevoClient{client: client}, nil
}

func (b *BrevoClient) SendEmail(ctx context.Context, sender *SendSmtpEmailSender, to []SendSmtpEmailTo, subject string, htmlContent string) error {
	senderEmail := lib.SendSmtpEmailSender{
		Name:  sender.Name,
		Email: sender.Email,
	}

	var recipients []lib.SendSmtpEmailTo
	for _, t := range to {
		recipients = append(recipients, lib.SendSmtpEmailTo{
			Email: t.Email,
			Name:  t.Name,
		})
	}

	smtpEmail := lib.SendSmtpEmail{
		Sender:      &senderEmail,
		To:          recipients,
		Subject:     subject,
		HtmlContent: htmlContent,
	}

	_, _, err := b.client.TransactionalEmailsApi.SendTransacEmail(ctx, smtpEmail)
	if err != nil {
		return fmt.Errorf("failed to send email: %w", err)
	}

	return nil
}

type SendSmtpEmailSender struct {
	Name  string
	Email string
}

type SendSmtpEmailTo struct {
	Name  string
	Email string
}
