#!/bin/bash

# Generate VAPID keys and store them as GitHub Actions secrets via gh CLI.
# Usage: ./scripts/setup-vapid-secrets.sh
#
# Requires: go, gh (authenticated: gh auth login)

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
VAPID_SUBJECT="${VAPID_SUBJECT:-mailto:admin@cargo.mleczki.pl}"

echo "Generating VAPID keys..."

if ! command -v go >/dev/null 2>&1; then
	echo "Go is not installed. Install Go and try again."
	exit 1
fi

if ! command -v gh >/dev/null 2>&1; then
	echo "GitHub CLI (gh) is not installed."
	echo "Install: https://cli.github.com/"
	exit 1
fi

if ! gh auth status >/dev/null 2>&1; then
	echo "gh is not authenticated. Run: gh auth login"
	exit 1
fi

TMPDIR="$(mktemp -d)"
trap 'rm -rf "$TMPDIR"' EXIT

cat > "$TMPDIR/generate_vapid.go" <<'EOF'
package main

import (
	"fmt"

	"github.com/SherClockHolmes/webpush-go"
)

func main() {
	privateKey, publicKey, err := webpush.GenerateVAPIDKeys()
	if err != nil {
		panic(err)
	}
	fmt.Println(privateKey)
	fmt.Println(publicKey)
}
EOF

cd "$REPO_ROOT"
go run "$TMPDIR/generate_vapid.go" >"$TMPDIR/vapid_keys.txt"

VAPID_PRIVATE_KEY="$(sed -n '1p' "$TMPDIR/vapid_keys.txt")"
VAPID_PUBLIC_KEY="$(sed -n '2p' "$TMPDIR/vapid_keys.txt")"

if [[ -z "$VAPID_PRIVATE_KEY" || -z "$VAPID_PUBLIC_KEY" ]]; then
	echo "Failed to generate VAPID keys."
	exit 1
fi

REPO="$(gh repo view --json nameWithOwner -q .nameWithOwner)"

echo ""
echo "=========================================="
echo "Generated VAPID keys"
echo "Repository: $REPO"
echo "=========================================="
echo "VAPID_PRIVATE_KEY=$VAPID_PRIVATE_KEY"
echo "VAPID_PUBLIC_KEY=$VAPID_PUBLIC_KEY"
echo "VAPID_SUBJECT=$VAPID_SUBJECT"
echo "=========================================="
echo ""
echo "Setting GitHub Actions secrets with gh..."

gh secret set VAPID_PRIVATE_KEY --body "$VAPID_PRIVATE_KEY"
gh secret set VAPID_PUBLIC_KEY --body "$VAPID_PUBLIC_KEY"
gh secret set VAPID_SUBJECT --body "$VAPID_SUBJECT"

echo ""
echo "Done. Secrets VAPID_PRIVATE_KEY, VAPID_PUBLIC_KEY, and VAPID_SUBJECT are set on $REPO."
