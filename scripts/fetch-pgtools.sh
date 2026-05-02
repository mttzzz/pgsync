#!/usr/bin/env bash
set -euo pipefail

MANIFEST="embed/pgtools-manifest.toml"
OUT="embed/bin"
PLATFORM=""
ALL=0

while [[ $# -gt 0 ]]; do
  case "$1" in
    --manifest) MANIFEST="$2"; shift 2 ;;
    --out) OUT="$2"; shift 2 ;;
    --platform) PLATFORM="$2"; shift 2 ;;
    --all) ALL=1; shift ;;
    *) echo "unknown arg: $1" >&2; exit 2 ;;
  esac
done

command -v curl >/dev/null || { echo "curl is required" >&2; exit 2; }
command -v tar >/dev/null || { echo "tar is required" >&2; exit 2; }

if [[ $ALL -ne 1 && -z "$PLATFORM" ]]; then
  echo "pass --platform <name> or --all" >&2
  exit 2
fi

echo "fetch-pgtools: manifest=$MANIFEST out=$OUT platform=${PLATFORM:-all}"
echo "This MVP script validates tooling and staging layout; replace placeholder URLs/SHA values before release."
mkdir -p "$OUT"
