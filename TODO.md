# Go-PHP Gateway — Task Tracker

> Track what's done, what's in progress, and what's next.
> Mark items with `[x]` when complete, `[ ]` when pending, `[-]` when skipped/blocked.

> **A note on what `[x]` means here.** A checked box in this file means the code was *written and
> tested*, not that it is reachable from the running binary. Those are currently different things:
> roughly a third of `internal/` is complete, well covered, and never constructed by `main`. See
> [ROADMAP.md](ROADMAP.md) for the full list. Where a checked item is written-but-unwired, it is
> annotated `[unwired]` below.

> **Untracked work.** Several subsystems the spec requires have no entry anywhere in this file —
> trusted proxies (§10.3), request framing hardening (§10.4), the upload pipeline (§25), the state
> store (§38), the reverse proxy and WebSockets (§31), the Windows backend (§17), per-route PHP
> runtimes (§33.9), and safe deployment replay (§33.8). They are catalogued in
> [ROADMAP.md](ROADMAP.md) rather than duplicated here.

---

## Phase 0: Architecture Prototypes

> Goal: Prove the core architecture works. Plain PHP request works, cancellation works, path traversal tests pass.

### Project Setup
- [x] Initialize Go module (`go mod init`) — `github.com/go-php/gateway`
- [x] Create directory structure per spec §7 — all `internal/`, `cmd/`, `pkg/`, `test/`, `docs/` dirs created
- [x] Set up `.golangci.yml` — errcheck, govet, staticcheck, unused, gosimple, ineffassign, misspell, unconvert, gocritic, gofmt, goimports
- [x] Set up `Makefile` or task runner — build, install, run, cross-compile, 7 test targets, fmt/vet/lint/coverage
- [x] Set up CI pipeline (GitHub Actions) — `ci.yml`, `e2e.yml`, `fuzz.yml`, `tests.yml`
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
- [x] Cache-control rules — `internal/filesystem/cache.go` (CacheControlledFileServer, CachePolicy, ETag, immutable/no-cache paths)
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
- [x] PATH_INFO behavior — `internal/php/cgi/params.go` maps PATH_INFO and PATH_TRANSLATED for front-controller patterns
- [x] Preserve original URI

### Config
- [x] Minimal `gateway.yaml` schema — `internal/config/config.go` with ServerConfig, PHPConfig, RouteConfig, LoggingConfig, SecurityConfig
- [x] Parse and validate config — `Load()` with YAML, `Validate()` with semantic checks
- [x] Generate defaults — `DefaultConfig()` with sensible defaults
- [x] Config init command — gateway config init and gateway init CLI commands
- [x] Config validate command — gateway config validate command wired to CLI

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
- [x] Redact secrets from logs — `internal/observability/redact.go` (SecretRedactor, RedactString, hmacSign, SecurityMiddleware)

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
- [x] Fetch signed index — `internal/runtime/signed.go` (SignedIndex, IndexFetcher, Ed25519 verification, freshness check)
- [x] Download to temporary location — `copyDir()` helper
- [x] Verify size and checksum — manifest SHA256
- [x] Verify artifact signature — `internal/runtime/signed.go` (ArtifactVerifier, FileSHA256, ComputeChecksum, VerifyDir)
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
> `internal/tls` is now wired: `tls.mode: files` starts an HTTPS listener with SNI, and Go
> negotiates HTTP/2 over it. `tls.mode: acme` is rejected at config load until real ACME lands.
- [x] Static certificate files — `CertManager.LoadCert()`, `LoadCertDir()`, wired via `tls.mode: files`
- [ ] Automatic ACME (Let's Encrypt) — **`acme.go:48` never contacts an ACME server**; it calls `generateSelfSigned` and returns that under a Let's Encrypt-shaped API. Needs a real implementation (e.g. `x/crypto/acme/autocert`) before the mode is offered
- [ ] Fix `acme.go:235` — slices `r.URL.Path[len(prefix):]` with no prefix check; a short request panics the server
- [ ] Atomic certificate renewal — plus §30.2 renewal jitter, persistent cert state, rate-limit-aware retry, concurrent renewal
- [x] Secure private key permissions — loaded via `tls.LoadX509KeyPair`
- [x] SNI routing — `CertManager.GetCertificate()` with wildcard matching
- [x] HTTP-to-HTTPS redirect — `RedirectHandler()` with 301, via `tls.redirect_from`
- [x] Unit tests: SNI selection, wildcard match, default cert, load dir, redirect handler

### Admin API
> `internal/admin.Guard` now protects the `internal/ui` mux: bearer token, origin validation, rate
> limiting, audit log, and security headers. A token is generated at startup and printed to stderr.
- [x] Bind to `127.0.0.1` only by default — plus a warning when `--ui-addr` is not loopback
- [x] Local token authentication (constant-time comparison) — `crypto/subtle.ConstantTimeCompare`
- [x] Make auth fail-closed — an unset token now denies rather than allowing
- [x] CSRF protection — hand-rolled SHA-256/HMAC replaced with `crypto/hmac` + `crypto/sha256`, and the MAC is now actually verified; browser flow uses an `HttpOnly`, `SameSite=Strict` cookie plus origin validation
- [x] Rate limit authentication attempts — per-IP token bucket
- [x] Endpoints: status, config validate, runtimes, metrics, audit, health — note `handleConfigValidate` is still a stub returning `{"status":"valid"}`
- [ ] Long-running operation IDs — plus §36.4 SSE progress streaming
- [ ] §36.3 separate read and write scopes, token rotation, short-lived operation tokens — one token currently grants full control
- [x] Audit log all operations — `AuditLog` with timestamp, action, remote, path

### Service Mode
- [ ] `gateway start`, `gateway stop`, `gateway reload`, `gateway status`
- [ ] Systemd service generation (`gateway service install`)
- [x] Graceful shutdown sequence — wired in `cmd/gateway/main.go` via SIGINT/SIGTERM

### Configuration Reload
> Wired: SIGHUP reloads, validates, and atomically swaps. The handler reads an immutable
> `serveState` snapshot once per request, so a reload cannot change the rules mid-request.
- [x] Add a SIGHUP handler and read config through the snapshot rather than a captured pointer
- [x] Validate new config before activation — `Reloader.Reload()` calls `Validate()` `[unwired]`
- [x] Build candidate snapshot — `Snapshot` struct with version and timestamp `[unwired]`
- [x] Atomic pointer swap — `atomic.Pointer[Snapshot]` in `internal/config/reload.go` `[unwired]`
- [x] Drain old state after swap — `Drainable` interface, `ReloadWithDrain()` `[unwired]`
- [x] Never replace known-good state with unvalidated config — validate-then-swap `[unwired]`

### Observability (Production)
- [x] Structured access logs with all fields — `internal/observability/access.go` (the only observability file that is wired)
- [x] Wire secret redaction — installed as the default slog handler before anything logs, driven by `observability.redact_keys`
- [x] Prometheus metrics (§32.3) — served at `observability.metrics.path` on the management listener only (§5.5)
- [x] Fix metrics before exposing — series are keyed on the matched route pattern with a hard cap and an overflow bucket, label values are escaped, and HELP/TYPE are emitted once per family
- [x] Request tracing (§32.4) — behind `observability.tracing.enabled`
- [x] Schedule `Tracer.Cleanup` before wiring — `Tracer.StartCleanup` runs on a ticker bounded by `observability.tracing.retention`
- [ ] §32.4 propagate trace context into PHP via controlled env vars/headers
- [x] `gateway doctor` diagnostics — `internal/diagnostics/doctor.go` (binary checks, port availability, PID limit, open files, disk space)
- [ ] `gateway status --verbose`
- [ ] `gateway inspect request <id>`
- [ ] `gateway inspect routes`
- [ ] `gateway inspect runtime`

### Resource Limits
- [ ] Request header limits
- [x] Request body limits — `security.max_body_size` is parsed at config load by `ParseByteSize` and enforced in the request path, returning 413
- [ ] Multipart limits — and the whole of §25 upload security
- [ ] Concurrency limits (per-project, per-PHP, per-client)
- [ ] Queue limits with backpressure — §24.2 bounded queue, 503/429 with `Retry-After`, health-route prioritization
- [x] Timeout configuration — `php.request_timeout` now bounds the Go-side PHP request as well as the generated FPM conf

### Rate Limiting
> Wired behind `security.rate_limit`. Two bypasses were fixed on the way in: the limiter keyed on
> the client-controlled `X-Forwarded-For` header, and otherwise on `RemoteAddr` including the port,
> so every new connection got a fresh bucket.
- [x] Wire `PerRouteLimiter.Middleware` into the request chain and schedule `Cleanup` on a ticker
- [x] Token bucket per client — `RateLimiter` in `internal/policy/ratelimit.go` `[unwired]`
- [x] Per route — `PerRouteLimiter.SetRoute()` `[unwired]`
- [x] Global emergency limit — `PerRouteLimiter` with global bucket `[unwired]`
- [x] Evict inactive entries, cap state — `RateLimiter.Cleanup()` removes 5min stale buckets `[unwired]`

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
- [x] Shared writable paths — `internal/filesystem/writable.go` (WritablePaths, DefaultWritablePaths, Ensure, Validate)
- [x] Release metadata and state — `Release` struct with JSON persistence

### Zero-Downtime PHP Version Switching
- [x] Blue/green pool model — `Switcher.Deploy()` creates, probes, activates, drains
- [x] Install candidate → generate config → start → probe → activate → drain old
- [x] Atomic routing via snapshot swap — symlink swap in `ReleaseManager.Activate()`
- [~] Health checks — `Prober.Probe` no longer returns `true` unconditionally: it verifies the release directory and entrypoint and can request a health URL. Full candidate verification (start a candidate pool, check PHP version and extensions, warm-up URLs) still needs the supervisor
- [x] Rollback on failure — `Switcher.Rollback()`, `ReleaseManager.Rollback()` `[unwired]`
- [ ] Fix `ReleaseManager.Rollback` locking (`release.go:221-225` unlocks mid-function under a deferred unlock)
- [x] Canary switching (optional, advanced) — `internal/deploy/canary.go` `[unwired]`
- [ ] Fix canary weighting — `randomInt` is `time.Now().UnixNano() % 100` (`canary.go:161`), a clock LSB rather than a uniform draw, so bursty traffic routes in correlated blocks. §21.5 also wants stable request hashing

### Deploy Hooks
- [x] Pre/post activate hooks (argument arrays, not shell strings) — `HookRunner` with `HookConfig`
- [x] Timeout, working directory, controlled environment — per-hook timeout, WorkDir, Env
- [x] Output limit, audit log — `HookAuditLog` with 100-entry ring buffer, stdout/stderr capture
- [x] Allowed executable policy — shell metacharacter rejection (`;|&$\``)
- [x] Never run hooks from untrusted HTTP requests — hooks only run via `Switcher.Deploy()`

### Deploy CLI
> The CLI reaches `ReleaseManager` only. `Switcher`, `CanarySwitcher`, `DeployCLI`, and the entire
> lock-file API are unreferenced, and both the CLI and UI hardcode `./releases` and `"php8.3"`.
- [x] `gateway deploy create` — `DeployCLI.Deploy()` with full zero-downtime cycle `[unwired]`
- [x] `gateway deploy activate <release>` — `Switcher.Deploy()` with pre/post hooks `[unwired]`
- [x] `gateway deploy rollback` — `DeployCLI.Rollback()` + `CanarySwitcher.Rollback()` `[unwired]`
- [x] `gateway deploy list` — `DeployCLI.Status()` with SwitcherStatus `[unwired]`
- [ ] Stop hardcoding the releases directory and runtime ID (`commands.go:214`, `handlers.go:831`)

### Exit Criteria — Phase 4
- [x] Crash recovery at every activation step — atomic symlink swap, fail-safe on each step
- [x] No broken active release after interrupted operation — atomic rename, state persistence

---

## Phase 5: Advanced Security

> Goal: WAF, OS isolation, network policy, incident snapshots.

### WAF / Policy Engine
> Wired behind `security.mode`. Denials name the rule that produced them (§23.3), and observe mode
> logs without blocking. Structural protections are unaffected by the mode, per §23.4.
- [x] Wire `Engine.Middleware` and add a `security.mode` config key (off | observe | balanced | strict)
- [ ] Fix `CondQueryParam` — `engine.go:237` does `strings.Contains` on the raw query rather than matching a parsed param; `CondBodySize` (`:246`) ignores `Sscanf` errors; `matchesCondition` (`:203`) recompiles regexes per evaluation
- [x] Policy engine interface and phases (§26.2) — `internal/policy/engine.go` with `Phase`, `Decision`, `Engine` `[unwired]`
- [x] Method allow/deny rules — `CondMethod` condition type
- [x] Path rules, header rules, query rules — `CondPath`, `CondPathPrefix`, `CondPathRegex`, `CondHeader`, `CondQueryParam`
- [x] Body size and content-type rules — `CondBodySize` condition type
- [x] IP/network rules — `CondIP`, `CondIPRange` with CIDR support
- [x] Protected file rules — via path regex conditions
- [x] Per-rule observe/block mode — `DecisionObserve` logs but doesn't block
- [x] Exclusions by project and route — `Exclusion` struct per rule
- [x] Rule safety: bounded execution, compiled regex, body inspection cap
- [x] HTTP middleware — `Engine.Middleware()` with 403 response for deny, X-Policy-Observed header for observe
- [x] Unit tests (15 tests): allow, deny, observe, path regex, host match, exclusion, negation, IP range, body size, priority, clear, middleware, scheme match, rules copy

### OS-Level Isolation
> Wired via `php.isolation`, defaulting to Tier 0 (none). §28.1 still forbids claiming safe
> untrusted multi-tenancy at these tiers, and the README says so.
- [x] Call `Isolator.ApplyIsolation(cmd)` before starting php-fpm, driven by a `php.isolation` config block defaulting to Tier 0
- [x] Tier definitions (dev, single-user, multi-project, multi-tenant) — `IsolationConfig` with mode: none, process, namespace, cgroup `[unwired]`
- [x] Linux controls: cgroup v2, namespaces, seccomp, AppArmor/SELinux — `internal/supervisor/isolation.go` `[unwired]`
- [x] Resource config: memory, CPU, PIDs, open files — MemoryLimit, CPULimit, PIDLimit in IsolationConfig
- [x] Filesystem: release read-only, writable paths — via WritablePaths + isolation environment
- [x] Build behind capability detection with clear diagnostics — runtime.GOOS checks, cleanup methods

### Network Policy
- [x] Outbound network allow-list — `NetworkPolicy.SetAllowList()`
- [x] Deny private ranges — `NetworkPolicy.SetDenyPrivate()` with 10.x, 172.16.x, 192.168.x, loopback, link-local, IPv6 private
- [x] DNS rebinding protection — `DNSRebindingDetector` with IP change detection
- [x] Denied CIDR ranges — `NetworkPolicy.AddDeniedRange()`
- [x] Unit tests (8 tests): default deny private, allow private, allowlist, deny range, invalid range, isPrivateIP, DNS rebinding, allowed host

### Incident Snapshot
- [x] `gateway incident capture` — `diagnostics.Capture()` with reason
- [x] Redacted bundle: config, build, runtime, errors, pool state, resources, routes, health — `Snapshot` struct
- [x] No secret values — `redactConfig()` redacts token, secret, password, api_key, csrf_secret recursively
- [x] Save/load/list snapshots — `Save()`, `LoadSnapshot()`, `ListSnapshots()`
- [x] Unit tests (8 tests): capture, add error, set health, set config redaction, save/load, list, nonexistent, invalid JSON, deep nested redaction

### Exit Criteria — Phase 5
- [x] Threat model reviewed — policy engine covers methods, paths, headers, query, body size, IP, scheme
- [ ] External security audit before strong multi-tenant claims

---

## Phase 6: Differentiators

> Goal: Request explorer, compatibility doctor, shadow testing.

### Request Decision Explorer
- [x] `internal/diagnostics/explain.go` — `RequestExplainer.Explain()` traces request through full pipeline
- [x] Shows: path normalization, policy decision, route match, file resolution, script resolution
- [x] Structured JSON output with summary and duration
- [x] Give `explain` a real router and policy engine — it now loads the config, builds the same router and mode-appropriate engine `serve` would, and accepts `--config`, `--method`, `--host`, and `--root`

### Compatibility Doctor
- [x] `internal/diagnostics/compat.go` — `CompatDoctor.Scan()` full project compatibility scan
- [x] Detect: framework (Laravel, WordPress, Symfony, Composer), .htaccess directives, public dir, risky files (.git, .env, .sql, .log), deprecated PHP functions, config files
- [ ] Fix `checkRiskyFiles` — `compat.go:262` uses `filepath.Glob("**/pattern")`, which does not recurse in Go, so the check silently never fires. Also `checkPHPFiles` reads every `.php` file into memory unbounded, and `ScannedAt` is the literal string `"now"` (`compat.go:45`)
- [x] Score calculation (0-100)
- [x] Suggestions for migration and fixes

### Route Contract Tests
> All three now have CLI surfaces. `gateway test routes` exits non-zero on failure so it is usable
> in CI.
- [x] Add `gateway test routes`, `gateway shadow`, and `gateway migrate htaccess`
- [x] `internal/diagnostics/contract.go` — `ContractTestSuite` with declarative test definitions `[unwired]`
- [x] Match expectations: route target, redirect URL, denied
- [x] Host, method, and header matching
- [x] `GenerateStandardTests()` for common patterns

### Shadow Runtime Testing
- [x] Duplicate safe requests to candidate runtime — `internal/diagnostics/shadow.go` (ShadowTester, Compare)
- [x] Compare status, headers, body hash, execution time, errors — StatusMatch, BodyMatch, ShadowSummary
- [x] Never duplicate state-changing requests — X-Shadow-Request header, GET-only by default

### Apache Compatibility Translator
- [x] Support documented subset: RewriteEngine, RewriteCond, RewriteRule, flags L/END/R/QSA/NC — `internal/diagnostics/htaccess.go` (HtaccessTranslator, Translate)
- [x] Unsupported directives generate warnings with suggestions — RewriteCond warnings, unknown directives flagged
- [ ] Fuzz test: `FuzzHtaccessTranslator`

### Exit Criteria — Phase 6
- [x] Request explorer provides accurate explanations
- [x] Compatibility doctor generates useful reports

---

## Cross-Cutting Concerns

### Testing Infrastructure
- [ ] Test fixtures directory populated — `test/compatibility/`, `test/fixtures/frameworks/`, `test/fixtures/security/` are empty dirs
- [x] Integration test framework with real PHP-FPM — `test/integration/` and `test/e2e/` (1473 lines, 24 tests), both build-tag gated
- [x] Security test suite — `test/security/`, but see below: three of its tests assert nothing
- [x] Load/benchmark test suite — `test/load/`, 6 benchmarks
- [x] Chaos test framework — `test/chaos/`, 9 tests
- [ ] Cross-platform CI (Linux, macOS, Windows) — every workflow is `ubuntu-latest`; the darwin cross-compile targets are never verified
- [ ] Soak test setup (24-72h)
- [ ] Fuzz regression corpus persisted — `testdata/fuzz` absent
- [ ] Fix `make test-fuzz` — it invokes `FuzzHtaccessTranslator`, which does not exist, so the target always fails
- [ ] Replace the three no-op tests in `test/security/security_test.go` (`:172` re-implements auth inline instead of calling `admin.Server`; `:215` is a `t.Log`; `:223` asserts `"POST" != "GET"`)
- [ ] Tests for `cmd/gateway` — 677 lines holding the entire request path, script resolution, MIME/ETag, and CLI dispatch, with 0% coverage
- [ ] Delete or use `test/wordpress/` — 94 MB of a WordPress checkout that no Go test references
- [ ] Remove committed `coverage.out` (129 KB) from the repo

### Documentation
- [x] README and quick start — `README.md`, `docs/QUICK_START.md`
- [ ] Installation guide
- [ ] Local development guide
- [x] Production deployment guide — `docs/DEPLOYMENT.md`, but it documents `X-Forwarded-For` handling that does not exist (§10.3 untracked)
- [x] Configuration reference — annotated `gateway.yaml`; note `examples/gateway.yaml` diverges from the loader and would fail to parse
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
- [ ] Architecture decision records (docs/adr/) — directory exists and is empty; §58 pre-names ADR-0001…0012, and `AGENTS.md` mandates an ADR per new package
- [ ] Vulnerability reporting policy
- [ ] ADR for the dashboard (in neither spec nor this file; adds a Node toolchain against a single-binary packaging goal — §36.4 permits it but requires the core server not depend on it)
- [ ] ADR for `internal/runtime/provision.go` (auto-runs `sudo apt-get install` at startup; in tension with §5.3 immutable runtimes and §18.4 signed artifacts)
- [ ] §52.3 license and notice inventory for redistributed runtimes, libraries, extensions, CA assets, dashboard assets, and Go modules

### Packaging
> `packaging/` and `scripts/` exist and are empty. There is no Dockerfile anywhere in the repo, no
> release workflow, and no tag trigger. `build-all` covers linux/amd64 and darwin/amd64+arm64 — no
> linux/arm64, no Windows — and CI is `ubuntu-latest` only, so the darwin targets are never verified.
- [ ] Standalone Go binary
- [x] Fix `Makefile` ldflags — they now name `internal/buildinfo`, and `GoVersion` comes from `runtime.Version()`
- [ ] Add `govulncheck`, `gosec`/CodeQL, dependabot, a dashboard lint/build job, and a multi-PHP-version e2e matrix (currently pinned to 8.3)
- [ ] Clean stale bundles on `dashboard-embed` — `cp -r` never removes, so ~690 KB of orphaned JS/CSS is compiled into the binary
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

### Landed alongside the wiring work
- [x] Watchdog for php-fpm — health check on a ticker, exponential backoff with jitter, max restart rate, circuit-open state, stable 503 for PHP routes while open (§16.5)
- [x] Fix php-fpm being killed 10s after every start — `Supervisor.Start` tied the child's lifetime to the caller's startup-timeout context
- [x] Graceful php-fpm shutdown — SIGTERM first, SIGKILL only on timeout, so workers are not orphaned (§16.6)
- [x] Fix flag parsing — `gateway serve . --php-fpm <path>` silently ignored every flag after the positional argument
- [x] Static `Cache-Control` with immutable-asset paths, and precompressed `.br`/`.gz` variants resolved through the hardened resolver (§12.1)
- [x] Route `internal/ui/siteManager.go` through `filesystem.Resolver` — it had no protected-pattern check, so `.env` under a site webroot was served
- [x] Bound the rate limiter's client-key map (§24.3)
- [x] `ParseByteSize` for `security.max_body_size` and `php.isolation.memory_limit`

### Housekeeping
- [ ] Adopt or delete `internal/php/fpm` — 203 lines at 85.3% coverage, zero importers, and `supervisor.generateConfig` (`supervisor.go:255-350`) re-implements a worse version inline. Adopting is the recommendation
- [ ] `go mod tidy` — `gorilla/websocket` is marked `// indirect` but is a direct import at `internal/ui/handlers.go:19`
- [ ] Change `.gitignore` line 3 from a bare `gateway` to `/gateway` — as written it ignores the `cmd/gateway/` **source directory** for every ignore-aware tool, so ripgrep silently skips the entire request path and `git add .` misses new files there
- [ ] Use `internal/errors` in the request path — 13 stable codes at 100% coverage, referenced exactly once as linter appeasement (`var _ = errors.IsCode`, `main.go:677`). All error handling is `fmt.Errorf`; §37 treats the codes as a compatibility contract
- [ ] Fix httpoxy — `cgi/params.go:71-74` copies all client headers to `HTTP_*` including `Proxy`; `SERVER_SOFTWARE` is hardcoded `"go-php-gateway/1.0"`
- [ ] Compile route regexes once — `router/match.go:32-36` compiles at load and discards the result; `:85` and `:112` recompile per request, discarding errors
- [ ] Make the dashboard honest — `/api/metrics/history` (`handlers.go:884`) generates data arithmetically, `/api/config` (`:415`) returns hardcoded literals and never reads `gateway.yaml`, `/api/config/save` (`:476`) persists nothing, `StatusProvider` counters are never incremented, and `LogBuffer` is fed only by UI handlers so the Logs page never shows a request
- [ ] Reconcile `examples/gateway.yaml` with the loader — it uses `schema: "1.0"` and an integer `max_body_size` against a string field, so it would fail to parse
- [ ] Resolve the §15.4 vs §44 fuzzer naming inconsistency in the spec, and pick one canonical set

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

Phase status reflects **exit criteria met**, not code written. A phase whose code exists but is not
reachable from `main` is not complete — see the `[unwired]` annotations and [ROADMAP.md](ROADMAP.md).

| Phase | Status | Blocking |
|-------|--------|----------|
| Phase 0: Architecture Prototypes | Code complete, criteria unmet | Script resolution chain (§14.5) still partly hardcoded; technical risks never documented |
| Phase 1: Local Dev Server | Code complete, criteria unmet | Smoke tests for plain PHP / WordPress / Laravel not run; concurrency stability unverified |
| Phase 2: Runtime Manager | Partial | `gateway php install` is a stub; `runtime/signed.go` unwired, so nothing is ever downloaded or verified; archive-extraction safety tests missing |
| Phase 3: Production Server | Partial | **No TLS listener exists.** Metrics, tracing, redaction, config reload, admin API all unwired. Resource limits (§24) unimplemented. Load and security suites not passing as exit gates |
| Phase 4: Deployment Manager | Partial | `Switcher`/`CanarySwitcher` unwired; `Prober.Probe` is a no-op, so the health gate cannot fail |
| Phase 5: Advanced Security | Partial | Policy engine and rate limiter unwired; isolation code unreachable; external security audit not done |
| Phase 6: Differentiators | Partial | `explain` passes a nil router and nil policy engine; htaccess translator, contract tests, and shadow tester have no CLI surface; per-route runtime (§33.9) and deployment replay (§33.8) not started |

### Stranded deferrals

Four items were deferred to Phase 1, Phase 1 was marked complete, and none were done. Re-listed here
so they stop being invisible:

- [ ] Fuzz test: `FuzzPathResolverInputs`
- [ ] Connection-level deadlines via `ConnContext`
- [ ] Unit tests for FPM config generation
- [ ] Technical risks documented (§59 names seven)

---

## Notes

- Start with the First Vertical Slice (§60): `gateway serve ./example --php-fpm /usr/sbin/php-fpm`
- Build path handling and FastCGI correctness first.
- Add runtime distribution only after local FPM supervision is reliable.
- Add production features only after config reload and failure recovery are proven.
- Add WAF/plugins only after core can be tested and observed clearly.
