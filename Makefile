.PHONY: build test test-race test-unit test-e2e test-integration test-security \
       test-load test-chaos test-fuzz test-all \
       lint vet fmt check clean install run doctor help \
       coverage coverage-html

# Variables
BINARY := gateway
BUILD_DIR := ./bin
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_DATE ?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
# These must name the package that actually declares the variables. The linker
# silently ignores an -X whose symbol does not exist, so pointing these at
# `main` (which declares none of them) made `gateway version` always print
# "dev" with no error anywhere.
BUILDINFO := github.com/go-php/gateway/internal/buildinfo
LDFLAGS := -ldflags "-X $(BUILDINFO).Version=$(VERSION) -X $(BUILDINFO).Commit=$(COMMIT) -X $(BUILDINFO).BuildDate=$(BUILD_DATE)"

# Default target
all: build

# ─── Build ───────────────────────────────────────────────────

## Build the React management UI dashboard
dashboard-build:
	npm run build --prefix dashboard

## Copy dashboard build artifacts to Go static embed directory
dashboard-embed:
	cp -r dashboard/dist/* internal/ui/static/

## Build and embed dashboard
dashboard: dashboard-build dashboard-embed

## Build the gateway binary
build:
	go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY) ./cmd/gateway

## Install the gateway binary
install:
	go install $(LDFLAGS) ./cmd/gateway

## Run the gateway dev server
run:
	go run $(LDFLAGS) ./cmd/gateway serve .

## Run the gateway doctor
doctor:
	go run $(LDFLAGS) ./cmd/gateway doctor

## Cross-compile for linux/amd64, darwin/amd64, darwin/arm64
build-all:
	GOOS=linux   GOARCH=amd64 go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY)-linux-amd64   ./cmd/gateway
	GOOS=darwin  GOARCH=amd64 go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY)-darwin-amd64  ./cmd/gateway
	GOOS=darwin  GOARCH=arm64 go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY)-darwin-arm64  ./cmd/gateway

# ─── Testing ────────────────────────────────────────────────

## Run all unit tests
test: test-unit

## Run all tests with race detector
test-race:
	go test -race -count=1 ./...

## Run unit tests only
test-unit:
	go test -race -count=1 ./internal/...

## Run end-to-end tests (requires PHP-FPM)
test-e2e:
	go test -tags=e2e -race -count=1 -timeout=180s -v ./test/e2e/...

## Run integration tests (requires PHP-FPM)
test-integration:
	go test -tags=integration -race -count=1 -timeout=60s ./test/integration/...

## Run security tests
test-security:
	go test -tags=security -race -count=1 -timeout=120s -v ./test/security/...

## Run load/benchmark tests
test-load:
	go test -tags=load -bench=. -benchmem -timeout=120s ./test/load/...

## Run chaos tests (requires PHP-FPM)
test-chaos:
	go test -tags=chaos -race -count=1 -timeout=120s -v ./test/chaos/...

## Run fuzz tests (30s smoke per target)
test-fuzz:
	go test -fuzz=FuzzPathParser -fuzztime=30s ./internal/filesystem/...
	go test -fuzz=FuzzHtaccessTranslator -fuzztime=30s ./internal/diagnostics/...
	go test -fuzz=FuzzFastCGIRecordParser -fuzztime=30s ./internal/php/fastcgi/...
	go test -fuzz=FuzzFastCGIParams -fuzztime=30s ./internal/php/fastcgi/...
	go test -fuzz=FuzzCGIResponseHeaders -fuzztime=30s ./internal/php/cgi/...

## Run all tests (unit + e2e + integration + security + load + chaos)
test-all: test-unit test-e2e test-integration test-security test-load test-chaos

# ─── Code Quality ───────────────────────────────────────────

## Format all Go files
fmt:
	gofmt -s -w .
	goimports -w .

## Check formatting without modifying files
fmt-check:
	@echo "Checking gofmt..."
	@test -z "$$(gofmt -s -l .)" || (echo "Files not formatted:"; gofmt -s -l .; exit 1)
	@echo "Checking goimports..."
	@test -z "$$(goimports -l -local github.com/go-php/gateway .)" || (echo "Unsorted imports:"; goimports -l -local github.com/go-php/gateway .; exit 1)
	@echo "All formatting OK"

## Run go vet
vet:
	go vet ./...

## Run linter
lint:
	golangci-lint run

## Run all code quality checks (used by CI)
check: fmt-check vet lint test-race

# ─── Coverage ───────────────────────────────────────────────

## Run tests with coverage report (opens HTML)
coverage:
	go test -coverprofile=coverage.out -covermode=atomic ./internal/...
	go tool cover -func=coverage.out

## Generate HTML coverage report
coverage-html: coverage
	go tool cover -html=coverage.out -o coverage.html
	@echo "Open coverage.html in a browser to view the report"

# ─── Clean ──────────────────────────────────────────────────

## Remove build artifacts
clean:
	rm -rf $(BUILD_DIR)
	rm -f coverage.out coverage.html bench.txt

# ─── Development ────────────────────────────────────────────

## Validate configuration
validate-config:
	go run ./cmd/gateway config validate --config gateway.yaml

# ─── Help ───────────────────────────────────────────────────

## Show help
help:
	@echo "Usage: make [target]"
	@echo ""
	@echo "Build:"
	@echo "  build            Build the gateway binary"
	@echo "  build-all        Cross-compile for linux/darwin"
	@echo "  install          Install the gateway binary"
	@echo "  run              Run dev server"
	@echo "  doctor           Run system diagnostics"
	@echo ""
	@echo "Testing:"
	@echo "  test             Run unit tests (alias for test-unit)"
	@echo "  test-race        Run all tests with race detector"
	@echo "  test-unit        Run unit tests only"
	@echo "  test-e2e         Run end-to-end tests (requires PHP-FPM)"
	@echo "  test-integration Run integration tests (requires PHP-FPM)"
	@echo "  test-security    Run security tests"
	@echo "  test-load        Run load/benchmark tests"
	@echo "  test-chaos       Run chaos tests (requires PHP-FPM)"
	@echo "  test-fuzz        Run fuzz tests (30s smoke)"
	@echo "  test-all         Run every test suite"
	@echo ""
	@echo "Code Quality:"
	@echo "  fmt              Format code"
	@echo "  fmt-check        Check formatting (no changes)"
	@echo "  vet              Run go vet"
	@echo "  lint             Run golangci-lint"
	@echo "  check            fmt-check + vet + lint + test-race"
	@echo ""
	@echo "Coverage:"
	@echo "  coverage         Generate coverage report"
	@echo "  coverage-html    Generate and open HTML coverage"
	@echo ""
	@echo "Other:"
	@echo "  clean            Remove build artifacts"
	@echo "  validate-config  Validate gateway.yaml"
	@echo "  help             Show this help"
