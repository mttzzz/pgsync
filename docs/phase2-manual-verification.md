# Phase 2 Manual Verification

Date: 2026-05-02

## Environment

- OS: Windows
- Go: `go1.26.2 windows/amd64`
- Docker/testcontainers: available for integration harness checks
- PostgreSQL client tools on host: `pg_dump` not installed in local `PATH`
- Race detector: unavailable locally because `-race` requires cgo and this Windows environment has no C compiler (`gcc`) in `PATH`

## Commands run locally

```bash
go test ./...
go test -covermode=atomic -coverprofile=coverage.out ./internal/... ./pkg/...
bash scripts/coverage-gate.sh coverage.out coverage.allow
C:/Users/kiril/go/bin/golangci-lint run ./...
C:/Users/kiril/go/bin/golangci-lint run --build-tags=integration ./test/integration/...
go test -tags=integration -timeout=10m ./test/integration/...
go build -o bin/pgsync ./cmd/pgsync
```

## Results

- Unit tests: passed.
- Strict internal coverage gate: passed, all `internal/` symbols at 100%.
- Lint: passed with `0 issues`.
- Integration package: passed locally; pg_dump-dependent end-to-end cases skip clearly when host `pg_dump` is missing.
- Build: passed.

## CI expectations

GitHub Actions installs `postgresql-client` before integration tests, so pg_dump-dependent Phase-2 integration tests should execute on Ubuntu runners instead of skipping.

## Known limitations after Phase 2

- No full-screen TUI yet; planned for Phase 3.
- No config editor wizard yet; planned for Phase 3.
- Embedded `pg_dump` / `pg_restore` are not implemented yet; Phase 2 requires system PostgreSQL client tools in `PATH`.
- External `pgcopydb` engine is still not implemented.
- Benchmark regression gate and release packaging are still Phase 4 scope.
- Partial sync may apply full pre-data schema, so unrelated tables can exist empty on target while data copy is filtered to FK closure tables.
