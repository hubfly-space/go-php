# Go-PHP Gateway — Task Tracker

> Track what's done, what's in progress, and what's next.
> Mark items with `[x]` when complete, `[ ]` when pending, `[-]` when skipped/blocked.

---

## Phase 0: Architecture Prototypes

> Goal: Prove the core architecture works. Plain PHP request works, cancellation works, path traversal tests pass.

### Project Setup
- [x] Initialize Go module (`go mod init`) — `github.com/go-php/gateway`
- [x] Create directory structure per spec §7 — all `internal/`, `cmd/`, `pkg/`, `test/`, `docs/` dirs created
- [ ] Set up `.golangci.yml`
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
- [ ] Plain PHP request works end-to-end — **blocked**: FastCGI client `Execute()` not yet wired to real FPM socket (stub returns error). All protocol building blocks are tested and working.
- [ ] Cancellation works (client disconnect kills PHP) — context cancellation wired, needs real FPM to verify
- [x] Path traversal tests pass — 10 resolver tests + 20 parser tests + fuzz all green
- [ ] FPM process can start, health-check, and stop — supervisor starts, needs real php-fpm binary to verify
- [ ] Technical risks documented — deferred to Phase 1

---

## Phase 1: Local Development Server

> Goal: `gateway serve` works for local dev with static + PHP + framework detection.

### Serve Command
- [ ] `gateway serve [path]` with auto-detection
- [ ] Detect `public/` directory for known frameworks
- [ ] Detect PHP files or known framework
- [ ] Find compatible local PHP runtime
- [ ] Offer to install runtime when permitted
- [ ] Start private PHP worker pool
- [ ] Print local URL and diagnostics

### Static File Server
- [ ] Directory index files
- [ ] MIME type detection
- [ ] ETag and Last-Modified
- [ ] Conditional requests (If-None-Match, If-Modified-Since)
- [ ] Byte ranges
- [ ] HEAD support
- [ ] Precompressed `.br` and `.gz` variants
- [ ] Cache-control rules
- [ ] No full file buffering
- [ ] Unit tests: range requests, conditional requests, HEAD, MIME types, dotfile denial, cache-control, directory index

### Routing
- [ ] Route types: static, php_front_controller, fixed, reverse_proxy, redirect
- [ ] Route matchers: exact, prefix, glob, method, host
- [ ] Route ordering: exact before wildcard
- [ ] Rewrite rules with max iterations and cycle detection
- [ ] Unit tests for all matcher types

### Front Controller
- [ ] Route to `/index.php` for unmatched paths
- [ ] PATH_INFO behavior
- [ ] Preserve original URI

### Config
- [ ] Minimal `gateway.yaml` schema
- [ ] Parse and validate config
- [ ] Generate defaults
- [ ] Config init command
- [ ] Config validate command

### Development Error Pages
- [ ] Detailed error pages in development mode
- [ ] Request ID visible
- [ ] PHP errors pass through

### Framework Detection
- [ ] Laravel detection (public/index.php, artisan)
- [ ] Symfony detection (public/index.php, bin/console)
- [ ] WordPress detection (wp-config.php, wp-login.php)
- [ ] Plain PHP detection

### Observability (Minimal)
- [ ] Structured access logs (JSON)
- [ ] Request ID in logs
- [ ] Duration, status, bytes in/out
- [ ] Redact secrets from logs

### Exit Criteria — Phase 1
- [ ] Plain PHP, WordPress, and Laravel smoke tests pass
- [ ] No known path escape
- [ ] Stable under basic concurrency

---

## Phase 2: Runtime Manager

> Goal: Per-project PHP versions, extensions, lock file, reproducible runtimes.

### Runtime Identity
- [ ] Runtime ID format: `php:<version>:<platform>:<arch>:<build-flavor>:<extension-set-hash>`
- [ ] Runtime directory structure (`~/.gateway/runtimes/`)
- [ ] Runtime manifest schema and parser

### Runtime Install/List/Use/Remove
- [ ] `gateway php install <version>`
- [ ] `gateway php list`
- [ ] `gateway php use <version>`
- [ ] `gateway php remove <version>`
- [ ] Fetch signed index
- [ ] Download to temporary location
- [ ] Verify size and checksum
- [ ] Verify artifact signature
- [ ] Safe archive extraction (reject traversal, symlinks, hard links, bombs)
- [ ] Atomic move into runtime registry
- [ ] Unit tests for archive extraction safety

### Extension Manager
- [ ] Extension artifact model
- [ ] Resolve compatible artifact by PHP version, platform, arch, thread safety
- [ ] Verify signature/checksum
- [ ] Install immutably
- [ ] Generate `conf.d` files
- [ ] Validate with `php -m`
- [ ] Extension profiles: minimal, web-standard, wordpress, laravel, development, custom
- [ ] Unit tests for extension compatibility resolution

### Version Selection Policies
- [ ] exact, patch, minor, locked
- [ ] Default to locked in production

### Lock File
- [ ] Generate `gateway.lock`
- [ ] Record PHP version, runtime_id, manifest_digest, extensions
- [ ] Use lock file for reproducible deploys

### FPM Pool Generator
- [ ] Generate FPM config from validated configuration
- [ ] Safe escaping of all values
- [ ] Unix socket path: `/run/user/<uid>/gateway/<project>/<runtime>.sock`
- [ ] Restrictive permissions (0600)

### PHP Config Layering
- [ ] Runtime defaults → safe baseline → env preset → project config → route overrides
- [ ] Classify directives: safe, warning, restricted, gateway-owned
- [ ] OPcache settings for dev and production
- [ ] Unit tests for config layering

### Exit Criteria — Phase 2
- [ ] Reproducible runtime selection
- [ ] Invalid artifacts rejected
- [ ] Version switch integration tests pass

---

## Phase 3: Production Server

> Goal: Multi-site, HTTPS, admin API, metrics, graceful reload.

### Multiple Sites
- [ ] Multi-project configuration
- [ ] Host-based routing
- [ ] SNI-based TLS routing
- [ ] Separate PHP pools per project

### HTTPS / TLS
- [ ] Static certificate files
- [ ] Automatic ACME (Let's Encrypt)
- [ ] Atomic certificate renewal
- [ ] Secure private key permissions
- [ ] SNI routing
- [ ] HTTP-to-HTTPS redirect
- [ ] Unit tests: SNI selection, unknown host, expired cert, renewal, corrupted state

### Admin API
- [ ] Bind to `127.0.0.1` only by default
- [ ] Local token authentication (constant-time comparison)
- [ ] CSRF protection
- [ ] Rate limit authentication attempts
- [ ] Endpoints: status, projects, config validate/activate, runtimes, metrics
- [ ] Long-running operation IDs
- [ ] Audit log all operations

### Service Mode
- [ ] `gateway start`, `gateway stop`, `gateway reload`, `gateway status`
- [ ] Systemd service generation (`gateway service install`)
- [ ] Graceful shutdown sequence (§39.3)

### Configuration Reload
- [ ] Validate new config before activation
- [ ] Build candidate snapshot
- [ ] Atomic pointer swap
- [ ] Drain old state after swap
- [ ] Never replace known-good state with unvalidated config

### Observability (Production)
- [ ] Structured access logs with all fields (§32.1)
- [ ] Prometheus metrics (§32.3)
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
- [ ] Timeout configuration

### Rate Limiting
- [ ] Token bucket per client
- [ ] Per route
- [ ] Global emergency limit
- [ ] Evict inactive entries, cap state

### Exit Criteria — Phase 3
- [ ] Load tests pass
- [ ] Graceful reload works
- [ ] Security suite passes
- [ ] Upgrade and rollback work

---

## Phase 4: Deployment Manager

> Goal: Immutable releases, zero-downtime switching, rollback, hooks.

### Immutable Releases
- [ ] Release directory structure
- [ ] Atomic release activation
- [ ] Shared writable paths
- [ ] Release metadata and state

### Zero-Downtime PHP Version Switching
- [ ] Blue/green pool model
- [ ] Install candidate → generate config → start → probe → activate → drain old
- [ ] Atomic routing via snapshot swap
- [ ] Health checks: PHP version, extensions, FastCGI, application /health
- [ ] Rollback on failure
- [ ] Canary switching (optional, advanced)

### Deploy Hooks
- [ ] Pre/post activate hooks (argument arrays, not shell strings)
- [ ] Timeout, working directory, controlled environment
- [ ] Output limit, audit log
- [ ] Allowed executable policy
- [ ] Never run hooks from untrusted HTTP requests

### Deploy CLI
- [ ] `gateway deploy create`
- [ ] `gateway deploy activate <release>`
- [ ] `gateway deploy rollback`
- [ ] `gateway deploy list`

### Exit Criteria — Phase 4
- [ ] Crash recovery at every activation step
- [ ] No broken active release after interrupted operation

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
