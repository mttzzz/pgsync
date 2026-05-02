#!/usr/bin/env bash
set -euo pipefail

ROOT="${1:-embed/bin}"
for platform in windows-amd64 darwin-arm64 darwin-amd64 linux-amd64; do
  dir="$ROOT/$platform"
  if [[ ! -d "$dir" ]]; then
    echo "missing pgtools dir: $dir" >&2
    exit 1
  fi
  if [[ "$platform" == windows-* ]]; then
    test -f "$dir/pg_dump.exe" && test -f "$dir/pg_restore.exe"
  else
    test -x "$dir/pg_dump" && test -x "$dir/pg_restore"
  fi
  if command -v sha256sum >/dev/null; then
    (cd "$dir" && sha256sum * > SHA256SUMS)
  elif command -v shasum >/dev/null; then
    (cd "$dir" && shasum -a 256 * > SHA256SUMS)
  else
    echo "sha256sum or shasum is required" >&2
    exit 2
  fi
done
