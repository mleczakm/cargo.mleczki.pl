#!/usr/bin/env bash
# Generates the one-time bank mail address and webhook key as GitHub production secrets.
# Existing secrets are never rotated: GitHub cannot read them back and the bank is
# configured with the address.
set -euo pipefail

REPO="mleczakm/cargo.mleczki.pl"
ENV_NAME="production"

gh api -X PUT "repos/$REPO/environments/$ENV_NAME" --silent
EXISTING="$(gh secret list -R "$REPO" --env "$ENV_NAME" --json name -q '.[].name')"

if ! grep -qx BANK_MAIL_ADDRESS <<<"$EXISTING"; then
  printf '%s@cargo.mleczki.pl' "$(openssl rand -hex 16)" | gh secret set BANK_MAIL_ADDRESS -R "$REPO" --env "$ENV_NAME"
  echo "BANK_MAIL_ADDRESS generated"
else
  echo "BANK_MAIL_ADDRESS exists — unchanged"
fi
if ! grep -qx BANK_MAIL_WEBHOOK_SECRET <<<"$EXISTING"; then
  openssl rand -hex 32 | tr -d '\n' | gh secret set BANK_MAIL_WEBHOOK_SECRET -R "$REPO" --env "$ENV_NAME"
  echo "BANK_MAIL_WEBHOOK_SECRET generated"
else
  echo "BANK_MAIL_WEBHOOK_SECRET exists — unchanged"
fi
