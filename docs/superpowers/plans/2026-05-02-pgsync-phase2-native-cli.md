# pgsync Native Engine + CLI Implementation Plan (Phase 2 of 4)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement the first usable non-TUI MVP path: `pgsync sync <db>` powered by the native pgx engine. Phase 2 owns catalog introspection, snapshot export, schema pre/post-data via system `pg_dump`, binary COPY data transfer, sequence repair, CLI config/flag wiring, human + NDJSON sync output, and integration tests against real PostgreSQL containers.

**Architecture:** The Phase-2 pipeline follows design spec §3.2 variant C:

1. Resolve config using `CLI flags > env vars > config file > defaults`.
2. Export a repeatable-read snapshot on the remote source DB and keep its transaction open for the full data-copy stage.
3. Run `pg_dump --section=pre-data --schema-only --no-owner --no-acl --format=plain` against the source DB and apply the SQL to the newly recreated local target DB.
4. Copy tables in FK/topological order with a bounded worker pool. Each source worker opens its own repeatable-read transaction, calls `SET TRANSACTION SNAPSHOT '<snapshot_id>'`, streams `COPY <table> TO STDOUT WITH (FORMAT binary)`, and pipes bytes into target `COPY <table> FROM STDIN WITH (FORMAT binary)`.
5. Run/apply post-data DDL (indexes, constraints, triggers).
6. Repair sequences on target with `setval`, emit final result, and return non-zero on failure.

**Phase boundary:** This phase intentionally uses **system** `pg_dump` from `$PATH` through the Phase-1 `pgtools.SystemLocator`. Embedded pg_tools extraction, releases, self-update, benchmark baselines, full TUI, and external `pgcopydb` engine remain out of scope for Phase 3/4.

**Tech Stack additions:** Go 1.25+, `jackc/pgx/v5`, `jackc/pgconn`, `jackc/pgx/v5/pgxpool`, `spf13/cobra`, `testcontainers-go`, `testcontainers-go/modules/postgres`. Keep Phase-1 dependencies (`BurntSushi/toml`, `testify`, `slog`, `x/net/proxy`).

**Reference docs:** See `docs/superpowers/specs/2026-05-02-pgsync-design.md` — sections 3 (Native pgx backend), 5 (Layout), 6 (CLI surface and NDJSON), 8 (Config override priority), 11.1–11.4 (coverage, mocking, integration tests), 12 (Modern Go), 16 (Tech stack).

**Conventions for every task:**
- TDD: failing test first, run it red, minimal implementation, run green, commit.
- Strict 100% line coverage for new/changed `internal/` packages. Any unavoidable OS/process entrypoint exception must be added to `coverage.allow` with a short comment.
- All side effects stay behind interfaces: pgx connection creation, process execution, fs, clock, logging/progress observers.
- Integration tests use `//go:build integration` and must not run in ordinary `go test ./...`.
- Do not log passwords, DSNs with passwords, or raw config structs.
- Commits use explicit file lists (`git add path/a path/b && git commit -m "..."`) — never `git add .`, `git add -A`, or `git commit -am`.
- Multi-line comments use `/* ... */`, not chained `//`.

---

## Task 1: Add Phase-2 dependencies and pgdb primitives

**Files:**
- Modify: `go.mod`
- Modify: `go.sum`
- Create: `internal/pgdb/dsn.go`
- Create: `internal/pgdb/dsn_test.go`
- Create: `internal/pgdb/ident.go`
- Create: `internal/pgdb/ident_test.go`
- Create: `internal/pgdb/conn.go`
- Create: `internal/pgdb/connector.go`
- Create: `internal/pgdb/connector_test.go`

**Purpose:** Centralize safe PostgreSQL DSN construction, identifier quoting, and fakeable pgx connection interfaces before any engine code touches the network.

- [ ] **Step 1: Add runtime/test dependencies**

```bash
cd /c/Users/kiril/projects/pgsync
go get github.com/jackc/pgx/v5@latest
go get github.com/spf13/cobra@latest
go get github.com/testcontainers/testcontainers-go@latest
go get github.com/testcontainers/testcontainers-go/modules/postgres@latest
go mod tidy
```

- [ ] **Step 2: Write failing DSN tests**

Cover:
- Remote DSN includes host, port, user, database, sslmode, and password when present.
- Local maintenance DSN defaults to database `postgres` when no database is supplied.
- Password masking returns a URL/string that never contains the secret.
- Proxy URL is ignored by DSN construction; proxy routing is handled by Phase-1 `proxy` dialer plumbing.
- Invalid/missing host or port returns a descriptive error before pgx is called.

- [ ] **Step 3: Implement `internal/pgdb/dsn.go`**

Required API:

```go
type Endpoint struct {
    Host     string
    Port     int
    User     string
    Password string
    Database string
    SSLMode  string
}

func EndpointFromConfig(c config.Connection, databaseOverride string) Endpoint
func BuildConnString(ep Endpoint) (string, error)
func MaskConnString(connString string) string
```

Implementation notes:
- Use `net/url` or `pgx.ParseConfig` roundtrip; do not hand-concatenate unescaped user/password.
- Preserve explicit database override from `sync <db>`.
- Accept `sslmode` values already validated by `config.Validate`.

- [ ] **Step 4: Write failing identifier tests**

Cover:
- `QuoteIdent("users") == "\"users\""`.
- Embedded quotes are doubled.
- `QuoteQualified(models.Table{Schema:"public", Name:"orders"}) == "\"public\".\"orders\""`.
- Empty schema/name returns error.
- `ParseTableName` accepts `users` and `public.users`, rejects `a.b.c` and blanks.

- [ ] **Step 5: Implement `internal/pgdb/ident.go`**

Required API:

```go
func QuoteIdent(s string) (string, error)
func QuoteQualified(t models.Table) (string, error)
func ParseTableName(raw string, defaultSchema string) (models.Table, error)
```

Do not use `%s` with raw user table names anywhere else in Phase 2; every SQL statement that includes an identifier must go through this package.

- [ ] **Step 6: Write failing connector tests with a fake connector**

Cover:
- Production connector stores a redacted DSN in errors.
- `Connect` passes the exact database override to `EndpointFromConfig`.
- `Close` is idempotent in the wrapper.

- [ ] **Step 7: Implement `internal/pgdb/conn.go` and `connector.go`**

Required interfaces:

```go
type Rows interface {
    Close()
    Err() error
    Next() bool
    Scan(dest ...any) error
}

type Querier interface {
    Query(ctx context.Context, sql string, args ...any) (Rows, error)
    QueryRow(ctx context.Context, sql string, args ...any) Row
    Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

type Row interface {
    Scan(dest ...any) error
}

type Conn interface {
    Querier
    Close(ctx context.Context) error
}

type CopyConn interface {
    Conn
    CopyTo(ctx context.Context, w io.Writer, sql string) (pgconn.CommandTag, error)
    CopyFrom(ctx context.Context, r io.Reader, sql string) (pgconn.CommandTag, error)
    ExecMulti(ctx context.Context, sql string) error
}

type Connector interface {
    Connect(ctx context.Context, ep Endpoint) (CopyConn, error)
}
```

Production wrapper should adapt `*pgx.Conn` / `*pgconn.PgConn`:
- `CopyTo`: `conn.PgConn().CopyTo(ctx, writer, sql)`.
- `CopyFrom`: `conn.PgConn().CopyFrom(ctx, reader, sql)`.
- `ExecMulti`: `conn.PgConn().Exec(ctx, sql).ReadAll()` to support multi-statement `pg_dump` output.

- [ ] **Step 8: Run tests and coverage**

```bash
go test -race ./internal/pgdb/...
bash scripts/coverage-gate.sh coverage.out coverage.allow
```

- [ ] **Step 9: Commit**

```bash
git add go.mod go.sum internal/pgdb/dsn.go internal/pgdb/dsn_test.go \
  internal/pgdb/ident.go internal/pgdb/ident_test.go \
  internal/pgdb/conn.go internal/pgdb/connector.go internal/pgdb/connector_test.go
git commit -m "feat(pgdb): pgx connection primitives and safe SQL identifiers"
```

---

## Task 2: Engine contract, options, events, and observer adapters

**Files:**
- Create: `internal/engine/engine.go`
- Create: `internal/engine/engine_test.go`
- Create: `internal/engine/events.go`
- Create: `internal/engine/events_test.go`
- Create: `internal/engine/observer.go`
- Create: `internal/engine/observer_test.go`

**Purpose:** Define the stable engine API used by CLI and later TUI. Keep it independent from `cobra`, pgx concrete types, and stdout.

- [ ] **Step 1: Write failing tests for option validation**

Cover:
- `PlanOptions.Validate` rejects missing remote/local endpoints, database, bad threads, unsupported engine mode.
- `Threads == 0` normalizes to `runtime.NumCPU()`.
- `Tables` are trimmed/deduplicated while preserving deterministic order.
- `DryRun` remains a plan property and must not trigger target mutations.

- [ ] **Step 2: Implement `internal/engine/engine.go`**

Required API:

```go
type Mode string

const (
    ModeAuto     Mode = "auto"
    ModeNative   Mode = "native"
    ModeExternal Mode = "external"
)

type PlanOptions struct {
    Remote            config.Connection
    Local             config.Connection
    Database          string
    Tables            []string
    Threads           int
    Mode              Mode
    UseSystemPgtools  bool
    DryRun            bool
    Yes               bool
    ConcurrentIndexes bool
    Analyze           bool
}

type Engine interface {
    Plan(ctx context.Context, opts PlanOptions) (*models.SyncPlan, error)
    Execute(ctx context.Context, plan *models.SyncPlan, observer ProgressObserver) (*models.SyncResult, error)
}
```

If Phase 1 already placed `ProgressObserver` in `internal/models`, either:
- type-alias it in `internal/engine`, or
- keep `engine.ProgressObserver` minimal and adapt from/to `models.ProgressObserver`.

Do not duplicate incompatible progress types.

- [ ] **Step 3: Write failing tests for event encoding values**

Expected event names must match spec §6:
- `sync.start`
- `schema.predata.start`
- `schema.predata.done`
- `table.copy.start`
- `table.copy.progress`
- `table.copy.done`
- `schema.postdata.start`
- `schema.postdata.done`
- `sync.done`
- `sync.failed`

- [ ] **Step 4: Implement `events.go`**

Required fields:

```go
type Event struct {
    Time       time.Time
    Level      string
    Name       string
    Stage      string
    Database   string
    Engine     string
    Table      string
    Tables     int
    Rows       int64
    Estimated  int64
    Bytes      int64
    Percent    float64
    BytesPerSec float64
    Duration   time.Duration
    Error      string
}
```

Keep event structs free of passwords and raw DSNs.

- [ ] **Step 5: Implement observer helpers**

Required API:

```go
type ProgressObserver interface {
    OnEvent(ctx context.Context, event Event)
}

type ObserverFunc func(ctx context.Context, event Event)
func (f ObserverFunc) OnEvent(ctx context.Context, event Event)

type MultiObserver []ProgressObserver
func (m MultiObserver) OnEvent(ctx context.Context, event Event)
```

Tests must cover nil observers, multiple observers, panic-free no-op behavior, and ordering.

- [ ] **Step 6: Run tests and commit**

```bash
go test -race ./internal/engine/...
git add internal/engine/engine.go internal/engine/engine_test.go \
  internal/engine/events.go internal/engine/events_test.go \
  internal/engine/observer.go internal/engine/observer_test.go
git commit -m "feat(engine): native sync contract and progress events"
```

---

## Task 3: pgx catalog service for databases, tables, FKs, and sequences

**Files:**
- Create: `internal/pgschema/service.go`
- Create: `internal/pgschema/service_test.go`
- Create: `internal/pgschema/catalog_sql.go`
- Create: `internal/pgschema/catalog_sql_test.go`
- Create: `internal/models/sequence.go`
- Create: `internal/models/sequence_test.go`

**Purpose:** Build the metadata layer that powers `Plan`: database listing, table sizes/row estimates, FK graph, and sequence ownership metadata.

- [ ] **Step 1: Write failing tests for model type**

Create `models.Sequence` with:

```go
type Sequence struct {
    Schema      string
    Name        string
    TableSchema string
    TableName   string
    ColumnName  string
}

func (s Sequence) QualifiedName() string
func (s Sequence) OwnedTable() Table
```

Tests cover schema-qualified names and empty fields.

- [ ] **Step 2: Implement catalog SQL constants**

Add constants/functions for:
- `ListDatabasesSQL()` excluding templates and system DBs unless explicitly requested.
- `ListTablesSQL()` for ordinary/partitioned tables in non-system schemas.
- `ListFKDepsSQL()` reading `pg_constraint` with `contype = 'f'`.
- `ListSequencesSQL()` using `pg_depend` / `pg_class` / `pg_attribute` to map owned sequences to table columns.

Tests should assert the SQL contains the required catalogs and filters, not exact whitespace.

- [ ] **Step 3: Write failing service tests with fake rows**

Use hand-written fake `pgdb.Rows` and `pgdb.Querier`.

Cover:
- Successful scan into `[]models.Database`.
- Table scan includes schema, name, size bytes, estimated rows.
- FK deps scan maps child table to parent table (`From` = referencing child, `To` = referenced parent).
- Sequence scan maps sequence to owned table/column.
- Query error propagates with operation name.
- Row scan error closes rows and returns an error.
- `Rows.Err()` after iteration propagates.

- [ ] **Step 4: Implement `Service`**

Required API:

```go
type Service struct { q pgdb.Querier }

func NewService(q pgdb.Querier) *Service
func (s *Service) ListDatabases(ctx context.Context) ([]models.Database, error)
func (s *Service) ListTables(ctx context.Context) ([]models.Table, error)
func (s *Service) ListFKDeps(ctx context.Context) ([]models.FKDep, error)
func (s *Service) ListSequences(ctx context.Context) ([]models.Sequence, error)
```

- [ ] **Step 5: Run unit tests**

```bash
go test -race ./internal/models/... ./internal/pgschema/...
```

- [ ] **Step 6: Commit**

```bash
git add internal/models/sequence.go internal/models/sequence_test.go \
  internal/pgschema/service.go internal/pgschema/service_test.go \
  internal/pgschema/catalog_sql.go internal/pgschema/catalog_sql_test.go
git commit -m "feat(pgschema): pgx catalog service for native planning"
```

---

## Task 4: Native snapshot manager

**Files:**
- Create: `internal/engine/native/snapshot.go`
- Create: `internal/engine/native/snapshot_test.go`

**Purpose:** Export and hold a consistent source snapshot while table-copy workers use `SET TRANSACTION SNAPSHOT`.

- [ ] **Step 1: Write failing snapshot tests with fake connection**

Cover:
- `ExportSnapshot` executes `BEGIN ISOLATION LEVEL REPEATABLE READ`, then `SELECT pg_export_snapshot()`.
- The returned handle keeps the source connection open until `Close`.
- `Close` executes `ROLLBACK` and then closes the connection.
- If export fails after `BEGIN`, the connection is rolled back/closed.
- `ApplySnapshot` starts repeatable-read transaction and executes `SET TRANSACTION SNAPSHOT $1` or safely quoted literal SQL.
- Context cancellation propagates.

- [ ] **Step 2: Implement snapshot manager**

Required API:

```go
type Snapshot struct {
    ID string
    Conn pgdb.CopyConn
}

func ExportSnapshot(ctx context.Context, conn pgdb.CopyConn) (*Snapshot, error)
func ApplySnapshot(ctx context.Context, conn pgdb.CopyConn, snapshotID string) error
func (s *Snapshot) Close(ctx context.Context) error
```

Implementation notes:
- Never log the snapshot ID at info level; debug only if needed.
- `SET TRANSACTION SNAPSHOT` must happen before any source query in the worker transaction.
- Use `errors.Join` when rollback and close both fail.

- [ ] **Step 3: Run tests and commit**

```bash
go test -race ./internal/engine/native/...
git add internal/engine/native/snapshot.go internal/engine/native/snapshot_test.go
git commit -m "feat(native): repeatable-read snapshot export and worker adoption"
```

---

## Task 5: Schema dump/apply and target database reset

**Files:**
- Create: `internal/engine/native/schema.go`
- Create: `internal/engine/native/schema_test.go`
- Create: `internal/engine/native/target.go`
- Create: `internal/engine/native/target_test.go`

**Purpose:** Delegate DDL to `pg_dump` while keeping apply fakeable and safe.

- [ ] **Step 1: Write failing tests for schema dumping**

Use fake `runner.CommandRunner` and fake `pgtools.Locator`.

Cover:
- Pre-data command includes `--section=pre-data`, `--schema-only`, `--no-owner`, `--no-acl`, `--format=plain`, and the source DB connection string.
- Post-data command includes `--section=post-data`, `--no-owner`, `--no-acl`, `--format=plain`.
- `UseSystemPgtools=false` still uses `SystemLocator` in Phase 2 but emits a clear warning/event that embedded tools are not implemented yet.
- Command errors include stderr but never include passwords.
- Empty dump output is an error for pre-data.

- [ ] **Step 2: Implement `SchemaDumper`**

Required API:

```go
type SchemaSection string

const (
    SchemaPreData  SchemaSection = "pre-data"
    SchemaPostData SchemaSection = "post-data"
)

type SchemaDumper struct {
    Runner  runner.CommandRunner
    Locator pgtools.Locator
}

func (d *SchemaDumper) Dump(ctx context.Context, source pgdb.Endpoint, section SchemaSection) (string, error)
```

Prefer passing the connection string as the final pg_dump argument. If env vars are used for password (`PGPASSWORD`), tests must verify env redaction.

- [ ] **Step 3: Write failing tests for apply/reset**

Cover:
- `ResetDatabase` connects to local maintenance DB, terminates existing target sessions, drops target DB if exists, creates target DB.
- Database names are quoted as identifiers.
- Disallow destructive reset when target database is `postgres`, `template0`, or `template1` unless a future explicit force flag exists.
- `ApplySQL` calls `ExecMulti` and wraps errors with section name.
- Progress events fire start/done around pre/post-data apply.

- [ ] **Step 4: Implement target helpers**

Required API:

```go
type TargetManager struct { Connector pgdb.Connector }

func (m *TargetManager) ResetDatabase(ctx context.Context, local config.Connection, database string) error
func ApplySQL(ctx context.Context, conn pgdb.CopyConn, section SchemaSection, sql string) error
```

Implementation notes:
- Maintenance DB default: `local.Database` if set, otherwise `postgres`.
- The target DB name is the `sync <db>` argument.
- Use `SELECT pg_terminate_backend(pid)` on `pg_stat_activity` before `DROP DATABASE`.
- Use `DROP DATABASE IF EXISTS` followed by `CREATE DATABASE`.

- [ ] **Step 5: Run tests and commit**

```bash
go test -race ./internal/engine/native/...
git add internal/engine/native/schema.go internal/engine/native/schema_test.go \
  internal/engine/native/target.go internal/engine/native/target_test.go
git commit -m "feat(native): schema dump/apply and target database reset"
```

---

## Task 6: Binary COPY pipeline with worker pool and progress

**Files:**
- Create: `internal/engine/native/copy.go`
- Create: `internal/engine/native/copy_test.go`
- Create: `internal/engine/native/progress.go`
- Create: `internal/engine/native/progress_test.go`

**Purpose:** Stream table data from source to target with `COPY ... FORMAT binary`, bounded concurrency, cancellation, and progress events.

- [ ] **Step 1: Write failing tests for progress counter**

Cover:
- Byte count increments as data passes through an `io.Reader` wrapper.
- Row estimate percent clamps to `[0,100]` and handles unknown estimate (`0`).
- Bytes/sec uses injected `clock.Clock` so tests are deterministic.
- Progress event throttling emits initial/final events even if interval suppresses middle events.

- [ ] **Step 2: Implement progress wrapper**

Required API:

```go
type ProgressReader struct { /* reader + counters */ }
func NewProgressReader(r io.Reader, opts ProgressOptions) *ProgressReader
func (p *ProgressReader) Read(buf []byte) (int, error)
func (p *ProgressReader) Bytes() int64
```

- [ ] **Step 3: Write failing tests for `CopyTable`**

Use fake source/target copy conns.

Cover:
- Source SQL is `COPY "schema"."table" TO STDOUT WITH (FORMAT binary)`.
- Target SQL is `COPY "schema"."table" FROM STDIN WITH (FORMAT binary)`.
- Source snapshot is applied before source `COPY`.
- Target copy error cancels source copy and returns the target error.
- Source copy error closes pipe and returns source error.
- Progress emits `table.copy.start`, `table.copy.done`, and at least one progress event for non-empty data.
- No goroutine leak: use a timeout in tests.

- [ ] **Step 4: Implement `CopyTable`**

Required API:

```go
type CopyTableOptions struct {
    Table      models.Table
    SnapshotID string
    Source     pgdb.CopyConn
    Target     pgdb.CopyConn
    Observer   engine.ProgressObserver
    Clock      clock.Clock
}

func CopyTable(ctx context.Context, opts CopyTableOptions) (models.SyncResult, error)
```

Implementation notes:
- Use `io.Pipe` between source and target.
- Run source and target copy in coordinated goroutines under `errgroup.WithContext`.
- Roll back/close per-table source transaction after copy.
- Target should copy inside a transaction if practical; otherwise the enclosing engine must reset database on failure.

- [ ] **Step 5: Write failing worker-pool tests**

Cover:
- Executes no more than `Threads` concurrent copies.
- Keeps result ordering deterministic for tests.
- Cancels remaining work on first error.
- Handles empty table list as a no-op.

- [ ] **Step 6: Implement `CopyTables`**

Required API:

```go
type CopyTablesOptions struct {
    Tables     []models.Table
    SnapshotID string
    Threads    int
    SourceFactory func(ctx context.Context) (pgdb.CopyConn, error)
    TargetFactory func(ctx context.Context) (pgdb.CopyConn, error)
    Observer   engine.ProgressObserver
    Clock      clock.Clock
}

func CopyTables(ctx context.Context, opts CopyTablesOptions) (*models.SyncResult, error)
```

- [ ] **Step 7: Run tests and commit**

```bash
go test -race ./internal/engine/native/...
git add internal/engine/native/copy.go internal/engine/native/copy_test.go \
  internal/engine/native/progress.go internal/engine/native/progress_test.go
git commit -m "feat(native): binary COPY pipeline with bounded workers"
```

---

## Task 7: Sequence repair on target

**Files:**
- Create: `internal/engine/native/sequences.go`
- Create: `internal/engine/native/sequences_test.go`

**Purpose:** After COPY, set target sequences to match copied table max values.

- [ ] **Step 1: Write failing sequence tests**

Cover:
- Generated SQL uses `setval(pg_get_serial_sequence(...), max(column), true)` or direct quoted sequence name safely.
- Empty table max uses `setval(..., 1, false)` semantics so the next insert starts at 1.
- Multiple sequences execute in deterministic schema/name order.
- Query/exec errors include sequence/table names and stop the stage.
- Observer emits a `sequences.done` internal event or records stage timing in final result. Do not add this event to public NDJSON unless the CLI maps it intentionally.

- [ ] **Step 2: Implement sequence repair**

Required API:

```go
func RepairSequences(ctx context.Context, conn pgdb.CopyConn, seqs []models.Sequence) error
```

Implementation notes:
- Quote all identifiers through `pgdb` helpers.
- Prefer one `SELECT setval(...)` per sequence for readable error attribution.
- Keep stage duration in `models.SyncResult.Stages["sequences"]` if Phase-1 result model supports it.

- [ ] **Step 3: Run tests and commit**

```bash
go test -race ./internal/engine/native/...
git add internal/engine/native/sequences.go internal/engine/native/sequences_test.go
git commit -m "feat(native): repair target sequences after copy"
```

---

## Task 8: NativeEngine Plan and Execute orchestration

**Files:**
- Create: `internal/engine/native/native.go`
- Create: `internal/engine/native/native_test.go`
- Create: `internal/engine/native/options.go`
- Create: `internal/engine/native/options_test.go`

**Purpose:** Wire all native stages into one engine implementation that CLI can call.

- [ ] **Step 1: Write failing constructor/options tests**

Cover:
- Missing connector/runner/locator/clock gets production defaults only in `NewDefault`; `New` requires explicit deps for tests.
- `ModeAuto` selects native in Phase 2.
- `ModeExternal` returns a clear “external engine not implemented in Phase 2” error.
- `UseSystemPgtools=false` does not fail in Phase 2; it warns and uses system tools.

- [ ] **Step 2: Implement constructors**

Required API:

```go
type Dependencies struct {
    Connector pgdb.Connector
    Runner    runner.CommandRunner
    Locator   pgtools.Locator
    Clock     clock.Clock
    Logger    *slog.Logger
}

type NativeEngine struct { deps Dependencies }

func New(deps Dependencies) (*NativeEngine, error)
func NewDefault(logger *slog.Logger) (*NativeEngine, error)
```

- [ ] **Step 3: Write failing `Plan` tests with fakes**

Cover:
- Connects to remote database from `PlanOptions.Database`.
- Lists tables/FKs/sequences.
- Applies `FKClosure` when `Tables` is non-empty.
- Applies `TopoSort` to final table list.
- Populates `models.SyncPlan` database, tables, dry-run, threads, engine.
- Rejects empty database, missing config, unsupported mode.
- Closes remote connection on success and error.

- [ ] **Step 4: Implement `Plan`**

Use Phase-1 `models.SyncPlan` if already present. If the existing model lacks fields required by execution, extend it carefully and update model tests in the same commit.

- [ ] **Step 5: Write failing `Execute` orchestration tests with stage fakes**

Because full pgx copy is covered by lower-level unit tests and integration tests, `NativeEngine.Execute` should be tested through injectable stage funcs.

Cover:
- Dry-run returns a result without resetting target or calling pg_dump.
- Happy path stage order: snapshot → dump pre-data → reset target → connect target → apply pre-data → copy → dump post-data → apply post-data → repair sequences → done.
- Every stage emits start/done/failure events with no passwords.
- Failure in any stage emits `sync.failed`, returns non-nil result with `Err`, closes snapshot and connections.
- Context cancellation stops before later stages.

- [ ] **Step 6: Implement `Execute`**

Implementation notes:
- Keep orchestration readable; split private methods if needed to satisfy lint complexity.
- Use `defer` for connection/snapshot cleanup and `errors.Join` for cleanup errors.
- On copy failure after target reset, return a clear retryable error. Do not attempt partial repair in Phase 2.
- Record stage durations in `models.SyncResult.Stages`.

- [ ] **Step 7: Run full internal tests and coverage gate**

```bash
go test -race ./internal/...
bash scripts/coverage-gate.sh coverage.out coverage.allow
```

- [ ] **Step 8: Commit**

```bash
git add internal/engine/native/native.go internal/engine/native/native_test.go \
  internal/engine/native/options.go internal/engine/native/options_test.go \
  internal/models/plan.go internal/models/plan_test.go \
  internal/models/result.go internal/models/result_test.go
git commit -m "feat(native): orchestrate plan and execute pipeline"
```

If no model files changed, omit them from `git add`.

---

## Task 9: CLI root, config resolution, and sync command

**Files:**
- Create: `internal/cli/commands.go`
- Create: `internal/cli/commands_test.go`
- Create: `internal/cli/sync.go`
- Create: `internal/cli/sync_test.go`
- Create: `internal/cli/config_resolver.go`
- Create: `internal/cli/config_resolver_test.go`
- Modify: `cmd/pgsync/main.go`
- Modify: `cmd/pgsync/main_test.go` (or create if absent)

**Purpose:** Provide the user-facing Phase-2 command: `pgsync sync <db>` with config/env/flag merge and safe confirmation behavior.

- [ ] **Step 1: Add Cobra command factory tests**

Use a fake engine factory; do not invoke real pgx.

Cover:
- `pgsync version` still works if Phase 1 provided it.
- `pgsync sync mydb --yes` calls `Plan` then `Execute`.
- `pgsync sync mydb --dry-run` calls `Plan`, prints the plan, and does not require `--yes`.
- `pgsync sync mydb` without `--yes` fails in non-interactive tests with a confirmation-required message.
- `--tables users,orders` passes `[]string{"users","orders"}`.
- Global flags `--config`, `--threads`, `--engine`, `--use-system-pgtools`, `--output`, `--quiet`, `--verbose`, `--no-color` parse correctly.
- CLI flags override env/config/defaults.
- Config file load errors surface unless all required connection fields are supplied by env/flags.
- Passwords are never printed in error output.

- [ ] **Step 2: Implement config resolver**

Required API:

```go
type Resolver struct {
    StorePath string
    Env       map[string]string
}

type FlagOverrides struct {
    ConfigPath       string
    Threads          int
    Engine           string
    UseSystemPgtools *bool
    Output           string
    Quiet            bool
    Verbose          bool
    NoColor          bool
    Remote           config.Connection
    Local            config.Connection
}

func Resolve(ctx context.Context, flags FlagOverrides) (config.Config, error)
func PlanOptionsFromConfig(cfg config.Config, db string, syncFlags SyncFlags) (engine.PlanOptions, error)
```

Reuse Phase-1 `config.Load`, `config.ApplyEnv`, and validators. If Phase-1 override code only supports env, add a narrow CLI merge function here and test priority.

- [ ] **Step 3: Implement commands**

Required API:

```go
type App struct {
    EngineFactory func(*slog.Logger) (engine.Engine, error)
    Out           io.Writer
    Err           io.Writer
    In            io.Reader
}

func NewRootCommand(app App) *cobra.Command
```

Root behavior in Phase 2:
- No args: print help and return nil if config/TUI is not implemented yet.
- `sync <db>`: native CLI path.
- `config`, `doctor`, `list`, `status`, `tui`, `text`, `upgrade`: may return “not implemented in Phase 2” with exit code 2, unless Phase 1 already implemented stubs.

- [ ] **Step 4: Wire `cmd/pgsync/main.go`**

Main should only:
- create logger defaults,
- call `cli.NewRootCommand`,
- execute command,
- `os.Exit(1)` on ordinary errors, `os.Exit(2)` on usage/not-implemented if modeled.

Keep `main.go` tiny; if coverage policy excludes it, verify `coverage.allow` already contains the entrypoint.

- [ ] **Step 5: Run CLI tests**

```bash
go test -race ./internal/cli/... ./cmd/pgsync/...
```

- [ ] **Step 6: Commit**

```bash
git add go.mod go.sum internal/cli/commands.go internal/cli/commands_test.go \
  internal/cli/sync.go internal/cli/sync_test.go \
  internal/cli/config_resolver.go internal/cli/config_resolver_test.go \
  cmd/pgsync/main.go cmd/pgsync/main_test.go
git commit -m "feat(cli): wire sync command to native engine"
```

---

## Task 10: Text and NDJSON output for sync

**Files:**
- Create: `internal/cli/plain_output.go`
- Create: `internal/cli/plain_output_test.go`
- Create: `internal/cli/agent_progress.go`
- Create: `internal/cli/agent_progress_test.go`

**Purpose:** Implement spec §6 output behavior for `pgsync sync`, including NDJSON lines suitable for automation.

- [ ] **Step 1: Write failing text output tests**

Cover:
- Sync start line includes database, table count, engine.
- Table progress updates are readable and respect `--quiet`.
- Dry-run plan output lists tables in order and includes FK auto-included marker if the plan model can represent it.
- Errors include stage/table but no password/DSN.
- `--no-color` disables ANSI escapes.

- [ ] **Step 2: Implement plain output observer**

Required API:

```go
type PlainObserver struct { /* writer, color, quiet */ }
func NewPlainObserver(w io.Writer, opts PlainOptions) *PlainObserver
func (o *PlainObserver) OnEvent(ctx context.Context, event engine.Event)
func PrintPlan(w io.Writer, plan *models.SyncPlan, opts PlainOptions) error
func PrintResult(w io.Writer, result *models.SyncResult, opts PlainOptions) error
```

- [ ] **Step 3: Write failing NDJSON tests**

Each emitted line must be valid JSON and match spec event names.

Cover exact examples semantically:
- `sync.start` has `ts`, `level`, `event`, `db`, `tables`, `engine`.
- `schema.predata.done` has `duration_ms`.
- `table.copy.progress` has `table`, `rows`, `pct`, `bytes_per_sec`.
- `sync.failed` has `level:error`, `stage`, `table`, `error`.
- `stderr` is unused for JSON progress in command tests except fatal command setup errors.
- Field order is not important, but no field may contain secrets.

- [ ] **Step 4: Implement NDJSON observer**

Required API:

```go
type NDJSONObserver struct { /* encoder/writer */ }
func NewNDJSONObserver(w io.Writer, clock clock.Clock) *NDJSONObserver
func (o *NDJSONObserver) OnEvent(ctx context.Context, event engine.Event)
```

Use one JSON object per line. Do not pretty-print.

- [ ] **Step 5: Wire observers into `sync` command**

`--output=text` → `PlainObserver`.

`--output=json` → `NDJSONObserver`; human-readable errors go to stderr only for fatal command setup errors. Engine events go to stdout.

- [ ] **Step 6: Run tests and commit**

```bash
go test -race ./internal/cli/...
git add internal/cli/plain_output.go internal/cli/plain_output_test.go \
  internal/cli/agent_progress.go internal/cli/agent_progress_test.go \
  internal/cli/sync.go internal/cli/sync_test.go
git commit -m "feat(cli): sync text and NDJSON progress output"
```

---

## Task 11: Integration test harness and deterministic fixtures

**Files:**
- Create: `test/helpers/postgres.go`
- Create: `test/helpers/postgres_test.go`
- Create: `test/helpers/assertions.go`
- Create: `test/helpers/assertions_test.go`
- Create: `test/integration/fixtures/tiny.sql`
- Create: `test/integration/fixtures/partial.sql`
- Create: `test/integration/harness_test.go`

**Purpose:** Establish reusable integration infrastructure for two Postgres containers and deterministic assertions.

- [ ] **Step 1: Create tiny fixture SQL**

`tiny.sql` should include:
- schema `public`,
- tables `users`, `orders`, `order_items`,
- FK chain `order_items → orders → users`,
- at least one sequence/identity column,
- at least one index and one check constraint,
- deterministic data rows.

`partial.sql` should include one unrelated table so partial sync can prove filtering.

- [ ] **Step 2: Implement container helper**

Required API:

```go
type PostgresContainer struct {
    Host string
    Port int
    User string
    Password string
    Database string
}

func StartPostgres(ctx context.Context, t testing.TB, database string) PostgresContainer
func (p PostgresContainer) Config() config.Connection
func ExecSQLFile(ctx context.Context, t testing.TB, pg PostgresContainer, path string)
```

Use `testcontainers-go/modules/postgres` and image `postgres:18` if available; otherwise use the newest supported `postgres` image in the local environment and document it in a test constant.

- [ ] **Step 3: Implement assertion helpers**

Required helpers:
- `AssertTableRowCountsEqual(ctx, t, source, target, tables)`.
- `AssertTableChecksumsEqual(ctx, t, source, target, tables)` using deterministic `jsonb`/text aggregation or row-to-json hashing.
- `AssertSequencesUsable(ctx, t, target, tables)` inserts a new row where practical or checks `nextval` greater than max ID.
- `AssertIndexExists(ctx, t, target, indexName)`.
- `AssertFKExists(ctx, t, target, constraintName)`.

- [ ] **Step 4: Unit-test helpers where possible**

`test/helpers` can have ordinary unit tests for SQL generation/checksum query builders without containers. Container tests must be under `test/integration` with the integration tag.

- [ ] **Step 5: Commit harness**

```bash
go test -race ./test/helpers/...
git add go.mod go.sum test/helpers/postgres.go test/helpers/postgres_test.go \
  test/helpers/assertions.go test/helpers/assertions_test.go \
  test/integration/fixtures/tiny.sql test/integration/fixtures/partial.sql \
  test/integration/harness_test.go
git commit -m "test(integration): postgres containers and fixture assertions"
```

---

## Task 12: Integration tests for native engine and CLI sync

**Files:**
- Create: `test/integration/pipeline_test.go`
- Create: `test/integration/partial_test.go`
- Create: `test/integration/cli_test.go`
- Create: `test/integration/dryrun_test.go`
- Create: `test/integration/interrupt_test.go`

**Purpose:** Prove Phase-2 MVP works against real PostgreSQL, including CLI behavior.

- [ ] **Step 1: Happy path native pipeline**

`pipeline_test.go`:
- Start source and target Postgres containers.
- Load `tiny.sql` into source DB.
- Build `native.NewDefault` with a test logger.
- `Plan` and `Execute` with `Threads=2`, `ModeNative`, `UseSystemPgtools=true`.
- Assert:
  - expected table count,
  - row counts equal,
  - per-table checksums equal,
  - indexes exist after post-data,
  - FKs exist after post-data,
  - sequences are usable,
  - result contains stage durations and no error.

- [ ] **Step 2: Partial sync with FK closure**

`partial_test.go`:
- Request only `order_items`.
- Assert plan includes `users`, `orders`, `order_items` and excludes unrelated tables.
- Assert copied data matches only included tables.
- Assert target does not contain the unrelated table if schema pre-data filtering is implemented. If Phase 2 applies full pre-data schema, document and assert unrelated table has zero rows; add a TODO in plan execution notes for schema filtering in a future phase.

Preferred Phase-2 behavior: full schema may exist, data is filtered to closure tables.

- [ ] **Step 3: CLI sync JSON mode**

`cli_test.go`:
- Build the `pgsync` binary into `t.TempDir()` with `go test` helper or invoke command in-process if subprocess is too slow.
- Use a temp TOML config file pointing source/target containers.
- Run:

```bash
pgsync --config <tmp-config> --output=json sync tiny --yes --threads=2 --use-system-pgtools
```

- Assert exit code 0.
- Parse stdout as NDJSON.
- Assert event sequence includes `sync.start`, `schema.predata.start`, `table.copy.start`, `table.copy.done`, `schema.postdata.done`, `sync.done`.
- Assert stderr is empty or only contains non-secret fatal setup text.

- [ ] **Step 4: Dry-run does not mutate target**

`dryrun_test.go`:
- Load source.
- Ensure target DB does not exist or has sentinel contents.
- Run CLI `sync tiny --dry-run`.
- Assert target DB was not dropped/created/modified.
- Assert output lists planned tables.

- [ ] **Step 5: Interrupt/cancellation behavior**

`interrupt_test.go`:
- Use a larger generated insert set inside test (not committed as a huge fixture).
- Start `sync` with a context that cancels during copy.
- Assert command returns non-zero / context cancellation error.
- Assert retrying the same sync succeeds.
- Assert failure event stage is `copy` when cancellation occurs during table copy.

If OS signal handling is not implemented in Phase 2, cancellation through context is sufficient; leave actual SIGINT subprocess coverage to Phase 3 CLI hardening.

- [ ] **Step 6: Run integration tests locally**

```bash
go test -race -tags=integration -timeout=15m ./test/integration/...
```

- [ ] **Step 7: Commit integration tests**

```bash
git add test/integration/pipeline_test.go test/integration/partial_test.go \
  test/integration/cli_test.go test/integration/dryrun_test.go \
  test/integration/interrupt_test.go
git commit -m "test(integration): native sync pipeline and CLI coverage"
```

---

## Task 13: CI/Makefile integration for Phase-2 tests

**Files:**
- Modify: `Makefile`
- Modify: `.github/workflows/ci.yml`
- Modify: `coverage.allow` only if a justified new entrypoint exception is required

**Purpose:** Make the Phase-2 test suite reproducible for contributors and CI.

- [ ] **Step 1: Update `Makefile` targets**

Required targets:
- `test`: ordinary unit tests with race and coverage.
- `test-integration`: `go test -race -tags=integration -timeout=15m ./test/integration/...`.
- `test-all`: unit + integration.
- `build`: builds `cmd/pgsync`.

If Phase 1 already created these targets, only adjust them as needed for Phase 2.

- [ ] **Step 2: Update CI integration job**

Use the existing Phase-1 CI job if present; ensure:
- Docker is available (GitHub-hosted Ubuntu runner is OK for testcontainers).
- Go version remains 1.25+.
- `pg_dump` is installed on Ubuntu. Install `postgresql-client` if the runner does not include it.
- Integration tests run with `-tags=integration -timeout=15m`.

- [ ] **Step 3: Run verification locally**

```bash
make lint
make test
make test-integration
make build
```

- [ ] **Step 4: Commit**

```bash
git add Makefile .github/workflows/ci.yml coverage.allow
git commit -m "ci: run native sync integration tests"
```

If `coverage.allow` did not change, omit it from `git add`.

---

## Task 14: Phase-2 wrap-up docs and manual verification

**Files:**
- Modify: `README.md`
- Modify: `CHANGELOG.md`
- Create: `docs/phase2-manual-verification.md`

**Purpose:** Document the usable CLI path and record manual checks before tagging the Phase-2 milestone.

- [ ] **Step 1: Update README**

Add a short Phase-2 usage section:

```bash
pgsync --config ~/.config/pgsync/config.toml sync my_database --yes --threads=8 --use-system-pgtools
pgsync --config ./ci-pgsync.toml --output=json sync my_database --yes
pgsync sync my_database --tables users,orders --dry-run
```

Document that Phase 2 requires system `pg_dump` in `$PATH`; embedded tools are Phase 4.

- [ ] **Step 2: Update CHANGELOG**

Add `Unreleased / Phase 2` entries:
- native pgx engine,
- CLI sync command,
- system pg_dump schema DDL,
- binary COPY pipeline,
- NDJSON output,
- integration tests.

- [ ] **Step 3: Add manual verification doc**

Include:
- OS and Postgres client versions.
- Commands run.
- Integration test duration.
- Known limitations:
  - no TUI yet,
  - no embedded pgtools yet,
  - no external pgcopydb engine yet,
  - no benchmark regression gate yet,
  - schema filtering for partial sync may still create empty unrelated tables if not solved in Task 12.

- [ ] **Step 4: Final verification**

```bash
golangci-lint run ./...
go test -race ./...
bash scripts/coverage-gate.sh coverage.out coverage.allow
go test -race -tags=integration -timeout=15m ./test/integration/...
go build ./cmd/pgsync
```

- [ ] **Step 5: Commit and tag milestone**

```bash
git add README.md CHANGELOG.md docs/phase2-manual-verification.md
git commit -m "docs: phase 2 native CLI usage and verification"
git tag phase-2-native-cli
```

---

## Self-Review Checklist

**Spec coverage** — every spec section that belongs in Phase 2 has a task:

| Spec § | Concern | Phase-2 task |
|---|---|---|
| 3.2 | Snapshot, pre-data, target reset, COPY, post-data, sequences | 4, 5, 6, 7, 8 |
| 3.3 | Engine abstraction and NativeEngine main path | 2, 8 |
| 5 | Layout for `internal/cli`, `internal/engine/native`, `internal/pgschema`, `test/integration` | 1–12 |
| 6 | `pgsync sync <db>`, global flags, `--tables`, `--yes`, `--dry-run`, NDJSON | 9, 10, 12 |
| 8.4 | Env/CLI override priority | 9 |
| 11.1 | Strict coverage for `internal/` | all internal tasks + 13 |
| 11.2 | Mocking side effects behind interfaces | 1, 3, 4, 5, 6, 8, 9 |
| 11.3 | Integration tests with two Postgres containers | 11, 12 |
| 11.4 | Tiny/small reproducible fixture basis | 11 |
| 12 | Modern Go conventions | all tasks, lint gate |
| 16 | pgx/cobra/testcontainers stack | 1, 9, 11, 12 |

**Out of Phase 2 scope (deferred to later plans):**
- Full-screen TUI, text-mode wizard, and config editor.
- Embedded `pg_dump` / `pg_restore` binaries and extraction cache.
- External `pgcopydb` fallback engine.
- `doctor`, `list`, `status`, `upgrade` full implementations.
- Benchmark suite, regression gate, fixture generator for medium/large/real DB.
- Release packaging/signing/self-update.
- Proxy integration test with SOCKS5 container (unless trivial after Phase-1 proxy wiring; otherwise Plan 3).

**Safety checklist:**
- [ ] No password/DSN leakage in logs, errors, text output, NDJSON, or tests.
- [ ] Target reset refuses `postgres`, `template0`, `template1`.
- [ ] Every SQL identifier from user/catalog is quoted.
- [ ] `--dry-run` never mutates target.
- [ ] Context cancellation closes pipes/connections and does not leak goroutines.
- [ ] Failed copy returns non-zero and retrying a clean sync succeeds.
- [ ] Integration tests pass on a clean machine with Docker and system `pg_dump`.

**Type consistency:**
- `models.Table.QualifiedName()` is used consistently by FK closure, topo sort, copy, and tests.
- `models.FKDep.From` is always the referencing child; `To` is the referenced parent.
- `engine.Event.Name` values match spec §6 NDJSON event strings.
- `engine.PlanOptions.Mode` values match config/runtime engine values (`auto`, `native`, `external`).
- `config.Connection.Database` override semantics are consistent: sync argument chooses source and target DB; local config database is only the maintenance DB unless explicitly documented otherwise.

**Placeholder scan:**
- [ ] No TBD/TODO/“similar to Task N” remains in implementation code.
- [ ] Every new exported type/function has a doc comment if lint requires it.
- [ ] Every created package has package documentation or lint-compatible comments.

---

## Execution Handoff

Plan complete and saved to `docs/superpowers/plans/2026-05-02-pgsync-phase2-native-cli.md`.

Recommended execution mode:

1. **Subagent-driven, one task per subagent** for Tasks 1–10 because strict coverage and interface boundaries need focused review.
2. **Integration-focused subagent** for Tasks 11–12 with Docker/testcontainers available.
3. **Final review subagent** for Tasks 13–14 to run lint, coverage, integration tests, and inspect for secret leakage.

Do not begin Phase 3 until:
- `pgsync sync <db> --yes` works end-to-end with system `pg_dump`,
- `--output=json` emits valid NDJSON events,
- full unit coverage gate passes,
- integration tests pass on at least Ubuntu + Docker.
