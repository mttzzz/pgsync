#!/usr/bin/env bash
set -euo pipefail

CURRENT="${1:-}"
if [[ -z "$CURRENT" ]]; then
  if git describe --tags --match 'v[0-9]*.[0-9]*.[0-9]*' --abbrev=0 >/dev/null 2>&1; then
    CURRENT="$(git describe --tags --match 'v[0-9]*.[0-9]*.[0-9]*' --abbrev=0 | sed 's/^v//')"
  elif [[ -f VERSION ]]; then
    CURRENT="$(tr -d '[:space:]' < VERSION)"
  else
    CURRENT="0.0.0"
  fi
fi

if [[ ! "$CURRENT" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  echo "invalid current version: $CURRENT" >&2
  exit 2
fi

IFS=. read -r MAJOR MINOR PATCH <<< "$CURRENT"
LATEST_TAG=""
if git describe --tags --match 'v[0-9]*.[0-9]*.[0-9]*' --abbrev=0 >/dev/null 2>&1; then
  LATEST_TAG="$(git describe --tags --match 'v[0-9]*.[0-9]*.[0-9]*' --abbrev=0)"
fi

RANGE="HEAD"
if [[ -n "$LATEST_TAG" ]]; then
  RANGE="$LATEST_TAG..HEAD"
fi

LOG="$(git log --format='%s%n%b' $RANGE 2>/dev/null || true)"
if [[ -z "$(echo "$LOG" | tr -d '[:space:]')" ]]; then
  echo "$CURRENT"
  exit 0
fi

if echo "$LOG" | grep -Eq 'BREAKING CHANGE|^[a-zA-Z]+(\([^)]*\))?!:'; then
  MAJOR=$((MAJOR + 1))
  MINOR=0
  PATCH=0
elif echo "$LOG" | grep -Eq '^feat(\([^)]*\))?:'; then
  MINOR=$((MINOR + 1))
  PATCH=0
else
  PATCH=$((PATCH + 1))
fi

echo "$MAJOR.$MINOR.$PATCH"
