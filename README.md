# go-php-gateway

A secure PHP application gateway and runtime manager written in Go. Serves PHP applications via FastCGI/PHP-FPM with security inspection, runtime management, and zero-downtime switching.

## Features

- **PHP-FPM Integration** — FastCGI protocol, connection pooling, process management
- **Security-First** — Path traversal prevention, WAF, rate limiting, CSRF protection, secrets management
- **Runtime Management** — Install, switch, and manage multiple PHP versions with extension profiles
- **Zero-Downtime Deploy** — Immutable releases, atomic symlinks, canary deployments, automatic rollback
- **Observability** — Structured access logs, Prometheus metrics, OpenTelemetry tracing, audit logs
- **Production Ready** — TLS/SNI, HTTP/2, Admin API, health checks, incident snapshots

## Quick Start

### Prerequisites

- Go 1.23+
- PHP-FPM (8.0+)

### Install

```bash
go build -o gateway ./cmd/gateway
```

### Run

```bash
./gateway serve . --php-fpm /run/php/php-fpm.sock
```
### Run (with go)

```bash
go run ./cmd/gateway serve . --php-fpm /usr/sbin/php-fpm8.3
```
## Architecture

```
gateway/
├── cmd/gateway/              # Main binary
├── internal/
│   ├── admin/                # Admin API, auth, CSRF
│   ├── config/               # Config parsing, validation, reload
│   ├── deploy/               # Releases, switching, canary, hooks
│   ├── diagnostics/          # Doctor, snapshots, explain, compat
│   ├── errors/               # Error codes and wrapping
│   ├── filesystem/           # Path resolution, static files, cache
│   ├── observability/        # Logs, metrics, tracing, redaction
│   ├── php/                  # FastCGI, CGI, FPM, headers, pool
│   ├── policy/               # WAF, rate limiting, network policy
│   ├── router/               # Route matching
│   ├── runtime/              # PHP runtime management, extensions
│   ├── state/                # State storage
│   ├── supervisor/           # Process supervision, OS isolation
│   └── tls/                  # TLS/SNI, ACME
├── test/
│   ├── integration/          # Integration tests
│   ├── security/             # Security tests
│   └── load/                 # Load/benchmark tests
└── docs/
```

## Configuration

```yaml
server:
  listen: "0.0.0.0:8080"
  workers: 4
  max_connections: 1024

php:
  fpm_socket: "/run/php/php-fpm.sock"
  timeout: "30s"
  extensions: ["mbstring", "json", "curl"]

routes:
  - path: "/"
    root: "./public"
    static: true
  - path: "/api/"
    scripts: ["index.php"]
    strip_prefix: false

tls:
  enabled: true
  cert_file: "/etc/ssl/cert.pem"
  key_file: "/etc/ssl/key.pem"

security:
  protection:
    enabled: true
    block_rules: [".env", ".git"]
  rate_limit:
    enabled: true
    requests_per_second: 100
```

See [examples/gateway.yaml](examples/gateway.yaml) for the full schema.

## Commands

```bash
# Serve an application
gateway serve ./app --php-fpm /run/php/php-fpm.sock

# Validate configuration
gateway config validate --config gateway.yaml

# Check system health
gateway doctor

# Install a PHP runtime
gateway runtime install 8.3

# List runtimes
gateway runtime list

# Switch runtime
gateway runtime use 8.3

# Create deployment
gateway deploy create v1.0.0

# Rollback deployment
gateway deploy rollback

# View status
gateway status
```

## Testing

```bash
# Unit tests
go test ./...

# With race detector
go test -race ./...

# Integration tests (requires php-fpm)
go test -tags=integration ./test/integration/...

# Security tests
go test -tags=security ./test/security/...

# Load tests
go test -tags=load -bench=. ./test/load/...

# Chaos tests
go test -tags=chaos ./test/chaos/...

# Coverage
make coverage
```

## Security

- Path traversal prevention with symlink detection
- Protected file patterns (`.env`, `.git`, etc.)
- Rate limiting per-IP and per-route
- CSRF token validation
- Secrets redaction in logs
- Audit logging
- TLS/SNI with ACME support

## Documentation

- [Quick Start Guide](docs/QUICK_START.md)
- [Deployment Guide](docs/DEPLOYMENT.md)
- [Configuration Reference](gateway.yaml)
- [Architecture Decision Records](docs/adr/)

## License

See LICENSE file.
