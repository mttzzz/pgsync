#!/usr/bin/env bash
set -euo pipefail

DIST="dist"
OUT="checksums.txt"

usage() {
  cat <<'USAGE'
Usage: scripts/checksums.sh [--dist DIR] [--out FILE]

Writes SHA-256 checksums for release archives in DIST.
USAGE
}

fail() { echo "checksums: $*" >&2; exit 2; }

while [ $# -gt 0 ]; do
  case "$1" in
    --dist) [ $# -ge 2 ] || fail "--dist requires a value"; DIST="$2"; shift 2 ;;
    --out) [ $# -ge 2 ] || fail "--out requires a value"; OUT="$2"; shift 2 ;;
    --help|-h) usage; exit 0 ;;
    *) fail "unknown arg: $1" ;;
  esac
done

[ -d "$DIST" ] || fail "dist directory not found: $DIST"
if command -v sha256sum >/dev/null; then
  SHA256_CMD="sha256sum"
elif command -v shasum >/dev/null; then
  SHA256_CMD="shasum -a 256"
else
  fail "sha256sum or shasum is required"
fi

(
  cd "$DIST"
  : > "$OUT.tmp"
  for artifact in pgsync-windows-amd64.exe pgsync-windows-amd64.zip pgsync-darwin-arm64.tar.gz pgsync-darwin-amd64.tar.gz pgsync-linux-amd64.tar.gz; do
    [ -f "$artifact" ] || fail "missing release artifact: $DIST/$artifact"
    # shellcheck disable=SC2086
    $SHA256_CMD "$artifact" | awk '{print $1 "  " $2}' >> "$OUT.tmp"
  done
  mv "$OUT.tmp" "$OUT"
)
echo "checksums: wrote $DIST/$OUT"
