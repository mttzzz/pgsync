#!/usr/bin/env bash
# Fails if any internal/ package has < 100% line coverage,
# excluding paths in coverage.allow.
set -euo pipefail

PROFILE="${1:-coverage.out}"
ALLOW_FILE="${2:-coverage.allow}"

if [[ ! -f "$PROFILE" ]]; then
  echo "coverage profile not found: $PROFILE" >&2
  exit 2
fi

# Build a grep -F pattern from allow-list (skip blank lines and #-comments).
ALLOW_PATTERN="$(grep -vE '^(#|$)' "$ALLOW_FILE" | tr '\n' '|' | sed 's/|$//')"

# Extract per-function coverage, drop allowed paths, find any < 100.0%.
FAILING="$(go tool cover -func="$PROFILE" \
  | grep -vE "^(${ALLOW_PATTERN})" \
  | grep -v '^total:' \
  | awk '$NF != "100.0%" { print }')"

if [[ -n "$FAILING" ]]; then
  echo "Coverage gate FAILED — these symbols are below 100%:" >&2
  echo "$FAILING" >&2
  exit 1
fi

echo "Coverage gate PASSED — all internal/ symbols at 100%."
