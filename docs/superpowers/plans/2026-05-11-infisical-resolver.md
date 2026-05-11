# Infisical Per-Project DB-name Resolver Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Restore `pgsync sync` (no args) so it resolves the project DB name from Infisical:dev — replacing the now-defunct `.env` loader — while keeping cluster creds in `~/.config/pgsync/config.toml`.

**Architecture:** A new isolated package `internal/secrets/infisical` walks up from cwd to find `.infisical.json`, shells out to `infisical export --env=dev --format=dotenv --silent`, and extracts the DB name from `POSTGRES_URL` (Nuxt) or `DB_DATABASE` (Laravel). `internal/cli/config_resolver.go` invokes the resolver only when DB name is not already provided via TOML / `--remote-database` / positional arg, so existing override paths are preserved. The old dotenv loader and `applyConventionalEnv` convention bindings are removed.

**Tech Stack:** Go 1.25+ (1.26 installed); `os/exec` for `infisical` invocation; no new dependencies. Tests use the same in-package stub pattern (`exec.LookPath` and a `Run` function pointer injected on the `Resolver`) already used elsewhere in pgsync (`internal/engine/system_tools_test.go`).

**Reference docs:** `docs/superpowers/specs/2026-05-11-infisical-resolver-design.md`.

**Conventions for every task:**
- TDD: failing test first → run red → minimal implementation → run green → commit.
- Strict coverage for new code in `internal/secrets/infisical/`. Any unavoidable OS/process gap added to `coverage.allow` with a short comment.
- Inject all side effects: process exec (`Run`), `exec.LookPath` (`LookPath`), working dir (`CWD`). No global state.
- Do not log secrets. The resolver only ever returns the DB name; never logs Infisical stdout verbatim.
- Commits use explicit file lists via `bash ~/.claude/scripts/commit-files.sh "<msg>" <files...>` — never `git add .` / `-A` / `commit -am`.
- Multi-line comments use `/* ... */`, not chained `//`.
- After all tasks: run scoped pipeline (`gofmt -w <files>`, `go vet ./...`, `go test ./...`) and a manual smoke test against one real project.

---

## File Structure

**New files:**
- `internal/secrets/infisical/resolver.go` — `Resolver` type, `ResolveDBName` method, dotenv-line parser, URL path extractor.
- `internal/secrets/infisical/resolver_test.go` — unit tests for walk-up, parsing, priority, fail-fast paths.
- `internal/cli/sync_e2e_test.go` — black-box e2e test driving `NewRootCommand` with a fake `infisical` shell-stub in `PATH`.

**Modified files:**
- `internal/cli/config_resolver.go` — invoke Infisical resolver from `Resolve()` when `Remote.Database`/`Local.Database` not yet set; remove `loadDotEnv`/`parseDotEnvLine`/`parseDotEnvValue` and adjust `processEnv()`.
- `internal/cli/config_resolver_test.go` — drop dotenv tests; add tests asserting resolver bypass when DB name is already set and that it's invoked otherwise.
- `internal/cli/sync.go` — in `runSync`, pre-populate `overrides.Remote.Database` and `overrides.Local.Database` from the positional `db` arg before calling `Resolve`, so the positional arg bypasses Infisical via the existing "databases provided" check.
- `internal/config/override.go` — delete `applyConventionalEnv`, `applyPostgresURL`, `postgresURLDatabase`; `ApplyEnv` keeps only the `PGSYNC_*` bindings.
- `internal/config/override_test.go` — delete 6 tests covering `POSTGRES_URL` / `DB_DATABASE` conventions.

**Boundary check:** The resolver knows nothing about pgsync config types. It returns `(string, error)`. The CLI layer is the only place that maps that DB name onto `cfg.Remote.Database`/`cfg.Local.Database`/`cfg.Runtime.DefaultDatabase`. This keeps the package reusable and trivially testable.

---

## Task 1: Scaffold `internal/secrets/infisical` package + walk-up to `.infisical.json`

**Files:**
- Create: `internal/secrets/infisical/resolver.go`
- Create: `internal/secrets/infisical/resolver_test.go`

- [ ] **Step 1: Write the failing test for walk-up failure**

Create `internal/secrets/infisical/resolver_test.go`:

```go
package infisical_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mttzzz/pgsync/internal/secrets/infisical"
)

func TestResolveDBNameFailsWhenNoInfisicalJSON(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	r := infisical.Resolver{CWD: dir}
	_, err := r.ResolveDBName(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no .infisical.json found")
	assert.Contains(t, err.Error(), dir)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/secrets/infisical/...`
Expected: build failure ("package not found").

- [ ] **Step 3: Write minimal implementation**

Create `internal/secrets/infisical/resolver.go`:

```go
/* Package infisical resolves a project's database name from its local
 * Infisical workspace, by locating the nearest .infisical.json and
 * shelling out to `infisical export --env=dev --format=dotenv --silent`.
 */
package infisical

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
)

/* Resolver looks up the DB name for the project rooted at CWD.
 * LookPath and Run are injectable for tests; nil means use real os/exec. */
type Resolver struct {
	CWD      string
	LookPath func(name string) (string, error)
	Run      func(ctx context.Context, dir, name string, args ...string) ([]byte, []byte, error)
}

/* ResolveDBName walks up from r.CWD until it finds a .infisical.json,
 * then runs `infisical export --env=dev --format=dotenv --silent` from
 * that directory and extracts the DB name from POSTGRES_URL or DB_DATABASE. */
func (r Resolver) ResolveDBName(ctx context.Context) (string, error) {
	root, err := findInfisicalRoot(r.CWD)
	if err != nil {
		return "", err
	}
	_ = root
	_ = ctx
	return "", fmt.Errorf("not implemented")
}

func findInfisicalRoot(start string) (string, error) {
	dir := start
	if dir == "" {
		var err error
		dir, err = os.Getwd()
		if err != nil {
			return "", fmt.Errorf("getwd: %w", err)
		}
	}
	dir, err := filepath.Abs(dir)
	if err != nil {
		return "", fmt.Errorf("abs: %w", err)
	}
	for {
		if _, statErr := os.Stat(filepath.Join(dir, ".infisical.json")); statErr == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("pgsync: no .infisical.json found walking up from %s", start)
		}
		dir = parent
	}
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/secrets/infisical/...`
Expected: PASS for `TestResolveDBNameFailsWhenNoInfisicalJSON` (the "not implemented" error path is hit on the *positive* walk-up scenario, but that test isn't written yet).

- [ ] **Step 5: Add positive walk-up test**

Append to `internal/secrets/infisical/resolver_test.go`:

```go
func TestResolveDBNameFindsInfisicalJSONInParent(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	require := requireT(t)
	require.NoError(os.WriteFile(filepath.Join(dir, ".infisical.json"), []byte(`{"workspaceId":"x"}`), 0o600))
	sub := filepath.Join(dir, "nested", "deep")
	require.NoError(os.MkdirAll(sub, 0o755))

	r := infisical.Resolver{CWD: sub}
	_, err := r.ResolveDBName(context.Background())
	require.Error(err)
	/* walk-up succeeded; we land in "not implemented" until later tasks. */
	assert.Contains(t, err.Error(), "not implemented")
}
```

Add helpers / imports as needed at the top of the file:

```go
import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mttzzz/pgsync/internal/secrets/infisical"
)

func requireT(t *testing.T) *require.Assertions {
	t.Helper()
	return require.New(t)
}
```

- [ ] **Step 6: Run tests**

Run: `go test ./internal/secrets/infisical/...`
Expected: both tests PASS.

- [ ] **Step 7: Commit**

```bash
bash ~/.claude/scripts/commit-files.sh "feat(secrets/infisical): scaffold resolver with .infisical.json walk-up" \
  internal/secrets/infisical/resolver.go \
  internal/secrets/infisical/resolver_test.go
```

---

## Task 2: Fail fast when `infisical` binary not in PATH

**Files:**
- Modify: `internal/secrets/infisical/resolver.go`
- Modify: `internal/secrets/infisical/resolver_test.go`

- [ ] **Step 1: Write the failing test**

Append to `internal/secrets/infisical/resolver_test.go`:

```go
func TestResolveDBNameFailsWhenInfisicalBinaryMissing(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".infisical.json"), []byte(`{}`), 0o600))

	r := infisical.Resolver{
		CWD:      dir,
		LookPath: func(string) (string, error) { return "", os.ErrNotExist },
	}
	_, err := r.ResolveDBName(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "'infisical' CLI not found in PATH")
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/secrets/infisical/...`
Expected: FAIL — current implementation still returns "not implemented".

- [ ] **Step 3: Implement PATH check**

Edit `internal/secrets/infisical/resolver.go`. Replace the `ResolveDBName` body:

```go
func (r Resolver) ResolveDBName(ctx context.Context) (string, error) {
	root, err := findInfisicalRoot(r.CWD)
	if err != nil {
		return "", err
	}
	lookPath := r.LookPath
	if lookPath == nil {
		lookPath = exec.LookPath
	}
	if _, err := lookPath("infisical"); err != nil {
		return "", fmt.Errorf("pgsync: 'infisical' CLI not found in PATH (install: https://infisical.com/docs/cli/overview)")
	}
	_ = root
	return "", fmt.Errorf("not implemented")
}
```

Add the import for `os/exec`:

```go
import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/secrets/infisical/...`
Expected: all three tests PASS.

- [ ] **Step 5: Commit**

```bash
bash ~/.claude/scripts/commit-files.sh "feat(secrets/infisical): fail fast when infisical CLI absent from PATH" \
  internal/secrets/infisical/resolver.go \
  internal/secrets/infisical/resolver_test.go
```

---

## Task 3: Execute `infisical export` and parse dotenv output

**Files:**
- Modify: `internal/secrets/infisical/resolver.go`
- Modify: `internal/secrets/infisical/resolver_test.go`

- [ ] **Step 1: Write the failing happy-path tests (POSTGRES_URL and DB_DATABASE)**

Append to `internal/secrets/infisical/resolver_test.go`:

```go
func TestResolveDBNameExtractsFromPostgresURL(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".infisical.json"), []byte(`{}`), 0o600))

	r := infisical.Resolver{
		CWD:      dir,
		LookPath: func(string) (string, error) { return "/usr/bin/infisical", nil },
		Run: func(_ context.Context, gotDir, name string, args ...string) ([]byte, []byte, error) {
			assert.Equal(t, dir, gotDir)
			assert.Equal(t, "infisical", name)
			assert.Equal(t, []string{"export", "--env=dev", "--format=dotenv", "--silent"}, args)
			return []byte("POSTGRES_URL='postgresql://u:p@h:5432/ai_pushka_biz?sslmode=require'\n"), nil, nil
		},
	}
	name, err := r.ResolveDBName(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "ai_pushka_biz", name)
}

func TestResolveDBNameExtractsFromDBDatabase(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".infisical.json"), []byte(`{}`), 0o600))

	r := infisical.Resolver{
		CWD:      dir,
		LookPath: func(string) (string, error) { return "/usr/bin/infisical", nil },
		Run: func(context.Context, string, string, ...string) ([]byte, []byte, error) {
			return []byte("DB_DATABASE=masterm_pushka_biz\n"), nil, nil
		},
	}
	name, err := r.ResolveDBName(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "masterm_pushka_biz", name)
}

func TestResolveDBNamePostgresURLBeatsDBDatabase(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".infisical.json"), []byte(`{}`), 0o600))

	r := infisical.Resolver{
		CWD:      dir,
		LookPath: func(string) (string, error) { return "/usr/bin/infisical", nil },
		Run: func(context.Context, string, string, ...string) ([]byte, []byte, error) {
			return []byte(strings.Join([]string{
				"DB_DATABASE=ignored",
				`POSTGRES_URL="postgres://u@h/from_url"`,
			}, "\n") + "\n"), nil, nil
		},
	}
	name, err := r.ResolveDBName(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "from_url", name)
}
```

Add `"strings"` to test file imports.

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/secrets/infisical/...`
Expected: FAIL — "not implemented".

- [ ] **Step 3: Implement export+parse**

Edit `internal/secrets/infisical/resolver.go`. Replace `ResolveDBName` body and add helper funcs:

```go
func (r Resolver) ResolveDBName(ctx context.Context) (string, error) {
	root, err := findInfisicalRoot(r.CWD)
	if err != nil {
		return "", err
	}
	lookPath := r.LookPath
	if lookPath == nil {
		lookPath = exec.LookPath
	}
	if _, err := lookPath("infisical"); err != nil {
		return "", fmt.Errorf("pgsync: 'infisical' CLI not found in PATH (install: https://infisical.com/docs/cli/overview)")
	}
	run := r.Run
	if run == nil {
		run = defaultRun
	}
	stdout, stderr, err := run(ctx, root, "infisical", "export", "--env=dev", "--format=dotenv", "--silent")
	if err != nil {
		return "", fmt.Errorf("pgsync: infisical export failed: %s: %w", strings.TrimSpace(string(stderr)), err)
	}
	env := parseDotenv(stdout)
	if url := strings.TrimSpace(env["POSTGRES_URL"]); url != "" {
		name, parseErr := dbFromPostgresURL(url)
		if parseErr != nil {
			return "", fmt.Errorf("pgsync: parse POSTGRES_URL from Infisical: %w", parseErr)
		}
		if name != "" {
			return name, nil
		}
	}
	if name := strings.TrimSpace(env["DB_DATABASE"]); name != "" {
		return name, nil
	}
	return "", fmt.Errorf("pgsync: cannot resolve DB name from Infisical (env=dev): neither POSTGRES_URL nor DB_DATABASE is set")
}

func defaultRun(ctx context.Context, dir, name string, args ...string) ([]byte, []byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return stdout.Bytes(), stderr.Bytes(), err
}

func parseDotenv(blob []byte) map[string]string {
	out := make(map[string]string)
	for _, line := range strings.Split(string(blob), "\n") {
		k, v, ok := parseDotenvLine(line)
		if ok {
			out[k] = v
		}
	}
	return out
}

func parseDotenvLine(line string) (string, string, bool) {
	line = strings.TrimSpace(strings.TrimSuffix(line, "\r"))
	if line == "" || strings.HasPrefix(line, "#") {
		return "", "", false
	}
	line = strings.TrimSpace(strings.TrimPrefix(line, "export "))
	key, value, ok := strings.Cut(line, "=")
	if !ok {
		return "", "", false
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return "", "", false
	}
	value = strings.TrimSpace(value)
	if len(value) >= 2 {
		q := value[0]
		if (q == '\'' || q == '"') && value[len(value)-1] == q {
			return key, value[1 : len(value)-1], true
		}
	}
	if before, _, hashOK := strings.Cut(value, " #"); hashOK {
		value = strings.TrimSpace(before)
	}
	return key, value, true
}

func dbFromPostgresURL(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("parse url: %w", err)
	}
	if u.Scheme != "postgres" && u.Scheme != "postgresql" {
		return "", fmt.Errorf("scheme must be postgres or postgresql, got %q", u.Scheme)
	}
	return strings.TrimPrefix(u.Path, "/"), nil
}
```

Update imports:

```go
import (
	"bytes"
	"context"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/secrets/infisical/...`
Expected: all tests PASS.

- [ ] **Step 5: Commit**

```bash
bash ~/.claude/scripts/commit-files.sh "feat(secrets/infisical): resolve DB name from POSTGRES_URL or DB_DATABASE" \
  internal/secrets/infisical/resolver.go \
  internal/secrets/infisical/resolver_test.go
```

---

## Task 4: Error paths — non-zero exit, missing vars, bad URL

**Files:**
- Modify: `internal/secrets/infisical/resolver_test.go`

- [ ] **Step 1: Write failing tests for the three error paths**

Append to `internal/secrets/infisical/resolver_test.go`:

```go
func TestResolveDBNameFailsWhenInfisicalReturnsError(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".infisical.json"), []byte(`{}`), 0o600))

	r := infisical.Resolver{
		CWD:      dir,
		LookPath: func(string) (string, error) { return "/usr/bin/infisical", nil },
		Run: func(context.Context, string, string, ...string) ([]byte, []byte, error) {
			return nil, []byte("Unauthorized: token expired"), errors.New("exit status 1")
		},
	}
	_, err := r.ResolveDBName(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "infisical export failed")
	assert.Contains(t, err.Error(), "Unauthorized: token expired")
}

func TestResolveDBNameFailsWhenNoDBVars(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".infisical.json"), []byte(`{}`), 0o600))

	r := infisical.Resolver{
		CWD:      dir,
		LookPath: func(string) (string, error) { return "/usr/bin/infisical", nil },
		Run: func(context.Context, string, string, ...string) ([]byte, []byte, error) {
			return []byte("SOME_OTHER=value\n"), nil, nil
		},
	}
	_, err := r.ResolveDBName(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot resolve DB name from Infisical")
}

func TestResolveDBNameFailsOnBadPostgresURL(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".infisical.json"), []byte(`{}`), 0o600))

	r := infisical.Resolver{
		CWD:      dir,
		LookPath: func(string) (string, error) { return "/usr/bin/infisical", nil },
		Run: func(context.Context, string, string, ...string) ([]byte, []byte, error) {
			return []byte("POSTGRES_URL='mysql://app@h/db'\n"), nil, nil
		},
	}
	_, err := r.ResolveDBName(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parse POSTGRES_URL")
}
```

Add `"errors"` to test imports.

- [ ] **Step 2: Run tests**

Run: `go test ./internal/secrets/infisical/...`
Expected: all PASS (implementation already covers these paths from Task 3).

- [ ] **Step 3: Verify coverage**

Run: `go test -cover ./internal/secrets/infisical/...`
Expected: `coverage: ≥ 95.0%`. If lower, add tests for any uncovered branch (likely the `os.Getwd` fallback when `CWD` is empty — add a tiny test that sets `CWD = ""` and verifies success in a tmpdir made cwd via `t.Chdir`).

- [ ] **Step 4: Commit**

```bash
bash ~/.claude/scripts/commit-files.sh "test(secrets/infisical): cover infisical exit failures and missing vars" \
  internal/secrets/infisical/resolver_test.go
```

---

## Task 5: Wire positional `pgsync sync <db>` into FlagOverrides so it bypasses Infisical

**Files:**
- Modify: `internal/cli/sync.go`
- Modify: `internal/cli/sync_test.go` (existing test file)

- [ ] **Step 1: Write the failing test**

The goal of this task is purely wiring: when the user types `pgsync sync <db>`, the positional `db` should appear in `FlagOverrides.Remote.Database` and `FlagOverrides.Local.Database` **before** `Resolve` runs. Currently `runSync` only passes `db` into `PlanOptionsFromConfig`, leaving the resolver blind to it.

We need a way for the test to observe what `Resolve` sees. Refactor `runSync` so its `Resolve` call goes through an injectable hook on `App` (default = the real `Resolve`). Add to `internal/cli/commands.go`:

```go
type App struct {
	EngineFactory func(*slog.Logger) (engine.Engine, error)
	TUIRunner     TUIRunner
	Updater       UpdateService
	Out           io.Writer
	Err           io.Writer
	In            io.Reader
	ResolveFn     func(context.Context, FlagOverrides) (config.Config, error) /* injectable for tests */
}
```

And in `normalizeApp`:

```go
if app.ResolveFn == nil {
	app.ResolveFn = Resolve
}
```

Then add a focused test in `internal/cli/sync_test.go`:

```go
func TestRunSyncPositionalArgPopulatesOverrideDatabases(t *testing.T) {
	t.Parallel()
	var seen FlagOverrides
	app := App{
		Out: io.Discard,
		Err: io.Discard,
		ResolveFn: func(_ context.Context, o FlagOverrides) (config.Config, error) {
			seen = o
			return config.Config{}, errors.New("stop after capture")
		},
	}
	root := NewRootCommand(app)
	root.SetArgs([]string{"sync", "from_positional", "--dry-run"})
	_ = root.ExecuteContext(context.Background())
	assert.Equal(t, "from_positional", seen.Remote.Database)
	assert.Equal(t, "from_positional", seen.Local.Database)
}
```

Add `"errors"` and `"io"` to imports if not already present.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/cli/...`
Expected: FAIL — `seen.Remote.Database` is empty (the wiring change in Step 3 is what populates it).

- [ ] **Step 3: Modify `runSync` to pre-populate overrides from the positional arg**

Edit `internal/cli/sync.go`. Replace the `runSync` function:

```go
func runSync(ctx context.Context, app App, overrides FlagOverrides, syncFlags SyncFlags, db string) error {
	db = strings.TrimSpace(db)
	if db != "" {
		if overrides.Remote.Database == "" {
			overrides.Remote.Database = db
		}
		if overrides.Local.Database == "" {
			overrides.Local.Database = db
		}
	}
	cfg, err := app.ResolveFn(ctx, overrides)
	if err != nil {
		return err
	}
	opts, err := PlanOptionsFromConfig(cfg, db, syncFlags)
	if err != nil {
		return sanitizeError(err, cfg)
	}
	logger := syncLogger(cfg, overrides, app.Err)
	eng, err := app.EngineFactory(logger)
	if err != nil {
		return sanitizeError(err, cfg)
	}
	plan, err := eng.Plan(ctx, opts)
	if err != nil {
		return sanitizeError(err, cfg)
	}
	plainOpts := plainOptions(overrides)
	if syncFlags.DryRun {
		return PrintPlan(app.Out, plan, plainOpts)
	}
	result, err := eng.Execute(ctx, plan, newSyncObserver(app.Out, overrides))
	if err != nil {
		return sanitizeError(err, cfg)
	}
	if overrides.Output == "json" {
		return nil
	}
	return PrintResult(app.Out, result, plainOpts)
}
```

(Only the first four lines after the function signature changed.)

- [ ] **Step 4: Run tests**

Run: `go test ./internal/cli/...`
Expected: tests PASS.

- [ ] **Step 5: Commit**

```bash
bash ~/.claude/scripts/commit-files.sh "feat(cli/sync): pre-populate FlagOverrides.Database from positional arg" \
  internal/cli/commands.go \
  internal/cli/sync.go \
  internal/cli/sync_test.go
```

---

## Task 6: Integrate Infisical resolver into `Resolve()` when DB name is unset

**Files:**
- Modify: `internal/cli/config_resolver.go`
- Modify: `internal/cli/config_resolver_test.go`

- [ ] **Step 1: Write failing tests for bypass + invocation**

Append to `internal/cli/config_resolver_test.go`:

```go
func TestResolverSkipsInfisicalWhenDatabaseAlreadySet(t *testing.T) {
	t.Parallel()
	called := 0
	r := Resolver{
		StorePath: writeTestConfig(t, testConfig()),
		Env:       map[string]string{},
		Infisical: &stubInfisical{
			fn: func(context.Context) (string, error) {
				called++
				return "should_not_be_called", nil
			},
		},
	}
	cfg, err := r.Resolve(context.Background(), FlagOverrides{
		Remote: config.Connection{Database: "from_flag"},
		Local:  config.Connection{Database: "from_flag"},
	})
	require.NoError(t, err)
	assert.Equal(t, "from_flag", cfg.Remote.Database)
	assert.Equal(t, 0, called)
}

func TestResolverInvokesInfisicalWhenDatabaseUnset(t *testing.T) {
	t.Parallel()
	cfg := testConfig()
	cfg.Remote.Database = ""
	cfg.Local.Database = ""
	cfg.Runtime.DefaultDatabase = ""

	r := Resolver{
		StorePath: writeTestConfig(t, cfg),
		Env:       map[string]string{},
		Infisical: &stubInfisical{
			fn: func(context.Context) (string, error) { return "resolved_db", nil },
		},
	}
	got, err := r.Resolve(context.Background(), FlagOverrides{})
	require.NoError(t, err)
	assert.Equal(t, "resolved_db", got.Remote.Database)
	assert.Equal(t, "resolved_db", got.Local.Database)
	assert.Equal(t, "resolved_db", got.Runtime.DefaultDatabase)
}

func TestResolverPropagatesInfisicalError(t *testing.T) {
	t.Parallel()
	cfg := testConfig()
	cfg.Remote.Database = ""
	cfg.Local.Database = ""
	cfg.Runtime.DefaultDatabase = ""
	r := Resolver{
		StorePath: writeTestConfig(t, cfg),
		Env:       map[string]string{},
		Infisical: &stubInfisical{
			fn: func(context.Context) (string, error) {
				return "", errors.New("pgsync: no .infisical.json found")
			},
		},
	}
	_, err := r.Resolve(context.Background(), FlagOverrides{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no .infisical.json")
}

type stubInfisical struct {
	fn func(context.Context) (string, error)
}

func (s *stubInfisical) ResolveDBName(ctx context.Context) (string, error) {
	return s.fn(ctx)
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/cli/...`
Expected: build failure ("Infisical field undefined on Resolver").

- [ ] **Step 3: Implement integration in `Resolve()`**

Edit `internal/cli/config_resolver.go`. Add a small interface and a field to `Resolver`, and call it after the existing overlay chain:

```go
/* DBNameResolver resolves the project DB name (e.g. via Infisical).
 * Implementations live in internal/secrets/*. */
type DBNameResolver interface {
	ResolveDBName(ctx context.Context) (string, error)
}

type Resolver struct {
	StorePath string
	Env       map[string]string
	Infisical DBNameResolver
}
```

In `Resolve`, **order matters**: surface TOML-load errors first (existing `TestResolverDefaultPathErrorIsLoadError` depends on it), then run the Infisical resolver. Final shape:

```go
cfg = applyFlagOverrides(cfg, flags)

if loadErr != nil && !connectionHostsProvided(env, flags) {
	return config.Config{}, fmt.Errorf("load config: %w", loadErr)
}

if strings.TrimSpace(cfg.Remote.Database) == "" || strings.TrimSpace(cfg.Local.Database) == "" {
	resolver := r.Infisical
	if resolver == nil {
		resolver = infisical.Resolver{}
	}
	name, resolveErr := resolver.ResolveDBName(ctx)
	if resolveErr != nil {
		return config.Config{}, resolveErr
	}
	if strings.TrimSpace(cfg.Remote.Database) == "" {
		cfg.Remote.Database = name
	}
	if strings.TrimSpace(cfg.Local.Database) == "" {
		cfg.Local.Database = name
	}
	if strings.TrimSpace(cfg.Runtime.DefaultDatabase) == "" {
		cfg.Runtime.DefaultDatabase = name
	}
}
```

Move/delete the original duplicate `if loadErr != nil ...` block that previously lived *below* the validation call. There should be exactly one such check, placed before the Infisical block as shown.

Add the import:

```go
"github.com/mttzzz/pgsync/internal/secrets/infisical"
```

Update the package-level `Resolve` factory to pass `nil` Infisical (so production uses the default):

```go
func Resolve(ctx context.Context, flags FlagOverrides) (config.Config, error) {
	return Resolver{Env: processEnv()}.Resolve(ctx, flags)
}
```

(no change needed — the nil-check above handles it.)

- [ ] **Step 4: Run tests**

Run: `go test ./internal/cli/... ./internal/secrets/...`
Expected: all PASS, including the existing `TestResolverDefaultPathErrorIsLoadError` (the load-error check sits before Infisical per Step 3).

- [ ] **Step 5: Commit**

```bash
bash ~/.claude/scripts/commit-files.sh "feat(cli): resolve DB name via Infisical when not provided" \
  internal/cli/config_resolver.go \
  internal/cli/config_resolver_test.go
```

---

## Task 7: Remove `applyConventionalEnv` (DB_DATABASE / POSTGRES_URL) from `override.go`

**Files:**
- Modify: `internal/config/override.go`
- Modify: `internal/config/override_test.go`

- [ ] **Step 1: Delete the convention-binding code**

In `internal/config/override.go`, delete:
- `applyConventionalEnv` (lines ~55–71)
- `applyPostgresURL` (lines ~119–150)
- `postgresURLDatabase` (lines ~152–154)
- The `cfg, err = applyConventionalEnv(cfg, env)` call inside `ApplyEnv` (lines ~42–45). The function now jumps straight to `applyPGSyncBindings`.

The new `ApplyEnv` body:

```go
func ApplyEnv(cfg Config, env map[string]string) (Config, error) {
	mustInt := func(target *int) func(string) error {
		return func(s string) error {
			value, err := strconv.Atoi(s)
			if err != nil {
				return fmt.Errorf("parse int: %w", err)
			}
			*target = value
			return nil
		}
	}
	mustBool := func(target *bool) func(string) error {
		return func(s string) error {
			value, err := strconv.ParseBool(s)
			if err != nil {
				return fmt.Errorf("parse bool: %w", err)
			}
			*target = value
			return nil
		}
	}
	mustStr := func(target *string) func(string) error {
		return func(s string) error {
			*target = s
			return nil
		}
	}
	return applyPGSyncBindings(cfg, env, mustInt, mustBool, mustStr)
}
```

Remove the now-unused imports `"net/url"` if no other code uses it.

- [ ] **Step 2: Delete affected tests in `override_test.go`**

Delete these test funcs:
- `TestApplyEnvPostgresURL`
- `TestApplyEnvPostgresURLCanBeOverriddenByPGSyncEnv`
- `TestApplyEnvPostgresURLDefaultsOptionalParts`
- `TestApplyEnvDBDatabaseSetsLocalAndDefaultDatabase`
- `TestApplyEnvPostgresURLOverridesDBDatabase`
- `TestApplyEnvBadPostgresURL`

Remove the `"strings"` import from `override_test.go` if no remaining test uses it.

- [ ] **Step 3: Run tests**

Run: `go test ./internal/config/...`
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
bash ~/.claude/scripts/commit-files.sh "refactor(config): drop DB_DATABASE/POSTGRES_URL env conventions (replaced by Infisical resolver)" \
  internal/config/override.go \
  internal/config/override_test.go
```

---

## Task 8: Remove `.env` dotenv loading from `config_resolver.go`

**Files:**
- Modify: `internal/cli/config_resolver.go`
- Modify: `internal/cli/config_resolver_test.go`

- [ ] **Step 1: Delete dotenv functions and rewrite `processEnv`**

In `internal/cli/config_resolver.go`:

Replace `processEnv` with:

```go
func processEnv() map[string]string {
	env := make(map[string]string)
	for key, value := range envMap(os.Environ()) {
		if strings.TrimSpace(value) == "" {
			continue
		}
		env[key] = value
	}
	return env
}
```

Delete: `loadDotEnv`, `parseDotEnvLine`, `parseDotEnvValue`.

Also remove the `local == "" → local = env["POSTGRES_URL"]` branch from `connectionHostsProvided`:

```go
func connectionHostsProvided(env map[string]string, flags FlagOverrides) bool {
	remote := strings.TrimSpace(flags.Remote.Host)
	if remote == "" {
		remote = strings.TrimSpace(env["PGSYNC_REMOTE_HOST"])
	}
	local := strings.TrimSpace(flags.Local.Host)
	if local == "" {
		local = strings.TrimSpace(env["PGSYNC_LOCAL_HOST"])
	}
	return remote != "" && local != ""
}
```

- [ ] **Step 2: Delete affected tests in `config_resolver_test.go`**

Delete:
- `TestResolveLoadsDotEnvPostgresURL`
- `TestResolverUsesPostgresURLForLocalAndDefaultDatabase`
- `TestDotEnvParsing`

Strip the `POSTGRES_URL` key from `clearPGSyncEnv`'s key list (it's no longer touched).

- [ ] **Step 3: Run tests**

Run: `go test ./internal/...`
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
bash ~/.claude/scripts/commit-files.sh "refactor(cli): drop .env dotenv loader (Infisical is the source for DB name)" \
  internal/cli/config_resolver.go \
  internal/cli/config_resolver_test.go
```

---

## Task 9: e2e test — fake `infisical` shell-stub in PATH, run pgsync sync --dry-run

**Files:**
- Create: `internal/cli/sync_e2e_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/cli/sync_e2e_test.go`:

```go
package cli_test

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mttzzz/pgsync/internal/cli"
	"github.com/mttzzz/pgsync/internal/config"
	"github.com/mttzzz/pgsync/internal/engine"
	"github.com/mttzzz/pgsync/internal/models"
)

func TestSyncResolvesDBNameViaInfisicalE2E(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell-stub e2e is POSIX-only")
	}

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".infisical.json"), []byte(`{"workspaceId":"x"}`), 0o600))

	binDir := filepath.Join(dir, "bin")
	require.NoError(t, os.Mkdir(binDir, 0o755))
	stub := filepath.Join(binDir, "infisical")
	require.NoError(t, os.WriteFile(stub, []byte("#!/bin/sh\necho \"POSTGRES_URL='postgresql://u:p@h:5432/e2e_db?sslmode=require'\"\n"), 0o755))

	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Chdir(dir)

	cfgPath := filepath.Join(dir, "config.toml")
	cfg := config.Defaults()
	cfg.Remote = config.Connection{Host: "remote.example.com", Port: 5432, User: "u", Password: "p", SSLMode: "require"}
	cfg.Local = config.Connection{Host: "localhost", Port: 5432, User: "u", SSLMode: "disable"}
	require.NoError(t, config.Save(cfgPath, cfg))

	out := &bytes.Buffer{}
	stubEng := &capturingEngine{}
	app := cli.App{
		Out:           out,
		Err:           io.Discard,
		EngineFactory: func(*slog.Logger) (engine.Engine, error) { return stubEng, nil },
	}
	root := cli.NewRootCommand(app)
	root.SetArgs([]string{"sync", "--dry-run", "--config", cfgPath})
	require.NoError(t, root.ExecuteContext(context.Background()))
	assert.Equal(t, "e2e_db", stubEng.lastPlanDB)
	assert.Contains(t, out.String(), "e2e_db")
}

type capturingEngine struct{ lastPlanDB string }

func (c *capturingEngine) Plan(_ context.Context, opts engine.PlanOptions) (*models.SyncPlan, error) {
	c.lastPlanDB = opts.Database
	return &models.SyncPlan{}, nil
}

func (c *capturingEngine) Execute(_ context.Context, _ *models.SyncPlan, _ engine.ProgressObserver) (*models.SyncResult, error) {
	return &models.SyncResult{}, nil
}
```

(Types confirmed from `internal/engine/engine.go:79-82`: `Engine.Plan` returns `*models.SyncPlan`, `Engine.Execute` returns `*models.SyncResult`. If `models.SyncPlan` has required fields preventing zero-value construction, populate the minimum needed — read `internal/models/` to verify.)

- [ ] **Step 2: Run the test**

Run: `go test ./internal/cli/...`
Expected: PASS. If the dry-run output doesn't print the DB name, inspect `PrintPlan` and verify the test asserts on a string that's actually emitted; otherwise drop the `assert.Contains(...)` on the output buffer and rely on `stubEng.lastPlanDB`.

- [ ] **Step 3: Commit**

```bash
bash ~/.claude/scripts/commit-files.sh "test(cli): e2e for pgsync sync with infisical shell-stub" \
  internal/cli/sync_e2e_test.go
```

---

## Task 10: Final pipeline + manual smoke against one real project

**Files:** none (verification only).

- [ ] **Step 1: Run scoped formatter on all changed files**

Run: `gofmt -w internal/secrets/infisical/resolver.go internal/secrets/infisical/resolver_test.go internal/cli/config_resolver.go internal/cli/config_resolver_test.go internal/cli/sync.go internal/cli/sync_test.go internal/cli/sync_e2e_test.go internal/config/override.go internal/config/override_test.go`
Expected: silent (or git status shows formatting nits — if so, amend prior commits is fine because they're local-only — or add a single follow-up commit `style: gofmt`).

- [ ] **Step 2: Run vet**

Run: `go vet ./...`
Expected: silent.

- [ ] **Step 3: Run full test suite**

Run: `go test ./...`
Expected: PASS.

- [ ] **Step 4: Build the binary**

Run: `go build -o /tmp/pgsync ./cmd/pgsync`
Expected: builds cleanly.

- [ ] **Step 5: Ensure local pgsync config has cluster creds**

Confirm `~/.config/pgsync/config.toml` exists and has both `[remote]` and `[local]` blocks with everything **except** `database`. If missing, write it once based on spec §5:

```toml
[remote]
host     = "private-db-postgresql-1-do-user-3397877-0.h.db.ondigitalocean.com"
port     = 25060
user     = "doadmin"
password = "<paste-AVNS_-secret-here>"
sslmode  = "require"

[local]
host = "localhost"
port = 5432
user = "mttzzzz"
```

(The password is the same one already in Infisical:prod for each project.)

- [ ] **Step 6: Manual smoke — dry-run sync against ai.pushka.biz**

```bash
cd /mnt/c/Users/kiril/projects/ai.pushka.biz
/tmp/pgsync sync --dry-run
```

Expected: exit code 0, output mentions `database: ai_pushka_biz`, remote host = DO cluster, local host = localhost.

- [ ] **Step 7: Manual smoke — dry-run sync against masterm.pushka.biz**

```bash
cd /mnt/c/Users/kiril/projects/masterm.pushka.biz
/tmp/pgsync sync --dry-run
```

Expected: exit 0, `database: masterm_pushka_biz`.

- [ ] **Step 8: Manual smoke — explicit override**

```bash
cd /tmp
/tmp/pgsync sync ai_pushka_biz --dry-run
```

Expected: exit 0. Even outside any project (no `.infisical.json`), the positional override skips the resolver.

- [ ] **Step 9: If anything needed adjusting, commit polish**

```bash
bash ~/.claude/scripts/commit-files.sh "<msg>" <files>
```

- [ ] **Step 10: Stop and tell user**

Print:
> Готово: Infisical-резолвер реализован, тесты зелёные, smoke-тест на ai/masterm прошёл. Push?

Do NOT push automatically — wait for explicit user permission per global git discipline.
