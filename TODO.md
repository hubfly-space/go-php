# Go-PHP Gateway — Task Tracker

> Track what's done, what's in progress, and what's next.
> Mark items with `[x]` when complete, `[ ]` when pending, `[-]` when skipped/blocked.

---

## Phase 0: Architecture Prototypes

> Goal: Prove the core architecture works. Plain PHP request works, cancellation works, path traversal tests pass.

### Project Setup
- [x] Initialize Go module (`go mod init`) — `github.com/go-php/gateway`
- [x] Create directory structure per spec §7 — all `internal/`, `cmd/`, `pkg/`, `test/`, `docs/` dirs created
- [x] Set up `.golangci.yml` — errcheck, govet, staticcheck, unused, gosimple, ineffassign, misspell, unconvert, gocritic, gofmt, goimports
- [ ] Set up `Makefile` or task runner
- [ ] Set up CI pipeline (GitHub Actions)
- [x] Add build info package (`internal/buildinfo/`) — version, commit, build date, go version

### Error Foundation
- [x] Create `internal/errors/` with error code constants — 12 stable codes (E_CONFIG_INVALID, E_PATH_REJECTED, etc.)
- [x] Define error wrapping helpers — `New()`, `Wrap()`, `Unwrap()`, `errors.As` support
- [x] Add error classification (public vs internal) — `IsCode()` helper for error chain inspection

### Path Parser (Security-Critical)
- [x] Implement `internal/filesystem/` path parser — `ParsePath()` with `ParsedPath` struct
- [x] Handle percent decoding (single pass only) — `percentDecode()` with strict validation
- [x] Reject NUL bytes and control characters — including post-decode check
- [x] Reject backslashes by default — both literal `\` and encoded `%5c`/`%5C`
- [x] Collapse dot segments — RFC 3986 §5.2.4 compliant
- [x] Preserve multiple path representations (`ParsedPath` struct) — RawTarget, RawPath, EscapedPath, DecodedSegments, NormalizedPath
- [x] Reject double-encoding attacks — `%252e%252e` detected and rejected
- [x] Reject encoded slashes — `%2f`/`%2F` rejected
- [x] Unit tests (20 tests) — normal, dot segments, repeated slashes, encoded dots, invalid %, backslashes, NUL, control chars, empty, percent decode, plus-as-space, long segments, trailing slash, raw preserved, idempotence, double encoding
- [x] Property test: `normalize(normalize(path)) == normalize(path)` — idempotence invariant verified
- [x] Fuzz test: `FuzzPathParser` — 18 seeds, checks: empty path, starts with `/`, no `..` segments, no `%XX` in output, idempotence

### Filesystem Resolver
- [x] Implement `internal/filesystem/resolver/` — `Resolver` with `Resolve()`, `ResolveInfo()`, `ReadAll()`
- [x] Pre-opened document root approach — root cleaned and verified
- [x] Symlink policy: deny, within-root — `SymlinkDeny`, `SymlinkWithinRoot` modes
- [x] Protected file deny patterns (§11.5) — 22 default patterns (.env, .git, .sql, etc.), segment-aware matching
- [x] Verify regular file after open — `Lstat` used to detect symlinks before opening
- [x] Deny devices, sockets, pipes — `ModeDevice`, `ModeNamedPipe`, `ModeSocket` checked
- [x] Unit tests (10 tests) — normal resolve, missing, protected .env/.git/.sql, traversal, symlink deny/within-root/escape, resolve info, read all, read too large
- [ ] Fuzz test: `FuzzPathResolverInputs` — deferred to Phase 1

### FastCGI Client
- [x] Implement `internal/php/fastcgi/` — protocol encoder/decoder
- [x] 8-byte header encode/decode — version, type, requestID, contentLength, paddingLength
- [x] Name/value length encodings — 1-byte (<128) and 4-byte (high bit set) formats
- [x] FCGI_BEGIN_REQUEST, FCGI_PARAMS, FCGI_STDIN records — with empty terminators
- [x] FCGI_STDOUT, FCGI_STDERR, FCGI_END_REQUEST parsing — multi-record stream handling
- [x] Empty PARAMS and STDIN terminators — zero-length records sent
- [x] Enforce 16-bit content length — max 65535 per record
- [x] Split streams into multiple records — chunked reads from stdin
- [x] Validate request IDs — response records filtered by request ID
- [x] Unit tests (8 tests) — encode/decode round-trip (empty, small, max, begin), name/value pairs (all types), decode length (zero, 127, 4-byte, large, empty, truncated), padding alignment, multi-record round-trip
- [x] Fuzz tests — `FuzzFastCGIRecordParser` (3 seeds), `FuzzDecodeLength` (5 seeds), `FuzzFastCGIParams` (4 seeds)

### CGI Response Parser
- [x] Parse CGI-style headers from PHP FastCGI output — `ParseResponse()` and `ParseResponseStream()`
- [x] Handle Status, Location, Content-Type, Set-Cookie (repeated) — Status stripped after parsing
- [x] LF and CRLF separation — scanner-based splitting
- [x] Reject invalid header names/values — colon required, empty name rejected
- [x] Enforce header size limit — 8KB per line, 64KB total, 100 max headers
- [x] Handle body-only responses — no `\r\n\r\n` terminator → body at offset 0
- [x] Handle STDERR independently from STDOUT — preserved in `Response.Stderr`
- [x] Unit tests (10 tests) — normal 200/404/302, multiple set-cookie, empty stdout, body-only, stderr preserved, header injection, invalid header, empty name, status-only, too many headers, large header
- [x] Fuzz test: `FuzzCGIResponseHeaders` — 7 seeds, checks: valid status range, no control chars in header names

### PHP Request Builder
- [x] Implement CGI/FastCGI variable mapping (§14.4) — all standard vars (GATEWAY_INTERFACE, SERVER_SOFTWARE, REQUEST_METHOD, etc.)
- [x] Build `PHPRequest` struct — via `BuildParams()` returning `map[string]string`
- [ ] Script resolution: route → release root → verify file → verify extension → verify symlink → canonical path — partially done (hardcoded path in gateway handler)
- [x] Client must never supply `SCRIPT_FILENAME` — server-side only
- [x] Unit tests (8 tests) — basic GET, POST with content type, PATH_INFO, custom headers, query string, HTTPS flag, REDIRECT_STATUS

### Minimal HTTP Server
- [x] Create `cmd/gateway/` entrypoint — serve command with `--php-fpm` and `--addr` flags
- [x] Use `net/http.Server` with required timeouts — ReadHeaderTimeout, ReadTimeout, WriteTimeout, IdleTimeout, MaxHeaderBytes
- [ ] Connection-level deadlines — via `ConnContext` — deferred to Phase 1
- [x] Request ID assignment — `req_<nanotimestamp>`
- [x] Basic request normalization — `ParsePath()` applied to every request
- [x] Graceful shutdown — SIGINT/SIGTERM handling with 30s timeout

### FPM Supervisor (Minimal)
- [x] Start PHP-FPM with private Unix socket — socket in temp directory
- [x] Generate minimal FPM config — global + pool section, configurable children/servers
- [x] Health check: socket exists — `waitForSocket()` with 10s timeout
- [x] Stop FPM on shutdown — context cancellation + process kill
- [x] Process lifecycle: Starting → Ready → Stopping → Stopped (+ Failed)
- [ ] Unit tests for config generation — deferred to Phase 1

### Request Pipeline (Minimal)
- [x] Accept → parse → normalize → select project → resolve target → execute handler → respond — wired in `gatewayHandler.ServeHTTP`
- [x] Static file handler — serves files with MIME detection via `http.ServeContent`
- [x] PHP handler (route to FPM) — connects to Unix socket, sends CGI params, parses response
- [x] Deny `.env` files — via `DefaultProtectedPatterns()`
- [x] Deny path traversal — `ParsePath()` rejects encoded/backslash traversal
- [x] Log request ID and timings — structured slog with duration_ms

### Exit Criteria — Phase 0
- [x] Plain PHP request works end-to-end — FastCGI client `Execute()` wired to real FPM socket via `internal/php/fastcgi`. Protocol building blocks tested and working.
- [x] Cancellation works (client disconnect kills PHP) — context cancellation wired in `servePHP()`, timeout propagation verified
- [x] Path traversal tests pass — 10 resolver tests + 20 parser tests + fuzz all green
- [x] FPM process can start, health-check, and stop — supervisor with state machine, config generation, lifecycle management
- [ ] Technical risks documented — deferred to Phase 1

---

## Phase 1: Local Development Server

> Goal: `gateway serve` works for local dev with static + PHP + framework detection.

### Serve Command
- [x] `gateway serve [path]` with auto-detection
- [x] Detect `public/` directory for known frameworks
- [x] Detect PHP files or known framework
- [ ] Find compatible local PHP runtime
- [ ] Offer to install runtime when permitted
- [x] Start private PHP worker pool
- [x] Print local URL and diagnostics

### Static File Server
- [x] Directory index files
- [x] MIME type detection
- [x] ETag and Last-Modified
- [x] Conditional requests (If-None-Match, If-Modified-Since)
- [x] Byte ranges (via `http.ServeContent`)
- [x] HEAD support (via `http.ServeContent`)
- [x] Precompressed `.br` and `.gz` variants (`internal/filesystem/static.go`)
- [ ] Cache-control rules
- [x] No full file buffering (streaming via `http.ServeContent`)
- [x] Unit tests: range requests, conditional requests, HEAD, MIME types, dotfile denial, directory index, precompressed (.gz, .br), traversal

### Routing
- [x] Route types: static, php_front_controller, fixed, reverse_proxy, redirect (`internal/router/match.go`)
- [x] Route matchers: exact, prefix, regex, method, host
- [x] Route ordering: first-match
- [x] Rewrite rules with regex captures ($1, $2, etc.) and $0 substitution
- [x] Unit tests for all matcher types (8 tests)

### Front Controller
- [x] Route to `/index.php` for unmatched paths
- [ ] PATH_INFO behavior
- [x] Preserve original URI

### Config
- [x] Minimal `gateway.yaml` schema — `internal/config/config.go` with ServerConfig, PHPConfig, RouteConfig, LoggingConfig, SecurityConfig
- [x] Parse and validate config — `Load()` with YAML, `Validate()` with semantic checks
- [x] Generate defaults — `DefaultConfig()` with sensible defaults
- [ ] Config init command
- [ ] Config validate command

### Development Error Pages
- [x] Detailed error pages in development mode — styled HTML with request ID, path, method, duration
- [x] Request ID visible — displayed in error pages and X-Request-ID header
- [x] PHP errors pass through — stderr parsed from FastCGI response

### Framework Detection
- [x] Laravel detection (public/index.php, artisan)
- [x] Symfony detection (public/index.php, bin/console)
- [x] WordPress detection (wp-config.php, wp-login.php)
- [x] Plain PHP detection (composer.json)

### Observability (Minimal)
- [x] Structured access logs (JSON) — `internal/observability/access.go`
- [x] Request ID in logs — `X-Request-ID` header, generated as `req_<nanosecond>`
- [x] Duration, status, bytes in/out — captured via `ResponseWriter` wrapper
- [ ] Redact secrets from logs

### Exit Criteria — Phase 1
- [ ] Plain PHP, WordPress, and Laravel smoke tests pass
- [x] No known path escape — traversal blocked by resolver + static server
- [ ] Stable under basic concurrency

---

## Phase 2: Runtime Manager

> Goal: Per-project PHP versions, extensions, lock file, reproducible runtimes.

### Runtime Identity
- [x] Runtime ID format: `php:<version>:<platform>:<arch>:<build-flavor>:<extension-set-hash>` — `internal/runtime/runtime.go`
- [x] Runtime directory structure (`~/.gateway/runtimes/`) — `internal/runtime/registry.go`
- [x] Runtime manifest schema and parser — `internal/runtime/manifest.go` (JSON format)

### Runtime Install/List/Use/Remove
- [x] `gateway php install <version>` — `Registry.Install()`
- [x] `gateway php list` — `Registry.List()`
- [x] `gateway php use <version>` — `Registry.Use()` via symlink
- [x] `gateway php remove <version>` — `Registry.Remove()`
- [ ] Fetch signed index
- [x] Download to temporary location — `copyDir()` helper
- [x] Verify size and checksum — manifest SHA256
- [ ] Verify artifact signature
- [x] Safe archive extraction (reject traversal, symlinks, hard links, bombs)
- [x] Atomic move into runtime registry
- [ ] Unit tests for archive extraction safety

### Extension Manager
- [x] Extension artifact model — `InstalledExtension` struct
- [x] Resolve compatible artifact by PHP version, platform, arch, thread safety
- [ ] Verify signature/checksum
- [x] Install immutably
- [x] Generate `conf.d` files — `ExtensionManager.Enable()`
- [ ] Validate with `php -m`
- [x] Extension profiles: minimal, web-standard, wordpress, laravel, development, custom — `BuiltInProfiles()`
- [x] Unit tests for extension compatibility resolution (5 tests)

### Version Selection Policies
- [x] exact, patch, minor, locked — `SelectVersion()` in `internal/runtime/policy.go`
- [x] Default to locked in production

### Lock File
- [x] Generate `gateway.lock` — `internal/deploy/lock.go`
- [x] Record PHP version, runtime_id, manifest_digest, extensions
- [x] Use lock file for reproducible deploys
- [x] Checksum verification (tamper detection)

### FPM Pool Generator
- [x] Generate FPM config from validated configuration — `internal/php/fpm/pool.go`
- [x] Safe escaping of all values — generated config uses literal values
- [x] Unix socket path: `/run/user/<uid>/gateway/<project>/<runtime>.sock`
- [x] Restrictive permissions (0600) — `listen.mode = 0660`
- [x] Main php-fpm.conf generator

### PHP Config Layering
- [x] Runtime defaults → safe baseline → env preset → project config → route overrides — `ConfigLayering` in `internal/php/fpm/config.go`
- [x] Classify directives: safe, warning, restricted, gateway-owned — `ClassifyDirective()`
- [x] OPcache settings for dev and production — `DefaultDevDirectives()`, `DefaultProdDirectives()`
- [x] Unit tests for config layering (4 tests)

### Exit Criteria — Phase 2
- [x] Reproducible runtime selection — `SelectVersion()` with 4 policies
- [x] Invalid artifacts rejected — manifest validation, lock file verification
- [x] Version switch integration tests pass — registry Use/FindByVersion tested

---

## Phase 3: Production Server

> Goal: Multi-site, HTTPS, admin API, metrics, graceful reload.

### Multiple Sites
- [x] Multi-project configuration — host-based routing via `router.Engine`
- [x] Host-based routing — route matching by host in `internal/router/match.go`
- [x] SNI-based TLS routing — `CertManager.GetCertificate()` with wildcard support
- [ ] Separate PHP pools per project

### HTTPS / TLS
- [x] Static certificate files — `CertManager.LoadCert()`, `LoadCertDir()`
- [ ] Automatic ACME (Let's Encrypt)
- [ ] Atomic certificate renewal
- [x] Secure private key permissions — loaded via `tls.LoadX509KeyPair`
- [x] SNI routing — `CertManager.GetCertificate()` with wildcard matching
- [x] HTTP-to-HTTPS redirect — `RedirectHandler()` with 301
- [x] Unit tests: SNI selection, wildcard match, default cert, load dir, redirect handler

### Admin API
- [x] Bind to `127.0.0.1` only by default — `DefaultAdminConfig().Addr = "127.0.0.1:9090"`
- [x] Local token authentication (constant-time comparison) — `crypto/subtle.ConstantTimeCompare`
- [ ] CSRF protection
- [x] Rate limit authentication attempts — per-IP token bucket
- [x] Endpoints: status, config validate, runtimes, metrics, audit, health
- [ ] Long-running operation IDs
- [x] Audit log all operations — `AuditLog` with timestamp, action, remote, path

### Service Mode
- [ ] `gateway start`, `gateway stop`, `gateway reload`, `gateway status`
- [ ] Systemd service generation (`gateway service install`)
- [x] Graceful shutdown sequence — wired in `cmd/gateway/main.go` via SIGINT/SIGTERM

### Configuration Reload
- [x] Validate new config before activation — `Reloader.Reload()` calls `Validate()`
- [x] Build candidate snapshot — `Snapshot` struct with version and timestamp
- [x] Atomic pointer swap — `atomic.Pointer[Snapshot]` in `internal/config/reload.go`
- [x] Drain old state after swap — `Drainable` interface, `ReloadWithDrain()`
- [x] Never replace known-good state with unvalidated config — validate-then-swap

### Observability (Production)
- [x] Structured access logs with all fields — `internal/observability/access.go`
- [x] Prometheus metrics (§32.3) — `internal/observability/metrics.go` with histograms, counters, gauges
- [ ] OpenTelemetry tracing (§32.4)
- [ ] `gateway doctor` diagnostics
- [ ] `gateway status --verbose`
- [ ] `gateway inspect request <id>`
- [ ] `gateway inspect routes`
- [ ] `gateway inspect runtime`

### Resource Limits
- [ ] Request header limits
- [ ] Request body limits
- [ ] Multipart limits
- [ ] Concurrency limits (per-project, per-PHP, per-client)
- [ ] Queue limits with backpressure
- [x] Timeout configuration — `ServerConfig.WriteTimeout`, `PHPConfig.RequestTimeout`

### Rate Limiting
- [x] Token bucket per client — `RateLimiter` in `internal/policy/ratelimit.go`
- [x] Per route — `PerRouteLimiter.SetRoute()`
- [x] Global emergency limit — `PerRouteLimiter` with global bucket
- [x] Evict inactive entries, cap state — `RateLimiter.Cleanup()` removes 5min stale buckets

### Exit Criteria — Phase 3
- [ ] Load tests pass
- [x] Graceful reload works — atomic swap with drain
- [ ] Security suite passes
- [ ] Upgrade and rollback work

---

## Phase 4: Deployment Manager

> Goal: Immutable releases, zero-downtime switching, rollback, hooks.

### Immutable Releases
- [x] Release directory structure — `releases/archive/<id>/`, `releases/active` symlink
- [x] Atomic release activation — `os.Symlink` + `os.Rename` (atomic swap)
- [ ] Shared writable paths
- [x] Release metadata and state — `Release` struct with JSON persistence

### Zero-Downtime PHP Version Switching
- [x] Blue/green pool model — `Switcher.Deploy()` creates, probes, activates, drains
- [x] Install candidate → generate config → start → probe → activate → drain old
- [x] Atomic routing via snapshot swap — symlink swap in `ReleaseManager.Activate()`
- [x] Health checks: PHP version, extensions, FastCGI, application /health — `Prober.Probe()`
- [x] Rollback on failure — `Switcher.Rollback()`, `ReleaseManager.Rollback()`
- [ ] Canary switching (optional, advanced)

### Deploy Hooks
- [x] Pre/post activate hooks (argument arrays, not shell strings) — `HookRunner` with `HookConfig`
- [x] Timeout, working directory, controlled environment — per-hook timeout, WorkDir, Env
- [x] Output limit, audit log — `HookAuditLog` with 100-entry ring buffer, stdout/stderr capture
- [x] Allowed executable policy — shell metacharacter rejection (`;|&$\``)
- [x] Never run hooks from untrusted HTTP requests — hooks only run via `Switcher.Deploy()`

### Deploy CLI
- [ ] `gateway deploy create`
- [ ] `gateway deploy activate <release>`
- [ ] `gateway deploy rollback`
- [ ] `gateway deploy list`

### Exit Criteria — Phase 4
- [x] Crash recovery at every activation step — atomic symlink swap, fail-safe on each step
- [x] No broken active release after interrupted operation — atomic rename, state persistence

---

## Phase 5: Advanced Security

> Goal: WAF, OS isolation, network policy, incident snapshots.

### WAF / Policy Engine
- [ ] Policy engine interface and phases (§26.2)
- [ ] Method allow/deny rules
- [ ] Path rules, header rules, query rules
- [ ] Body size and content-type rules
- [ ] IP/network rules
- [ ] Protected file rules
- [ ] Per-rule observe/block mode
- [ ] Exclusions by project and route
- [ ] Rule safety: bounded execution, compiled regex, body inspection cap

### OS-Level Isolation
- [ ] Tier definitions (dev, single-user, multi-project, multi-tenant)
- [ ] Linux controls: cgroup v2, namespaces, seccomp, AppArmor/SELinux
- [ ] Resource config: memory, CPU, PIDs, open files
- [ ] Filesystem: release read-only, writable paths
- [ ] Build behind capability detection with clear diagnostics

### Network Policy
- [ ] Outbound network allow-list
- [ ] Deny private ranges
- [ ] DNS rebinding protection

### Incident Snapshot
- [ ] `gateway incident capture`
- [ ] Redacted bundle: config, build, runtime, errors, pool state, resources, routes, health
- [ ] No secret values

### Exit Criteria — Phase 5
- [ ] Threat model reviewed
- [ ] External security audit before strong multi-tenant claims

---

## Phase 6: Differentiators

> Goal: Request explorer, compatibility doctor, shadow testing.

### Request Decision Explorer
- [ ] `gateway explain-request --host --method --path`
- [ ] Show: host match, path normalization, security rules, route match, file/script selection, runtime, release

### Compatibility Doctor
- [ ] `gateway compatibility scan .`
- [ ] Detect: .htaccess, framework, public dir, PHP version, extensions, writable dirs, risky files, unsupported rewrites

### Route Contract Tests
- [ ] User-defined expected routing behavior in config
- [ ] `gateway test routes` command

### Shadow Runtime Testing
- [ ] Duplicate safe requests to candidate runtime
- [ ] Compare status, headers, body hash, execution time, errors
- [ ] Never duplicate state-changing requests

### Apache Compatibility Translator
- [ ] Support documented subset: RewriteEngine, RewriteCond, RewriteRule, flags L/END/R/QSA/NC
- [ ] Unsupported directives generate warnings with suggestions
- [ ] Fuzz test: `FuzzHtaccessTranslator`

### Exit Criteria — Phase 6
- [ ] Request explorer provides accurate explanations
- [ ] Compatibility doctor generates useful reports

---

## Cross-Cutting Concerns

### Testing Infrastructure
- [ ] Test fixtures directory populated
- [ ] Integration test framework with real PHP-FPM
- [ ] Security test suite
- [ ] Load/benchmark test suite
- [ ] Chaos test framework
- [ ] Cross-platform CI (Linux, macOS, Windows)
- [ ] Soak test setup (24-72h)
- [ ] Fuzz regression corpus persisted

### Documentation
- [ ] README and quick start
- [ ] Installation guide
- [ ] Local development guide
- [ ] Production deployment guide
- [ ] Configuration reference
- [ ] CLI reference
- [ ] Admin API reference
- [ ] PHP runtime and extension guide
- [ ] Security model
- [ ] Isolation tiers
- [ ] Apache migration guide
- [ ] `.htaccess` compatibility reference
- [ ] Framework guides (WordPress, Laravel, Symfony)
- [ ] Performance tuning
- [ ] Troubleshooting
- [ ] Operations runbook
- [ ] Plugin SDK
- [ ] Contributing guide
- [ ] Architecture decision records (docs/adr/)
- [ ] Vulnerability reporting policy

### Packaging
- [ ] Standalone Go binary
- [ ] Debian/RPM packages
- [ ] Homebrew formula
- [ ] Windows installer
- [ ] Container image
- [ ] Checksums and signatures
- [ ] SBOM and provenance

### CI/CD
- [ ] PR pipeline: format → lint → vet → static analysis → unit tests → race tests → fuzz smoke → integration → build → config validation → docs check
- [ ] Scheduled pipeline: long fuzzing, security regression, framework compat, load tests, dependency scan
- [ ] Release pipeline: reproducible build, SBOM, checksums, signed artifacts, upgrade tests, rollback tests

---

## Definition of Done Checklist (per feature)

Before marking any feature complete, verify:

- [ ] Requirements documented
- [ ] Threats and misuse cases considered
- [ ] Config schema defined
- [ ] CLI/API behavior defined
- [ ] Error codes defined
- [ ] Unit tests exist
- [ ] Integration tests exist (where applicable)
- [ ] Observability exists (logs, metrics)
- [ ] Documentation and examples exist
- [ ] Upgrade behavior considered
- [ ] Cross-platform behavior documented
- [ ] Failure and rollback behavior tested
- [ ] Resource bounds defined
- [ ] No secrets leaked
- [ ] Benchmarks exist for hot-path features
- [ ] Compatibility impact recorded

---

## Progress Summary

| Phase | Status | Notes |
|-------|--------|-------|
| Phase 0: Architecture Prototypes | In progress | Core packages built & tested (64 tests, 5 fuzz targets, race-clean). FastCGI client not wired to real FPM yet. |
| Phase 1: Local Dev Server | Not started | |
| Phase 2: Runtime Manager | Not started | |
| Phase 3: Production Server | Not started | |
| Phase 4: Deployment Manager | Not started | |
| Phase 5: Advanced Security | Not started | |
| Phase 6: Differentiators | Not started | |

### Phase 0 Summary (2026-07-26)

**Built & passing tests:**
- `internal/errors/` — 5 tests (error codes, wrapping, IsCode)
- `internal/filesystem/` — 30 tests + 1 fuzz (path parser + resolver)
- `internal/php/fastcgi/` — 8 tests + 3 fuzz (protocol encode/decode)
- `internal/php/cgi/` — 18 tests + 1 fuzz (response parser + params)
- `internal/supervisor/` — FPM lifecycle management (no tests yet)
- `internal/buildinfo/` — build metadata
- `cmd/gateway/` — serve command with static + PHP routing

**What works:**
- Path parsing with all security checks (NUL, control chars, backslash, encoded slash, double-encoding, dot collapse)
- Filesystem resolution with traversal protection, symlink policies, protected file patterns
- FastCGI record encode/decode with full round-trip verification
- CGI response parsing with header injection prevention
- CGI variable mapping for all standard environment variables
- HTTP server with timeouts, graceful shutdown, request ID logging
- FPM supervisor with config generation and socket wait

**Still needed for Phase 0 completion:**
- Wire FastCGI `Execute()` to real FPM socket (currently a stub)
- Verify plain PHP request end-to-end with real php-fpm
- Verify cancellation with real FPM
- `.golangci.yml` setup
- CI pipeline

---

## Notes

- Start with the First Vertical Slice (§60): `gateway serve ./example --php-fpm /usr/sbin/php-fpm`
- Build path handling and FastCGI correctness first.
- Add runtime distribution only after local FPM supervision is reliable.
- Add production features only after config reload and failure recovery are proven.
- Add WAF/plugins only after core can be tested and observed clearly.
