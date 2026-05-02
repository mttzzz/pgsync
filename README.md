# pgsync

Fast PostgreSQL prod→local sync for developers. Cross-platform single binary.

> **Status:** Phase 1 of 4 complete (foundation). CLI sync ships in Phase 2.

## Phases
- ✅ **Phase 1 — Foundation:** repo, CI with strict 100% coverage, config (TOML + validators), runner/clock/fsx interfaces, logger, models, proxy tunnel, pgschema (FK graph + closure), pgtools locator.
- ⏳ **Phase 2 — Native engine + CLI sync:** pgx-backed pipeline, cobra commands, integration tests on testcontainers.
- ⏳ **Phase 3 — TUI + ConfigEditor + NDJSON.**
- ⏳ **Phase 4 — Embed pg_tools + bench suite + release pipeline.**

See [design spec](docs/superpowers/specs/2026-05-02-pgsync-design.md) and [Phase-1 plan](docs/superpowers/plans/2026-05-02-pgsync-foundation.md).

## Build / dev

```bash
make help        # list targets
make test        # unit tests
make coverage-gate
make lint
make build
```

## License

MIT — see `LICENSE`.
