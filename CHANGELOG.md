# Changelog

All notable changes to pgsync. Format: [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [Unreleased]

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
