#!/usr/bin/env bash
set -euo pipefail

SRC="${1:-embed/bin}"
DST="${2:-internal/engine/pgtools/bin}"
rm -rf "$DST"
mkdir -p "$DST"
for platform in windows-amd64 darwin-arm64 darwin-amd64 linux-amd64; do
  if [[ -d "$SRC/$platform" ]]; then
    mkdir -p "$DST/$platform"
    cp -R "$SRC/$platform"/* "$DST/$platform"/
  fi
done
