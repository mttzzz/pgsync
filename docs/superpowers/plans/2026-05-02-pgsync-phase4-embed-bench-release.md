# pgsync Phase 4 Implementation Plan — Embedded pg_tools, Benchmarks, Release Pipeline, Updater

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:subagent-driven-development` (recommended) or `superpowers:executing-plans` to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Finish MVP distribution quality: embed PostgreSQL 18 `pg_dump` / `pg_restore` per platform, extract them safely at runtime, wire `--use-system-pgtools`, add reproducible fixtures and benchmark regression gates, package signed/checksummed release artifacts, and implement `pgsync upgrade` against GitHub Releases.

**Architecture:** Phase 4 assumes Phases 1–3 already provide the Go module, strict coverage gate, CLI/TUI, config, `runner`/`fsx`/`clock` abstractions, pgx-native sync engine, schema parser, progress observers, and integration tests. This phase keeps the same rule: every side effect is behind an interface and every new line in `internal/` is covered.

**Tech Stack:** Go 1.25+, stdlib `embed`, build tags, `log/slog`, `testing.B`, `golang.org/x/perf/cmd/benchstat`, GitHub Actions, GitHub Releases API. No `goreleaser` for MVP; use explicit scripts/Make targets as in the spec.

**Reference docs:** See `docs/superpowers/specs/2026-05-02-pgsync-design.md` — sections 4 (Embedded `pg_dump` / `pg_restore`), 6 (`pgsync upgrade`, `--use-system-pgtools`), 11.1–11.5 (strict coverage, extraction tests, fixtures, benchmarks), 13 (distribution + release pipeline), 15 (`internal/updater/` port from dbsync), and 16 (tech stack).

**Phase 4 prerequisites:**
- Native sync engine can execute schema pre/post-data using located `pg_dump` / `pg_restore` paths.
- CLI has root flags, `doctor`, `sync`, `version`, and placeholder or existing `upgrade` command wiring.
- `Makefile`, `build.ps1`, `.github/workflows/ci.yml`, `coverage.allow`, and strict lint config exist.
- Integration tests can create source/target Postgres 18 containers.

**Conventions for every task:**
- TDD: failing test first, run it red, minimal implementation, run green, commit.
- Strict 100% coverage for `internal/`; update `coverage.allow` only for platform-specific embed files allowed by the spec.
- Use hand-written fakes in `_test.go`; no gomock.
- Keep actual PostgreSQL binary payloads out of git unless a later explicit product decision changes this.
- Commands must use explicit file lists in `git add`; never `git add .`, `git add -A`, or `git commit -am`.
- Multi-line comments use `/* ... */`, not chained `//` comments.
- Download-on-first-run remains future work. MVP embeds tools at build time and extracts offline at runtime.

---

## Task 1: PostgreSQL 18 pg_tools manifest and fetch/verify scripts

**Purpose:** Define the exact PostgreSQL 18 binary inputs for every release platform and make CI/developers able to populate the embed staging directories reproducibly with SHA-256 verification.

**Files:**
- Create: `C:\Users\kiril\projects\pgsync\embed\pgtools-manifest.toml`
- Create: `C:\Users\kiril\projects\pgsync\scripts\fetch-pgtools.sh`
- Create: `C:\Users\kiril\projects\pgsync\scripts\verify-pgtools.sh`
- Create: `C:\Users\kiril\projects\pgsync\scripts\sync-pgtools-embed.sh`
- Modify: `C:\Users\kiril\projects\pgsync\.gitignore`
- Modify: `C:\Users\kiril\projects\pgsync\Makefile`
- Modify: `C:\Users\kiril\projects\pgsync\build.ps1`
- Optional test helper: `C:\Users\kiril\projects\pgsync\scripts\testdata\pgtools-manifest.invalid.toml`

- [ ] **Step 1: Add `embed/pgtools-manifest.toml`**
  - Schema version: `1`.
  - Tool version: PostgreSQL `18.x` selected for MVP.
  - One entry per platform: `windows-amd64`, `darwin-arm64`, `darwin-amd64`, `linux-amd64`.
  - Each entry includes:
    - `url` for the official binary archive source.
    - `archive_sha256`.
    - `files` allow-list copied into the embed staging directory.
    - `expected_binaries`: `pg_dump` / `pg_restore` with `.exe` suffix on Windows.
  - Include comments stating that URLs/SHAs must be refreshed intentionally when PostgreSQL patch version changes.

- [ ] **Step 2: Implement `scripts/fetch-pgtools.sh`**
  - Inputs:
    - `--platform <platform>` or `--all`.
    - `--manifest embed/pgtools-manifest.toml` default.
    - `--out embed/bin` default.
  - Behavior:
    - Download each archive into a temp directory.
    - Verify `archive_sha256` before extraction.
    - Extract only allow-listed files.
    - Normalize layout to `embed/bin/<platform>/...`.
    - Mark extracted files read-only where practical to prevent accidental edits.
    - Print clear errors for missing `curl`, missing `sha256sum`/`shasum`, bad platform, bad checksum, or missing allow-listed file.
  - Keep the script POSIX-compatible enough for Linux/macOS CI; Windows release jobs may run it under Git Bash.

- [ ] **Step 3: Implement `scripts/verify-pgtools.sh`**
  - Validate that every manifest platform directory exists under `embed/bin/<platform>`.
  - Validate every allow-listed file exists.
  - Recompute per-file SHA-256 and emit `embed/bin/<platform>/SHA256SUMS`.
  - Fail if `pg_dump` or `pg_restore` is missing or not executable on Unix platforms.
  - This script is a release gate before `make build-all`.

- [ ] **Step 4: Implement `scripts/sync-pgtools-embed.sh`**
  - Because Go `//go:embed` cannot embed files outside its package tree, mirror verified payloads from `embed/bin/<platform>/` into `internal/engine/pgtools/bin/<platform>/`.
  - Delete stale files in the package-local staging directory before copying.
  - Copy only allow-listed files from the manifest.
  - Preserve executable bits on Unix platforms.
  - Generate or copy per-platform `SHA256SUMS` into the staging directory.
  - Keep both `embed/bin/` and `internal/engine/pgtools/bin/` ignored by git except for `.gitkeep` placeholders if needed.

- [ ] **Step 5: Update `.gitignore`**
  - Continue ignoring downloaded payloads:
    - `embed/bin/*/`
    - `internal/engine/pgtools/bin/*/`
  - Keep the manifest and scripts tracked.
  - Do not ignore `benchmarks/results/main/`; Phase 4 needs committed benchmark baselines. Keep local/per-SHA benchmark outputs ignored unless explicitly promoted.

- [ ] **Step 6: Add Makefile/build.ps1 targets**
  - `make pgtools-fetch PLATFORM=linux-amd64`
  - `make pgtools-fetch-all`
  - `make pgtools-verify`
  - `make pgtools-sync-embed`
  - `make pgtools-prepare-release` = fetch all + verify + sync embed.
  - Equivalent PowerShell functions/targets in `build.ps1` for Windows contributors.

- [ ] **Step 7: Verification**
  - Run script help paths:
    - `bash scripts/fetch-pgtools.sh --help`
    - `bash scripts/verify-pgtools.sh --help`
    - `bash scripts/sync-pgtools-embed.sh --help`
  - Run negative tests manually with a copied manifest containing a bogus SHA.
  - Run:
    - `make pgtools-fetch PLATFORM=linux-amd64`
    - `make pgtools-verify`
    - `make pgtools-sync-embed`
  - Confirm no binary payloads appear in `git status --short`.

- [ ] **Step 8: Commit**

```bash
cd /c/Users/kiril/projects/pgsync
git add embed/pgtools-manifest.toml \
  scripts/fetch-pgtools.sh scripts/verify-pgtools.sh scripts/sync-pgtools-embed.sh \
  .gitignore Makefile build.ps1
git commit -m "build: add verified PostgreSQL tools fetch pipeline"
```

---

## Task 2: Build-tagged embedded pg_tools filesystem

**Purpose:** Make each release binary carry only the PostgreSQL tools for its own `GOOS/GOARCH`, while unsupported platforms fail clearly.

**Files:**
- Create: `C:\Users\kiril\projects\pgsync\internal\engine\pgtools\embed.go`
- Create: `C:\Users\kiril\projects\pgsync\internal\engine\pgtools\embed_windows_amd64.go`
- Create: `C:\Users\kiril\projects\pgsync\internal\engine\pgtools\embed_darwin_arm64.go`
- Create: `C:\Users\kiril\projects\pgsync\internal\engine\pgtools\embed_darwin_amd64.go`
- Create: `C:\Users\kiril\projects\pgsync\internal\engine\pgtools\embed_linux_amd64.go`
- Create: `C:\Users\kiril\projects\pgsync\internal\engine\pgtools\embed_unsupported.go`
- Create: `C:\Users\kiril\projects\pgsync\internal\engine\pgtools\embed_test.go`
- Modify: `C:\Users\kiril\projects\pgsync\coverage.allow`

- [ ] **Step 1: Define the embed abstraction**
  - Add an unexported `embeddedBundle` struct containing:
    - `Platform string`
    - `Version string`
    - `FS fs.FS`
    - `Root string`
    - `Available bool`
  - Add exported or package-level functions:
    - `EmbeddedBundle() embeddedBundle`
    - `EmbeddedAvailable() bool`
    - `EmbeddedPlatform() string`
  - Return deterministic errors for unsupported platforms.

- [ ] **Step 2: Add platform-specific files**
  - Each supported file has build tags for exactly one platform, for example `//go:build linux && amd64`.
  - Each file embeds `bin/<platform>/*` from `internal/engine/pgtools/bin/<platform>/`.
  - Each file returns `Platform` as `<goos>-<goarch>` matching release asset names.
  - Windows embeds `pg_dump.exe`, `pg_restore.exe`, and required DLLs.
  - Darwin/Linux embeds `pg_dump`, `pg_restore`, `libpq` and required runtime libraries.

- [ ] **Step 3: Add unsupported fallback**
  - Build tag excludes all supported combinations.
  - `EmbeddedAvailable()` returns false.
  - Locator errors mention the exact `runtime.GOOS/runtime.GOARCH` and recommend `--use-system-pgtools`.

- [ ] **Step 4: Update `coverage.allow`**
  - Add only the platform-specific embed files allowed by spec §11.1.
  - Do not allow-list `extract.go`, `locate.go`, updater, benchmarks helpers, or scripts.

- [ ] **Step 5: Tests**
  - Unit test current-platform bundle metadata.
  - Unit test unsupported error path via a small pure function that accepts injected GOOS/GOARCH strings; do not rely on cross-compiling inside the test.
  - Ensure test does not require real PostgreSQL payloads in normal unit-test runs by separating metadata logic from `//go:embed` file availability where needed.

- [ ] **Step 6: Verification**
  - After `make pgtools-prepare-release`, run:
    - `go test ./internal/engine/pgtools`
    - `go test ./internal/...`
    - `bash scripts/coverage-gate.sh coverage.out coverage.allow`
  - Cross-compile smoke after payload sync:
    - `GOOS=linux GOARCH=amd64 go build ./cmd/pgsync`
    - `GOOS=windows GOARCH=amd64 go build ./cmd/pgsync`
    - `GOOS=darwin GOARCH=amd64 go build ./cmd/pgsync`
    - `GOOS=darwin GOARCH=arm64 go build ./cmd/pgsync`

- [ ] **Step 7: Commit**

```bash
git add internal/engine/pgtools/embed.go \
  internal/engine/pgtools/embed_windows_amd64.go \
  internal/engine/pgtools/embed_darwin_arm64.go \
  internal/engine/pgtools/embed_darwin_amd64.go \
  internal/engine/pgtools/embed_linux_amd64.go \
  internal/engine/pgtools/embed_unsupported.go \
  internal/engine/pgtools/embed_test.go \
  coverage.allow
git commit -m "feat: add platform-specific embedded pgtools bundles"
```

---

## Task 3: Runtime extraction cache with hash validation and macOS codesign hook

**Purpose:** Extract embedded tools once into a content-addressed cache, recover from corrupt cache state, and return absolute executable paths to the engine.

**Files:**
- Create: `C:\Users\kiril\projects\pgsync\internal\engine\pgtools\extract.go`
- Create: `C:\Users\kiril\projects\pgsync\internal\engine\pgtools\cache_path.go`
- Create: `C:\Users\kiril\projects\pgsync\internal\engine\pgtools\signer.go`
- Create: `C:\Users\kiril\projects\pgsync\internal\engine\pgtools\extract_test.go`
- Create: `C:\Users\kiril\projects\pgsync\internal\engine\pgtools\cache_path_test.go`
- Create: `C:\Users\kiril\projects\pgsync\internal\engine\pgtools\signer_test.go`
- Modify only if required by existing abstractions: `C:\Users\kiril\projects\pgsync\internal\fsx\fs.go`, `C:\Users\kiril\projects\pgsync\internal\fsx\os.go`, `C:\Users\kiril\projects\pgsync\internal\fsx\os_test.go`

- [ ] **Step 1: Define extraction API**
  - `type Extractor struct { FS ..., Runner ..., Logger ..., CacheRoot ..., Signer ... }`.
  - `func (e *Extractor) Ensure(ctx context.Context, bundle embeddedBundle) (Paths, error)`.
  - `Paths` includes:
    - `PgDump string`
    - `PgRestore string`
    - `Root string`
    - `Hash string`
    - `Platform string`
  - Do not expose temp directory internals.

- [ ] **Step 2: Compute bundle hash**
  - Hash every embedded file path and content in sorted path order.
  - Include platform and PostgreSQL tool version in the hash input.
  - Use hex SHA-256; cache directory is `<cache-root>/<hash>/`.
  - Tests prove hash is deterministic regardless of input map/read order.

- [ ] **Step 3: Cache paths**
  - Unix default per spec: `~/.pgsync/cache/<hash>/`.
  - Windows default per spec: `%LOCALAPPDATA%\pgsync\cache\<hash>\`.
  - Permit explicit `CacheRoot` for tests and portable runs.
  - Return clear errors when home/local-app-data cannot be resolved.

- [ ] **Step 4: Extract atomically**
  - If `<hash>/.installed` exists and validates, return existing paths without rewriting files.
  - If missing/corrupt:
    - Extract into sibling temp dir.
    - Create directories with `0700` where supported.
    - Write files with `0600`, then chmod executables/libraries to `0755` on Unix.
    - Write `.installed` last with JSON metadata: schema version, platform, pgtools version, hash, file list, created_at.
    - Rename temp dir to final dir.
  - On failure, remove temp dir and return joined contextual errors.

- [ ] **Step 5: Recover from corrupt cache**
  - Validate existing cache by checking:
    - `.installed` parses.
    - metadata hash/platform/version match current bundle.
    - `pg_dump` and `pg_restore` exist.
    - all extracted file hashes match metadata or bundled source.
  - If validation fails, remove the final cache dir and re-extract.
  - Tests cover missing marker, malformed marker, missing executable, bad file content, and failed cleanup.

- [ ] **Step 6: macOS codesign hook**
  - Define `Signer` interface:
    - no-op implementation for non-darwin.
    - darwin implementation invokes `codesign --sign - --identifier dev.pgsync.bundled <path>` where required.
  - Use `runner.CommandRunner` to execute `codesign`, not `os/exec` directly.
  - Codesign only executable files, after chmod, before `.installed`.
  - Tests use fake runner and injected platform string to cover success/failure without running actual `codesign`.

- [ ] **Step 7: Tests**
  - Idempotent extraction: second `Ensure` does not rewrite files.
  - Concurrent extraction: two goroutines for same hash do not leave partial state. If cross-process locks are not implemented, use atomic rename + revalidation and document process-level race behavior.
  - Permission behavior on Unix is asserted where supported; skip only platform-specific permission assertions with explicit conditions.
  - Unsupported bundle returns actionable error.
  - Runner/signing failures include stderr/context.

- [ ] **Step 8: Verification**
  - `go test -race ./internal/engine/pgtools`
  - `go test ./internal/...`
  - `make coverage`
  - Manually run a local built binary with embedded tools and verify cache created once:
    - `pgsync doctor --verbose`
    - `pgsync doctor --verbose` again; second run should report cached tools.

- [ ] **Step 9: Commit**

```bash
git add internal/engine/pgtools/extract.go \
  internal/engine/pgtools/cache_path.go \
  internal/engine/pgtools/signer.go \
  internal/engine/pgtools/extract_test.go \
  internal/engine/pgtools/cache_path_test.go \
  internal/engine/pgtools/signer_test.go \
  internal/fsx/fs.go internal/fsx/os.go internal/fsx/os_test.go
git commit -m "feat: extract embedded pgtools into verified runtime cache"
```

---

## Task 4: pgtools locator integration and `--use-system-pgtools` behavior

**Purpose:** Make the engine and diagnostics choose embedded pg_tools by default, while preserving an explicit system-tools escape hatch.

**Files:**
- Modify: `C:\Users\kiril\projects\pgsync\internal\engine\pgtools\locate.go`
- Modify/Create: `C:\Users\kiril\projects\pgsync\internal\engine\pgtools\locate_test.go`
- Modify: `C:\Users\kiril\projects\pgsync\internal\cli\commands.go`
- Modify: `C:\Users\kiril\projects\pgsync\internal\cli\doctor.go`
- Modify: `C:\Users\kiril\projects\pgsync\internal\cli\sync.go`
- Modify: `C:\Users\kiril\projects\pgsync\internal\config\config.go`
- Modify/Create: relevant CLI/config tests under `internal/cli/` and `internal/config/`

- [ ] **Step 1: Define locator options**
  - `UseSystem bool` mirrors config/CLI `use_system_pgtools`.
  - `CacheRoot string` for tests and advanced debug use.
  - `System Locator` for existing PATH lookup.
  - `Extractor *Extractor` for embedded mode.
  - `Logger *slog.Logger` for debug events.

- [ ] **Step 2: Implement selection rules**
  - If `UseSystem` is true:
    - Require both `pg_dump` and `pg_restore` from PATH.
    - Return exact system paths.
    - Do not touch embedded cache.
  - If `UseSystem` is false:
    - Require embedded bundle availability.
    - Extract/validate cache.
    - Return embedded paths.
  - If embedded unavailable on an unsupported platform:
    - Error recommends installing PostgreSQL client tools and rerunning with `--use-system-pgtools`.

- [ ] **Step 3: Wire CLI/config**
  - Confirm root flag `--use-system-pgtools` overrides env/config for one run only.
  - Ensure `Runtime.UseSystemPgtools` is not changed by one-shot flag overrides.
  - In `sync`, pass resolved locator paths into schema dump/restore code.
  - In `doctor`, show:
    - mode: `embedded` or `system`.
    - `pg_dump` path.
    - `pg_restore` path.
    - PostgreSQL tools version from `pg_dump --version` if available.
    - cache root/hash for embedded mode.

- [ ] **Step 4: Tests**
  - Embedded default path selected when config false and no CLI override.
  - System mode selected when flag true.
  - Env/config/flag precedence remains `CLI > env > config > defaults`.
  - Missing system `pg_restore` fails with clear message.
  - Embedded extractor failure propagates stage/context.
  - `doctor --output=json` redacts nothing sensitive but emits machine-parseable pgtools fields if JSON output exists from Phase 3.

- [ ] **Step 5: Verification**
  - `go test ./internal/engine/pgtools ./internal/cli ./internal/config`
  - `go test ./internal/...`
  - Manual smoke:
    - `pgsync doctor`
    - `pgsync doctor --use-system-pgtools`
    - `pgsync sync <tiny-db> --dry-run --use-system-pgtools`

- [ ] **Step 6: Commit**

```bash
git add internal/engine/pgtools/locate.go internal/engine/pgtools/locate_test.go \
  internal/cli/commands.go internal/cli/doctor.go internal/cli/sync.go \
  internal/config/config.go internal/cli/*_test.go internal/config/*_test.go
git commit -m "feat: use embedded pgtools by default with system override"
```

---

## Task 5: Reproducible fixture generator and fixture management scripts

**Purpose:** Provide deterministic tiny/medium/large fixtures from code, plus public small fixture setup, so benchmarks and integration tests do not depend on production databases.

**Files:**
- Create: `C:\Users\kiril\projects\pgsync\fixtures\genfixture\main.go`
- Create: `C:\Users\kiril\projects\pgsync\fixtures\genfixture\generator.go`
- Create: `C:\Users\kiril\projects\pgsync\fixtures\genfixture\schema.go`
- Create: `C:\Users\kiril\projects\pgsync\fixtures\genfixture\generator_test.go`
- Create/Modify: `C:\Users\kiril\projects\pgsync\fixtures\README.md`
- Create: `C:\Users\kiril\projects\pgsync\fixtures\download-dvdrental.sh`
- Create: `C:\Users\kiril\projects\pgsync\fixtures\upload-prod.sh`
- Create/Modify: `C:\Users\kiril\projects\pgsync\test\helpers\fixtures.go`
- Modify: `C:\Users\kiril\projects\pgsync\Makefile`

- [ ] **Step 1: Implement `pgsync-genfixture`**
  - Invocation:
    - `go run ./fixtures/genfixture --size=tiny --seed=42 --out=fixtures/tiny.sql.gz`
    - `go run ./fixtures/genfixture --size=medium --seed=42 --out=fixtures/medium.sql.gz`
    - `go run ./fixtures/genfixture --size=large --seed=42 --out=fixtures/large.sql.gz`
  - Sizes:
    - `tiny`: ~50 KB, 3 tables, FK chain, ~500 rows, 1 sequence.
    - `medium`: ~500 MB, 50 tables, jsonb, arrays, enums, partial indexes, GIN, ~2M rows.
    - `large`: ~5 GB, 100 tables, partitioned tables, materialized views, GIN/jsonb, FK cascades, ~50M rows.
  - Output is deterministic `.sql.gz` for a fixed seed.
  - Generated SQL must target PostgreSQL 18.

- [ ] **Step 2: Model fixture metadata**
  - Emit a sidecar JSON file next to each generated fixture with:
    - size.
    - seed.
    - schema version.
    - expected table count.
    - expected row count per table.
    - expected sequence names.
  - Integration/benchmark loaders use this metadata for verification.

- [ ] **Step 3: Add small fixture downloader**
  - `fixtures/download-dvdrental.sh` downloads and normalizes public dvdrental SQL into `fixtures/dvdrental.sql.gz`.
  - Verify checksum of the downloaded archive.
  - Do not commit downloaded SQL unless product decision changes; document the command.

- [ ] **Step 4: Add `fixtures/upload-prod.sh`**
  - One-shot helper to upload generated fixtures to a configured remote cluster under `pgsync_fixture_<size>` database names.
  - Require explicit confirmation unless `--yes`.
  - Read connection parameters from env; do not parse app config or store secrets.

- [ ] **Step 5: Add test helpers**
  - `test/helpers/fixtures.go` exposes fixture generation/loading helpers for integration tests and benchmarks.
  - Helpers should be context-aware and return cleanup functions.
  - Avoid shelling out where a Go helper can call generator code directly.

- [ ] **Step 6: Tests**
  - Generator determinism: same seed produces identical gzip content after normalizing gzip timestamp/name fields.
  - Different seed changes data but not schema shape.
  - Tiny fixture loads successfully into a Postgres 18 testcontainer and metadata row counts match.
  - Bad size/negative seed/unwritable output return clear errors.

- [ ] **Step 7: Make targets**
  - `make fixture-tiny`
  - `make fixture-medium`
  - `make fixture-large`
  - `make fixture-small` for dvdrental download.
  - `make fixtures` runs tiny + small and prints how to generate medium/large manually.

- [ ] **Step 8: Verification**
  - `go test ./fixtures/genfixture ./test/helpers`
  - `go test -tags=integration ./test/integration/...` with tiny fixture.
  - `make fixture-tiny`
  - Confirm generated SQL/gzip outputs remain ignored by git.

- [ ] **Step 9: Commit**

```bash
git add fixtures/genfixture/main.go fixtures/genfixture/generator.go \
  fixtures/genfixture/schema.go fixtures/genfixture/generator_test.go \
  fixtures/README.md fixtures/download-dvdrental.sh fixtures/upload-prod.sh \
  test/helpers/fixtures.go Makefile
git commit -m "test: add deterministic PostgreSQL fixture generator"
```

---

## Task 6: Benchmark result model, writer, and comparison CLI

**Purpose:** Establish the JSON format from the spec and a deterministic comparison command for CI regression gates.

**Files:**
- Create: `C:\Users\kiril\projects\pgsync\benchmarks\result.go`
- Create: `C:\Users\kiril\projects\pgsync\benchmarks\result_test.go`
- Create: `C:\Users\kiril\projects\pgsync\benchmarks\host.go`
- Create: `C:\Users\kiril\projects\pgsync\benchmarks\host_test.go`
- Create: `C:\Users\kiril\projects\pgsync\benchmarks\writer.go`
- Create: `C:\Users\kiril\projects\pgsync\benchmarks\writer_test.go`
- Create: `C:\Users\kiril\projects\pgsync\benchmarks\compare.go`
- Create: `C:\Users\kiril\projects\pgsync\benchmarks\compare_test.go`
- Create: `C:\Users\kiril\projects\pgsync\benchmarks\results\HISTORY.md`

- [ ] **Step 1: Implement result schema**
  - Match spec §11.5 exactly:
    - `schema_version`
    - `fixture`
    - `git_sha`
    - `git_dirty`
    - `host`
    - `engine`
    - `threads`
    - `duration_ms`
    - `stages_ms`
    - `throughput`
    - `rows_total`
    - `bytes_total`
    - `tables`
    - `memory`
  - Add JSON tags and validation for required fields.
  - Keep schema version at `1`.

- [ ] **Step 2: Capture host metadata**
  - OS/arch from `runtime`.
  - CPU model via best-effort platform-specific detection behind injectable readers/functions.
  - Core count from `runtime.NumCPU()`.
  - RAM GB best-effort; if unavailable, return `0` plus no error.
  - Tests inject fake `/proc/cpuinfo`, sysctl output, or Windows command output; no production command execution in unit tests without an interface.

- [ ] **Step 3: Write benchmark output**
  - Default output directory: `benchmarks/results/<git-sha>/`.
  - Allow override with `PGSYNC_BENCH_OUT`.
  - File name: `<fixture>.json`.
  - Atomic write: temp file then rename.
  - Stable pretty JSON for readable diffs.

- [ ] **Step 4: Implement `benchmarks/compare.go`**
  - CLI usage:
    - `go run ./benchmarks/compare.go --baseline benchmarks/results/main --candidate benchmarks/results/<sha> --threshold 0.15`
  - Compare each fixture present in baseline.
  - Fail if:
    - `duration_ms > baseline * (1 + threshold)`.
    - `throughput.rows_per_sec < baseline * (1 - threshold)`.
    - `throughput.bytes_per_sec < baseline * (1 - threshold)`.
  - Emit human-readable summary and JSON optional output for CI.
  - Exit code `0` pass, `1` regression, `2` usage/read/schema error.

- [ ] **Step 5: Initialize `HISTORY.md`**
  - Explain how baselines are promoted.
  - Include an empty table with columns: date, git sha, fixture, duration, rows/s, MB/s, note.
  - Do not fake benchmark data.

- [ ] **Step 6: Tests**
  - Result schema JSON roundtrip equals expected field names.
  - Writer creates directories and writes atomically.
  - Compare passes within threshold.
  - Compare fails above duration threshold.
  - Compare fails below throughput threshold.
  - Compare handles missing candidate fixture and malformed JSON.

- [ ] **Step 7: Verification**
  - `go test ./benchmarks/...`
  - `go run ./benchmarks/compare.go --help`
  - Run compare against testdata baseline/candidate files.

- [ ] **Step 8: Commit**

```bash
git add benchmarks/result.go benchmarks/result_test.go \
  benchmarks/host.go benchmarks/host_test.go \
  benchmarks/writer.go benchmarks/writer_test.go \
  benchmarks/compare.go benchmarks/compare_test.go \
  benchmarks/results/HISTORY.md
git commit -m "bench: add result schema and regression comparator"
```

---

## Task 7: Benchmark harness for tiny/small/medium/large and native-vs-system comparison

**Purpose:** Measure real sync performance with the same fixtures used by integration tests and write spec-compliant JSON results.

**Files:**
- Create: `C:\Users\kiril\projects\pgsync\benchmarks\runner_test.go`
- Create: `C:\Users\kiril\projects\pgsync\benchmarks\harness.go`
- Create: `C:\Users\kiril\projects\pgsync\benchmarks\harness_test.go`
- Create: `C:\Users\kiril\projects\pgsync\benchmarks\README.md`
- Modify: `C:\Users\kiril\projects\pgsync\Makefile`
- Modify: `C:\Users\kiril\projects\pgsync\build.ps1`
- Modify/Create: `C:\Users\kiril\projects\pgsync\benchmarks\results\main\tiny.json`
- Modify/Create: `C:\Users\kiril\projects\pgsync\benchmarks\results\main\small.json`
- Modify/Create after an accepted local run: `C:\Users\kiril\projects\pgsync\benchmarks\results\main\medium.json`

- [ ] **Step 1: Implement harness lifecycle**
  - For each benchmark fixture:
    - Start source and target Postgres 18 containers.
    - Load fixture into source.
    - Run `pgsync` native engine with configured threads.
    - Verify row counts and checksums using existing integration helpers.
    - Capture stage timings from engine progress events.
    - Capture peak memory around the sync run.
    - Write JSON result.
  - Keep container setup outside `b.ResetTimer()`; measure only sync time.

- [ ] **Step 2: Expose benchmark selection flags/env**
  - `PGSYNC_BENCH_FIXTURES=tiny,small,medium` default for CI.
  - `PGSYNC_BENCH_FIXTURES=large` for release/manual runs.
  - `PGSYNC_BENCH_THREADS=<n>` default `runtime.NumCPU()`.
  - `PGSYNC_BENCH_ENGINE=native` for MVP; keep field flexible.
  - `PGSYNC_BENCH_OUT=<dir>` to override output.

- [ ] **Step 3: Add benchmark functions**
  - `BenchmarkSyncTiny`
  - `BenchmarkSyncSmall`
  - `BenchmarkSyncMedium`
  - `BenchmarkSyncLarge`
  - Each skips with a clear message if not selected.
  - Large skips by default unless explicitly selected.

- [ ] **Step 4: Add native-vs-system pg_dump comparison**
  - On tiny/small at minimum, run existing system `pg_dump | pg_restore` path or external engine if implemented.
  - Record comparison in benchmark logs, not necessarily in baseline JSON unless schema is extended later.
  - Fail only if the test explicitly runs a comparison gate; do not make developer `make bench` flaky.

- [ ] **Step 5: Add Make/build targets**
  - `make bench` = tiny + small by default.
  - `make bench-ci` = tiny + small + medium with regression compare.
  - `make bench-large` = large only, manual/release.
  - `make bench-compare BASELINE=benchmarks/results/main CANDIDATE=...`.
  - PowerShell equivalents in `build.ps1`.

- [ ] **Step 6: Baseline promotion process**
  - First run on accepted baseline hardware/CI:
    - Generate `tiny.json`, `small.json`, `medium.json` under `benchmarks/results/main/`.
    - Update `benchmarks/results/HISTORY.md` with real data from the run.
  - Never hand-edit metric values.
  - Large baseline is optional for MVP release; if produced, store under release artifacts or `benchmarks/results/main/large.json` only after accepting storage/time cost.

- [ ] **Step 7: Tests**
  - Unit-test harness option parsing without containers.
  - Unit-test stage timing aggregation from fake progress events.
  - Integration benchmark smoke for tiny fixture under `-tags=integration` if CI time permits.
  - Ensure normal `go test ./...` does not start containers unintentionally.

- [ ] **Step 8: Verification**
  - `go test ./benchmarks/...`
  - `go test -bench=BenchmarkSyncTiny -run=^$ ./benchmarks/...`
  - `make bench`
  - `make bench-compare BASELINE=benchmarks/results/main CANDIDATE=<generated-dir>`

- [ ] **Step 9: Commit**

```bash
git add benchmarks/runner_test.go benchmarks/harness.go benchmarks/harness_test.go \
  benchmarks/README.md Makefile build.ps1 \
  benchmarks/results/main/tiny.json benchmarks/results/main/small.json \
  benchmarks/results/main/medium.json benchmarks/results/HISTORY.md
git commit -m "bench: add sync benchmark harness and baselines"
```

---

## Task 8: Release packaging scripts and local build-all workflow

**Purpose:** Produce the four GitHub Release artifacts named in the spec, with embedded pg_tools, version ldflags, archives, and checksums.

**Files:**
- Create: `C:\Users\kiril\projects\pgsync\scripts\package-release.sh`
- Create: `C:\Users\kiril\projects\pgsync\scripts\checksums.sh`
- Modify: `C:\Users\kiril\projects\pgsync\Makefile`
- Modify: `C:\Users\kiril\projects\pgsync\build.ps1`
- Modify: `C:\Users\kiril\projects\pgsync\internal\version\version.go` only if Phase 1 version variables need release metadata additions
- Modify/Create tests as needed: `C:\Users\kiril\projects\pgsync\internal\version\version_test.go`

- [ ] **Step 1: Define artifact names**
  - `pgsync-windows-amd64.zip`
  - `pgsync-darwin-arm64.tar.gz`
  - `pgsync-darwin-amd64.tar.gz`
  - `pgsync-linux-amd64.tar.gz`
  - `checksums.txt`

- [ ] **Step 2: Implement `scripts/package-release.sh`**
  - Inputs:
    - `--version vX.Y.Z`.
    - `--git-commit <sha>`.
    - `--build-date <RFC3339>`.
    - `--dist dist` default.
  - Run `make pgtools-prepare-release` or fail if embed payloads are missing.
  - Build each platform with:
    - `CGO_ENABLED=0` if the project remains pure Go.
    - `GOOS`, `GOARCH` set per target.
    - `-ldflags` setting version, git commit, build date.
  - Package binary only; pg_tools are embedded in the binary.
  - Include `README.md`, `LICENSE`, and a generated `VERSION.txt` in each archive.
  - Ensure Windows binary is named `pgsync.exe` inside the zip.

- [ ] **Step 3: Implement checksums**
  - Generate SHA-256 checksums for every archive.
  - Stable format: `<sha256>  <filename>`.
  - Include `checksums.txt` in release upload, not inside each archive.

- [ ] **Step 4: Update Makefile/build.ps1**
  - `make build-all VERSION=vX.Y.Z`
  - `make package VERSION=vX.Y.Z`
  - `make checksums`
  - `make release-local VERSION=vX.Y.Z` = test + pgtools prepare + package + checksums.
  - PowerShell equivalents for Windows contributors.

- [ ] **Step 5: Version command verification**
  - Built artifacts must print the injected version/commit/date via `pgsync version`.
  - Unit tests cover formatting; release script smoke-tests each local executable it can run.

- [ ] **Step 6: Verification**
  - `make release-local VERSION=v0.0.0-dev`
  - Inspect `dist/` contains exactly four archives + checksums.
  - Extract each archive and confirm binary exists with expected name and no unembedded pgtools payload directory.
  - On current platform, run extracted binary:
    - `./pgsync version`
    - `./pgsync doctor --help`

- [ ] **Step 7: Commit**

```bash
git add scripts/package-release.sh scripts/checksums.sh \
  Makefile build.ps1 internal/version/version.go internal/version/version_test.go
git commit -m "build: package cross-platform release artifacts"
```

---

## Task 9: Self-updater (`pgsync upgrade`) using GitHub Releases

**Purpose:** Port the dbsync updater design into pgsync so users can update from published GitHub Releases with checksum verification and safe binary replacement.

**Files:**
- Create/Modify: `C:\Users\kiril\projects\pgsync\internal\updater\updater.go`
- Create: `C:\Users\kiril\projects\pgsync\internal\updater\github.go`
- Create: `C:\Users\kiril\projects\pgsync\internal\updater\assets.go`
- Create: `C:\Users\kiril\projects\pgsync\internal\updater\apply.go`
- Create: `C:\Users\kiril\projects\pgsync\internal\updater\updater_test.go`
- Create: `C:\Users\kiril\projects\pgsync\internal\updater\github_test.go`
- Create: `C:\Users\kiril\projects\pgsync\internal\updater\assets_test.go`
- Create: `C:\Users\kiril\projects\pgsync\internal\updater\apply_test.go`
- Modify/Create: `C:\Users\kiril\projects\pgsync\internal\cli\upgrade.go`
- Modify/Create: `C:\Users\kiril\projects\pgsync\internal\cli\upgrade_test.go`

- [ ] **Step 1: Define updater interfaces**
  - `ReleaseClient` for GitHub Releases API.
  - `HTTPDoer` for downloads.
  - `Applier` for replacing current binary.
  - `ArchiveExtractor` for zip/tar.gz.
  - `CurrentExecutable` function injection for `os.Executable`.
  - Use `fsx` and `runner` abstractions where practical; avoid direct global side effects in core logic.

- [ ] **Step 2: Implement GitHub release lookup**
  - Default repo: `mttzzz/pgsync` unless Phase 1/2 establishes a different module owner.
  - Query latest release unless user passes a specific version flag.
  - Ignore prereleases unless `--prerelease` exists or is intentionally added.
  - Compare semantic versions; if current version is already latest, print no-op success.
  - Tests use `httptest.Server` with fixture JSON.

- [ ] **Step 3: Select platform asset**
  - Map runtime platform to exact release asset names from Task 8.
  - Download `checksums.txt` and the selected archive.
  - Verify archive SHA-256 before extraction.
  - Error if checksum missing or mismatched.
  - Tests cover windows/darwin/linux asset selection with injected GOOS/GOARCH.

- [ ] **Step 4: Extract and apply update safely**
  - Extract only `pgsync` or `pgsync.exe` from archive.
  - Reject path traversal entries.
  - Write candidate binary to same directory as current executable.
  - Preserve executable permissions on Unix.
  - Atomic replacement strategy:
    - Unix: rename current to backup, rename candidate to current, rollback on failure.
    - Windows: use dbsync's proven rename/helper strategy; if direct replacement is impossible while running, write `.new` and spawn a short-lived helper via `runner.CommandRunner` to swap after process exit.
  - Never delete backup until candidate version smoke check passes where feasible.

- [ ] **Step 5: CLI command**
  - `pgsync upgrade`:
    - Shows current version.
    - Checks latest release.
    - Downloads selected asset.
    - Verifies checksum.
    - Applies update.
    - Prints final instruction to rerun `pgsync version`.
  - Flags if needed:
    - `--version vX.Y.Z` for explicit version.
    - `--dry-run` to report what would happen.
    - `--prerelease` only if product wants prerelease updates; otherwise omit.
  - In `--output=json`, emit NDJSON events such as `upgrade.check`, `upgrade.download`, `upgrade.apply`, `upgrade.done`, `upgrade.failed`.

- [ ] **Step 6: Tests**
  - Latest release newer downloads and applies.
  - Already latest no-ops.
  - No matching platform asset errors.
  - Checksum mismatch errors before apply.
  - Archive path traversal rejected.
  - Apply rollback on rename failure.
  - CLI dry-run does not download archive body or apply.
  - JSON output contains stable event names.

- [ ] **Step 7: Verification**
  - `go test ./internal/updater ./internal/cli`
  - `go test ./internal/...`
  - Manual dry-run against a test GitHub release or `httptest`-backed command if CLI supports endpoint injection for tests:
    - `pgsync upgrade --dry-run --verbose`

- [ ] **Step 8: Commit**

```bash
git add internal/updater/updater.go internal/updater/github.go \
  internal/updater/assets.go internal/updater/apply.go \
  internal/updater/updater_test.go internal/updater/github_test.go \
  internal/updater/assets_test.go internal/updater/apply_test.go \
  internal/cli/upgrade.go internal/cli/upgrade_test.go
git commit -m "feat: add GitHub Releases self-updater"
```

---

## Task 10: CI benchmark regression and release workflows

**Purpose:** Turn Phase 4 capabilities into automated gates: benchmark regression on main/PR, cross-platform tests, and tag-triggered GitHub Releases.

**Files:**
- Modify: `C:\Users\kiril\projects\pgsync\.github\workflows\ci.yml`
- Create: `C:\Users\kiril\projects\pgsync\.github\workflows\release.yml`
- Optional create: `C:\Users\kiril\projects\pgsync\.github\workflows\bench.yml` if keeping manual large benchmarks separate is cleaner
- Modify: `C:\Users\kiril\projects\pgsync\Makefile`
- Modify: `C:\Users\kiril\projects\pgsync\build.ps1`

- [ ] **Step 1: Update CI matrix**
  - Keep existing jobs:
    - `lint`
    - `test-unit`
    - `test-integration`
    - `coverage-gate`
  - Ensure unit matrix covers supported release platforms where GitHub-hosted runners exist:
    - Linux amd64.
    - Windows amd64.
    - macOS arm64 if available.
    - macOS amd64 if available or via cross-compile smoke.
  - Do not require Docker/testcontainers on Windows/macOS unit jobs.

- [ ] **Step 2: Add `bench-regression` job**
  - Runs on Linux only.
  - Depends on tests/coverage or runs after unit success.
  - Prepares pgtools payloads if the benchmark uses built binary with embedded tools.
  - Runs tiny + small + medium:
    - `make bench-ci`
  - Compares candidate results to `benchmarks/results/main/` with 15% threshold.
  - Uploads candidate benchmark JSON as an artifact on every run.
  - Fails on regression; if CI hardware variance is high, first collect data and document whether medium gate should be workflow_dispatch-only until stable.

- [ ] **Step 3: Add manual large benchmark workflow**
  - `workflow_dispatch` inputs:
    - fixture: `large` default.
    - threads.
    - baseline ref.
  - Runs `make bench-large`.
  - Uploads JSON and logs.
  - Does not block normal PRs.

- [ ] **Step 4: Add tag-triggered release workflow**
  - Trigger: pushed tags `v*`.
  - Steps:
    - checkout with full history.
    - setup Go 1.25+.
    - lint.
    - unit tests.
    - integration tests.
    - coverage gate.
    - bench tiny/small/medium regression.
    - fetch/verify/sync pgtools for all platforms.
    - `make package VERSION=${tag}`.
    - `make checksums`.
    - create GitHub Release.
    - upload the four archives and `checksums.txt`.
    - release notes from `CHANGELOG.md` section matching tag.
  - Use GitHub token permissions narrowly: contents write only in release job.

- [ ] **Step 5: Add workflow guards**
  - Release job fails if tag version does not match `CHANGELOG.md` heading.
  - Release job fails if `pgsync version` inside current-platform archive does not match tag.
  - Release job fails if artifact names differ from spec.
  - Release job fails if `checksums.txt` lacks any archive.

- [ ] **Step 6: Verification**
  - Use `act` only if already supported locally; otherwise validate workflow YAML syntax through GitHub or `actionlint` if present.
  - Trigger dry-run release logic locally:
    - `make release-local VERSION=v0.0.0-dev`
  - Run:
    - `make bench-ci`
    - `go test ./internal/... ./benchmarks/...`

- [ ] **Step 7: Commit**

```bash
git add .github/workflows/ci.yml .github/workflows/release.yml \
  .github/workflows/bench.yml Makefile build.ps1
git commit -m "ci: add benchmark regression and release workflows"
```

---

## Task 11: Release documentation, changelog, and final MVP verification

**Purpose:** Document the release/update workflow for maintainers and users, then perform a complete final verification pass.

**Files:**
- Modify: `C:\Users\kiril\projects\pgsync\README.md`
- Modify/Create: `C:\Users\kiril\projects\pgsync\CHANGELOG.md`
- Create/Modify: `C:\Users\kiril\projects\pgsync\docs\README.md`
- Create: `C:\Users\kiril\projects\pgsync\docs\release.md`
- Modify: `C:\Users\kiril\projects\pgsync\benchmarks\README.md`
- Modify: `C:\Users\kiril\projects\pgsync\fixtures\README.md`

- [ ] **Step 1: README updates**
  - Document single-binary UX and embedded PostgreSQL 18 tools.
  - Document `--use-system-pgtools` escape hatch.
  - Document `pgsync upgrade`.
  - Document supported platforms and minimum PostgreSQL version 18.
  - Document benchmark targets and where results live.

- [ ] **Step 2: Changelog**
  - Add unreleased MVP section if no release tag exists yet.
  - Add release checklist subsection:
    - update manifest SHAs.
    - run `make release-local`.
    - run medium benchmark and inspect trend.
    - tag `vX.Y.Z`.
    - verify GitHub Release assets.
    - test `pgsync upgrade` from previous release where possible.

- [ ] **Step 3: `docs/release.md`**
  - Maintainer-focused release instructions:
    - pgtools update procedure.
    - baseline benchmark promotion.
    - tag/release workflow.
    - rollback if release asset is bad.
    - updater compatibility considerations.
  - Explain that download-on-first-run is intentionally not implemented for MVP; future work can add `pgsync setup` if binary size becomes unacceptable.

- [ ] **Step 4: Final verification matrix**
  - Local:
    - `make lint`
    - `make test`
    - `make coverage`
    - `make integration`
    - `make bench`
    - `make release-local VERSION=v0.0.0-dev`
  - Current-platform artifact smoke:
    - extract archive.
    - `pgsync version`.
    - `pgsync doctor`.
    - `pgsync sync <tiny-db> --dry-run`.
  - Upgrade dry-run:
    - `pgsync upgrade --dry-run`.

- [ ] **Step 5: Commit**

```bash
git add README.md CHANGELOG.md docs/README.md docs/release.md \
  benchmarks/README.md fixtures/README.md
git commit -m "docs: document embedded tools benchmarks and releases"
```

---

## Self-Review Checklist

**Spec coverage — every spec section that belongs in Phase 4 has a task:**

| Spec § | Concern | Plan-4 task |
|---|---|---|
| 4.1 | Single-binary UX, known/tested pg_tools, system override | 1, 2, 4, 8 |
| 4.2 | Per-platform PostgreSQL 18 payload contents | 1, 2 |
| 4.3 | Build-time embedding with build tags | 2, 8, 10 |
| 4.4 | Runtime extraction, cache by hash, chmod, macOS codesign | 3 |
| 4.5 | Download-on-first-run alternative deferred | 11 |
| 6 | `pgsync upgrade`, `--use-system-pgtools`, `doctor` visibility | 4, 9 |
| 11.1 | Coverage allow-list for platform embed files only | 2, all internal tasks |
| 11.3 | Unit tests for pgtools extraction and corrupt-cache recovery | 3 |
| 11.4 | Reproducible fixtures and generator | 5 |
| 11.5 | Benchmark suite, JSON schema, regression threshold | 6, 7, 10 |
| 13 | GitHub Release assets, self-update, CI jobs | 8, 9, 10 |
| 15 | Port `internal/updater/` concept from dbsync | 9 |
| 16 | stdlib embed/build tags, benchstat, no goreleaser | 1, 2, 6, 8, 10 |

**Out of Phase 4 scope / future:**
- Download-on-first-run (`pgsync setup`) for pg_tools. MVP embeds tools.
- Homebrew/winget/scoop packages.
- macOS notarization and Windows Authenticode signing.
- Keyring storage for passwords.
- Fixtures larger than 5 GB.
- Public performance dashboard beyond committed JSON/HISTORY.

**Risk checks:**
- Go `//go:embed` package-local limitation handled by `scripts/sync-pgtools-embed.sh`.
- `.gitignore` conflict with committed benchmark baselines explicitly fixed in Task 1.
- Bench CI variance acknowledged; collect data before making medium gate mandatory if hosted runner noise is too high.
- Windows self-replacement requires dbsync-proven helper strategy and dedicated tests.
- PostgreSQL binary license/redistribution terms must be reviewed before first public release; document source URLs and checksums in the manifest.

**Definition of done for Phase 4:**
- `pgsync` release binary runs on supported platforms without system PostgreSQL client tools installed.
- `pgsync doctor` reports embedded pgtools paths/cache and system override works.
- Corrupt cache is detected and repaired automatically.
- Fixtures can be generated reproducibly.
- Benchmarks write spec-compliant JSON and compare against baselines with a 15% regression threshold.
- Tag `v*` creates four release archives + `checksums.txt` in GitHub Releases.
- `pgsync upgrade` can discover, verify, download, and apply a newer release.
- Strict coverage and lint gates pass.

---

## Execution Handoff

Plan complete and saved to `docs/superpowers/plans/2026-05-02-pgsync-phase4-embed-bench-release.md`.

Recommended execution mode: **Subagent-Driven Development** with one fresh implementation subagent per task and a separate review subagent after each commit. Phase 4 touches release-critical code, so do not batch updater, packaging, and CI changes into a single review.
