#!/usr/bin/env bash
set -euo pipefail

VERSION=""
GIT_COMMIT=""
BUILD_DATE=""
DIST="dist"

usage() {
  cat <<'USAGE'
Usage: scripts/package-release.sh --version vX.Y.Z [--git-commit SHA] [--build-date RFC3339] [--dist DIR]

Builds and packages pgsync release artifacts. pgtools payloads must already be
synced into internal/engine/pgtools/bin with scripts/sync-pgtools-embed.sh.
USAGE
}

fail() { echo "package-release: $*" >&2; exit 2; }

while [ $# -gt 0 ]; do
  case "$1" in
    --version) [ $# -ge 2 ] || fail "--version requires a value"; VERSION="$2"; shift 2 ;;
    --git-commit) [ $# -ge 2 ] || fail "--git-commit requires a value"; GIT_COMMIT="$2"; shift 2 ;;
    --build-date) [ $# -ge 2 ] || fail "--build-date requires a value"; BUILD_DATE="$2"; shift 2 ;;
    --dist) [ $# -ge 2 ] || fail "--dist requires a value"; DIST="$2"; shift 2 ;;
    --help|-h) usage; exit 0 ;;
    *) fail "unknown arg: $1" ;;
  esac
done

[ -n "$VERSION" ] || fail "--version is required"
if [ -z "$GIT_COMMIT" ]; then
  GIT_COMMIT="$(git rev-parse --short HEAD 2>/dev/null || echo unknown)"
fi
if [ -z "$BUILD_DATE" ]; then
  BUILD_DATE="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
fi

check_payload() {
  platform="$1"
  dir="internal/engine/pgtools/bin/$platform"
  [ -d "$dir" ] || fail "missing embedded pgtools directory: $dir"
  case "$platform" in
    windows-*)
      [ -n "$(find "$dir" -type f -name pg_dump.exe -print -quit)" ] && [ -n "$(find "$dir" -type f -name pg_restore.exe -print -quit)" ] || fail "missing Windows pgtools payload in $dir"
      ;;
    *)
      [ -n "$(find "$dir" -type f -name pg_dump -print -quit)" ] && [ -n "$(find "$dir" -type f -name pg_restore -print -quit)" ] || fail "missing pgtools payload in $dir"
      ;;
  esac
}

ldflags="-X github.com/mttzzz/pgsync/internal/version.Version=$VERSION -X github.com/mttzzz/pgsync/internal/version.GitCommit=$GIT_COMMIT -X github.com/mttzzz/pgsync/internal/version.BuildDate=$BUILD_DATE"
rm -rf "$DIST"
mkdir -p "$DIST/work"

build_one() {
  goos="$1"
  goarch="$2"
  platform="$goos-$goarch"
  artifact="$3"
  exe_name="pgsync"
  if [ "$goos" = "windows" ]; then
    exe_name="pgsync.exe"
  fi
  check_payload "$platform"
  work="$DIST/work/$platform"
  mkdir -p "$work"
  echo "package-release: building $platform"
  CGO_ENABLED=0 GOOS="$goos" GOARCH="$goarch" go build -ldflags "$ldflags" -o "$work/$exe_name" ./cmd/pgsync
  cp README.md LICENSE "$work/"
  printf 'version=%s\ngit_commit=%s\nbuild_date=%s\n' "$VERSION" "$GIT_COMMIT" "$BUILD_DATE" > "$work/VERSION.txt"
  if [ "$goos" = "windows" ]; then
    if command -v zip >/dev/null; then
      (cd "$work" && zip -q -r "../../$artifact" "$exe_name" README.md LICENSE VERSION.txt)
    else
      python scripts/zip-release.py "$work" "$DIST/$artifact" "$exe_name" README.md LICENSE VERSION.txt
    fi
  else
    tar -C "$work" -czf "$DIST/$artifact" "$exe_name" README.md LICENSE VERSION.txt
  fi
}

build_one windows amd64 pgsync-windows-amd64.zip
build_one darwin arm64 pgsync-darwin-arm64.tar.gz
build_one darwin amd64 pgsync-darwin-amd64.tar.gz
build_one linux amd64 pgsync-linux-amd64.tar.gz
rm -rf "$DIST/work"
echo "package-release: artifacts written to $DIST"
