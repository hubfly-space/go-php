# go-php-gateway

A secure PHP application gateway and runtime manager written in Go. Serves PHP applications via
FastCGI/PHP-FPM with path-safety enforcement, framework detection, and release management.

PHP stays a separate runtime — the gateway supervises php-fpm over a private Unix socket rather than
embedding a PHP SAPI. That is a deliberate design choice ([spec §1, §4](.material/go-php-gateway-complete-engineering-spec.md)):
it keeps process isolation and lets unmodified FPM-style apps (WordPress, Laravel, Symfony, plain
PHP) run without application changes.

> **Status: pre-release.** This README describes what the binary actually does today. Several
> subsystems exist in `internal/` with good test coverage but are **not yet wired into the running
> server** — see [Not yet implemented](#not-yet-implemented) and [ROADMAP.md](ROADMAP.md). Per spec
> §2.1, this is not a complete Apache replacement and won't claim to be until a compatibility suite
> proves it.

## What works today

- **PHP via FastCGI** — the gateway generates an FPM pool config, spawns and stops `php-fpm`, and
  proxies requests over a private Unix socket
- **Hardened path handling** — single-pass percent-decoding, rejection of NUL/control characters,
  backslashes, encoded slashes and double-encoding; RFC 3986 dot-segment collapsing; Lstat-first
  symlink detection with `deny` / `within_root` modes; a default protected-file deny list
- **Routing** — exact path, prefix, regex, host, method, and header matchers with rewrites and
  redirects
- **Extension management** — profiles (`minimal`, `web-standard`, `wordpress`, `laravel`,
  `development`) or an explicit list, with per-route overrides and `php.ini` settings
- **Framework detection** — Laravel, Symfony, WordPress, and Composer projects, with an automatic
  pivot to `public/`
- **Static serving** — MIME detection, ETag, conditional requests
- **Diagnostics** — `doctor` (system readiness), `compat` (framework and `.htaccess` scan with a
  0–100 score), `explain` (request decision trace), `incident capture` (redacted snapshot bundle)
- **Releases** — immutable release directories with atomic symlink activation and rollback
- **Management UI** — an embedded React dashboard on a separate loopback listener

## Quick start

### Prerequisites

- Go 1.25.3+ (see `go.mod`)
- PHP-FPM 8.0+

### Build and run

```bash
go build -o gateway ./cmd/gateway
```

```bash
./gateway serve ./examples/basic --php-fpm /usr/sbin/php-fpm8.3
```

Or without building:

```bash
go run ./cmd/gateway serve . --php-fpm /usr/sbin/php-fpm8.3
```

The app listens on `:8080` by default and the management UI on `127.0.0.1:30200`
(`--ui-addr ""` disables it).

## Commands

```
serve       Start gateway dev/production server
init        Initialize a new Go-PHP Gateway project
doctor      Run system readiness & environment checks
compat      Scan project for framework & .htaccess compatibility
explain     Trace a request through the decision pipeline
config      Manage configuration (validate, init)
deploy      Manage releases and deployments (create, activate, rollback, list)
php         Manage PHP runtimes (list, install, use, remove)
incident    Capture diagnostic incident snapshot
service     Install systemd service unit
version     Show build and version metadata
```

Note: `gateway php install` is currently a stub — it registers a manifest for an empty directory
rather than downloading a runtime.

## Configuration

The shipped schema is `gateway/v1`. See [gateway.yaml](gateway.yaml) for a complete annotated file;
this is an abbreviated version:

```yaml
schema: gateway/v1

server:
  addr: ":8080"                # host:port — host must be an IP or empty ("localhost" is rejected)
  read_timeout: 30s
  write_timeout: 60s
  read_header_timeout: 5s
  idle_timeout: 120s
  max_header_bytes: 1048576

php:
  binary: /usr/sbin/php-fpm
  # socket_path: /tmp/gateway.sock   # generated when empty
  max_children: 20
  start_servers: 2
  min_spare: 2
  max_spare: 6
  max_requests: 500
  request_timeout: 60s
  extension_profile: web-standard    # mutually exclusive with php.extensions
  php_ini:
    - { name: memory_limit, value: 256M }

routes:
  - path_prefix: /api/
    target: /index.php
    methods: [GET, POST, PUT, DELETE]
  - path: /old-page
    target: /new-page
    status: 301

logging:
  format: json                 # json | text
  level: info

security:
  symlink_mode: within_root    # deny | within_root
  max_body_size: 20MB
  protected_patterns: [.env, .git, "*.sql", composer.json, gateway.yaml]
```

## Architecture

```
gateway/
├── cmd/gateway/              # main binary and request handler
├── internal/
│   ├── admin/                # admin API, token auth, CSRF, audit log   [not wired]
│   ├── buildinfo/            # version metadata
│   ├── config/               # parsing, validation, reload
│   ├── deploy/               # releases, switching, canary, hooks
│   ├── diagnostics/          # doctor, compat, explain, htaccess, shadow, contract, snapshot
│   ├── errors/               # stable error codes
│   ├── filesystem/           # path parsing, safe resolution, static, cache
│   ├── observability/        # access logs, metrics, tracing, redaction
│   ├── php/{cgi,fastcgi,fpm} # CGI vars, FastCGI client, pool config
│   ├── policy/               # WAF, rate limiting, network policy       [not wired]
│   ├── router/               # route matching and rewrites
│   ├── runtime/              # runtime registry, manifests, extensions
│   ├── supervisor/           # php-fpm supervision, OS isolation
│   ├── tls/                  # TLS/SNI cert manager, ACME              [not wired]
│   └── ui/                   # management API and embedded dashboard
├── dashboard/                # React + TypeScript + Vite source for internal/ui/static
├── test/{e2e,integration,security,load,chaos}/   # build-tag gated
└── docs/
```

`pkg/{configapi,policyapi,pluginapi}` are reserved for the public SDK and are currently empty.

## Not yet implemented

These are documented here rather than omitted, because several of them exist as tested packages that
the running binary never reaches. Details and file references are in [ROADMAP.md](ROADMAP.md).

| Area | Status |
|---|---|
| HTTPS / TLS | `internal/tls.CertManager` is complete but unwired — `main.go` only calls `ListenAndServe`. **The gateway cannot serve HTTPS.** The ACME path is a stub that returns self-signed certs. |
| WAF and rate limiting | `internal/policy` is complete and tested; nothing constructs it at runtime |
| Prometheus metrics | `PrometheusHandler` exists; no `/metrics` endpoint is served |
| Tracing | `TraceMiddleware` exists and is unwired (and needs a cleanup ticker before it is) |
| Secret redaction in logs | `SecretRedactor` exists; the access log is currently **unredacted** |
| Config reload | `config.Reloader` exists; there is no SIGHUP handler or file watcher |
| php-fpm supervision | started once, never health-checked or restarted; `HealthCheck` is called from nowhere |
| OS isolation tiers | `supervisor/isolation.go` implements cgroups/namespaces/credentials and is referenced by nothing |
| Management API auth | **the management API has no authentication, CSRF, or origin checking** — see below |
| Connection pooling | one Unix socket dial per PHP request; no reuse |
| Response streaming | PHP responses are fully buffered before the first byte is sent; SSE and large downloads don't work |
| HTTP/2, reverse proxy, WebSockets | not implemented; `internal/proxy/` is empty |
| Upload pipeline, trusted proxies, request framing hardening | not implemented |
| State store | `internal/state/` is specified in §7 and does not exist |
| Windows backend | not implemented; `internal/platform/*` are empty |
| Packaging | `packaging/` and `scripts/` are empty — no Docker image, deb/rpm, Homebrew, or release workflow |

### Security notice

The management UI listener (default `127.0.0.1:30200`) currently has **no authentication**. Among
other things, an unauthenticated `POST /api/sites` can start a listener on an arbitrary port serving
an arbitrary directory with PHP execution enabled, and the log WebSocket accepts any origin. Until
this is fixed:

- do not pass `--ui-addr` a non-loopback address
- do not run the gateway on a host where untrusted local users have network access to loopback
- consider `--ui-addr ""` to disable the management listener entirely

## Testing

```bash
go test ./...                                      # unit tests, all packages
go test -race ./...                                # with race detector
go test -tags=e2e ./test/e2e/...                   # requires php-fpm
go test -tags=integration ./test/integration/...   # requires php-fpm
go test -tags=security ./test/security/...
go test -tags=load -bench=. ./test/load/...
go test -tags=chaos ./test/chaos/...
make coverage
```

`make test-fuzz` currently fails — it references a fuzz target that does not exist.

## Security model

Implemented today:

- single-pass percent-decoding with rejection of invalid encodings, NUL and control characters,
  backslashes, encoded slashes, and double-encoded sequences
- dot-segment collapsing with URL semantics, before any filesystem mapping
- Lstat-first symlink detection with `deny` and `within_root` modes
- regular-file verification; device, socket, and pipe files rejected
- a protected-file deny list (`.env`, `.git`, `composer.json`, `*.sql`, `*.log`, …)
- the client cannot supply `SCRIPT_FILENAME`
- CGI response header validation, with control-character and injection rejection
- fuzz targets over the path parser, the CGI response parser, and the FastCGI record codec

Not yet enforced: rate limiting, WAF rules, request/multipart limits, upload policy, CSRF, audit
logging, and management API authentication. See the table above.

To report a vulnerability, please open a private security advisory rather than a public issue.

## Documentation

- [Roadmap](ROADMAP.md)
- [Quick Start Guide](docs/QUICK_START.md)
- [Deployment Guide](docs/DEPLOYMENT.md) — note this describes running behind a reverse proxy with
  `X-Forwarded-For`, which the gateway does not yet parse
- [Configuration Reference](gateway.yaml)
- [Engineering spec](.material/go-php-gateway-complete-engineering-spec.md)
- Architecture Decision Records — `docs/adr/` is currently empty; §58 pre-names ADR-0001…0012

## License

See LICENSE file.
