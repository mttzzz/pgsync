.PHONY: help build build-all test test-unit test-race test-integration test-all test-coverage coverage-gate \
        lint fmt deps deps-update pgtools-fetch pgtools-verify pgtools-sync bench clean

BINARY := pgsync
BUILD_DIR := bin
GO_FILES := $(shell find . -name '*.go' -not -path './vendor/*' -not -path './embed/*')

help:
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) \
	  | awk 'BEGIN {FS = ":.*?## "}; {printf "  %-22s %s\n", $$1, $$2}'

build: ## Build the binary
	@mkdir -p $(BUILD_DIR)
	go build -o $(BUILD_DIR)/$(BINARY) ./cmd/pgsync

build-all: ## Build release binaries for common platforms
	@mkdir -p dist
	GOOS=linux GOARCH=amd64 go build -o dist/$(BINARY)-linux-amd64 ./cmd/pgsync
	GOOS=darwin GOARCH=amd64 go build -o dist/$(BINARY)-darwin-amd64 ./cmd/pgsync
	GOOS=darwin GOARCH=arm64 go build -o dist/$(BINARY)-darwin-arm64 ./cmd/pgsync
	GOOS=windows GOARCH=amd64 go build -o dist/$(BINARY)-windows-amd64.exe ./cmd/pgsync

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

pgtools-fetch: ## Fetch pgtools payloads into embed/bin
	bash scripts/fetch-pgtools.sh --all

pgtools-verify: ## Verify pgtools payloads
	bash scripts/verify-pgtools.sh embed/bin

pgtools-sync: ## Mirror pgtools payloads into package embed tree
	bash scripts/sync-pgtools-embed.sh

bench: ## Run benchmarks
	go test -bench=. -benchmem -run=^$$ ./benchmarks/...

clean:
	rm -rf $(BUILD_DIR) coverage.out coverage.html
	go clean
