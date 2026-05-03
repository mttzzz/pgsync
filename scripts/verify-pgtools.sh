#!/usr/bin/env bash
set -euo pipefail

ROOT="embed/bin"
MANIFEST="embed/pgtools-manifest.toml"

usage() {
  cat <<'USAGE'
Usage: scripts/verify-pgtools.sh [ROOT] [--manifest FILE]

Validates staged pgtools directories and writes SHA256SUMS for each platform.
USAGE
}

fail() { echo "verify-pgtools: $*" >&2; exit 2; }

if [ $# -gt 0 ] && [ "${1:-}" != "--manifest" ] && [ "${1:-}" != "--help" ] && [ "${1:-}" != "-h" ]; then
  ROOT="$1"
  shift
fi
while [ $# -gt 0 ]; do
  case "$1" in
    --manifest) [ $# -ge 2 ] || fail "--manifest requires a value"; MANIFEST="$2"; shift 2 ;;
    --help|-h) usage; exit 0 ;;
    *) fail "unknown arg: $1" ;;
  esac
done

[ -f "$MANIFEST" ] || fail "manifest not found: $MANIFEST"
if command -v sha256sum >/dev/null; then
  SHA256_CMD="sha256sum"
elif command -v shasum >/dev/null; then
  SHA256_CMD="shasum -a 256"
else
  fail "sha256sum or shasum is required"
fi

platforms() {
  awk '/^\[platforms\.[^]]+\]/{gsub(/^\[platforms\.|\]$/, ""); print}' "$MANIFEST"
}

field() {
  awk -v platform="$1" -v key="$2" '
    $0 == "[platforms." platform "]" {in_section=1; next}
    /^\[platforms\./ {in_section=0}
    in_section && $1 == key {sub(/^[^=]+=[[:space:]]*/, ""); gsub(/^"|"$/, ""); print; exit}
  ' "$MANIFEST"
}

list_field() {
  field "$1" "$2" | sed 's/^\[//; s/\]$//; s/,/ /g; s/"//g'
}

for platform in $(platforms); do
  dir="$ROOT/$platform"
  [ -d "$dir" ] || fail "missing pgtools dir: $dir"
  for bin in $(list_field "$platform" expected_binaries); do
    base="$(basename "$bin")"
    found="$(find "$dir" -type f -name "$base" -print -quit)"
    [ -n "$found" ] || fail "missing expected binary: $base under $dir"
    : "found $found"
  done
  tmp="$dir/SHA256SUMS.tmp"
  : > "$tmp"
  (
    cd "$dir"
    find . -type f ! -name SHA256SUMS ! -name SHA256SUMS.tmp ! -name .gitkeep -print | sort | while read -r file; do
      clean="${file#./}"
      # shellcheck disable=SC2086
      $SHA256_CMD "$clean" | awk '{print $1 "  " $2}'
    done
  ) >> "$tmp"
  mv "$tmp" "$dir/SHA256SUMS"
  echo "verify-pgtools: verified $platform"
done
