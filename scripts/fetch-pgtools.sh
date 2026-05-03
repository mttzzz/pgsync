#!/usr/bin/env bash
set -euo pipefail

PLATFORM=""
ALL=0
VERSION="18.3"
OUT="embed/bin"
EMBED_OUT="internal/engine/pgtools/bin"

usage() {
  cat <<'USAGE'
Usage: scripts/fetch-pgtools.sh [--version 18.3] [--out DIR] [--embed-out DIR] (--platform NAME | --all)

Fetches PostgreSQL client tools and runtime libraries from conda-forge, then
stages them both in embed/bin/<platform>/ and internal/engine/pgtools/bin/<platform>/.
USAGE
}

fail() { echo "fetch-pgtools: $*" >&2; exit 2; }

while [ $# -gt 0 ]; do
  case "$1" in
    --platform) [ $# -ge 2 ] || fail "--platform requires a value"; PLATFORM="$2"; shift 2 ;;
    --all) ALL=1; shift ;;
    --version) [ $# -ge 2 ] || fail "--version requires a value"; VERSION="$2"; shift 2 ;;
    --out) [ $# -ge 2 ] || fail "--out requires a value"; OUT="$2"; shift 2 ;;
    --embed-out) [ $# -ge 2 ] || fail "--embed-out requires a value"; EMBED_OUT="$2"; shift 2 ;;
    --manifest) [ $# -ge 2 ] || fail "--manifest requires a value"; shift 2 ;; # accepted for backward-compatible no-op
    --help|-h) usage; exit 0 ;;
    *) fail "unknown arg: $1" ;;
  esac
done

if [ "$ALL" -eq 1 ]; then
  python scripts/fetch-pgtools-conda.py --all --version "$VERSION" --out "$OUT" --embed-out "$EMBED_OUT"
else
  [ -n "$PLATFORM" ] || fail "pass --platform <name> or --all"
  python scripts/fetch-pgtools-conda.py --platform "$PLATFORM" --version "$VERSION" --out "$OUT" --embed-out "$EMBED_OUT"
fi
