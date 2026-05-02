# Changelog

All notable changes to pgsync. Format: [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [Unreleased]

### Added — Phase 2: Native engine + CLI sync
- `internal/pgdb`: safe PostgreSQL DSN construction, identifier quoting, and pgx connection adapters.
- `internal/engine`: stable sync engine contract, typed progress events, and observer fan-out.
- `internal/pgschema`: pgx catalog service for databases, tables, FKs, and owned sequences.
- `internal/engine/native`: repeatable-read snapshots, schema dump/apply, target reset, binary COPY pipeline, sequence repair, and stage orchestration.
- `internal/cli`: Cobra root and `sync <db>` command with config/env/flag resolution, confirmation safety, dry-run, text output, and NDJSON progress output.
- Testcontainers-based PostgreSQL integration harness and native sync/CLI integration tests.
- CI and Makefile targets for Phase-2 unit, coverage, race, and integration checks.

### Added — Phase 1: Foundation
- Go module skeleton, strict golangci-lint v2 profile, `make` targets.
- GitHub Actions: lint, test (matrix: linux/macOS/win), coverage-gate (100%), integration.
- `internal/config`: TOML types, atomic save/load, OS-specific paths, validators, env override.
- `internal/runner`: `CommandRunner` interface + `os/exec` impl.
- `internal/clock`: `Clock` interface + system impl.
- `internal/fsx`: `FS` interface + os impl.
- `internal/observability`: `slog` logger with text + JSON handlers.
- `internal/models`: `Database`, `Table`, `FKDep`, `SyncPlan`, `SyncResult`, `Progress`, `ProgressObserver`.
- `internal/proxy`: SOCKS5/SOCKS5h/HTTP CONNECT dialer + tunnel wrapper.
- `internal/pgschema`: FK graph + Kahn topological sort, `--tables` auto-FK closure.
- `internal/engine/pgtools`: `SystemLocator` (PATH lookup) + cross-OS binary names.
- `internal/version`: build-time identity.
- `cmd/pgsync`: minimal entrypoint that prints version.
