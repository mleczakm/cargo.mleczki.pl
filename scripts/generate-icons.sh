#!/bin/bash

# Render the PNG app icons (Android/iOS home screen, PWA manifest, push badge)
# from the bike logo. Needs rsvg-convert (librsvg). Re-run after changing the logo.
# Usage: ./scripts/generate-icons.sh

set -euo pipefail

OUT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)/web/static/icons"
mkdir -p "$OUT"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

command -v rsvg-convert >/dev/null || { echo "rsvg-convert is required (librsvg)"; exit 1; }

BIKE='<g fill="none" stroke="#fff" stroke-width="5" stroke-linecap="round" stroke-linejoin="round"><circle cx="18" cy="75" r="14"/><circle cx="86" cy="79" r="10"/><circle cx="18" cy="75" r="2" fill="#fff"/><circle cx="86" cy="79" r="2" fill="#fff"/><path d="M 18 75 L 86 79"/><path d="M 38 75 L 26 40"/><path d="M 18 75 L 26 40"/><path d="M 18 40 L 34 40" stroke-width="6"/><path d="M 49 76 L 46 32"/><path d="M 38 32 L 52 32" stroke-width="6"/><path d="M 47 40 L 76 52 Q 86 56, 79 78 L 49 76 Z" fill="#fff" fill-opacity="0.15"/></g>'

# icon <file> <size> <corner radius of 512> <bike scale>
# The bike spans x 4..96 and y 32..93, so its centre (50,62.5) is moved to the middle.
icon() {
	local file="$1" size="$2" radius="$3" scale="$4"
	cat >"$TMP/icon.svg" <<SVG
<svg xmlns="http://www.w3.org/2000/svg" width="512" height="512" viewBox="0 0 512 512">
<rect width="512" height="512" rx="$radius" fill="#059669"/>
<g transform="translate(256 256) scale($scale) translate(-50 -62.5)">$BIKE</g>
</svg>
SVG
	rsvg-convert -w "$size" -h "$size" "$TMP/icon.svg" -o "$OUT/$file"
}

icon icon-192.png 192 96 4.6               # purpose "any": rounded
icon icon-512.png 512 96 4.6
icon icon-maskable-512.png 512 0 3.6       # purpose "maskable": full bleed, bike inside the 80% safe zone
icon apple-touch-icon-180.png 180 0 4.2    # iOS rounds the corners itself and dislikes transparency

# Android status-bar badge: white shape on transparency.
cat >"$TMP/badge.svg" <<SVG
<svg xmlns="http://www.w3.org/2000/svg" width="96" height="96" viewBox="0 0 96 96">
<g transform="translate(48 48) scale(0.85) translate(-50 -62.5)">$BIKE</g>
</svg>
SVG
rsvg-convert -w 96 -h 96 "$TMP/badge.svg" -o "$OUT/badge-96.png"

echo "Icons written to $OUT"
