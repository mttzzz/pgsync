#!/usr/bin/env bash
set -euo pipefail

NEXT="${1:-}"
if [[ -z "$NEXT" ]]; then
  NEXT="$(bash scripts/next-version.sh)"
fi

if [[ ! "$NEXT" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  echo "invalid next version: $NEXT" >&2
  exit 2
fi

printf '%s\n' "$NEXT" > VERSION
