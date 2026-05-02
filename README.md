# pgsync

Fast PostgreSQL prod→local sync for developers. Cross-platform single binary.

> **Status:** Phase 3 of 4 complete (TUI shell + config commands + NDJSON/diagnostic routing). Embedded pg_tools and release pipeline ship in Phase 4.

## Phases
- ✅ **Phase 1 — Foundation:** repo, CI with strict 100% coverage, config (TOML + validators), runner/clock/fsx interfaces, logger, models, proxy tunnel, pgschema (FK graph + closure), pgtools locator.
- ✅ **Phase 2 — Native engine + CLI sync:** pgx-backed pipeline, cobra commands, NDJSON output, integration tests on testcontainers.
- ✅ **Phase 3 — TUI + ConfigEditor + NDJSON hardening:** default TUI entrypoint, config commands, TUI state machine shell, redacted config display, diagnostic command routing.
- ⏳ **Phase 4 — Embed pg_tools + bench suite + release pipeline.**

See [design spec](docs/superpowers/specs/2026-05-02-pgsync-design.md), [Phase-1 plan](docs/superpowers/plans/2026-05-02-pgsync-foundation.md), and [Phase-2 plan](docs/superpowers/plans/2026-05-02-pgsync-phase2-native-cli.md).

## Interactive usage

```bash
pgsync          # launch full-screen TUI
pgsync tui      # launch TUI explicitly
pgsync config   # open config editor flow
pgsync config show
pgsync config path
```

Config is managed by `pgsync config` and the TUI; the TOML file is internal storage.

## CLI sync usage

Phase 2/3 sync requires system `pg_dump` / PostgreSQL client tools in `PATH`. Embedded pg_tools arrive in Phase 4.

```bash
pgsync --config ~/.config/pgsync/config.toml sync my_database --yes --threads=8 --use-system-pgtools
pgsync --config ./ci-pgsync.toml --output=json sync my_database --yes  # NDJSON
pgsync sync my_database --tables users,orders --dry-run
```

## Build / dev

```bash
make help             # list targets
make test             # unit tests with coverage
make coverage-gate
make lint
make test-integration # requires Docker, pg_dump, and cgo/toolchain for -race
make build
```

## License

MIT — see `LICENSE`.
