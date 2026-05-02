#!/usr/bin/env bash
# Fails if any internal/ package has < 100% line coverage,
# excluding fixed-string paths in coverage.allow.
set -euo pipefail

PROFILE="${1:-coverage.out}"
ALLOW_FILE="${2:-coverage.allow}"

if [[ ! -f "$PROFILE" ]]; then
  echo "coverage profile not found: $PROFILE" >&2
  exit 2
fi

COVERAGE_LINES="$(go tool cover -func="$PROFILE" | grep -v '^total:')"

if [[ -f "$ALLOW_FILE" ]]; then
  while IFS= read -r pattern || [[ -n "$pattern" ]]; do
    pattern="${pattern%$'\r'}"
    if [[ -z "$pattern" || "$pattern" == \#* ]]; then
      continue
    fi
    COVERAGE_LINES="$(printf '%s\n' "$COVERAGE_LINES" | grep -vF "$pattern" || true)"
  done < "$ALLOW_FILE"
fi

FAILING="$(printf '%s\n' "$COVERAGE_LINES" | awk '$NF != "100.0%" { print }')"

if [[ -n "$FAILING" ]]; then
  echo "Coverage gate FAILED — these symbols are below 100%:" >&2
  echo "$FAILING" >&2
  exit 1
fi

echo "Coverage gate PASSED — all internal/ symbols at 100%."
