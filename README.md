# pgsync

Fast PostgreSQL prod→local sync for developers. Cross-platform single binary.

> **Status:** Phase 4 of 4 complete (embedded pg_tools scaffolding, benchmarks, release pipeline, updater). MVP implementation is in place.

## Phases
- ✅ **Phase 1 — Foundation:** repo, CI with strict 100% coverage, config (TOML + validators), runner/clock/fsx interfaces, logger, models, proxy tunnel, pgschema (FK graph + closure), pgtools locator.
- ✅ **Phase 2 — Native engine + CLI sync:** pgx-backed pipeline, cobra commands, NDJSON output, integration tests on testcontainers.
- ✅ **Phase 3 — TUI + ConfigEditor + NDJSON hardening:** default TUI entrypoint, config commands, TUI state machine shell, redacted config display, diagnostic command routing.
- ✅ **Phase 4 — Embed pg_tools + bench suite + release pipeline:** pgtools manifest/scripts, embedded locator/cache, benchmark scaffold, release workflow, updater client.

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

Phase 2/3 sync can use system `pg_dump` / PostgreSQL client tools in `PATH`; Phase 4 adds embedded pgtools manifest/extraction scaffolding for release builds.

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

## CI and releases

Every push runs build, lint, unit, race, coverage-gate, and integration jobs through GitHub Actions. After CI passes on `main`, `.github/workflows/version-bump.yml` computes the next semantic version, updates `VERSION`, creates a `vX.Y.Z` tag, and pushes it. Tags trigger `.github/workflows/release.yml`, which builds release artifacts with version metadata and publishes a GitHub Release.

See `docs/versioning.md` for bump rules.

## License

MIT — see `LICENSE`.
