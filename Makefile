.PHONY: build test test-race test-unit test-integration test-security test-load test-fuzz \
       lint vet fmt check clean install run doctor

# Variables
BINARY := gateway
BUILD_DIR := ./bin
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_DATE ?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
LDFLAGS := -ldflags "-X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.buildDate=$(BUILD_DATE)"

# Default target
all: build

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

# ─── Testing ────────────────────────────────────────────────

## Run all unit tests
test: test-unit

## Run all tests with race detector
test-race:
	go test -race -count=1 ./...

## Run unit tests only (packages in internal/)
test-unit:
	go test -race -count=1 ./internal/...

## Run integration tests (requires PHP-FPM)
test-integration:
	go test -tags=integration -race -count=1 -timeout=60s ./test/integration/...

## Run security tests
test-security:
	go test -tags=security -race -count=1 -timeout=120s ./test/security/...

## Run load/benchmark tests
test-load:
	go test -tags=load -bench=. -benchmem -timeout=120s ./test/load/...

## Run fuzz tests (short smoke)
test-fuzz:
	go test -fuzz=FuzzPathParser -fuzztime=30s ./internal/filesystem/...
	go test -fuzz=FuzzHtaccessTranslator -fuzztime=30s ./internal/diagnostics/...

## Run all tests (unit + integration + security)
test-all: test-unit test-integration test-security

# ─── Code Quality ───────────────────────────────────────────

## Format all Go files
fmt:
	gofmt -s -w .
	goimports -w .

## Run go vet
vet:
	go vet ./...

## Run linter
lint:
	golangci-lint run

## Run all code quality checks
check: fmt vet lint test-race

# ─── Coverage ───────────────────────────────────────────────

## Run tests with coverage report
coverage:
	go test -coverprofile=coverage.out ./internal/...
	go tool cover -func=coverage.out
	go tool cover -html=coverage.out -o coverage.html

## Show coverage summary
coverage-summary: coverage
	@echo "Coverage report: coverage.html"

# ─── Clean ──────────────────────────────────────────────────

## Remove build artifacts
clean:
	rm -rf $(BUILD_DIR)
	rm -f coverage.out coverage.html

# ─── Development ────────────────────────────────────────────

## Validate configuration
validate-config:
	go run ./cmd/gateway config validate --config gateway.yaml

## Show help
help:
	@echo "Targets:"
	@echo "  build            Build the gateway binary"
	@echo "  install          Install the gateway binary"
	@echo "  run              Run dev server"
	@echo "  doctor           Run system diagnostics"
	@echo "  test             Run unit tests (alias for test-unit)"
	@echo "  test-race        Run all tests with race detector"
	@echo "  test-unit        Run unit tests"
	@echo "  test-integration Run integration tests"
	@echo "  test-security    Run security tests"
	@echo "  test-load        Run load/benchmark tests"
	@echo "  test-fuzz        Run fuzz tests (30s smoke)"
	@echo "  test-all         Run unit + integration + security"
	@echo "  fmt              Format code"
	@echo "  vet              Run go vet"
	@echo "  lint             Run golangci-lint"
	@echo "  check            Format + vet + lint + test-race"
	@echo "  coverage         Generate coverage report"
	@echo "  clean            Remove build artifacts"
	@echo "  help             Show this help"
