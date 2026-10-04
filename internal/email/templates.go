package email

import (
	"bytes"
	"fmt"
	"html/template"
)

const (
	siteURL    = "https://cargo.mleczki.pl"
	authorURL  = "https://mleczakm.github.io/platnosci-blik/"
	authorName = "Michał Mleczko"
)

var layoutTemplate = template.Must(template.New("layout").Parse(`<!doctype html>
<html lang="pl"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>{{.Title}}</title></head>
<body style="margin:0;padding:0;background:#f3f4f6;font-family:Arial,Helvetica,sans-serif;color:#1f2937">
<table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="background:#f3f4f6;padding:24px 0"><tr><td align="center">
<table role="presentation" width="600" cellpadding="0" cellspacing="0" style="max-width:600px;width:100%;background:#ffffff;border-radius:14px;overflow:hidden">
<tr><td style="height:5px;background:#10b981"></td></tr>
<tr><td style="padding:28px 32px 8px;font-size:22px;font-weight:bold;color:#111827">cargo.<span style="color:#059669">mleczki</span>.pl</td></tr>
<tr><td style="padding:8px 32px 28px;font-size:15px;line-height:1.6">
<h1 style="margin:8px 0 16px;font-size:20px;color:#111827">{{.Heading}}</h1>
{{.Body}}
</td></tr>
<tr><td style="padding:18px 32px;background:#f9fafb;font-size:12px;color:#6b7280;line-height:1.5">
Cargo Mleczki · <a href="{{.SiteURL}}" style="color:#6b7280">{{.SiteURL}}</a> · <a href="mailto:{{.ReplyTo}}" style="color:#6b7280">{{.ReplyTo}}</a><br>
Ta wiadomość została wysłana automatycznie — możesz na nią odpowiedzieć, trafi do nas.
</td></tr>
<tr><td style="padding:12px 32px 16px;font-size:11px;color:#9ca3af;line-height:1.5;border-top:1px solid #e5e7eb">
Stronę i system rezerwacji wykonał {{.AuthorName}}. Chcesz taki sklep albo system rezerwacji z płatnościami BLIK? Bez umowy z operatorem płatności i bez abonamentu.
<a href="{{.AuthorURL}}" style="color:#059669">Zobacz, jak to działa →</a>
</td></tr></table></td></tr></table></body></html>`))

var passwordResetTemplate = template.Must(template.New("reset").Parse(`<p>Dzień dobry,</p>
<p>otrzymaliśmy prośbę o zresetowanie hasła do konta w serwisie cargo.mleczki.pl. Kliknij przycisk poniżej, aby ustawić nowe hasło. Link jest ważny przez 1 godzinę.</p>
<p style="margin:24px 0"><a href="{{.URL}}" style="display:inline-block;background:#059669;color:#ffffff;text-decoration:none;padding:12px 22px;border-radius:999px;font-weight:bold">Ustaw nowe hasło</a></p>
<p style="font-size:13px;color:#6b7280">Jeśli przycisk nie działa, skopiuj ten adres do przeglądarki:<br><a href="{{.URL}}" style="color:#059669;word-break:break-all">{{.URL}}</a></p>
<p style="font-size:13px;color:#6b7280">Jeśli to nie Ty prosiłeś(-aś) o reset hasła, zignoruj tę wiadomość — Twoje hasło pozostanie bez zmian.</p>
<hr style="border:none;border-top:1px solid #e5e7eb;margin:24px 0">
<p style="font-size:13px;color:#6b7280"><strong>English:</strong> we received a request to reset your password for cargo.mleczki.pl. Use the button above to choose a new one; the link expires in 1 hour. If you did not ask for this, you can ignore this email.</p>`))

var orderNoticeTemplate = template.Must(template.New("notice").Parse(`<p>Nowe zamówienie czeka na ręczne potwierdzenie.</p>
<table role="presentation" cellpadding="0" cellspacing="0" style="width:100%;background:#ecfdf5;border-radius:10px;font-size:15px"><tr><td style="padding:16px">
<strong>ID zamówienia:</strong> {{.OrderID}}<br>
<strong>Klient:</strong> {{.UserName}} ({{.UserEmail}})<br>
<strong>Metoda płatności:</strong> {{.PaymentMethod}}<br>
<strong>Kwota:</strong> {{.Amount}} zł<br>
<strong>Typ:</strong> pierwsze zamówienie klienta — płatność przy odbiorze
</td></tr></table>
<p style="margin:24px 0"><a href="{{.AdminURL}}" style="display:inline-block;background:#111827;color:#ffffff;text-decoration:none;padding:12px 22px;border-radius:999px;font-weight:bold">Przejdź do panelu administratora</a></p>`))

type layoutData struct {
	Title, Heading        string
	Body                  template.HTML
	SiteURL, ReplyTo      string
	AuthorName, AuthorURL string
}

func renderLayout(title, heading string, body template.HTML) (string, error) {
	var out bytes.Buffer
	err := layoutTemplate.Execute(&out, layoutData{
		Title: title, Heading: heading, Body: body, SiteURL: siteURL,
		ReplyTo: DefaultReplyToEmail(), AuthorName: authorName, AuthorURL: authorURL,
	})
	if err != nil {
		return "", fmt.Errorf("render email layout: %w", err)
	}
	return out.String(), nil
}

func renderBody(tmpl *template.Template, data any, title, heading string) (string, error) {
	var body bytes.Buffer
	if err := tmpl.Execute(&body, data); err != nil {
		return "", fmt.Errorf("render %s email: %w", tmpl.Name(), err)
	}
	return renderLayout(title, heading, template.HTML(body.String())) // #nosec G203 // body was produced by html/template
}

// ResetEmailSubject is the subject of the password reset message.
const ResetEmailSubject = "Reset hasła — cargo.mleczki.pl"

// RenderPasswordReset returns the Polish (with a short English note) password reset message.
func RenderPasswordReset(resetURL string) (string, error) {
	return renderBody(passwordResetTemplate, struct{ URL string }{resetURL}, ResetEmailSubject, "Reset hasła")
}

// OrderNotice describes an order that an administrator has to confirm manually.
type OrderNotice struct {
	OrderID, UserName, UserEmail, PaymentMethod string
	Amount                                      float64
}

// RenderOrderNotice returns the administrator notification about an order awaiting confirmation.
func RenderOrderNotice(notice OrderNotice) (string, error) {
	data := struct {
		OrderNotice
		AdminURL string
		Amount   string
	}{notice, siteURL + "/admin", fmt.Sprintf("%.2f", notice.Amount)}
	return renderBody(orderNoticeTemplate, data, "Zamówienie wymaga potwierdzenia", "Zamówienie wymaga potwierdzenia")
}
