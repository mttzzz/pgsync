.PHONY: help build build-all package checksums release-local test test-unit test-race test-integration test-all test-coverage coverage-gate \
        lint fmt deps deps-update pgtools-fetch pgtools-fetch-all pgtools-verify pgtools-sync pgtools-sync-embed pgtools-prepare-release fixture-tiny fixture-medium fixture-large fixture-small fixtures bench bench-ci bench-large bench-compare clean

BINARY := pgsync
BUILD_DIR := bin
VERSION ?= dev
PLATFORM ?= linux-amd64
BASELINE ?= benchmarks/results/main
CANDIDATE ?= benchmarks/results/local
GO_FILES := $(shell find . -name '*.go' -not -path './vendor/*' -not -path './embed/*')

help:
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) \
	  | awk 'BEGIN {FS = ":.*?## "}; {printf "  %-22s %s\n", $$1, $$2}'

build: ## Build the binary
	@mkdir -p $(BUILD_DIR)
	go build -o $(BUILD_DIR)/$(BINARY) ./cmd/pgsync

build-all: ## Build release binaries for common platforms
	@mkdir -p dist
	GOOS=linux GOARCH=amd64 go build -ldflags "-X github.com/mttzzz/pgsync/internal/version.Version=$(VERSION)" -o dist/$(BINARY)-linux-amd64 ./cmd/pgsync
	GOOS=darwin GOARCH=amd64 go build -ldflags "-X github.com/mttzzz/pgsync/internal/version.Version=$(VERSION)" -o dist/$(BINARY)-darwin-amd64 ./cmd/pgsync
	GOOS=darwin GOARCH=arm64 go build -ldflags "-X github.com/mttzzz/pgsync/internal/version.Version=$(VERSION)" -o dist/$(BINARY)-darwin-arm64 ./cmd/pgsync
	GOOS=windows GOARCH=amd64 go build -ldflags "-X github.com/mttzzz/pgsync/internal/version.Version=$(VERSION)" -o dist/$(BINARY)-windows-amd64.exe ./cmd/pgsync

package: ## Build packaged release archives; requires embedded pgtools payloads
	bash scripts/package-release.sh --version $(VERSION)

checksums: ## Generate release checksums
	bash scripts/checksums.sh --dist dist

release-local: test pgtools-prepare-release package checksums ## Run local release workflow

test: test-unit ## Run ordinary unit tests with coverage

test-unit: ## Run unit tests with per-package coverage
	go test -covermode=atomic -coverprofile=coverage.out ./internal/... ./pkg/...

test-race: ## Run unit tests with the race detector (requires cgo/toolchain)
	go test -race ./...

test-integration: ## Run integration tests (requires Docker, pg_dump, cgo/toolchain for -race)
	go test -race -tags=integration -timeout=15m ./test/integration/...

test-all: test-race coverage-gate test-integration ## Run all unit, coverage, and integration checks

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

pgtools-fetch: ## Fetch one pgtools payload into embed/bin; set PLATFORM=linux-amd64
	python scripts/fetch-pgtools-conda.py --platform $(PLATFORM)

pgtools-fetch-all: ## Fetch all pgtools payloads into embed/bin
	python scripts/fetch-pgtools-conda.py --all

pgtools-verify: ## Verify pgtools payloads
	bash scripts/verify-pgtools.sh embed/bin

pgtools-sync pgtools-sync-embed: ## Mirror pgtools payloads into package embed tree
	bash scripts/sync-pgtools-embed.sh

pgtools-prepare-release: pgtools-fetch-all pgtools-verify pgtools-sync-embed ## Fetch, verify, and sync pgtools for release builds

fixture-tiny: ## Generate tiny deterministic fixture
	go run ./fixtures/genfixture --size=tiny --seed=42 --out=fixtures/tiny.sql.gz

fixture-medium: ## Generate medium deterministic fixture
	go run ./fixtures/genfixture --size=medium --seed=42 --out=fixtures/medium.sql.gz

fixture-large: ## Generate large deterministic fixture
	go run ./fixtures/genfixture --size=large --seed=42 --out=fixtures/large.sql.gz

fixture-small: ## Download public small fixture
	bash fixtures/download-dvdrental.sh

fixtures: fixture-tiny ## Generate default fixtures
	@echo "Run make fixture-medium or make fixture-large for heavier benchmark fixtures."

bench: ## Run benchmarks
	PGSYNC_BENCH_FIXTURES=tiny,small go test -bench=. -benchmem -run=^$$ ./benchmarks/...

bench-ci: ## Run CI benchmark set
	PGSYNC_BENCH_FIXTURES=tiny,small,medium go test -bench=. -benchmem -run=^$$ ./benchmarks/...
	$(MAKE) bench-compare CANDIDATE=$(CANDIDATE)

bench-large: ## Run large benchmark only
	PGSYNC_BENCH_FIXTURES=large go test -bench=. -benchmem -run=^$$ ./benchmarks/...

bench-compare: ## Compare benchmark JSON results
	go run ./benchmarks/compare.go --baseline $(BASELINE) --candidate $(CANDIDATE) --threshold 0.15

clean:
	rm -rf $(BUILD_DIR) coverage.out coverage.html
	go clean
