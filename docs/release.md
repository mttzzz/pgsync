# Release workflow

## Embedded pgtools

Release binaries are expected to embed PostgreSQL 18 `pg_dump` and `pg_restore` for the target platform. The payload flow is:

1. Install the fetch dependency once if needed: `python -m pip install zstandard`.
2. Run `make pgtools-prepare-release` or `./build.ps1 -Task pgtools-prepare-release`.
3. The fetcher resolves PostgreSQL `18.3` and runtime libraries from conda-forge for `windows-amd64`, `linux-amd64`, `darwin-amd64`, and `darwin-arm64`.
4. Confirm `internal/engine/pgtools/bin/<platform>/` contains real `pg_dump` and `pg_restore` payloads.
5. Run `make release-local VERSION=vX.Y.Z`.

`package-release.sh` fails if payloads are missing, so release jobs cannot accidentally ship an empty embedded-tools binary.

## Benchmarks

Run `make bench` for tiny/small smoke coverage. Promote reviewed benchmark JSON into `benchmarks/results/main/` only after a stable CI or release-hardware run, then update `benchmarks/results/HISTORY.md`.

## Publishing

Push a `vX.Y.Z` tag after updating `CHANGELOG.md`. The release workflow runs tests, benchmark smoke, package generation, checksum generation, and uploads the four platform archives plus `checksums.txt`.

## Rollback

If an asset is bad, delete or mark the GitHub Release as prerelease, publish a fixed patch tag, and keep updater compatibility by preserving artifact names and checksums format.
