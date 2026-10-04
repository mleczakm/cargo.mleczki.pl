# Bank notifications via Cloudflare

The bank sends "Uznanie rachunku" mails to a random address `<32 hex>@cargo.mleczki.pl`.
Cloudflare Email Routing (enabled on the `cargo.mleczki.pl` subdomain only; the apex keeps Zoho)
runs the Worker in `cloudflare/bank-mail`, which checks recipient and sender, parses the MIME,
queues the message and delivers it to `POST /api/bank-mail` with an HMAC signature
(`X-Cargo-Bank-Timestamp`, `X-Cargo-Bank-Signature`).

The app (`internal/transfers/bank_mail.go`) stores the raw message and the transfer in one transaction
(repeat deliveries are ignored), then auto-pays an `awaiting_payment` BLIK order when the 4-character
payment code is in the title **and** the amount equals the order total. Anything else stays
unmatched in the admin panel ("Skrzynka wpłat") for manual linking.

## Secrets (GitHub environment `production`)
`BANK_MAIL_ADDRESS`, `BANK_MAIL_WEBHOOK_SECRET` (generate once: `scripts/setup-bank-mail-secrets.sh`; never rotated
automatically), `CLOUDFLARE_WORKER_TOKEN` (`scripts/provision-cloudflare-worker-token.mjs`), `CLOUDFLARE_ZONE_ID`,
`BREVO_API_KEY`. Optional repository variables: `BANK_MAIL_FROM` (default `powiadomienia@alior.pl`),
`BANK_MAIL_CHECK_FROM` (`0` only for tests). The address is the `BANK_MAIL_ADDRESS` secret — set it in the bank
as the recipient of incoming-transfer notifications. To read it, check `docker exec cargo printenv BANK_MAIL_ADDRESS` on the server.

## Outgoing mail
Brevo, sender `noreply@mleczki.pl` (domain authenticated), `Reply-To: do@mleczki.pl` (`MAIL_REPLY_TO`).
