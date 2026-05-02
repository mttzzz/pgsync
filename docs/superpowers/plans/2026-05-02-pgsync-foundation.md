# pgsync Foundation Implementation Plan (Phase 1 of 4)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the project skeleton, CI with strict 100% coverage gate, and all foundation packages (config, runner/clock/fsx interfaces, logger, models, proxy tunnel, pgschema introspection, pgtools locator). No network DB I/O at this phase — that comes in Plan 2.

**Architecture:** Greenfield Go module (Go 1.25+) at `C:\Users\kiril\projects\pgsync`. Strict-100% coverage discipline drives interface boundaries: every side-effect (process exec, fs, time, network dialing) lives behind an interface in its own package, with hand-rolled fakes in `_test.go`. Production wiring happens in `cmd/pgsync/main.go`.

**Tech Stack:** Go 1.25+, `BurntSushi/toml`, `jackc/pgx` v5, `golang.org/x/net/proxy`, `stretchr/testify`, `golangci-lint` v2 (strict profile), `slog`. No `viper`, no `gomock`, no `ioutil`.

**Reference docs:** See `docs/superpowers/specs/2026-05-02-pgsync-design.md` — sections 5 (Layout), 8 (Config), 9 (Proxy), 11.1–11.2 (Coverage + Mocking), 12 (Modern Go), 16 (Tech stack).

**Conventions for every task:**
- TDD: failing test first, run it red, minimal impl, run green, commit.
- Strict 100% coverage: if you wrote a line, there must be a test that exercises it.
- Use modern Go (`slog`, `slices`, `maps`, `errors.Join`, generics, `iter.Seq` where idiomatic).
- Forbidden: `interface{}` (use `any`), `ioutil`, ручной `sort.Slice`, `log.Printf`, глобальный `*rand.Rand`.
- Commits use explicit file lists (`git add path/a path/b && git commit -m "..."`) — never `git add .` / `-A` / `-am`.
- Multi-line comments use `/* ... */`, not chained `//`.

---

## Task 1: Repo skeleton — git, module, dirs, base files

**Files:**
- Create: `C:\Users\kiril\projects\pgsync\.gitignore`
- Create: `C:\Users\kiril\projects\pgsync\go.mod`
- Create: `C:\Users\kiril\projects\pgsync\README.md`
- Create: `C:\Users\kiril\projects\pgsync\LICENSE`
- Create: empty dirs `cmd/pgsync/`, `internal/{cli,tui,engine,pgschema,proxy,config,models,observability,runner,clock,fsx,version,updater}/`, `internal/engine/{native,external,pgtools}/`, `pkg/utils/`, `test/{helpers,integration,e2e}/`, `benchmarks/`, `fixtures/`, `embed/bin/`, `scripts/`, `docs/`

- [ ] **Step 1: Initialize git and Go module**

```bash
cd /c/Users/kiril/projects/pgsync
git init
git config core.autocrlf false
go mod init github.com/mttzzz/pgsync
```

- [ ] **Step 2: Create `.gitignore`**

```gitignore
# Binaries
bin/
dist/
*.exe
*.test
*.out

# Go
vendor/
go.work.sum

# IDE
.vscode/*
!.vscode/settings.json
.idea/

# OS
.DS_Store
Thumbs.db

# Project
.env
.env.local
~/.pgsync/
benchmarks/results/main/
benchmarks/results/local/
fixtures/*.sql
fixtures/*.sql.gz
fixtures/dvdrental/
coverage.out
coverage.html

# Embedded pg_tools (downloaded by scripts/fetch-pgtools.sh, not committed)
embed/bin/*/

# Plans/specs are committed
!docs/superpowers/specs/
!docs/superpowers/plans/
```

- [ ] **Step 3: Create empty dir tree**

```bash
mkdir -p cmd/pgsync \
  internal/{cli,tui,engine,pgschema,proxy,config,models,observability,runner,clock,fsx,version,updater} \
  internal/engine/{native,external,pgtools} \
  pkg/utils \
  test/{helpers,integration,e2e} \
  benchmarks fixtures embed/bin scripts
touch cmd/pgsync/.keep internal/cli/.keep  # placeholder for empty dirs
```

- [ ] **Step 4: Create `README.md` skeleton**

```markdown
# pgsync

Fast PostgreSQL prod→local sync for developers. Cross-platform single binary.

> **Status:** in development (Phase 1 of 4 — foundation).

See [design spec](docs/superpowers/specs/2026-05-02-pgsync-design.md) for architecture.

## Build

```bash
make build
```

## License

MIT — see `LICENSE`.
```

- [ ] **Step 5: Create `LICENSE` (MIT)**

Use the standard MIT template, year 2026, name "Kiril Taborrnd".

- [ ] **Step 6: Commit**

```bash
git add .gitignore go.mod README.md LICENSE \
  cmd/pgsync/.keep internal/cli/.keep
git commit -m "chore: initial repo skeleton (Go module, dirs, license)"
```

---

## Task 2: Strict golangci-lint config

**Files:**
- Create: `C:\Users\kiril\projects\pgsync\.golangci.yml`

- [ ] **Step 1: Write `.golangci.yml`**

```yaml
version: "2"

run:
  timeout: 5m
  tests: true
  go: "1.25"

linters:
  default: none
  enable:
    - errcheck
    - errorlint
    - gocritic
    - gocognit
    - gocyclo
    - gosec
    - govet
    - ineffassign
    - nilerr
    - prealloc
    - revive
    - staticcheck
    - unparam
    - unused
    - wastedassign

  settings:
    gocognit:
      min-complexity: 15
    gocyclo:
      min-complexity: 10
    gosec:
      excludes:
        - G104  # errors checked via errcheck
    revive:
      rules:
        - name: var-naming
        - name: exported
        - name: error-return
        - name: error-naming
        - name: blank-imports
        - name: context-as-argument
        - name: context-keys-type
        - name: errorf
        - name: package-comments
        - name: range
        - name: receiver-naming
        - name: time-naming
        - name: indent-error-flow
        - name: superfluous-else
        - name: unreachable-code

issues:
  max-issues-per-linter: 0
  max-same-issues: 0

formatters:
  enable:
    - gofmt
    - goimports
  settings:
    goimports:
      local-prefixes:
        - github.com/mttzzz/pgsync
```

- [ ] **Step 2: Verify install of golangci-lint v2**

```bash
golangci-lint version
# Expected: v2.x.x. If v1 or missing:
go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest
```

- [ ] **Step 3: Run linter on empty repo (must succeed cleanly)**

```bash
golangci-lint run ./...
# Expected: no output, exit 0 (no Go files yet)
```

- [ ] **Step 4: Commit**

```bash
git add .golangci.yml
git commit -m "chore: add strict golangci-lint config"
```

---

## Task 3: Coverage-gate script

**Files:**
- Create: `C:\Users\kiril\projects\pgsync\scripts\coverage-gate.sh`
- Create: `C:\Users\kiril\projects\pgsync\coverage.allow`

- [ ] **Step 1: Create `coverage.allow` (allow-list of paths exempt from 100%)**

```
# One pattern per line. Paths matched by `grep -F`.
# Per spec §11.1: only entry point, build-time-injected version, and OS-specific embed files.
github.com/mttzzz/pgsync/cmd/pgsync
github.com/mttzzz/pgsync/internal/version
github.com/mttzzz/pgsync/internal/engine/pgtools/embed_
```

- [ ] **Step 2: Create `scripts/coverage-gate.sh`**

```bash
#!/usr/bin/env bash
# Fails if any internal/ package has < 100% line coverage,
# excluding paths in coverage.allow.
set -euo pipefail

PROFILE="${1:-coverage.out}"
ALLOW_FILE="${2:-coverage.allow}"

if [[ ! -f "$PROFILE" ]]; then
  echo "coverage profile not found: $PROFILE" >&2
  exit 2
fi

# Build a grep -F pattern from allow-list (skip blank lines and #-comments).
ALLOW_PATTERN="$(grep -vE '^(#|$)' "$ALLOW_FILE" | tr '\n' '|' | sed 's/|$//')"

# Extract per-function coverage, drop allowed paths, find any < 100.0%.
FAILING="$(go tool cover -func="$PROFILE" \
  | grep -vE "^(${ALLOW_PATTERN})" \
  | grep -v '^total:' \
  | awk '$NF != "100.0%" { print }')"

if [[ -n "$FAILING" ]]; then
  echo "Coverage gate FAILED — these symbols are below 100%:" >&2
  echo "$FAILING" >&2
  exit 1
fi

echo "Coverage gate PASSED — all internal/ symbols at 100%."
```

- [ ] **Step 3: Make executable + commit**

```bash
chmod +x scripts/coverage-gate.sh
git add scripts/coverage-gate.sh coverage.allow
git commit -m "chore: add 100% coverage gate script with allow-list"
```

---

## Task 4: Makefile

**Files:**
- Create: `C:\Users\kiril\projects\pgsync\Makefile`

- [ ] **Step 1: Write Makefile**

```makefile
.PHONY: help build test test-unit test-integration test-coverage coverage-gate \
        lint fmt deps deps-update bench clean

BINARY := pgsync
BUILD_DIR := bin
GO_FILES := $(shell find . -name '*.go' -not -path './vendor/*' -not -path './embed/*')

help:
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) \
	  | awk 'BEGIN {FS = ":.*?## "}; {printf "  %-22s %s\n", $$1, $$2}'

build: ## Build the binary
	@mkdir -p $(BUILD_DIR)
	go build -o $(BUILD_DIR)/$(BINARY) ./cmd/pgsync

test: test-unit ## Alias for test-unit

test-unit: ## Run unit tests with coverage
	go test -race -covermode=atomic -coverprofile=coverage.out \
	  -coverpkg=./internal/...,./pkg/... ./internal/... ./pkg/...

test-integration: ## Run integration tests (requires Docker)
	go test -race -tags=integration -timeout=10m ./test/integration/...

test-coverage: test-unit ## Generate HTML coverage report
	go tool cover -html=coverage.out -o coverage.html

coverage-gate: test-unit ## Fail if internal/ coverage < 100%
	bash scripts/coverage-gate.sh coverage.out coverage.allow

lint: ## Run golangci-lint with auto-fix
	golangci-lint run --fix ./...

fmt: ## Format code
	go fmt ./...
	gofmt -s -w $(GO_FILES)

deps: ## Download deps
	go mod download
	go mod verify

deps-update: ## Update deps
	go get -u ./...
	go mod tidy

bench: ## Run benchmarks (requires Docker)
	go test -tags=integration -bench=. -benchmem -run=^$$ ./benchmarks/...

clean:
	rm -rf $(BUILD_DIR) coverage.out coverage.html
	go clean
```

- [ ] **Step 2: Verify make works**

```bash
make help
# Expected: list of targets
make fmt
# Expected: no output, exit 0
```

- [ ] **Step 3: Commit**

```bash
git add Makefile
git commit -m "chore: add Makefile (build, test, lint, coverage-gate, bench)"
```

---

## Task 5: CI skeleton — GitHub Actions

**Files:**
- Create: `C:\Users\kiril\projects\pgsync\.github\workflows\ci.yml`

- [ ] **Step 1: Create workflow**

```yaml
name: CI

on:
  push:
    branches: [main]
  pull_request:

jobs:
  lint:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.25'
      - uses: golangci/golangci-lint-action@v6
        with:
          version: v2.0

  test-unit:
    strategy:
      fail-fast: false
      matrix:
        os: [ubuntu-latest, macos-latest, windows-latest]
    runs-on: ${{ matrix.os }}
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.25'
      - name: Test
        run: |
          go test -race -covermode=atomic -coverprofile=coverage.out \
            -coverpkg=./internal/...,./pkg/... ./internal/... ./pkg/...
        shell: bash
      - name: Upload coverage
        if: matrix.os == 'ubuntu-latest'
        uses: actions/upload-artifact@v4
        with:
          name: coverage
          path: coverage.out

  coverage-gate:
    runs-on: ubuntu-latest
    needs: test-unit
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.25'
      - uses: actions/download-artifact@v4
        with:
          name: coverage
      - name: Run coverage gate
        run: bash scripts/coverage-gate.sh coverage.out coverage.allow

  test-integration:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.25'
      - name: Test integration
        run: go test -race -tags=integration -timeout=15m ./test/integration/...
```

- [ ] **Step 2: Commit**

```bash
git add .github/workflows/ci.yml
git commit -m "ci: lint + test (matrix) + coverage-gate + integration jobs"
```

---

## Task 6: Config types and TOML marshaling

**Files:**
- Create: `internal/config/config.go`
- Create: `internal/config/config_test.go`

- [ ] **Step 1: Add toml dep**

```bash
go get github.com/BurntSushi/toml@latest
```

- [ ] **Step 2: Write failing test `internal/config/config_test.go`**

```go
package config_test

import (
	"strings"
	"testing"

	"github.com/BurntSushi/toml"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mttzzz/pgsync/internal/config"
)

func TestConfigRoundtrip(t *testing.T) {
	t.Parallel()

	src := config.Config{
		Remote: config.Connection{
			Host:     "prod.example.com",
			Port:     5432,
			User:     "readonly",
			Password: "secret",
			Database: "ai_pushka_biz",
			SSLMode:  "require",
			ProxyURL: "socks5://proxy:1080",
		},
		Local: config.Connection{
			Host:     "localhost",
			Port:     5432,
			User:     "postgres",
			Password: "postgres",
			SSLMode:  "disable",
		},
		Runtime: config.Runtime{
			Threads:           8,
			Engine:            "native",
			UseSystemPgtools:  false,
			DefaultDatabase:   "ai_pushka_biz",
			ConcurrentIndexes: false,
		},
		Logging: config.Logging{Level: "info", Format: "text"},
	}

	var buf strings.Builder
	require.NoError(t, toml.NewEncoder(&buf).Encode(src))

	var got config.Config
	_, err := toml.Decode(buf.String(), &got)
	require.NoError(t, err)
	assert.Equal(t, src, got)
}

func TestConfigDefaults(t *testing.T) {
	t.Parallel()
	got := config.Defaults()
	assert.Equal(t, 5432, got.Remote.Port)
	assert.Equal(t, 5432, got.Local.Port)
	assert.Equal(t, "require", got.Remote.SSLMode)
	assert.Equal(t, "disable", got.Local.SSLMode)
	assert.Equal(t, "native", got.Runtime.Engine)
	assert.Equal(t, "info", got.Logging.Level)
	assert.Equal(t, "text", got.Logging.Format)
	assert.Greater(t, got.Runtime.Threads, 0)
}
```

- [ ] **Step 3: Run test — must fail (no `config` package yet)**

```bash
go test ./internal/config/...
# Expected: cannot find package "github.com/mttzzz/pgsync/internal/config"
```

- [ ] **Step 4: Implement `internal/config/config.go`**

```go
/*
 * Package config defines the pgsync configuration types and defaults.
 * The file format is TOML; see internal/config/store.go for atomic save/load.
 */
package config

import "runtime"

type Config struct {
	Remote  Connection `toml:"remote"`
	Local   Connection `toml:"local"`
	Runtime Runtime    `toml:"runtime"`
	Logging Logging    `toml:"logging"`
}

type Connection struct {
	Host     string `toml:"host"`
	Port     int    `toml:"port"`
	User     string `toml:"user"`
	Password string `toml:"password"`
	Database string `toml:"database,omitempty"`
	SSLMode  string `toml:"ssl_mode"`
	ProxyURL string `toml:"proxy_url,omitempty"`
}

type Runtime struct {
	Threads           int    `toml:"threads"`
	Engine            string `toml:"engine"`
	UseSystemPgtools  bool   `toml:"use_system_pgtools"`
	DefaultDatabase   string `toml:"default_database,omitempty"`
	ConcurrentIndexes bool   `toml:"concurrent_indexes"`
}

type Logging struct {
	Level  string `toml:"level"`
	Format string `toml:"format"`
}

func Defaults() Config {
	return Config{
		Remote: Connection{Port: 5432, SSLMode: "require"},
		Local:  Connection{Port: 5432, SSLMode: "disable"},
		Runtime: Runtime{
			Threads: runtime.NumCPU(),
			Engine:  "native",
		},
		Logging: Logging{Level: "info", Format: "text"},
	}
}
```

- [ ] **Step 5: Run tests — must pass**

```bash
go test -race ./internal/config/...
# Expected: ok
```

- [ ] **Step 6: Commit**

```bash
git add go.mod go.sum internal/config/config.go internal/config/config_test.go
git commit -m "feat(config): types + TOML marshaling + defaults"
```

---

## Task 7: Config store — atomic save, load, OS-specific paths

**Files:**
- Create: `internal/config/store.go`
- Create: `internal/config/path.go`
- Create: `internal/config/store_test.go`
- Create: `internal/config/path_test.go`

- [ ] **Step 1: Write failing tests `internal/config/store_test.go`**

```go
package config_test

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mttzzz/pgsync/internal/config"
)

func TestStoreSaveLoadRoundtrip(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")

	want := config.Defaults()
	want.Remote.Host = "prod.example.com"
	want.Remote.User = "readonly"

	require.NoError(t, config.Save(path, want))

	got, err := config.Load(path)
	require.NoError(t, err)
	assert.Equal(t, want, got)
}

func TestStoreSaveAtomicAndPerms(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")

	require.NoError(t, config.Save(path, config.Defaults()))

	info, err := os.Stat(path)
	require.NoError(t, err)
	if runtime.GOOS != "windows" {
		assert.Equal(t, os.FileMode(0o600), info.Mode().Perm(),
			"config file must be 0600 on unix")
	}

	/* No leftover tmp files */
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	assert.Len(t, entries, 1)
	assert.Equal(t, "config.toml", entries[0].Name())
}

func TestStoreLoadMissingReturnsErrNotExist(t *testing.T) {
	t.Parallel()
	_, err := config.Load(filepath.Join(t.TempDir(), "nope.toml"))
	require.Error(t, err)
	assert.ErrorIs(t, err, os.ErrNotExist)
}

func TestStoreLoadMalformedTOMLReturnsError(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	require.NoError(t, os.WriteFile(path, []byte("not = [valid toml"), 0o600))
	_, err := config.Load(path)
	require.Error(t, err)
}
```

- [ ] **Step 2: Write failing tests `internal/config/path_test.go`**

```go
package config_test

import (
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mttzzz/pgsync/internal/config"
)

func TestDefaultPath(t *testing.T) {
	t.Parallel()

	got, err := config.DefaultPath(map[string]string{
		"HOME":        "/home/u",
		"XDG_CONFIG_HOME": "",
		"APPDATA":     "C:\\Users\\u\\AppData\\Roaming",
	})
	require.NoError(t, err)

	if runtime.GOOS == "windows" {
		assert.Equal(t, filepath.Join("C:\\Users\\u\\AppData\\Roaming", "pgsync", "config.toml"), got)
	} else {
		assert.Equal(t, filepath.Join("/home/u", ".config", "pgsync", "config.toml"), got)
	}
}

func TestDefaultPathHonorsXDG(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("XDG not used on windows")
	}
	got, err := config.DefaultPath(map[string]string{
		"HOME":            "/home/u",
		"XDG_CONFIG_HOME": "/custom/cfg",
	})
	require.NoError(t, err)
	assert.Equal(t, "/custom/cfg/pgsync/config.toml", got)
}
```

- [ ] **Step 3: Run tests — must fail**

```bash
go test ./internal/config/...
# Expected: undefined: config.Save / config.Load / config.DefaultPath
```

- [ ] **Step 4: Implement `internal/config/path.go`**

```go
package config

import (
	"errors"
	"path/filepath"
	"runtime"
)

const (
	appDir   = "pgsync"
	fileName = "config.toml"
)

/*
 * DefaultPath resolves the config file path from environment variables.
 * Pass a map (typically os.Environ-derived); this lets tests inject env without
 * mutating process state.
 */
func DefaultPath(env map[string]string) (string, error) {
	if runtime.GOOS == "windows" {
		appData := env["APPDATA"]
		if appData == "" {
			return "", errors.New("APPDATA not set")
		}
		return filepath.Join(appData, appDir, fileName), nil
	}
	if xdg := env["XDG_CONFIG_HOME"]; xdg != "" {
		return filepath.Join(xdg, appDir, fileName), nil
	}
	home := env["HOME"]
	if home == "" {
		return "", errors.New("HOME not set")
	}
	return filepath.Join(home, ".config", appDir, fileName), nil
}
```

- [ ] **Step 5: Implement `internal/config/store.go`**

```go
package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

/*
 * Save writes cfg to path atomically: write to <path>.tmp, fsync, rename.
 * On unix, file mode is 0600. The parent dir is created with 0700 if missing.
 */
func Save(path string, cfg Config) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("mkdir config dir: %w", err)
	}

	tmp := path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return fmt.Errorf("open tmp: %w", err)
	}

	if err := toml.NewEncoder(f).Encode(cfg); err != nil {
		_ = f.Close()
		_ = os.Remove(tmp)
		return fmt.Errorf("encode: %w", err)
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		_ = os.Remove(tmp)
		return fmt.Errorf("fsync: %w", err)
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("close tmp: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("rename: %w", err)
	}
	return nil
}

/* Load reads + parses the config from path. Returns os.ErrNotExist wrapped. */
func Load(path string) (Config, error) {
	var cfg Config
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return Config{}, err
		}
		return Config{}, fmt.Errorf("read: %w", err)
	}
	if _, err := toml.Decode(string(data), &cfg); err != nil {
		return Config{}, fmt.Errorf("decode: %w", err)
	}
	return cfg, nil
}
```

- [ ] **Step 6: Run tests — must pass**

```bash
go test -race ./internal/config/...
# Expected: ok
```

- [ ] **Step 7: Commit**

```bash
git add internal/config/store.go internal/config/path.go \
        internal/config/store_test.go internal/config/path_test.go
git commit -m "feat(config): atomic save + load + OS-specific path resolver"
```

---

## Task 8: Config validators (for huh inline-validation in TUI)

**Files:**
- Create: `internal/config/validate.go`
- Create: `internal/config/validate_test.go`

- [ ] **Step 1: Write failing tests `internal/config/validate_test.go`**

```go
package config_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/mttzzz/pgsync/internal/config"
)

func TestValidateHost(t *testing.T) {
	t.Parallel()
	cases := map[string]bool{
		"":                   false,
		"localhost":          true,
		"prod.example.com":   true,
		"10.0.0.1":           true,
		"with space":         false,
		"::1":                true,
	}
	for in, ok := range cases {
		err := config.ValidateHost(in)
		if ok {
			assert.NoError(t, err, "host=%q", in)
		} else {
			assert.Error(t, err, "host=%q", in)
		}
	}
}

func TestValidatePort(t *testing.T) {
	t.Parallel()
	assert.NoError(t, config.ValidatePort(5432))
	assert.NoError(t, config.ValidatePort(1))
	assert.NoError(t, config.ValidatePort(65535))
	assert.Error(t, config.ValidatePort(0))
	assert.Error(t, config.ValidatePort(65536))
	assert.Error(t, config.ValidatePort(-1))
}

func TestValidateSSLMode(t *testing.T) {
	t.Parallel()
	for _, m := range []string{"disable", "require", "verify-ca", "verify-full"} {
		assert.NoError(t, config.ValidateSSLMode(m))
	}
	assert.Error(t, config.ValidateSSLMode("yes"))
	assert.Error(t, config.ValidateSSLMode(""))
}

func TestValidateProxyURL(t *testing.T) {
	t.Parallel()
	for _, u := range []string{
		"",
		"socks5://proxy:1080",
		"socks5h://proxy:1080",
		"http://proxy:8080",
		"https://proxy:8080",
	} {
		assert.NoError(t, config.ValidateProxyURL(u), "url=%q", u)
	}
	for _, u := range []string{
		"ftp://x",
		"socks4://x",
		"://broken",
	} {
		assert.Error(t, config.ValidateProxyURL(u), "url=%q", u)
	}
}

func TestValidateAll(t *testing.T) {
	t.Parallel()
	cfg := config.Defaults()
	cfg.Remote.Host = "prod"
	cfg.Remote.User = "u"
	cfg.Local.Host = "localhost"
	cfg.Local.User = "postgres"
	assert.NoError(t, config.Validate(cfg))

	bad := config.Defaults()
	bad.Remote.Port = 0
	err := config.Validate(bad)
	assert.Error(t, err)
}
```

- [ ] **Step 2: Run — must fail**

- [ ] **Step 3: Implement `internal/config/validate.go`**

```go
package config

import (
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strings"
)

var validSSLModes = []string{"disable", "require", "verify-ca", "verify-full"}
var validProxySchemes = []string{"socks5", "socks5h", "http", "https"}

func ValidateHost(h string) error {
	if h == "" {
		return errors.New("host is required")
	}
	if strings.ContainsAny(h, " \t\n") {
		return errors.New("host must not contain whitespace")
	}
	return nil
}

func ValidatePort(p int) error {
	if p < 1 || p > 65535 {
		return fmt.Errorf("port out of range: %d", p)
	}
	return nil
}

func ValidateSSLMode(m string) error {
	if !slices.Contains(validSSLModes, m) {
		return fmt.Errorf("ssl_mode must be one of %v, got %q", validSSLModes, m)
	}
	return nil
}

func ValidateProxyURL(raw string) error {
	if raw == "" {
		return nil
	}
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("parse proxy url: %w", err)
	}
	if !slices.Contains(validProxySchemes, u.Scheme) {
		return fmt.Errorf("proxy scheme must be one of %v, got %q", validProxySchemes, u.Scheme)
	}
	if u.Host == "" {
		return errors.New("proxy url has no host")
	}
	return nil
}

/* Validate returns the first error found, or nil if cfg is fully valid. */
func Validate(c Config) error {
	checks := []func() error{
		func() error { return ValidateHost(c.Remote.Host) },
		func() error { return ValidatePort(c.Remote.Port) },
		func() error { return ValidateSSLMode(c.Remote.SSLMode) },
		func() error { return ValidateProxyURL(c.Remote.ProxyURL) },
		func() error { return ValidateHost(c.Local.Host) },
		func() error { return ValidatePort(c.Local.Port) },
		func() error { return ValidateSSLMode(c.Local.SSLMode) },
	}
	for _, ck := range checks {
		if err := ck(); err != nil {
			return err
		}
	}
	if c.Runtime.Threads < 1 {
		return errors.New("runtime.threads must be >= 1")
	}
	if !slices.Contains([]string{"native", "external", "auto"}, c.Runtime.Engine) {
		return fmt.Errorf("runtime.engine must be native|external|auto")
	}
	if !slices.Contains([]string{"debug", "info", "warn", "error"}, c.Logging.Level) {
		return fmt.Errorf("logging.level must be debug|info|warn|error")
	}
	if !slices.Contains([]string{"text", "json"}, c.Logging.Format) {
		return fmt.Errorf("logging.format must be text|json")
	}
	return nil
}
```

- [ ] **Step 4: Run — must pass**

```bash
go test -race ./internal/config/...
```

- [ ] **Step 5: Commit**

```bash
git add internal/config/validate.go internal/config/validate_test.go
git commit -m "feat(config): per-field + whole-config validators (TUI-ready)"
```

---

## Task 9: Config override layer — env + flags merge

**Files:**
- Create: `internal/config/override.go`
- Create: `internal/config/override_test.go`

- [ ] **Step 1: Write failing tests**

```go
package config_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/mttzzz/pgsync/internal/config"
)

func TestApplyEnv(t *testing.T) {
	t.Parallel()

	base := config.Defaults()
	env := map[string]string{
		"PGSYNC_REMOTE_HOST":     "prod",
		"PGSYNC_REMOTE_PORT":     "6432",
		"PGSYNC_REMOTE_USER":     "ro",
		"PGSYNC_REMOTE_PASSWORD": "x",
		"PGSYNC_REMOTE_DATABASE": "ai",
		"PGSYNC_REMOTE_SSL_MODE": "require",
		"PGSYNC_LOCAL_HOST":      "localhost",
		"PGSYNC_THREADS":         "16",
		"PGSYNC_LOG_LEVEL":       "debug",
		"OTHER":                  "ignored",
	}
	got, err := config.ApplyEnv(base, env)
	assert.NoError(t, err)
	assert.Equal(t, "prod", got.Remote.Host)
	assert.Equal(t, 6432, got.Remote.Port)
	assert.Equal(t, "ro", got.Remote.User)
	assert.Equal(t, "x", got.Remote.Password)
	assert.Equal(t, "ai", got.Remote.Database)
	assert.Equal(t, "localhost", got.Local.Host)
	assert.Equal(t, 16, got.Runtime.Threads)
	assert.Equal(t, "debug", got.Logging.Level)
}

func TestApplyEnvBadInt(t *testing.T) {
	t.Parallel()
	_, err := config.ApplyEnv(config.Defaults(), map[string]string{
		"PGSYNC_REMOTE_PORT": "not-a-number",
	})
	assert.Error(t, err)
}

func TestApplyEnvNoOpsForBlank(t *testing.T) {
	t.Parallel()
	base := config.Defaults()
	base.Remote.Host = "kept"
	got, err := config.ApplyEnv(base, map[string]string{
		"PGSYNC_REMOTE_HOST": "",
	})
	assert.NoError(t, err)
	assert.Equal(t, "kept", got.Remote.Host)
}
```

- [ ] **Step 2: Run — must fail**

- [ ] **Step 3: Implement `internal/config/override.go`**

```go
package config

import (
	"fmt"
	"strconv"
	"strings"
)

/*
 * ApplyEnv overlays PGSYNC_* environment variables onto cfg, returning a new
 * Config. Empty values are ignored (no-op), so partial env still merges with
 * the file-stored config. CLI flags wrap this — see cli/flags.go.
 */
func ApplyEnv(cfg Config, env map[string]string) (Config, error) {
	type binding struct {
		key string
		set func(string) error
	}

	mustInt := func(target *int) func(string) error {
		return func(s string) error {
			n, err := strconv.Atoi(s)
			if err != nil {
				return fmt.Errorf("parse int: %w", err)
			}
			*target = n
			return nil
		}
	}
	mustBool := func(target *bool) func(string) error {
		return func(s string) error {
			b, err := strconv.ParseBool(s)
			if err != nil {
				return fmt.Errorf("parse bool: %w", err)
			}
			*target = b
			return nil
		}
	}
	mustStr := func(target *string) func(string) error {
		return func(s string) error { *target = s; return nil }
	}

	bindings := []binding{
		{"PGSYNC_REMOTE_HOST", mustStr(&cfg.Remote.Host)},
		{"PGSYNC_REMOTE_PORT", mustInt(&cfg.Remote.Port)},
		{"PGSYNC_REMOTE_USER", mustStr(&cfg.Remote.User)},
		{"PGSYNC_REMOTE_PASSWORD", mustStr(&cfg.Remote.Password)},
		{"PGSYNC_REMOTE_DATABASE", mustStr(&cfg.Remote.Database)},
		{"PGSYNC_REMOTE_SSL_MODE", mustStr(&cfg.Remote.SSLMode)},
		{"PGSYNC_REMOTE_PROXY_URL", mustStr(&cfg.Remote.ProxyURL)},
		{"PGSYNC_LOCAL_HOST", mustStr(&cfg.Local.Host)},
		{"PGSYNC_LOCAL_PORT", mustInt(&cfg.Local.Port)},
		{"PGSYNC_LOCAL_USER", mustStr(&cfg.Local.User)},
		{"PGSYNC_LOCAL_PASSWORD", mustStr(&cfg.Local.Password)},
		{"PGSYNC_LOCAL_SSL_MODE", mustStr(&cfg.Local.SSLMode)},
		{"PGSYNC_THREADS", mustInt(&cfg.Runtime.Threads)},
		{"PGSYNC_ENGINE", mustStr(&cfg.Runtime.Engine)},
		{"PGSYNC_USE_SYSTEM_PGTOOLS", mustBool(&cfg.Runtime.UseSystemPgtools)},
		{"PGSYNC_DEFAULT_DATABASE", mustStr(&cfg.Runtime.DefaultDatabase)},
		{"PGSYNC_CONCURRENT_INDEXES", mustBool(&cfg.Runtime.ConcurrentIndexes)},
		{"PGSYNC_LOG_LEVEL", mustStr(&cfg.Logging.Level)},
		{"PGSYNC_LOG_FORMAT", mustStr(&cfg.Logging.Format)},
	}

	for _, b := range bindings {
		v, ok := env[b.key]
		if !ok || strings.TrimSpace(v) == "" {
			continue
		}
		if err := b.set(v); err != nil {
			return Config{}, fmt.Errorf("env %s=%q: %w", b.key, v, err)
		}
	}
	return cfg, nil
}
```

- [ ] **Step 4: Run — must pass**

- [ ] **Step 5: Commit**

```bash
git add internal/config/override.go internal/config/override_test.go
git commit -m "feat(config): env override layer (PGSYNC_*)"
```

---

## Task 10: Runner interface + os/exec implementation

**Files:**
- Create: `internal/runner/runner.go`
- Create: `internal/runner/exec.go`
- Create: `internal/runner/exec_test.go`

- [ ] **Step 1: Write failing test `internal/runner/exec_test.go`**

```go
package runner_test

import (
	"context"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mttzzz/pgsync/internal/runner"
)

func TestExecRunSuccess(t *testing.T) {
	t.Parallel()
	r := runner.NewExec()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	name, args := echoCommand("hello")
	stdout, _, err := r.Run(ctx, name, args, nil)
	require.NoError(t, err)
	assert.Contains(t, string(stdout), "hello")
}

func TestExecRunFailingExitCode(t *testing.T) {
	t.Parallel()
	r := runner.NewExec()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	name, args := exitCommand(7)
	_, _, err := r.Run(ctx, name, args, nil)
	require.Error(t, err)
	var ee *runner.ExitError
	require.ErrorAs(t, err, &ee)
	assert.Equal(t, 7, ee.Code)
}

func TestExecRunCancellation(t *testing.T) {
	t.Parallel()
	r := runner.NewExec()
	ctx, cancel := context.WithCancel(t.Context())

	name, args := sleepCommand(10)
	done := make(chan error, 1)
	go func() {
		_, _, err := r.Run(ctx, name, args, nil)
		done <- err
	}()
	time.Sleep(50 * time.Millisecond)
	cancel()
	err := <-done
	require.Error(t, err)
}

func echoCommand(s string) (string, []string) {
	if runtime.GOOS == "windows" {
		return "cmd", []string{"/c", "echo", s}
	}
	return "sh", []string{"-c", "echo " + s}
}

func exitCommand(code int) (string, []string) {
	if runtime.GOOS == "windows" {
		return "cmd", []string{"/c", "exit", "/b", "7"}
	}
	return "sh", []string{"-c", "exit 7"}
}

func sleepCommand(seconds int) (string, []string) {
	if runtime.GOOS == "windows" {
		return "cmd", []string{"/c", "ping", "-n", "11", "127.0.0.1"}
	}
	return "sh", []string{"-c", "sleep 10"}
}
```

- [ ] **Step 2: Run — must fail (no `runner` package)**

- [ ] **Step 3: Implement `internal/runner/runner.go`**

```go
/*
 * Package runner is the seam between pgsync code and child processes
 * (pg_dump, pg_restore, etc.). All exec.Cmd usage in the project must go
 * through CommandRunner, which lets tests inject deterministic fakes.
 */
package runner

import (
	"context"
	"fmt"
)

type CommandRunner interface {
	/* Run executes name with args + env, returning stdout/stderr buffers. */
	Run(ctx context.Context, name string, args []string, env []string) (stdout []byte, stderr []byte, err error)
}

/* ExitError is returned when a child process exits non-zero. */
type ExitError struct {
	Code   int
	Stderr []byte
}

func (e *ExitError) Error() string {
	return fmt.Sprintf("exit code %d", e.Code)
}
```

- [ ] **Step 4: Implement `internal/runner/exec.go`**

```go
package runner

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
)

type Exec struct{}

func NewExec() *Exec { return &Exec{} }

func (e *Exec) Run(ctx context.Context, name string, args, env []string) ([]byte, []byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	if env != nil {
		cmd.Env = env
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return stdout.Bytes(), stderr.Bytes(), &ExitError{
				Code:   ee.ExitCode(),
				Stderr: stderr.Bytes(),
			}
		}
		return stdout.Bytes(), stderr.Bytes(), err
	}
	return stdout.Bytes(), stderr.Bytes(), nil
}
```

- [ ] **Step 5: Run — must pass**

```bash
go test -race ./internal/runner/...
```

- [ ] **Step 6: Commit**

```bash
git add internal/runner/runner.go internal/runner/exec.go internal/runner/exec_test.go
git commit -m "feat(runner): CommandRunner interface + os/exec implementation"
```

---

## Task 11: Clock interface + system implementation

**Files:**
- Create: `internal/clock/clock.go`
- Create: `internal/clock/system.go`
- Create: `internal/clock/system_test.go`

- [ ] **Step 1: Write failing test**

```go
package clock_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/mttzzz/pgsync/internal/clock"
)

func TestSystemNow(t *testing.T) {
	t.Parallel()
	c := clock.NewSystem()
	before := time.Now().Add(-time.Millisecond)
	got := c.Now()
	after := time.Now().Add(time.Millisecond)
	assert.True(t, !got.Before(before) && !got.After(after),
		"got=%v, want in [%v, %v]", got, before, after)
}
```

- [ ] **Step 2: Run — fail**

- [ ] **Step 3: Implement `clock.go`**

```go
/*
 * Package clock makes the wall clock injectable so progress-timer logic
 * and timeouts are testable without sleep().
 */
package clock

import "time"

type Clock interface {
	Now() time.Time
}
```

- [ ] **Step 4: Implement `system.go`**

```go
package clock

import "time"

type System struct{}

func NewSystem() *System { return &System{} }

func (System) Now() time.Time { return time.Now() }
```

- [ ] **Step 5: Run — pass**

- [ ] **Step 6: Commit**

```bash
git add internal/clock/clock.go internal/clock/system.go internal/clock/system_test.go
git commit -m "feat(clock): Clock interface + system impl"
```

---

## Task 12: fsx interface + os implementation

**Files:**
- Create: `internal/fsx/fs.go`
- Create: `internal/fsx/os.go`
- Create: `internal/fsx/os_test.go`

- [ ] **Step 1: Write failing test**

```go
package fsx_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mttzzz/pgsync/internal/fsx"
)

func TestOSWriteRead(t *testing.T) {
	t.Parallel()
	f := fsx.NewOS()
	path := filepath.Join(t.TempDir(), "x.bin")
	require.NoError(t, f.WriteFile(path, []byte("hello"), 0o644))
	got, err := f.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, []byte("hello"), got)
}

func TestOSStatNotExist(t *testing.T) {
	t.Parallel()
	f := fsx.NewOS()
	_, err := f.Stat(filepath.Join(t.TempDir(), "nope"))
	require.Error(t, err)
	assert.ErrorIs(t, err, os.ErrNotExist)
}

func TestOSMkdirAllAndRename(t *testing.T) {
	t.Parallel()
	f := fsx.NewOS()
	dir := t.TempDir()
	require.NoError(t, f.MkdirAll(filepath.Join(dir, "a", "b"), 0o755))
	require.NoError(t, f.WriteFile(filepath.Join(dir, "src"), []byte("x"), 0o644))
	require.NoError(t, f.Rename(filepath.Join(dir, "src"), filepath.Join(dir, "dst")))
	_, err := f.Stat(filepath.Join(dir, "dst"))
	require.NoError(t, err)
}
```

- [ ] **Step 2: Run — fail**

- [ ] **Step 3: Implement `fsx/fs.go`**

```go
/*
 * Package fsx is the filesystem seam — production wraps os.*; tests can
 * substitute an in-memory fake. Used by config store, pgtools extraction,
 * and updater.
 */
package fsx

import "io/fs"

type FS interface {
	ReadFile(path string) ([]byte, error)
	WriteFile(path string, data []byte, perm fs.FileMode) error
	Stat(path string) (fs.FileInfo, error)
	MkdirAll(path string, perm fs.FileMode) error
	Rename(oldpath, newpath string) error
	Remove(path string) error
}
```

- [ ] **Step 4: Implement `fsx/os.go`**

```go
package fsx

import (
	"io/fs"
	"os"
)

type OS struct{}

func NewOS() *OS { return &OS{} }

func (OS) ReadFile(path string) ([]byte, error)              { return os.ReadFile(path) }
func (OS) WriteFile(path string, b []byte, p fs.FileMode) error { return os.WriteFile(path, b, p) }
func (OS) Stat(path string) (fs.FileInfo, error)             { return os.Stat(path) }
func (OS) MkdirAll(path string, p fs.FileMode) error         { return os.MkdirAll(path, p) }
func (OS) Rename(o, n string) error                          { return os.Rename(o, n) }
func (OS) Remove(p string) error                             { return os.Remove(p) }
```

- [ ] **Step 5: Run — pass**

- [ ] **Step 6: Commit**

```bash
git add internal/fsx/fs.go internal/fsx/os.go internal/fsx/os_test.go
git commit -m "feat(fsx): FS interface + os implementation"
```

---

## Task 13: Logger setup (slog with text + JSON handlers)

**Files:**
- Create: `internal/observability/logger.go`
- Create: `internal/observability/logger_test.go`

- [ ] **Step 1: Write failing test**

```go
package observability_test

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mttzzz/pgsync/internal/observability"
)

func TestNewLoggerText(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	log, err := observability.NewLogger(observability.Options{
		Level: "info", Format: "text", Out: &buf,
	})
	require.NoError(t, err)
	log.Info("hello", "key", "val")
	out := buf.String()
	assert.Contains(t, out, "hello")
	assert.Contains(t, out, "key=val")
}

func TestNewLoggerJSON(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	log, err := observability.NewLogger(observability.Options{
		Level: "debug", Format: "json", Out: &buf,
	})
	require.NoError(t, err)
	log.Debug("event", "n", 1)
	line := strings.TrimSpace(buf.String())
	var m map[string]any
	require.NoError(t, json.Unmarshal([]byte(line), &m))
	assert.Equal(t, "event", m["msg"])
	assert.EqualValues(t, 1, m["n"])
	assert.Equal(t, "DEBUG", m["level"])
}

func TestNewLoggerInvalidLevel(t *testing.T) {
	t.Parallel()
	_, err := observability.NewLogger(observability.Options{Level: "bogus", Format: "text"})
	require.Error(t, err)
}

func TestNewLoggerInvalidFormat(t *testing.T) {
	t.Parallel()
	_, err := observability.NewLogger(observability.Options{Level: "info", Format: "yaml"})
	require.Error(t, err)
}

func TestNewLoggerLevelFiltersDebug(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	log, err := observability.NewLogger(observability.Options{Level: "info", Format: "text", Out: &buf})
	require.NoError(t, err)
	log.Debug("filtered")
	assert.Empty(t, buf.String())
}
```

- [ ] **Step 2: Run — fail**

- [ ] **Step 3: Implement `internal/observability/logger.go`**

```go
/*
 * Package observability owns the slog setup. Two handlers — text (human) and
 * json (NDJSON for the agent --output=json mode). Logger is constructed at
 * cmd/pgsync/main and threaded through context where appropriate.
 */
package observability

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
)

type Options struct {
	Level  string
	Format string
	Out    io.Writer
}

func NewLogger(opts Options) (*slog.Logger, error) {
	out := opts.Out
	if out == nil {
		out = os.Stderr
	}
	lvl, err := parseLevel(opts.Level)
	if err != nil {
		return nil, err
	}

	hopts := &slog.HandlerOptions{Level: lvl}
	var h slog.Handler
	switch opts.Format {
	case "text":
		h = slog.NewTextHandler(out, hopts)
	case "json":
		h = slog.NewJSONHandler(out, hopts)
	default:
		return nil, fmt.Errorf("unknown format %q", opts.Format)
	}
	return slog.New(h), nil
}

func parseLevel(s string) (slog.Level, error) {
	switch s {
	case "debug":
		return slog.LevelDebug, nil
	case "info":
		return slog.LevelInfo, nil
	case "warn":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return 0, errors.New("level must be debug|info|warn|error")
	}
}
```

- [ ] **Step 4: Run — pass**

- [ ] **Step 5: Commit**

```bash
git add internal/observability/logger.go internal/observability/logger_test.go
git commit -m "feat(observability): slog logger with text + JSON handlers"
```

---

## Task 14: Models — Database, Table, FKDep, SyncPlan, SyncResult, Progress

**Files:**
- Create: `internal/models/database.go`
- Create: `internal/models/table.go`
- Create: `internal/models/plan.go`
- Create: `internal/models/progress.go`
- Create: `internal/models/models_test.go`

- [ ] **Step 1: Write failing test `internal/models/models_test.go`**

```go
package models_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/mttzzz/pgsync/internal/models"
)

func TestDatabaseString(t *testing.T) {
	t.Parallel()
	d := models.Database{Name: "ai", SizeBytes: 1024 * 1024}
	assert.Contains(t, d.String(), "ai")
	assert.Contains(t, d.String(), "1.0 MB")
}

func TestTableQualifiedName(t *testing.T) {
	t.Parallel()
	tbl := models.Table{Schema: "public", Name: "users"}
	assert.Equal(t, `"public"."users"`, tbl.QualifiedName())
}

func TestSyncPlanIsEmpty(t *testing.T) {
	t.Parallel()
	assert.True(t, (&models.SyncPlan{}).IsEmpty())
	assert.False(t, (&models.SyncPlan{Database: "x"}).IsEmpty())
}

func TestProgressPercent(t *testing.T) {
	t.Parallel()
	p := models.Progress{Done: 25, Total: 100}
	assert.InDelta(t, 25.0, p.Percent(), 0.001)

	zero := models.Progress{Done: 1, Total: 0}
	assert.Equal(t, 0.0, zero.Percent())
}

func TestSyncResultDuration(t *testing.T) {
	t.Parallel()
	start := time.Now()
	r := models.SyncResult{StartedAt: start, FinishedAt: start.Add(2 * time.Second)}
	assert.Equal(t, 2*time.Second, r.Duration())
}

func TestFormatBytes(t *testing.T) {
	t.Parallel()
	cases := map[int64]string{
		0:                "0 B",
		512:              "512 B",
		1024:             "1.0 KB",
		1024 * 1024:      "1.0 MB",
		1024 * 1024 * 5:  "5.0 MB",
		1024 * 1024 * 1024: "1.0 GB",
	}
	for in, want := range cases {
		assert.Equal(t, want, models.FormatBytes(in), "in=%d", in)
	}
}
```

- [ ] **Step 2: Run — fail**

- [ ] **Step 3: Implement `internal/models/database.go`**

```go
package models

import "fmt"

type Database struct {
	Name       string
	SizeBytes  int64
	Owner      string
}

func (d Database) String() string {
	return fmt.Sprintf("%s (%s)", d.Name, FormatBytes(d.SizeBytes))
}

/* FormatBytes is a small human-friendly byte formatter (KB / MB / GB). */
func FormatBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for n2 := n / unit; n2 >= unit; n2 /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(n)/float64(div), "KMGTPE"[exp])
}
```

- [ ] **Step 4: Implement `internal/models/table.go`**

```go
package models

import "fmt"

type Table struct {
	Schema     string
	Name       string
	SizeBytes  int64
	Rows       int64
}

/* QualifiedName returns the SQL-quoted "schema"."name". */
func (t Table) QualifiedName() string {
	return fmt.Sprintf(`"%s"."%s"`, t.Schema, t.Name)
}

/* FKDep represents a foreign-key edge: From depends on To. */
type FKDep struct {
	From Table
	To   Table
}
```

- [ ] **Step 5: Implement `internal/models/plan.go`**

```go
package models

import "time"

type SyncPlan struct {
	Database string
	Tables   []Table /* If empty, the engine syncs the whole DB. */
	DryRun   bool
	Threads  int
	Engine   string
}

func (p *SyncPlan) IsEmpty() bool { return p.Database == "" }

type SyncResult struct {
	Database     string
	StartedAt    time.Time
	FinishedAt   time.Time
	BytesCopied  int64
	RowsCopied   int64
	TablesCopied int
	Stages       map[string]time.Duration
	Err          error
}

func (r SyncResult) Duration() time.Duration {
	return r.FinishedAt.Sub(r.StartedAt)
}
```

- [ ] **Step 6: Implement `internal/models/progress.go`**

```go
package models

import "context"

type Progress struct {
	Stage string
	Table string
	Done  int64
	Total int64
}

func (p Progress) Percent() float64 {
	if p.Total <= 0 {
		return 0
	}
	return float64(p.Done) / float64(p.Total) * 100.0
}

/*
 * ProgressObserver receives events from the engine. Implementations:
 *  - cli/plain_output → human text
 *  - cli/agent_progress → NDJSON
 *  - tui screens → bubbletea messages
 */
type ProgressObserver interface {
	OnEvent(ctx context.Context, p Progress)
}
```

- [ ] **Step 7: Run — pass**

- [ ] **Step 8: Commit**

```bash
git add internal/models/database.go internal/models/table.go \
        internal/models/plan.go internal/models/progress.go \
        internal/models/models_test.go
git commit -m "feat(models): Database, Table, FKDep, SyncPlan/Result, Progress"
```

---

## Task 15: Proxy tunnel — port from dbsync

**Files:**
- Create: `internal/proxy/tunnel.go`
- Create: `internal/proxy/dialer.go`
- Create: `internal/proxy/tunnel_test.go`

> Reference: `C:\Users\kiril\projects\go\dbsync\internal\services\proxy_tunnel.go` — read it first to understand the SOCKS5 / HTTP CONNECT logic. We re-implement here in a smaller, PG-only form.

- [ ] **Step 1: Add x/net dep**

```bash
go get golang.org/x/net/proxy@latest
```

- [ ] **Step 2: Read dbsync reference**

```bash
cat /c/Users/kiril/projects/go/dbsync/internal/services/proxy_tunnel.go | head -100
```
Don't copy verbatim — understand SOCKS5 + HTTP CONNECT shape, then write idiomatic Go.

- [ ] **Step 3: Write failing test `internal/proxy/tunnel_test.go`**

```go
package proxy_test

import (
	"context"
	"errors"
	"net"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	pgproxy "github.com/mttzzz/pgsync/internal/proxy"
)

type fakeDialer struct {
	calls atomic.Int32
	conn  net.Conn
	err   error
}

func (f *fakeDialer) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	f.calls.Add(1)
	if f.err != nil {
		return nil, f.err
	}
	return f.conn, nil
}

func TestNewDirectDialerNoProxy(t *testing.T) {
	t.Parallel()
	d, err := pgproxy.NewDialer("")
	require.NoError(t, err)
	assert.NotNil(t, d)
}

func TestNewDialerSocks5(t *testing.T) {
	t.Parallel()
	d, err := pgproxy.NewDialer("socks5://localhost:1080")
	require.NoError(t, err)
	assert.NotNil(t, d)
}

func TestNewDialerInvalidScheme(t *testing.T) {
	t.Parallel()
	_, err := pgproxy.NewDialer("ftp://nope")
	require.Error(t, err)
}

func TestNewDialerInvalidURL(t *testing.T) {
	t.Parallel()
	_, err := pgproxy.NewDialer("://broken")
	require.Error(t, err)
}

func TestTunnelDialFailure(t *testing.T) {
	t.Parallel()
	fake := &fakeDialer{err: errors.New("boom")}
	tn := pgproxy.NewTunnel(fake, "host:5432")

	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	_, err := tn.Dial(ctx)
	require.Error(t, err)
	assert.EqualValues(t, 1, fake.calls.Load())
}
```

- [ ] **Step 4: Run — fail**

- [ ] **Step 5: Implement `internal/proxy/dialer.go`**

```go
/*
 * Package proxy resolves PGSYNC_REMOTE_PROXY_URL into a context-aware Dialer.
 * Used by both pgx connection setup and child pg_dump (via local TCP tunnel
 * — added in tunnel.go).
 */
package proxy

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"

	xproxy "golang.org/x/net/proxy"
)

type Dialer interface {
	DialContext(ctx context.Context, network, addr string) (net.Conn, error)
}

/* directDialer wraps net.Dialer to satisfy the Dialer interface. */
type directDialer struct{ d net.Dialer }

func (dd *directDialer) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	return dd.d.DialContext(ctx, network, addr)
}

/*
 * NewDialer returns a Dialer that respects rawURL. Empty string → direct.
 * Schemes: socks5, socks5h, http, https. http/https use HTTP CONNECT.
 */
func NewDialer(rawURL string) (Dialer, error) {
	if rawURL == "" {
		return &directDialer{}, nil
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("parse proxy url: %w", err)
	}
	switch u.Scheme {
	case "socks5", "socks5h":
		var auth *xproxy.Auth
		if pw, ok := u.User.Password(); ok {
			auth = &xproxy.Auth{User: u.User.Username(), Password: pw}
		}
		dialer, err := xproxy.SOCKS5("tcp", u.Host, auth, &net.Dialer{})
		if err != nil {
			return nil, fmt.Errorf("init socks5: %w", err)
		}
		ctxDialer, ok := dialer.(xproxy.ContextDialer)
		if !ok {
			return nil, errors.New("socks5 dialer is not context-aware")
		}
		return socksAdapter{ctxDialer}, nil
	case "http", "https":
		return &httpConnectDialer{proxyURL: u}, nil
	default:
		return nil, fmt.Errorf("unsupported proxy scheme %q", u.Scheme)
	}
}

type socksAdapter struct{ d xproxy.ContextDialer }

func (s socksAdapter) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	return s.d.DialContext(ctx, network, addr)
}

/* httpConnectDialer issues HTTP CONNECT for tunneling TCP through a proxy. */
type httpConnectDialer struct{ proxyURL *url.URL }

func (h *httpConnectDialer) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	var d net.Dialer
	conn, err := d.DialContext(ctx, "tcp", h.proxyURL.Host)
	if err != nil {
		return nil, fmt.Errorf("dial http proxy: %w", err)
	}
	req := fmt.Sprintf("CONNECT %s HTTP/1.1\r\nHost: %s\r\n\r\n", addr, addr)
	if _, err := conn.Write([]byte(req)); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("write CONNECT: %w", err)
	}
	br := make([]byte, 256)
	n, err := conn.Read(br)
	if err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("read CONNECT response: %w", err)
	}
	if n < 12 || string(br[:12]) != "HTTP/1.1 200" && string(br[:12]) != "HTTP/1.0 200" {
		_ = conn.Close()
		return nil, fmt.Errorf("CONNECT failed: %q", br[:n])
	}
	return conn, nil
}
```

- [ ] **Step 6: Implement `internal/proxy/tunnel.go`**

```go
package proxy

import (
	"context"
	"net"
)

/*
 * Tunnel wraps a Dialer + remote address, providing a single Dial call that
 * returns the live connection. Used both by pgx (passed as DialFunc) and by
 * a local TCP listener for the pg_dump child process.
 */
type Tunnel struct {
	d    Dialer
	addr string
}

func NewTunnel(d Dialer, remoteAddr string) *Tunnel {
	return &Tunnel{d: d, addr: remoteAddr}
}

func (t *Tunnel) Dial(ctx context.Context) (net.Conn, error) {
	return t.d.DialContext(ctx, "tcp", t.addr)
}
```

- [ ] **Step 7: Run — pass**

```bash
go test -race ./internal/proxy/...
```

- [ ] **Step 8: Commit**

```bash
git add go.mod go.sum internal/proxy/dialer.go internal/proxy/tunnel.go internal/proxy/tunnel_test.go
git commit -m "feat(proxy): SOCKS5/SOCKS5h/HTTP CONNECT dialer + tunnel wrapper"
```

---

## Task 16: pgschema deps — FK graph + topological sort

**Files:**
- Create: `internal/pgschema/deps.go`
- Create: `internal/pgschema/deps_test.go`

> Plan-2 NativeEngine drives data-copy in dependency order: parents before children. We isolate the graph logic here so it's pure / unit-tested without a database.

- [ ] **Step 1: Write failing test `internal/pgschema/deps_test.go`**

```go
package pgschema_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mttzzz/pgsync/internal/models"
	"github.com/mttzzz/pgsync/internal/pgschema"
)

func tbl(name string) models.Table {
	return models.Table{Schema: "public", Name: name}
}

func dep(from, to string) models.FKDep {
	return models.FKDep{From: tbl(from), To: tbl(to)}
}

func TestTopoSortLinear(t *testing.T) {
	t.Parallel()
	tables := []models.Table{tbl("a"), tbl("b"), tbl("c")}
	deps := []models.FKDep{dep("c", "b"), dep("b", "a")}
	got, err := pgschema.TopoSort(tables, deps)
	require.NoError(t, err)
	assert.Equal(t, []string{"a", "b", "c"}, names(got))
}

func TestTopoSortDiamond(t *testing.T) {
	t.Parallel()
	tables := []models.Table{tbl("a"), tbl("b"), tbl("c"), tbl("d")}
	deps := []models.FKDep{
		dep("d", "b"), dep("d", "c"),
		dep("b", "a"), dep("c", "a"),
	}
	got, err := pgschema.TopoSort(tables, deps)
	require.NoError(t, err)
	idx := map[string]int{}
	for i, t := range got {
		idx[t.Name] = i
	}
	assert.Less(t, idx["a"], idx["b"])
	assert.Less(t, idx["a"], idx["c"])
	assert.Less(t, idx["b"], idx["d"])
	assert.Less(t, idx["c"], idx["d"])
}

func TestTopoSortSelfReference(t *testing.T) {
	t.Parallel()
	/* Self-FK (categories.parent_id → categories.id) is not a graph cycle
	   for our purposes — we treat it as a single node and let the engine
	   handle ordering inside the table via insert order. */
	tables := []models.Table{tbl("categories")}
	deps := []models.FKDep{dep("categories", "categories")}
	got, err := pgschema.TopoSort(tables, deps)
	require.NoError(t, err)
	assert.Equal(t, []string{"categories"}, names(got))
}

func TestTopoSortRealCycleErrors(t *testing.T) {
	t.Parallel()
	tables := []models.Table{tbl("a"), tbl("b")}
	deps := []models.FKDep{dep("a", "b"), dep("b", "a")}
	_, err := pgschema.TopoSort(tables, deps)
	require.Error(t, err)
	var ce *pgschema.CycleError
	require.ErrorAs(t, err, &ce)
	assert.NotEmpty(t, ce.Cycle)
}

func TestTopoSortIgnoresUnknownDep(t *testing.T) {
	t.Parallel()
	tables := []models.Table{tbl("a"), tbl("b")}
	deps := []models.FKDep{dep("a", "ghost")}
	got, err := pgschema.TopoSort(tables, deps)
	require.NoError(t, err)
	assert.Equal(t, []string{"a", "b"}, names(got))
}

func names(ts []models.Table) []string {
	out := make([]string, len(ts))
	for i, t := range ts {
		out[i] = t.Name
	}
	return out
}
```

- [ ] **Step 2: Run — fail**

- [ ] **Step 3: Implement `internal/pgschema/deps.go`**

```go
/*
 * Package pgschema isolates Postgres catalog logic: FK graph, topological
 * sort, --tables filter with auto-FK closure. Pure data structures only;
 * actual catalog queries live in Plan 2 (with a pgx-backed Service).
 */
package pgschema

import (
	"fmt"
	"sort"

	"github.com/mttzzz/pgsync/internal/models"
)

type CycleError struct {
	Cycle []string
}

func (e *CycleError) Error() string {
	return fmt.Sprintf("FK cycle detected: %v", e.Cycle)
}

/*
 * TopoSort returns tables in dependency-first order: parents (referenced by
 * FKs) come before children (referencing). Self-FKs are ignored. Real cycles
 * across distinct nodes return *CycleError.
 *
 * Algorithm: Kahn's, with deterministic tie-breaking (sort by qualified name)
 * so the output is reproducible across runs.
 */
func TopoSort(tables []models.Table, deps []models.FKDep) ([]models.Table, error) {
	keyOf := func(t models.Table) string { return t.QualifiedName() }
	byKey := make(map[string]models.Table, len(tables))
	for _, t := range tables {
		byKey[keyOf(t)] = t
	}

	indeg := make(map[string]int, len(tables))
	children := make(map[string][]string, len(tables))
	for k := range byKey {
		indeg[k] = 0
	}

	for _, d := range deps {
		fromK, toK := keyOf(d.From), keyOf(d.To)
		if fromK == toK {
			continue /* self-FK — ignore */
		}
		if _, ok := byKey[fromK]; !ok {
			continue /* unknown table — skip */
		}
		if _, ok := byKey[toK]; !ok {
			continue
		}
		children[toK] = append(children[toK], fromK)
		indeg[fromK]++
	}

	/* Initial set: all in-degree 0, sorted lexicographically. */
	ready := make([]string, 0, len(tables))
	for k, deg := range indeg {
		if deg == 0 {
			ready = append(ready, k)
		}
	}
	sort.Strings(ready)

	out := make([]models.Table, 0, len(tables))
	for len(ready) > 0 {
		head := ready[0]
		ready = ready[1:]
		out = append(out, byKey[head])

		next := children[head]
		sort.Strings(next)
		for _, c := range next {
			indeg[c]--
			if indeg[c] == 0 {
				ready = append(ready, c)
				sort.Strings(ready)
			}
		}
	}

	if len(out) != len(tables) {
		var stuck []string
		for k, deg := range indeg {
			if deg > 0 {
				stuck = append(stuck, k)
			}
		}
		sort.Strings(stuck)
		return nil, &CycleError{Cycle: stuck}
	}
	return out, nil
}
```

- [ ] **Step 4: Run — pass**

```bash
go test -race ./internal/pgschema/...
```

- [ ] **Step 5: Commit**

```bash
git add internal/pgschema/deps.go internal/pgschema/deps_test.go
git commit -m "feat(pgschema): FK graph + Kahn topological sort"
```

---

## Task 17: pgschema filter — auto-FK closure for `--tables`

**Files:**
- Create: `internal/pgschema/filter.go`
- Create: `internal/pgschema/filter_test.go`

> When the user passes `--tables=orders,users`, we must also include any tables those depend on (transitively), so FKs apply on the target. We compute the closure here.

- [ ] **Step 1: Write failing test**

```go
package pgschema_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mttzzz/pgsync/internal/models"
	"github.com/mttzzz/pgsync/internal/pgschema"
)

func TestFKClosureLinear(t *testing.T) {
	t.Parallel()
	all := []models.Table{tbl("users"), tbl("orders"), tbl("items"), tbl("unrelated")}
	deps := []models.FKDep{
		dep("orders", "users"),
		dep("items", "orders"),
	}
	got, err := pgschema.FKClosure(all, deps, []string{"items"})
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"users", "orders", "items"}, names(got))
}

func TestFKClosureUnknownTable(t *testing.T) {
	t.Parallel()
	all := []models.Table{tbl("a")}
	_, err := pgschema.FKClosure(all, nil, []string{"ghost"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ghost")
}

func TestFKClosureAlreadyClosed(t *testing.T) {
	t.Parallel()
	all := []models.Table{tbl("a"), tbl("b")}
	deps := []models.FKDep{dep("b", "a")}
	got, err := pgschema.FKClosure(all, deps, []string{"a", "b"})
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"a", "b"}, names(got))
}

func TestFKClosureEmptyRequestReturnsAll(t *testing.T) {
	t.Parallel()
	all := []models.Table{tbl("a"), tbl("b")}
	got, err := pgschema.FKClosure(all, nil, nil)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"a", "b"}, names(got))
}
```

- [ ] **Step 2: Run — fail**

- [ ] **Step 3: Implement `internal/pgschema/filter.go`**

```go
package pgschema

import (
	"fmt"
	"slices"
	"strings"

	"github.com/mttzzz/pgsync/internal/models"
)

/*
 * FKClosure returns the union of `requested` and every table reachable
 * by following FK edges (From → To) from any requested table. If
 * `requested` is empty, it returns all tables (the "sync everything" case).
 *
 * Matching is by table Name (schema-qualified `schema.name` also accepted).
 * Returns an error if any requested name is not found.
 */
func FKClosure(all []models.Table, deps []models.FKDep, requested []string) ([]models.Table, error) {
	if len(requested) == 0 {
		out := make([]models.Table, len(all))
		copy(out, all)
		return out, nil
	}

	byName := make(map[string]models.Table, len(all)*2)
	for _, t := range all {
		byName[t.Name] = t
		byName[t.Schema+"."+t.Name] = t
	}

	seed := make(map[string]models.Table)
	for _, r := range requested {
		t, ok := byName[r]
		if !ok {
			return nil, fmt.Errorf("table not found: %s", r)
		}
		seed[t.QualifiedName()] = t
	}

	/* BFS along FK edges, From -> To. */
	parents := make(map[string][]models.Table, len(all))
	for _, d := range deps {
		k := d.From.QualifiedName()
		parents[k] = append(parents[k], d.To)
	}

	queue := make([]models.Table, 0, len(seed))
	for _, t := range seed {
		queue = append(queue, t)
	}
	for len(queue) > 0 {
		head := queue[0]
		queue = queue[1:]
		for _, p := range parents[head.QualifiedName()] {
			k := p.QualifiedName()
			if _, ok := seed[k]; !ok {
				seed[k] = p
				queue = append(queue, p)
			}
		}
	}

	out := make([]models.Table, 0, len(seed))
	for _, t := range seed {
		out = append(out, t)
	}
	slices.SortFunc(out, func(a, b models.Table) int {
		return strings.Compare(a.QualifiedName(), b.QualifiedName())
	})
	return out, nil
}
```

- [ ] **Step 4: Run — pass**

- [ ] **Step 5: Commit**

```bash
git add internal/pgschema/filter.go internal/pgschema/filter_test.go
git commit -m "feat(pgschema): --tables auto-FK closure (transitive parents)"
```

---

## Task 18: pgtools locator — system PATH lookup

**Files:**
- Create: `internal/engine/pgtools/locate.go`
- Create: `internal/engine/pgtools/locate_test.go`

> In Phase 1, locator only finds system-installed `pg_dump` / `pg_restore` in `$PATH`. Embedded extraction comes in Plan 4. The interface here lets Plan 4 swap in an embed-aware Locator without touching callers.

- [ ] **Step 1: Write failing test**

```go
package pgtools_test

import (
	"errors"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mttzzz/pgsync/internal/engine/pgtools"
)

type fakeLooker struct {
	paths map[string]string
}

func (f fakeLooker) LookPath(file string) (string, error) {
	if p, ok := f.paths[file]; ok {
		return p, nil
	}
	return "", errors.New("not found")
}

func TestSystemLocatorFound(t *testing.T) {
	t.Parallel()
	binDump := filepath.Join("/usr/bin", pgtools.BinDump())
	binRestore := filepath.Join("/usr/bin", pgtools.BinRestore())
	loc := pgtools.NewSystemLocator(fakeLooker{
		paths: map[string]string{
			pgtools.BinDump():    binDump,
			pgtools.BinRestore(): binRestore,
		},
	})
	dump, err := loc.PgDump()
	require.NoError(t, err)
	assert.Equal(t, binDump, dump)

	restore, err := loc.PgRestore()
	require.NoError(t, err)
	assert.Equal(t, binRestore, restore)
}

func TestSystemLocatorMissing(t *testing.T) {
	t.Parallel()
	loc := pgtools.NewSystemLocator(fakeLooker{paths: map[string]string{}})
	_, err := loc.PgDump()
	require.Error(t, err)
}

func TestBinNamesPlatform(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		assert.Equal(t, "pg_dump.exe", pgtools.BinDump())
		assert.Equal(t, "pg_restore.exe", pgtools.BinRestore())
	} else {
		assert.Equal(t, "pg_dump", pgtools.BinDump())
		assert.Equal(t, "pg_restore", pgtools.BinRestore())
	}
}
```

- [ ] **Step 2: Run — fail**

- [ ] **Step 3: Implement `internal/engine/pgtools/locate.go`**

```go
/*
 * Package pgtools resolves paths to pg_dump / pg_restore. Phase-1 covers
 * SystemLocator (PATH lookup); Plan-4 will add EmbeddedLocator that extracts
 * shipped binaries into ~/.pgsync/cache/<sha>/bin and returns those paths.
 */
package pgtools

import (
	"fmt"
	"runtime"
)

type Looker interface {
	LookPath(file string) (string, error)
}

type Locator interface {
	PgDump() (string, error)
	PgRestore() (string, error)
}

func BinDump() string {
	if runtime.GOOS == "windows" {
		return "pg_dump.exe"
	}
	return "pg_dump"
}

func BinRestore() string {
	if runtime.GOOS == "windows" {
		return "pg_restore.exe"
	}
	return "pg_restore"
}

type SystemLocator struct{ look Looker }

func NewSystemLocator(look Looker) *SystemLocator {
	return &SystemLocator{look: look}
}

func (s *SystemLocator) PgDump() (string, error)    { return s.lookup(BinDump()) }
func (s *SystemLocator) PgRestore() (string, error) { return s.lookup(BinRestore()) }

func (s *SystemLocator) lookup(name string) (string, error) {
	p, err := s.look.LookPath(name)
	if err != nil {
		return "", fmt.Errorf("%s not found in PATH: %w", name, err)
	}
	return p, nil
}
```

- [ ] **Step 4: Run — pass**

- [ ] **Step 5: Commit**

```bash
git add internal/engine/pgtools/locate.go internal/engine/pgtools/locate_test.go
git commit -m "feat(pgtools): SystemLocator (PATH lookup) + cross-OS binary names"
```

---

## Task 19: Version package + cmd/pgsync entrypoint smoke

**Files:**
- Create: `internal/version/version.go`
- Create: `internal/version/version_test.go`
- Create: `cmd/pgsync/main.go`
- Modify: `coverage.allow` (already covers `cmd/pgsync` and `internal/version`)

- [ ] **Step 1: Write failing test for version**

```go
package version_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/mttzzz/pgsync/internal/version"
)

func TestStringContainsAllFields(t *testing.T) {
	t.Parallel()
	got := version.String()
	assert.Contains(t, got, version.Version)
	assert.Contains(t, got, version.GitCommit)
	assert.Contains(t, got, version.BuildDate)
}
```

- [ ] **Step 2: Run — fail**

- [ ] **Step 3: Implement `internal/version/version.go`**

```go
/*
 * Package version exposes build-time identity. Values are overridden by
 * -ldflags at release; unit tests cover only String() composition.
 */
package version

import "fmt"

var (
	Version   = "dev"
	GitCommit = "none"
	BuildDate = "unknown"
)

func String() string {
	return fmt.Sprintf("pgsync %s (commit %s, built %s)", Version, GitCommit, BuildDate)
}
```

- [ ] **Step 4: Implement minimal `cmd/pgsync/main.go`**

```go
/*
 * Phase-1 entrypoint: prints version. Real CLI surface lands in Plan 2.
 */
package main

import (
	"fmt"
	"os"

	"github.com/mttzzz/pgsync/internal/version"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "version" {
		fmt.Println(version.String())
		return
	}
	fmt.Println("pgsync — Phase 1 foundation. CLI commands ship in Plan 2.")
	fmt.Println(version.String())
}
```

- [ ] **Step 5: Build and smoke-test**

```bash
make build
./bin/pgsync version
# Expected: pgsync dev (commit none, built unknown)
./bin/pgsync
# Expected: 2 lines (Phase 1 + version)
```

- [ ] **Step 6: Run all tests + lint + coverage gate**

```bash
make fmt
make lint
make test-unit
make coverage-gate
# Expected: all green
```

- [ ] **Step 7: Delete `.keep` placeholders that are no longer needed**

```bash
find cmd/pgsync internal -name '.keep' -delete
```

- [ ] **Step 8: Commit**

```bash
git add internal/version/version.go internal/version/version_test.go cmd/pgsync/main.go
git commit -m "feat(version,cmd): version package + Phase-1 entrypoint"
```

(Empty `.keep` deletions stage automatically; if anything is unstaged after this, run another commit.)

---

## Task 20: Phase-1 wrap-up — README, CHANGELOG, milestone tag

**Files:**
- Create: `CHANGELOG.md`
- Modify: `README.md`

- [ ] **Step 1: Update `README.md`**

```markdown
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
```

- [ ] **Step 2: Create `CHANGELOG.md`**

```markdown
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
```

- [ ] **Step 3: Final verification**

```bash
make fmt
make lint
make coverage-gate
make build
git status
# Expected: clean working tree after the commits below
```

- [ ] **Step 4: Commit + tag**

```bash
git add README.md CHANGELOG.md
git commit -m "docs: Phase-1 README/CHANGELOG; foundation complete"
git tag phase1-foundation
```

(Push and tag-push only when explicitly requested.)

---

## Self-Review Checklist

**Spec coverage** — every spec section that belongs in Phase 1 has a task:

| Spec § | Concern | Plan-1 task |
|---|---|---|
| 5 | Layout (config, runner, clock, fsx, models, proxy, pgschema, pgtools, version) | 1, 6–18 |
| 8 | Config: TOML, atomic save, paths, validators, env override | 6, 7, 8, 9 |
| 9 | Proxy: SOCKS5/SOCKS5h/HTTP/HTTPS dialer + tunnel | 15 |
| 11.1 | Coverage policy: strict 100% on `internal/`, allow-list | 3, 5 |
| 11.2 | Mocking discipline (interfaces for exec/clock/fs/dialer) | 10, 11, 12, 15 |
| 12 | Modern Go (`slog`, `slices`, `errors.Join`, generics) | enforced via lint config (Task 2) |
| 13 | CI skeleton | 5 |
| 16 | Tech stack (toml, pgx¹, x/net/proxy, golangci-lint v2) | 6, 15, 2 |

¹ `pgx` dependency is added in Plan 2 when the engine actually uses it. Phase 1 doesn't need it.

**Out of Phase 1 scope (deferred to later plans):**
- TUI screens / huh forms / ConfigEditor → Plan 3
- pgschema parser for `pg_dump` output → Plan 2 (requires real pg_dump invocations to test against)
- pgschema `Service` (ListDatabases / ListTables via pgx) → Plan 2
- NativeEngine implementation → Plan 2
- NDJSON `--output=json` observer → Plan 3
- Embed pg_tools binaries → Plan 4
- Self-update + release pipeline → Plan 4
- Fixture generator + benchmarks → Plan 4

**Type consistency** — verified across tasks:
- `models.Table` shape used identically in Tasks 14, 16, 17.
- `models.FKDep` (From, To) used identically in Tasks 14, 16, 17.
- `Looker.LookPath` signature consistent in Task 18.
- Config types referenced from validators (Task 8) and override (Task 9) match Task 6 definitions.

**Placeholder scan** — no TBD/TODO/"similar to Task N" left. All code blocks complete.

---

## Execution Handoff

Plan complete and saved to `docs/superpowers/plans/2026-05-02-pgsync-foundation.md`. Two execution options:

1. **Subagent-Driven (recommended)** — fresh subagent per task with two-stage review between tasks. Best for strict 100%-coverage discipline because each task is reviewed before moving on.
2. **Inline Execution** — tasks executed in this session with batch checkpoints.

**Which approach?**
