#!/usr/bin/env bash
set -euo pipefail

YES=0
SIZE="tiny"
while [ $# -gt 0 ]; do
  case "$1" in
    --yes) YES=1; shift ;;
    --size) SIZE="$2"; shift 2 ;;
    *) echo "upload-prod: unknown arg $1" >&2; exit 2 ;;
  esac
done

: "${PGSYNC_FIXTURE_REMOTE_DSN:?set PGSYNC_FIXTURE_REMOTE_DSN}"
if [ "$YES" -ne 1 ]; then
  echo "Refusing to upload fixture $SIZE without --yes" >&2
  exit 2
fi

echo "upload-prod: load fixtures/${SIZE}.sql.gz into a temporary database, then pg_dump/restore to the configured remote DSN" >&2
echo "upload-prod: direct upload is intentionally left as an operator script hook" >&2
