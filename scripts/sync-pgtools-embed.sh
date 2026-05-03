#!/usr/bin/env bash
set -euo pipefail

SRC="embed/bin"
DST="internal/engine/pgtools/bin"
MANIFEST="embed/pgtools-manifest.toml"

usage() {
  cat <<'USAGE'
Usage: scripts/sync-pgtools-embed.sh [SRC] [DST] [--manifest FILE]

Mirrors verified pgtools payloads from embed/bin into the package-local tree
used by go:embed. Only manifest allow-listed files and SHA256SUMS are copied.
USAGE
}

fail() { echo "sync-pgtools-embed: $*" >&2; exit 2; }

if [ $# -gt 0 ] && [ "${1:-}" != "--manifest" ] && [ "${1:-}" != "--help" ] && [ "${1:-}" != "-h" ]; then
  SRC="$1"
  shift
fi
if [ $# -gt 0 ] && [ "${1:-}" != "--manifest" ] && [ "${1:-}" != "--help" ] && [ "${1:-}" != "-h" ]; then
  DST="$1"
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

platforms() {
  awk '/^\[platforms\.[^]]+\]/{name=$0; sub(/^\[platforms\./, "", name); sub(/].*$/, "", name); print name}' "$MANIFEST"
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

mkdir -p "$DST"
for platform in $(platforms); do
  src_dir="$SRC/$platform"
  dst_dir="$DST/$platform"
  [ -d "$src_dir" ] || fail "missing verified source dir: $src_dir"
  rm -rf "$dst_dir"
  mkdir -p "$dst_dir"
  : > "$dst_dir/.gitkeep"
  cp -R "$src_dir"/. "$dst_dir"/
  : > "$dst_dir/.gitkeep"
  echo "sync-pgtools-embed: synced $platform"
done
