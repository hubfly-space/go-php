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

- **PHP via FastCGI** — the gateway generates an FPM pool config, spawns `php-fpm`, health-checks it
  on an interval, and restarts it with exponential backoff and a circuit breaker when it dies
- **HTTPS** — SNI certificate selection with wildcard support, TLS 1.2 minimum, HTTP/2, and an
  optional HTTP→HTTPS redirect listener
- **Hardened path handling** — single-pass percent-decoding, rejection of NUL/control characters,
  backslashes, encoded slashes and double-encoding; RFC 3986 dot-segment collapsing; Lstat-first
  symlink detection with `deny` / `within_root` modes; a default protected-file deny list
- **Security policy** — a rule engine with `off` / `observe` / `balanced` / `strict` modes and
  explainable, rule-attributed denials; per-client token-bucket rate limiting with bounded state
- **Request limits** — configured body-size and PHP execution timeouts enforced in the request path
- **Observability** — structured access logs with secret redaction, Prometheus metrics with bounded
  label cardinality, and optional request tracing
- **Routing** — exact path, prefix, regex, host, method, and header matchers with rewrites and
  redirects
- **Config reload** — `SIGHUP` validates and atomically swaps configuration; an invalid file is
  rejected and the running configuration is kept
- **Extension management** — profiles (`minimal`, `web-standard`, `wordpress`, `laravel`,
  `development`) or an explicit list, with per-route overrides and `php.ini` settings
- **Framework detection** — Laravel, Symfony, WordPress, and Composer projects, with an automatic
  pivot to `public/`
- **Static serving** — MIME detection, ETag, conditional requests, `Cache-Control` policy with
  immutable-asset paths, and precompressed `.br` / `.gz` variants
- **OS isolation** — optional cgroup, namespace, and credential isolation for the php-fpm process
  (see the caveat under [Security model](#security-model))
- **Diagnostics** — `doctor` (system readiness), `compat` (framework and `.htaccess` scan with a
  0–100 score), `explain` (request decision trace), `migrate htaccess` (Apache rewrite translation),
  `test routes` (route contract tests), `shadow` (runtime A/B comparison), `incident capture`
- **Releases** — immutable release directories with atomic symlink activation and rollback
- **Management UI** — an embedded React dashboard on a separate loopback listener, behind bearer
  token authentication

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
migrate     Translate Apache config to gateway routes (htaccess)
test        Run route contract tests (routes)
shadow      Compare an active runtime against a candidate
incident    Capture diagnostic incident snapshot
service     Install systemd service unit
version     Show build and version metadata
```

Translate an `.htaccess` into a routes block you can paste into `gateway.yaml`:

```bash
gateway migrate htaccess ./public
```

Check routing behavior in CI — the command exits non-zero if any contract fails:

```bash
gateway test routes --config gateway.yaml
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
  max_body_size: 20MB          # enforced before the request reaches PHP
  mode: balanced               # off | observe | balanced | strict
  rate_limit:
    enabled: false
    requests_per_minute: 600
    burst: 100
  protected_patterns: [.env, .git, "*.sql", composer.json, gateway.yaml]

tls:
  mode: disabled               # disabled | files   ("acme" is rejected — see below)
  cert_file: /etc/ssl/cert.pem
  key_file: /etc/ssl/key.pem
  cert_dir: /etc/ssl/gateway   # optional, scanned for per-host SNI certificates
  redirect_from: ":80"         # optional HTTP→HTTPS redirect listener

static:
  max_age: 1h                  # 0 disables Cache-Control
  immutable_paths: ["/assets/", "/static/"]
  no_cache_paths: []
  precompressed: true          # serve a sibling .br or .gz when accepted

observability:
  metrics:
    enabled: true              # served on the management listener, not the app port
    path: /metrics
  tracing:
    enabled: false
    retention: 5m
  redact_keys: [authorization, cookie, set-cookie, password, token]
```

`security.mode` governs policy *rules* only. Structural protections — path canonicalization, script
mapping, the filesystem boundary — are never disabled by it, including at `off`.

Sending `SIGHUP` reloads the file. Routes, protected patterns, symlink mode, body limits, and cache
policy take effect immediately; the listen address, TLS settings, php-fpm binary, security mode, and
rate limits require a restart, and the gateway logs which of those changed.

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
| Automatic TLS (ACME) | **Not implemented.** `tls.mode: acme` is rejected at config load, because the ACME code never contacts a CA — it would serve a self-signed certificate under a Let's Encrypt-shaped API. Use `tls.mode: files`. |
| Connection pooling | one Unix socket dial per PHP request; no reuse |
| Response streaming | PHP responses are fully buffered before the first byte is sent, so SSE and very large downloads don't work |
| Request abort | a timed-out PHP request does not send `FCGI_ABORT_REQUEST`; the backend keeps working on it |
| Reverse proxy, WebSocket proxying | not implemented; `internal/proxy/` is empty |
| Upload pipeline (§25) | not implemented — no multipart limits or executable-path policy |
| Trusted proxies (§10.3) | not implemented; `X-Forwarded-For` is deliberately **not** trusted anywhere |
| Request framing hardening (§10.4) | not implemented |
| Concurrency limits and backpressure (§24) | not implemented |
| State store | `internal/state/` is specified in §7 and does not exist |
| Windows backend | not implemented; `internal/platform/*` are empty |
| Per-route PHP runtime, deployment replay | not implemented |
| Deploy pipeline | `ReleaseManager` is wired; `Switcher` and `CanarySwitcher` are not |
| Public SDK | `pkg/{configapi,policyapi,pluginapi}` are empty |
| Packaging | `packaging/` and `scripts/` are empty — no Docker image, deb/rpm, Homebrew, or release workflow |

### Management API

The management listener (default `127.0.0.1:30200`) requires a bearer token. One is generated at
startup and printed to stderr along with a dashboard URL:

```
Management dashboard: http://127.0.0.1:30200/?token=<token>
Management API token: <token>
```

Loading that URL sets an `HttpOnly`, `SameSite=Strict` session cookie, so the token does not stay in
the address bar and page JavaScript never holds it. API clients send `Authorization: Bearer <token>`.
Cross-origin requests are refused even with a valid token, and the log WebSocket enforces the same
origin check.

This matters because the management API can create directories, start listeners on arbitrary ports
with PHP execution enabled, and activate releases. Binding it to a non-loopback address exposes all
of that — the gateway logs a warning if you do. Pass `--ui-addr ""` to disable it entirely.

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
- a protected-file deny list (`.env`, `.git`, `composer.json`, `*.sql`, `*.log`, …), applied to both
  the main gateway and UI-managed sites
- the client cannot supply `SCRIPT_FILENAME`
- CGI response header validation, with control-character and injection rejection
- policy rules with observe-before-enforce, and denials attributed to a named rule
- per-client rate limiting keyed on the transport peer address, with bounded state
- bearer-token authentication, origin validation, audit logging, and security headers on the
  management API
- secret redaction in the access log
- fuzz targets over the path parser, the CGI response parser, and the FastCGI record codec

Not yet enforced: multipart and upload policy (§25), concurrency limits and backpressure (§24),
trusted-proxy handling (§10.3), and request framing hardening (§10.4). See the table above.

Two honesty notes, per §28.1 and §26.1:

- **Isolation.** `php.isolation` can apply cgroup, namespace, and credential restrictions, but this
  does not make the gateway safe for untrusted multi-tenant workloads. That requires a container or
  microVM boundary the gateway does not provide.
- **Policy rules.** The rule engine is a risk-reduction layer, not a guarantee that applications are
  secure.

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
