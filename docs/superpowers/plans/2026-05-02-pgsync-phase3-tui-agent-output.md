# pgsync Phase 3 Implementation Plan — TUI, ConfigEditor, and Agent Output

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:subagent-driven-development` (recommended) or `superpowers:executing-plans` to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement the user-facing experience layer for pgsync: full-screen Bubble Tea TUI, huh-powered ConfigEditor, config CLI commands, and LLM/agent-friendly NDJSON output for non-interactive CLI runs. This phase must make `pgsync` usable by a developer without hand-editing config files, while preserving scriptability through `--output=json`.

**Architecture:** Phase 3 is a presentation/orchestration phase. It should not reimplement sync internals. It consumes Phase 1 foundation packages (`config`, `models`, `clock`, `fsx`, `observability`) and Phase 2 backend contracts (`engine`, `pgschema`, sync planner/executor). All terminal, file, time, and network effects remain behind interfaces so `internal/` stays at strict 100% line coverage. The actual terminal program launch should be kept in `cmd/pgsync/main.go` or injectable adapters so testable `internal/tui` code remains pure Bubble Tea model logic.

**Tech Stack:** Go 1.25+, `spf13/cobra`, `charmbracelet/bubbletea` v1.3+, `charmbracelet/huh` v0.6+, `charmbracelet/lipgloss` v1.1+, stdlib `encoding/json`, `log/slog`, Phase 1 `config` and `models` packages. No `viper`, no `gomock`, no global mutable terminal state in package init.

**Reference docs:** See `docs/superpowers/specs/2026-05-02-pgsync-design.md` — sections 1–2 (UX goals), 5 (layout), 6 (CLI surface + NDJSON), 7 (TUI state machine), 8 (Config managed by TUI), 11.1–11.3 (coverage/test pyramid), and 16 (tech stack).

**Phase assumptions:**
- Phase 1 foundation is complete: config types/store/validators, logger, clock/fs abstractions, models, proxy foundation, and coverage gate.
- Phase 2 backend is complete enough to provide list/status/plan/sync capabilities through package-level services or interfaces. If exact Phase 2 names differ, create thin Phase 3 adapters in `internal/tui/services.go` and `internal/cli/services.go`; do not change backend behavior just to satisfy UI tests.
- Config field names follow Phase 1 structs: `config.Config`, `config.Connection`, `config.Runtime`, `config.Logging`.

**Conventions for every task:**
- TDD: failing test first, run it red, minimal implementation, run it green, commit.
- Strict 100% coverage for every new/changed file under `internal/`.
- Use hand-written fakes in `_test.go`; no `gomock`.
- Keep terminal side effects injectable. Test Bubble Tea models by calling `Init`, `Update`, and `View`; do not require a real TTY in unit tests.
- Do not print passwords or proxy credentials. Any config display must redact secrets.
- JSON mode means stdout is NDJSON only; stderr is empty except unrecoverable fatal text suitable for shell diagnostics.
- Human/TUI output must respect `--no-color` and avoid ANSI when stdout is not a TTY.
- Commits use explicit file lists (`git add path/a path/b && git commit -m "..."`) — never `git add .`, `git add -A`, or `git commit -am`.
- Multi-line comments use `/* ... */`, not chained `//` blocks.

---

## Task 1: Add UI dependencies and presentation-layer contracts

**Files:**
- Modify: `go.mod`, `go.sum`
- Modify or create: `internal/cli/commands.go`
- Create: `internal/cli/options.go`
- Create: `internal/cli/options_test.go`
- Create: `internal/cli/services.go`
- Create: `internal/tui/services.go`
- Create: `internal/tui/services_test.go`

- [ ] **Step 1: Add Charm dependencies**

```bash
go get github.com/charmbracelet/bubbletea@latest \
  github.com/charmbracelet/huh@latest \
  github.com/charmbracelet/lipgloss@latest
```

- [ ] **Step 2: Define CLI runtime options in `internal/cli/options.go`**

Create a small, testable struct for global options:
- `Output string` (`text` or `json`)
- `Quiet bool`
- `Verbose bool`
- `NoColor bool`
- `ConfigPath string`
- `Threads int`
- `Engine string`
- `UseSystemPgtools bool`

Add validation helpers:
- `ValidateOutputMode(mode string) error`
- `ValidateEngineMode(mode string) error`
- `LogLevel() string` with precedence `--quiet` > `--verbose` > config/default

- [ ] **Step 3: Write failing option tests**

Cover:
- accepts only `text` and `json`
- accepts only `auto`, `native`, `external`
- quiet wins over verbose
- zero/blank config path means "use config.DefaultPath later", not an error

```bash
go test ./internal/cli -run 'TestOptions|TestValidate' -count=1
# Expected: fail before options.go exists or before validation is implemented.
```

- [ ] **Step 4: Define service interfaces for CLI and TUI**

`internal/cli/services.go` should define only the ports CLI needs, for example:
- `ConfigStore` with `Load(path string)`, `Save(path string, cfg config.Config)`, `DefaultPath() string`, `Remove(path string)` if reset is supported by store/fs.
- `DatabaseService` with list/status/test methods from Phase 2.
- `SyncService` with plan/execute methods from Phase 2.
- `TUIRunner` callback/interface so cobra commands can call the TUI without constructing a real terminal in unit tests.

`internal/tui/services.go` should define UI-facing ports with stable, small methods:
- `ConfigStore`
- `ConnectionTester`
- `CatalogService` (`ListDatabases`, `ListTables`)
- `Planner`
- `SyncExecutor`
- `Clock`

Prefer adapters over changing Phase 2 engine APIs.

- [ ] **Step 5: Write service-contract tests**

Use compile-time assertions and fakes to ensure CLI/TUI code can be constructed without a real DB, file system, clock, or TTY.

- [ ] **Step 6: Wire global cobra flags without command behavior yet**

In `internal/cli/commands.go`, register global flags:
- `--output text|json`
- `--threads`
- `--engine auto|native|external`
- `--use-system-pgtools`
- `--config`
- `--quiet`
- `--verbose`
- `--no-color`

Make `PersistentPreRunE` validate options before any command runs. Tests should execute commands against a `bytes.Buffer` stdout/stderr.

- [ ] **Step 7: Run tests and coverage gate for touched packages**

```bash
go test -race ./internal/cli ./internal/tui
```

- [ ] **Step 8: Commit**

```bash
git add go.mod go.sum \
  internal/cli/commands.go internal/cli/options.go internal/cli/options_test.go internal/cli/services.go \
  internal/tui/services.go internal/tui/services_test.go
git commit -m "feat(ui): add presentation contracts and global CLI options"
```

---

## Task 2: NDJSON event model and encoder

**Files:**
- Create: `internal/cli/agent_progress.go`
- Create: `internal/cli/agent_progress_test.go`

- [ ] **Step 1: Write failing tests for required sync event schema**

Cover the spec examples exactly as NDJSON lines:
- `sync.start` includes `ts`, `level`, `event`, `db`, `tables`, `engine`
- `schema.predata.start`
- `schema.predata.done` includes `duration_ms`
- `table.copy.start` includes `table`, `est_rows`
- `table.copy.progress` includes `table`, `rows`, `pct`, `bytes_per_sec`
- `table.copy.done` includes `table`, `rows`, `duration_ms`
- `schema.postdata.start`
- `schema.postdata.done` includes `indexes`, `fks`, `duration_ms`
- `sync.done` includes `duration_ms`, `tables`, `bytes`
- `sync.failed` includes `level:error`, `stage`, optional `table`, and `error`

Test guarantees:
- each call writes exactly one line ending with `\n`
- each line is valid JSON object, not an array
- timestamps are UTC RFC3339 strings
- zero-valued optional fields are omitted where appropriate
- no ANSI sequences are present
- password-looking keys never appear

```bash
go test ./internal/cli -run TestAgentProgress -count=1
# Expected: fail before encoder exists.
```

- [ ] **Step 2: Implement event constants and payload type**

Use a single struct with explicit JSON tags and `omitempty`, for example:
- `Timestamp string` with tag `json:"ts"`
- `Level string` with tag `json:"level,omitempty"`
- `Event string` with tag `json:"event"`
- `Database string` with tag `json:"db,omitempty"`
- `Tables int` with tag `json:"tables,omitempty"`
- `Engine string` with tag `json:"engine,omitempty"`
- stage/table/rows/pct/bytes/duration/index/fk fields matching the spec

Keep event names as constants to avoid typos in tests and command wiring.

- [ ] **Step 3: Implement `AgentWriter`**

`AgentWriter` should:
- accept `io.Writer` and `clock.Clock`
- encode one compact JSON object per line using `json.Encoder`
- not use `slog` JSON output, because the schema is command-progress schema, not log schema
- expose helpers such as `SyncStart`, `SchemaPreDataStart`, `TableCopyProgress`, `SyncDone`, `SyncFailed`
- be safe to call sequentially from sync progress callbacks; no goroutine safety required unless Phase 2 executor calls observers concurrently. If callbacks can be concurrent, add a mutex and test it with `t.Parallel` or a race test.

- [ ] **Step 4: Map Phase 2 progress/result models to agent events**

Add adapter methods that implement the Phase 2 progress observer contract. If Phase 2 uses `models.Progress`, map stages consistently:
- pre-data stage → `schema.predata.*`
- copy stage → `table.copy.*`
- post-data stage → `schema.postdata.*`
- final result → `sync.done` or `sync.failed`

- [ ] **Step 5: Add fuzz/property-style tests for JSON lines**

Use representative table names, errors, and DB names with quotes/unicode/newlines. Assert every output line remains parseable JSON and does not leak raw control characters outside JSON encoding.

- [ ] **Step 6: Run tests**

```bash
go test -race ./internal/cli -run 'TestAgentProgress|FuzzAgentProgress' -count=1
```

- [ ] **Step 7: Commit**

```bash
git add internal/cli/agent_progress.go internal/cli/agent_progress_test.go
git commit -m "feat(cli): add NDJSON agent progress writer"
```

---

## Task 3: Human output, output routing, and JSON-mode invariants

**Files:**
- Create: `internal/cli/output.go`
- Create: `internal/cli/output_test.go`
- Create or modify: `internal/cli/plain_output.go`
- Create or modify: `internal/cli/plain_output_test.go`
- Modify: `internal/cli/commands.go`

- [ ] **Step 1: Write failing tests for output router**

Cover:
- `--output=text` returns a human writer
- `--output=json` returns `AgentWriter`
- `--output=json --no-color` is accepted and still has no ANSI output
- invalid output mode fails before command execution
- JSON mode leaves stderr empty on normal command errors converted into `sync.failed` events
- fatal pre-run errors may write concise text to stderr and return non-zero

- [ ] **Step 2: Implement output router**

Create a small factory, for example `NewOutput(opts Options, stdout, stderr io.Writer, clk clock.Clock)`, returning:
- human/plain output writer for text mode
- agent/NDJSON writer for JSON mode

Plain output should remain boring and script-tolerant:
- no color if `NoColor` is true or output is not a TTY
- no secrets
- readable summaries for list/status/sync dry-run

- [ ] **Step 3: Add plain-output tests**

Test table/list/status rendering with:
- empty lists
- long database names
- bytes and row counts
- redaction of passwords/proxy credentials
- no color when requested

- [ ] **Step 4: Wire command context with output router**

`commands.go` should build a per-command runtime/context object containing parsed options, output writer, config path, and service dependencies. This avoids package globals and makes command tests deterministic.

- [ ] **Step 5: Run tests**

```bash
go test -race ./internal/cli -run 'TestOutput|TestPlainOutput|TestRootCommand' -count=1
```

- [ ] **Step 6: Commit**

```bash
git add internal/cli/output.go internal/cli/output_test.go \
  internal/cli/plain_output.go internal/cli/plain_output_test.go internal/cli/commands.go
git commit -m "feat(cli): route text and NDJSON output modes"
```

---

## Task 4: Config CLI commands and redacted config display

**Files:**
- Modify or create: `internal/cli/config.go`
- Create: `internal/cli/config_test.go`
- Create or modify: `internal/config/redact.go`
- Create or modify: `internal/config/redact_test.go`

- [ ] **Step 1: Write failing tests for `config show` redaction**

Cover:
- remote password masked
- local password masked
- credentials in `proxy_url` masked, e.g. `socks5://user:pass@host:1080` → `socks5://user:xxxxx@host:1080`
- non-secret fields preserved
- TOML/text output stays deterministic for snapshot-style assertions

- [ ] **Step 2: Implement config redaction helper**

Add a pure helper in `internal/config` such as `Redacted(Config) Config`. It must not mutate the original value.

- [ ] **Step 3: Write failing command tests**

Cover:
- `pgsync config path` prints resolved path and newline
- `pgsync config show` loads config and prints redacted config
- `pgsync config show --output=json` emits one NDJSON event/object with redacted config data, not raw TOML
- `pgsync config reset` asks the injected TUI runner to open ConfigEditor in wizard/reset mode after removing or ignoring old config
- bare `pgsync config` invokes ConfigEditor via the injected TUI runner

- [ ] **Step 4: Implement config commands**

Command behavior:
- `pgsync config` → full-screen ConfigEditor wizard
- `pgsync config show` → read-only, redacted display
- `pgsync config path` → path only, explicitly for debugging
- `pgsync config reset` → clear existing file through store/fs and launch ConfigEditor wizard

Do not add a command that opens the file in an editor; spec says the file is internal storage, not the interface.

- [ ] **Step 5: Verify no secret leakage in command failures**

Test load/validation errors with config containing secrets. Error strings must not include raw password values.

- [ ] **Step 6: Run tests**

```bash
go test -race ./internal/config ./internal/cli -run 'TestRedact|TestConfigCommand' -count=1
```

- [ ] **Step 7: Commit**

```bash
git add internal/config/redact.go internal/config/redact_test.go \
  internal/cli/config.go internal/cli/config_test.go
git commit -m "feat(config): add redacted config CLI commands"
```

---

## Task 5: TUI app shell, state machine, and key routing

**Files:**
- Create: `internal/tui/app.go`
- Create: `internal/tui/app_test.go`
- Create: `internal/tui/messages.go`
- Create: `internal/tui/keymap.go`
- Create: `internal/tui/keymap_test.go`
- Create: `internal/tui/screens/screen.go`
- Create: `internal/tui/test_helpers_test.go`

- [ ] **Step 1: Write failing app-state tests**

Cover the spec state machine:

```text
SettingsCheck -> MainMenu -> DatabaseList -> [TablesPick] -> ConfirmPlan -> Progress -> Result
       ^           |                                                                    |
       |           +-> ConfigEditor (global shortcut 's')                               |
       +------------------------- back to MainMenu (Esc) --------------------------------+
```

Test transitions without rendering a real terminal:
- initial state is `SettingsCheck`
- valid settings move to `MainMenu`
- missing/invalid settings open `ConfigEditor` in wizard mode
- `s` opens ConfigEditor from MainMenu, DatabaseList, TablesPick, ConfirmPlan, Progress, and Result
- `Esc` returns to MainMenu except where a modal confirmation intercepts it
- `q` requests quit when no sync is running
- `q` during sync asks for confirmation/cancel instead of instantly killing state

- [ ] **Step 2: Define screen interface**

Use a small interface under `internal/tui/screens`:
- `Init() tea.Cmd`
- `Update(tea.Msg) (Screen, tea.Cmd)` or similar reducer shape
- `View() string`
- optional `Title()`, `Help()` if useful for common layout

Keep screen models immutable-by-convention: updates return the next model/screen.

- [ ] **Step 3: Implement app model and routing**

`internal/tui/app.go` should own:
- current screen/state
- navigation stack or explicit return target
- shared services
- loaded config
- selected databases/tables/plan
- global error/status message

Global key handling happens before delegating to screen unless the active screen declares it is editing text/form fields.

- [ ] **Step 4: Add test helpers for Bubble Tea messages**

Create test helpers for key messages and command execution so tests stay concise and cover command-return paths.

- [ ] **Step 5: Run tests**

```bash
go test -race ./internal/tui ./internal/tui/screens -run 'TestApp|TestKeymap' -count=1
```

- [ ] **Step 6: Commit**

```bash
git add internal/tui/app.go internal/tui/app_test.go internal/tui/messages.go \
  internal/tui/keymap.go internal/tui/keymap_test.go internal/tui/test_helpers_test.go \
  internal/tui/screens/screen.go
git commit -m "feat(tui): add app shell and navigation state machine"
```

---

## Task 6: TUI styles, layout primitives, and UX guardrails

**Files:**
- Create: `internal/tui/styles/theme.go`
- Create: `internal/tui/styles/theme_test.go`
- Create: `internal/tui/screens/layout.go`
- Create: `internal/tui/screens/layout_test.go`
- Modify: `internal/tui/app.go`
- Modify: `internal/tui/app_test.go`

- [ ] **Step 1: Write failing style/layout tests**

Cover:
- `NoColor` theme emits no ANSI escape sequences
- narrow terminal width does not panic and truncates safely
- title/status/help footer are present
- errors render with a clear prefix in text and styled variant in color mode
- passwords/proxy credentials are never shown by layout helpers

- [ ] **Step 2: Implement theme package**

Create lipgloss styles for:
- app title/header
- section title
- help footer
- status line
- error line
- selected row
- muted metadata
- progress bar colors

Expose a `Theme` value built from options (`NoColor`, terminal width) rather than package globals.

- [ ] **Step 3: Implement reusable layout helpers**

In `screens/layout.go`, add pure helpers:
- `Frame(theme, title, body, help, status string) string`
- `ErrorPanel(theme, err error) string`
- `RedactText(s string) string` only if needed for defensive rendering

- [ ] **Step 4: Apply layout in app view**

Update `App.View()` so every screen is wrapped consistently. Avoid duplicating borders/help footer in each screen.

- [ ] **Step 5: Run tests**

```bash
go test -race ./internal/tui/... -run 'TestTheme|TestLayout|TestAppView' -count=1
```

- [ ] **Step 6: Commit**

```bash
git add internal/tui/styles/theme.go internal/tui/styles/theme_test.go \
  internal/tui/screens/layout.go internal/tui/screens/layout_test.go \
  internal/tui/app.go internal/tui/app_test.go
git commit -m "feat(tui): add responsive styles and layout primitives"
```

---

## Task 7: ConfigEditor form metadata and validation

**Files:**
- Create: `internal/tui/screens/config_editor.go`
- Create: `internal/tui/screens/config_editor_test.go`
- Create: `internal/tui/screens/config_fields.go`
- Create: `internal/tui/screens/config_fields_test.go`

- [ ] **Step 1: Write failing tests for field metadata**

Every config field must have:
- Russian title/label
- Russian description/help text
- default value from `config.Defaults()` or loaded config
- validator function
- correct secret behavior for password fields

Required sections and fields:
- Remote: host, port, user, password, database/default database, SSL mode, proxy URL
- Local: host, port, user, password, SSL mode
- Runtime: threads, engine (`auto`, `native`, `external`), use system pgtools, default database
- Logging: level (`debug`, `info`, `warn`, `error`), format (`text`, `json`)

- [ ] **Step 2: Implement field builders**

Keep huh-specific construction behind functions that are easy to test:
- `RemoteFields(cfg config.Config) []FieldSpec`
- `LocalFields(cfg config.Config) []FieldSpec`
- `RuntimeFields(cfg config.Config) []FieldSpec`
- `LoggingFields(cfg config.Config) []FieldSpec`
- `BuildConfigForm(cfg config.Config, mode ConfigEditorMode) *huh.Form`

A lightweight `FieldSpec` struct can make labels/defaults/validators testable without inspecting huh internals.

- [ ] **Step 3: Reuse Phase 1 config validators**

Use `config.ValidateHost`, `ValidatePort`, `ValidateSSLMode`, `ValidateProxyURL`, and full `config.Validate` where appropriate. Add UI-level validators for enum fields not covered by Phase 1, such as logging format if not already present.

- [ ] **Step 4: Implement ConfigEditor screen shell**

Modes:
- `WizardMode`: required first-run setup, cannot skip invalid config
- `EditMode`: opened from shortcut or `pgsync config`
- `ResetMode`: prefilled defaults after reset

The screen should expose actions:
- Save
- Test connection
- Cancel/back (only if not required wizard)

- [ ] **Step 5: Test screen update paths**

Cover:
- editing a field updates draft config but not saved config
- invalid field prevents save and shows inline error
- cancel discards draft
- wizard mode does not allow escape to MainMenu until valid config is saved
- password fields render masked values

- [ ] **Step 6: Run tests**

```bash
go test -race ./internal/tui/screens -run 'TestConfigEditor|TestConfigFields' -count=1
```

- [ ] **Step 7: Commit**

```bash
git add internal/tui/screens/config_editor.go internal/tui/screens/config_editor_test.go \
  internal/tui/screens/config_fields.go internal/tui/screens/config_fields_test.go
git commit -m "feat(tui): add ConfigEditor form metadata and validation"
```

---

## Task 8: ConfigEditor save, reset, and connection testing

**Files:**
- Modify: `internal/tui/services.go`
- Modify: `internal/tui/screens/config_editor.go`
- Modify: `internal/tui/screens/config_editor_test.go`
- Modify: `internal/cli/config.go`
- Modify: `internal/cli/config_test.go`

- [ ] **Step 1: Write failing tests for connection testing**

Cover:
- clicking/triggering `Test connection` calls remote and local testers
- success status includes both remote and local results
- remote failure does not attempt sync/planning
- local failure is displayed clearly
- proxy URL from config is passed to tester
- test errors are redacted

- [ ] **Step 2: Implement `ConnectionTester` port**

Add or refine TUI service interface:
- `TestRemote(ctx context.Context, cfg config.Config) error`
- `TestLocal(ctx context.Context, cfg config.Config) error`

The implementation should be an adapter to Phase 2 DB/status logic. Unit tests use a fake.

- [ ] **Step 3: Write failing tests for save/reset**

Cover:
- save validates entire config before writing
- save uses config store atomically through Phase 1 store abstraction/function
- save success updates app-level config and returns to MainMenu/Edit return target
- save failure keeps draft open and displays error
- reset starts from defaults and does not preserve old passwords unless explicitly loaded in edit mode

- [ ] **Step 4: Implement save/reset flow**

Important behavior:
- ConfigEditor is the only writer of config file in interactive flows.
- CLI/env overrides never get persisted by ConfigEditor unless they were loaded into the draft intentionally by command wiring.
- Saving writes TOML through Phase 1 store, not through ad-hoc file writes.

- [ ] **Step 5: Ensure CLI `config reset` opens reset wizard**

Update `internal/cli/config.go` so reset command invokes TUI runner with `ResetMode` or equivalent.

- [ ] **Step 6: Run tests**

```bash
go test -race ./internal/tui ./internal/tui/screens ./internal/cli -run 'TestConfig' -count=1
```

- [ ] **Step 7: Commit**

```bash
git add internal/tui/services.go internal/tui/screens/config_editor.go \
  internal/tui/screens/config_editor_test.go internal/cli/config.go internal/cli/config_test.go
git commit -m "feat(tui): wire ConfigEditor save reset and connection tests"
```

---

## Task 9: SettingsCheck gate and first-run wizard behavior

**Files:**
- Create: `internal/tui/screens/settings_check.go`
- Create: `internal/tui/screens/settings_check_test.go`
- Modify: `internal/tui/app.go`
- Modify: `internal/tui/app_test.go`

- [ ] **Step 1: Write failing SettingsCheck tests**

Cover:
- missing config file opens ConfigEditor in wizard mode
- malformed config opens ConfigEditor with load error visible and defaults as draft
- incomplete config opens ConfigEditor with validation errors
- valid config skips to MainMenu
- wizard save returns to MainMenu
- `Esc` cannot bypass required wizard

- [ ] **Step 2: Implement SettingsCheck screen/command**

SettingsCheck should:
- run automatically on startup
- call `ConfigStore.Load`
- merge runtime CLI/env overrides if command wiring supplies them for this run
- call full config validation
- route to ConfigEditor when invalid/missing
- route to MainMenu when valid

- [ ] **Step 3: Add user-facing messages**

Keep messages concise and Russian-friendly:
- config missing → "Нужно настроить подключение перед первым запуском"
- config invalid → "Конфиг неполный, проверьте поля"
- save success → "Настройки сохранены"

- [ ] **Step 4: Run tests**

```bash
go test -race ./internal/tui ./internal/tui/screens -run 'TestSettingsCheck|TestAppInitial' -count=1
```

- [ ] **Step 5: Commit**

```bash
git add internal/tui/screens/settings_check.go internal/tui/screens/settings_check_test.go \
  internal/tui/app.go internal/tui/app_test.go
git commit -m "feat(tui): add first-run settings gate"
```

---

## Task 10: MainMenu and DatabaseList screens

**Files:**
- Create: `internal/tui/screens/main_menu.go`
- Create: `internal/tui/screens/main_menu_test.go`
- Create: `internal/tui/screens/database_list.go`
- Create: `internal/tui/screens/database_list_test.go`
- Modify: `internal/tui/app.go`
- Modify: `internal/tui/app_test.go`

- [ ] **Step 1: Write failing MainMenu tests**

MainMenu should expose actions:
- Sync database(s)
- Open ConfigEditor
- Status/doctor summary if Phase 2 exposes it
- Quit

Cover keyboard navigation, Enter selection, `s` shortcut, and help footer text.

- [ ] **Step 2: Implement MainMenu screen**

Keep it a simple list model. It should not fetch DBs until the user selects sync/list action.

- [ ] **Step 3: Write failing DatabaseList tests**

Cover:
- loading state calls `CatalogService.ListDatabases`
- success renders database name and size via `models.FormatBytes`
- empty list shows actionable message
- error state supports retry
- multi-select toggles rows
- selected DBs are stored in app state
- Enter proceeds to ConfirmPlan when advanced table-pick mode is off
- Enter proceeds to TablesPick when advanced table-pick mode is on

- [ ] **Step 4: Implement DatabaseList screen**

Use huh multi-select if it is testable enough through the screen wrapper; otherwise implement Bubble Tea list behavior and reserve huh for forms. The user-facing behavior matters more than forcing every list through huh.

- [ ] **Step 5: Run tests**

```bash
go test -race ./internal/tui ./internal/tui/screens -run 'TestMainMenu|TestDatabaseList' -count=1
```

- [ ] **Step 6: Commit**

```bash
git add internal/tui/screens/main_menu.go internal/tui/screens/main_menu_test.go \
  internal/tui/screens/database_list.go internal/tui/screens/database_list_test.go \
  internal/tui/app.go internal/tui/app_test.go
git commit -m "feat(tui): add main menu and database selection"
```

---

## Task 11: TablesPick and ConfirmPlan screens

**Files:**
- Create: `internal/tui/screens/tables_pick.go`
- Create: `internal/tui/screens/tables_pick_test.go`
- Create: `internal/tui/screens/confirm_plan.go`
- Create: `internal/tui/screens/confirm_plan_test.go`
- Modify: `internal/tui/app.go`
- Modify: `internal/tui/app_test.go`

- [ ] **Step 1: Write failing TablesPick tests**

Cover:
- table list loads through `CatalogService.ListTables`
- table rows show schema-qualified name, estimated rows, and size
- selected table subset is passed to planner
- FK closure added by Phase 2 service is displayed as "auto-added dependencies"
- error state supports retry/back
- empty selection means full DB sync, not invalid input

- [ ] **Step 2: Implement TablesPick screen**

This screen is optional/advanced. It should be reachable only from a clear MainMenu/DatabaseList action, not required for normal full DB sync.

- [ ] **Step 3: Write failing ConfirmPlan tests**

ConfirmPlan must render:
- db name(s)
- total estimated size
- target action: drop & recreate
- engine
- thread count
- selected/auto-added table counts
- estimated time if Phase 2 planner provides it, otherwise `unknown`

Test actions:
- confirm starts sync/progress
- back returns to DB/table selection
- dry-run mode returns result/summary without executing

- [ ] **Step 4: Implement ConfirmPlan screen**

Call Phase 2 planner exactly once per input set unless user changes selection. Cache plan in app state. On errors, show error panel and allow back/retry.

- [ ] **Step 5: Run tests**

```bash
go test -race ./internal/tui ./internal/tui/screens -run 'TestTablesPick|TestConfirmPlan' -count=1
```

- [ ] **Step 6: Commit**

```bash
git add internal/tui/screens/tables_pick.go internal/tui/screens/tables_pick_test.go \
  internal/tui/screens/confirm_plan.go internal/tui/screens/confirm_plan_test.go \
  internal/tui/app.go internal/tui/app_test.go
git commit -m "feat(tui): add table selection and plan confirmation"
```

---

## Task 12: Sync queue, Progress screen, and Result screen

**Files:**
- Create: `internal/tui/screens/progress.go`
- Create: `internal/tui/screens/progress_test.go`
- Create: `internal/tui/screens/result.go`
- Create: `internal/tui/screens/result_test.go`
- Create: `internal/tui/queue.go`
- Create: `internal/tui/queue_test.go`
- Modify: `internal/tui/app.go`
- Modify: `internal/tui/app_test.go`

- [ ] **Step 1: Write failing queue tests**

Queue behavior from spec/dbsync carry-over:
- multiple DBs can be prepared and run as a batch
- queue starts in FIFO order
- pause prevents starting next sync but does not corrupt current state
- resume continues with next queued item
- cancel requests context cancellation for current item
- completed/failed results are retained for Result screen

- [ ] **Step 2: Implement queue model**

Keep queue pure and separate from Bubble Tea rendering. The executor should be called through `SyncExecutor` so tests can simulate progress/failure.

- [ ] **Step 3: Write failing Progress tests**

Progress screen must render:
- overall progress
- per-table progress
- current stage: predata/copy/postdata/sequences/analyze if provided
- rows, percent, bytes/sec when available
- pause/resume controls
- add-more affordance if queue is paused or idle

Update tests should feed Phase 2 progress messages and assert rendered state changes.

- [ ] **Step 4: Implement Progress screen**

Use Bubble Tea commands to bridge executor callbacks into `tea.Msg` values. Do not let engine goroutines mutate TUI state directly.

- [ ] **Step 5: Write failing Result tests**

Result screen must show:
- duration
- MB/s or bytes/sec summary
- total MB/bytes
- total rows
- tables copied
- errors grouped by DB/table/stage
- next actions: back to MainMenu, rerun failed, quit

- [ ] **Step 6: Implement Result screen**

Use Phase 1/2 result models. Ensure failed syncs do not show as successful if partial progress exists.

- [ ] **Step 7: Run tests with race detector**

```bash
go test -race ./internal/tui ./internal/tui/screens -run 'TestQueue|TestProgress|TestResult' -count=1
```

- [ ] **Step 8: Commit**

```bash
git add internal/tui/queue.go internal/tui/queue_test.go \
  internal/tui/screens/progress.go internal/tui/screens/progress_test.go \
  internal/tui/screens/result.go internal/tui/screens/result_test.go \
  internal/tui/app.go internal/tui/app_test.go
git commit -m "feat(tui): add sync queue progress and results"
```

---

## Task 13: CLI command wiring for TUI, sync, list, status, and doctor output

**Files:**
- Modify or create: `internal/cli/tui.go`
- Create: `internal/cli/tui_test.go`
- Modify or create: `internal/cli/sync.go`
- Modify or create: `internal/cli/sync_test.go`
- Modify or create: `internal/cli/list.go`
- Modify or create: `internal/cli/list_test.go`
- Modify or create: `internal/cli/status.go`
- Modify or create: `internal/cli/status_test.go`
- Modify or create: `internal/cli/doctor.go`
- Modify or create: `internal/cli/doctor_test.go`
- Modify: `internal/cli/commands.go`
- Modify: `cmd/pgsync/main.go`

- [ ] **Step 1: Write failing command routing tests**

Cover:
- `pgsync` with no args runs TUI
- `pgsync tui` runs TUI explicitly
- `pgsync config` runs ConfigEditor TUI mode
- non-interactive commands do not start TUI
- non-TTY no-args returns helpful error or help text if production runner says TTY is unavailable

- [ ] **Step 2: Implement TUI runner injection**

`cmd/pgsync/main.go` may build the real Bubble Tea program. `internal/cli` should accept a `TUIRunner` fake in tests. Keep `main.go` thin and within the existing coverage allow-list expectations.

- [ ] **Step 3: Write failing sync command tests for output modes**

Cover:
- `sync <db> --dry-run --output=text` prints plan summary
- `sync <db> --dry-run --output=json` emits NDJSON plan/done events
- `sync <db> --yes --output=json` emits spec sync events during fake execution
- missing confirmation in text mode prompts or fails deterministically in tests
- JSON mode never prompts; require `--yes` or `--dry-run` for non-interactive sync if needed
- execution failure emits `sync.failed` NDJSON and non-zero exit

- [ ] **Step 4: Implement sync command output integration**

Use Phase 2 planner/executor. Human mode can print confirmation and progress text; JSON mode must use `AgentWriter` only.

- [ ] **Step 5: Write failing list/status/doctor tests for JSON and text modes**

Even though the spec gives detailed NDJSON examples for sync, global `--output=json` should be predictable for other commands:
- `list --output=json` emits one object per database or a stable wrapper event
- `status --output=json` emits connection status events
- `doctor --output=json` emits diagnostic events with pass/fail state

Document event names in tests. Keep them stable and simple.

- [ ] **Step 6: Implement list/status/doctor command output**

Text mode: readable developer output.
JSON mode: compact NDJSON, no color, no secret leakage, no progress spinners.

- [ ] **Step 7: Run command tests**

```bash
go test -race ./internal/cli ./cmd/pgsync -run 'Test.*Command|TestRootNoArgs' -count=1
```

- [ ] **Step 8: Commit**

```bash
git add internal/cli/tui.go internal/cli/tui_test.go \
  internal/cli/sync.go internal/cli/sync_test.go \
  internal/cli/list.go internal/cli/list_test.go \
  internal/cli/status.go internal/cli/status_test.go \
  internal/cli/doctor.go internal/cli/doctor_test.go \
  internal/cli/commands.go cmd/pgsync/main.go
git commit -m "feat(cli): wire TUI entrypoints and command output modes"
```

---

## Task 14: Text-mode interactive fallback

**Files:**
- Modify or create: `internal/cli/text_interactive.go`
- Create: `internal/cli/text_interactive_test.go`
- Modify: `internal/cli/commands.go`

- [ ] **Step 1: Write failing tests for `pgsync text`**

The spec lists `pgsync text` as interactive but not full-screen. Cover:
- command exists in help
- it does not launch Bubble Tea full-screen runner
- it can run ConfigEditor-style prompts through an injected prompter
- it supports a minimal happy path: choose DB → confirm plan → start sync
- it errors clearly when input is not interactive

- [ ] **Step 2: Implement a minimal text interactive flow**

Keep this intentionally small:
- use huh forms/prompts in non-fullscreen mode if practical
- reuse the same services as TUI
- reuse `AgentWriter` only if user explicitly sets `--output=json`; otherwise human prompts only

Do not duplicate ConfigEditor validation or sync planning logic.

- [ ] **Step 3: Run tests**

```bash
go test -race ./internal/cli -run TestTextInteractive -count=1
```

- [ ] **Step 4: Commit**

```bash
git add internal/cli/text_interactive.go internal/cli/text_interactive_test.go internal/cli/commands.go
git commit -m "feat(cli): add text-mode interactive fallback"
```

---

## Task 15: End-to-end smoke tests for TUI routing and NDJSON CLI behavior

**Files:**
- Create or modify: `test/e2e/cli_output_test.go`
- Create or modify: `test/e2e/tui_smoke_test.go`
- Create or modify: `test/helpers/cli.go`
- Create or modify: `test/helpers/tui.go`
- Modify if needed: `.github/workflows/ci.yml`

- [ ] **Step 1: Add CLI subprocess helpers**

Create helpers to run the built `pgsync` binary or `go run ./cmd/pgsync` with temp config and fake/test services where possible. Keep tests hermetic; do not require real prod credentials.

- [ ] **Step 2: Add NDJSON smoke tests**

Cover subprocess-level behavior:
- `pgsync config path --output=json` either ignores JSON for path by design or emits documented JSON; assert chosen behavior
- `pgsync sync demo --dry-run --yes --output=json` emits valid NDJSON only
- every stdout line parses as JSON
- stderr is empty on handled command failures
- process exit code is non-zero for simulated sync failure

- [ ] **Step 3: Add TUI routing smoke tests**

Do not try to snapshot the whole terminal. Smoke-test:
- no-args resolves to TUI runner in injectable command tests
- production binary prints a helpful message instead of hanging when TTY is unavailable in CI, if using non-TTY guard
- `pgsync tui --help` documents default behavior and shortcuts

- [ ] **Step 4: Wire CI if tests are reliable on all OSes**

If subprocess e2e tests are deterministic and fast, include them in normal CI. If PTY-sensitive, keep them behind an `e2e` build tag and document local/manual execution.

- [ ] **Step 5: Run smoke tests**

```bash
go test -race ./test/e2e/... -count=1
```

- [ ] **Step 6: Commit**

```bash
git add test/helpers/cli.go test/helpers/tui.go \
  test/e2e/cli_output_test.go test/e2e/tui_smoke_test.go .github/workflows/ci.yml
git commit -m "test(e2e): add TUI routing and NDJSON smoke coverage"
```

---

## Task 16: Phase 3 verification, documentation, and milestone tag

**Files:**
- Modify: `README.md`
- Modify or create: `CHANGELOG.md`
- Modify if needed: `docs/README.md`

- [ ] **Step 1: Run full verification suite**

```bash
gofmt -w cmd internal test

golangci-lint run ./...

go test -race ./...

go test -covermode=atomic -coverprofile=coverage.out ./internal/...

bash scripts/coverage-gate.sh coverage.out coverage.allow
```

- [ ] **Step 2: Manual CLI verification**

Run and capture outcomes:

```bash
go run ./cmd/pgsync --help
go run ./cmd/pgsync tui --help
go run ./cmd/pgsync config path
go run ./cmd/pgsync config show
go run ./cmd/pgsync list --output=json
go run ./cmd/pgsync sync demo --dry-run --yes --output=json
```

Expected:
- no secret leakage
- no color in JSON mode
- NDJSON lines parse with `jq -c .`
- stderr empty for handled JSON-mode failures

- [ ] **Step 3: Manual TUI verification**

Run:

```bash
go run ./cmd/pgsync
```

Verify:
- first-run SettingsCheck opens ConfigEditor if config is absent/incomplete
- Russian labels/help text are readable
- `s` opens ConfigEditor from every screen
- `Esc` returns to MainMenu where expected
- ConfigEditor Test connection reports both remote and local status
- Save writes TOML atomically and next launch skips wizard
- DatabaseList, ConfirmPlan, Progress, Result flows render without panics on narrow terminal widths

- [ ] **Step 4: Update README/CHANGELOG**

README should document:
- default TUI launch
- config is managed by `pgsync config`, not manual file editing
- `--output=json` NDJSON mode with one compact example
- `--no-color` and non-interactive command usage

CHANGELOG should add a Phase 3 entry.

- [ ] **Step 5: Commit docs**

```bash
git add README.md CHANGELOG.md docs/README.md
git commit -m "docs: document Phase 3 TUI and agent output"
```

- [ ] **Step 6: Tag milestone**

```bash
git tag phase-3-tui-agent-output
```

---

## Self-Review Checklist

**Spec coverage** — every spec section that belongs in Phase 3 has a task:

| Spec § | Concern | Phase-3 task |
|---|---|---|
| 1 | Developer can pull DB with one command / a few TUI clicks | 5, 10–13 |
| 2 | Full-screen TUI default and CLI commands | 5, 10–13 |
| 2 | LLM-friendly `--output=json` NDJSON stream | 2, 3, 13, 15 |
| 2 / 8 | Config fully managed through TUI, never manual editing | 4, 7–9 |
| 5 | `internal/cli` and `internal/tui` layout | 1–15 |
| 6 | CLI surface and global flags | 1, 3, 4, 13, 14 |
| 6 | NDJSON examples and stderr behavior | 2, 3, 13, 15 |
| 7 | TUI state machine and screens | 5, 9–12 |
| 7 | Queue, progress, result UX | 12 |
| 8.1 | SettingsCheck and ConfigEditor as only edit path | 4, 7–9, 13 |
| 8.2 | Remote/Local/Runtime/Logging fields | 7 |
| 8.3 | Atomic save through config store | 8 |
| 8.4 | Env/CLI overrides are one-shot and not persisted | 8, 9, 13 |
| 11.1–11.3 | Strict coverage, fakes, Bubble Tea model tests | every task |
| 16 | bubbletea/huh/lipgloss/cobra stack | 1, 5–14 |

**Out of Phase 3 scope (deferred or already handled):**
- Native engine internals, pgx COPY pipeline, pg_dump parser → Phase 2
- Embedded pg_dump/pg_restore binaries and extraction → Phase 4
- Self-update implementation → Phase 4
- Benchmark suite and fixture generator → Phase 4
- Keyring storage for passwords → post-MVP future from spec §8.5
- Web UI / GUI outside terminal → non-goal

**Security/UX checklist:**
- Passwords and proxy credentials redacted in all text, JSON, error, and test snapshot outputs.
- Config file path may be printed only by `config path`; no command tells the user to hand-edit it.
- JSON mode never emits spinners, ANSI color, tables, or mixed log text to stdout.
- Fatal stderr in JSON mode is concise and does not duplicate NDJSON errors.
- TUI is usable without mouse and with narrow terminal widths.
- Wizard cannot be bypassed when config is missing or invalid.

**Coverage checklist:**
- Every new `internal/` line covered by unit tests.
- Real terminal launch isolated outside coverage-sensitive internals or behind injectable interfaces.
- Huh form metadata tested via `FieldSpec`/builders, not by relying on terminal interaction.
- Engine/catalog/connection tests use fakes; no real network in unit tests.
- Race detector passes for progress callback bridge.

---

## Execution Handoff

Plan complete and saved to `docs/superpowers/plans/2026-05-02-pgsync-phase3-tui-agent-output.md`.

Recommended execution mode:
1. **Subagent-driven** — one fresh implementation subagent per task, with review after each commit. This is best for strict coverage and avoids mixing TUI, CLI, and config concerns.
2. **Inline execution** — execute tasks sequentially in this session, but still stop after every commit for test/coverage review.

Start with Task 1 only. Do not begin TUI screen work until output/options contracts and fakes are stable.
