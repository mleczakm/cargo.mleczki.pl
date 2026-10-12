package email_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/getbrevo/brevo-go/lib"

	"cargo.mleczki.pl/internal/email"
)

func TestBrevoSendEmail(t *testing.T) {
	for _, status := range []int{http.StatusCreated, http.StatusBadRequest} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			var received lib.SendSmtpEmail
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost || r.URL.Path != "/smtp/email" {
					t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
				}
				if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
					t.Errorf("decode message: %v", err)
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(status)
				_, _ = w.Write([]byte(`{"messageId":"test","code":"invalid_parameter","message":"test response"}`))
			}))
			defer server.Close()
			cfg := lib.NewConfiguration()
			cfg.BasePath = server.URL
			cfg.HTTPClient = server.Client()
			client := email.NewBrevoClientForTest(lib.NewAPIClient(cfg))
			err := client.SendEmail(t.Context(), &email.EmailSender{
				Name: "Cargo", Email: "sender@example.com", ReplyTo: "reply@example.com",
			}, []email.EmailRecipient{{Name: "Customer", Email: "customer@example.com"}}, "Booking", "<p>Confirmed</p>")
			if status == http.StatusCreated && err != nil {
				t.Fatalf("send: %v", err)
			}
			if status == http.StatusBadRequest && (err == nil || !strings.Contains(err.Error(), "failed to send email")) {
				t.Fatalf("expected wrapped API error, got %v", err)
			}
			if received.Sender == nil || received.Sender.Email != "sender@example.com" || received.Sender.Name != "Cargo" ||
				len(received.To) != 1 || received.To[0].Email != "customer@example.com" || received.To[0].Name != "Customer" ||
				received.ReplyTo == nil || received.ReplyTo.Email != "reply@example.com" ||
				received.Subject != "Booking" || received.HtmlContent != "<p>Confirmed</p>" {
				t.Errorf("unexpected message: %+v", received)
			}
		})
	}
}
