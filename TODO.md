# Go-PHP Gateway — Task Tracker

> Track what's done, what's in progress, and what's next.
> Mark items with `[x]` when complete, `[ ]` when pending, `[-]` when skipped/blocked.

---

## Phase 0: Architecture Prototypes

> Goal: Prove the core architecture works. Plain PHP request works, cancellation works, path traversal tests pass.

### Project Setup
- [ ] Initialize Go module (`go mod init`)
- [ ] Create directory structure per spec §7
- [ ] Set up `.golangci.yml`
- [ ] Set up `Makefile` or task runner
- [ ] Set up CI pipeline (GitHub Actions)
- [ ] Add build info package (`internal/buildinfo/`)

### Error Foundation
- [ ] Create `internal/errors/` with error code constants
- [ ] Define error wrapping helpers
- [ ] Add error classification (public vs internal)

### Path Parser (Security-Critical)
- [ ] Implement `internal/filesystem/` path parser
- [ ] Handle percent decoding (single pass only)
- [ ] Reject NUL bytes and control characters
- [ ] Reject backslashes by default
- [ ] Collapse dot segments
- [ ] Preserve multiple path representations (`ParsedPath` struct)
- [ ] Unit tests: normal paths, dot segments, repeated slashes, encoded dots/slashes, invalid percent encoding, backslashes, Unicode, NUL, control chars, empty path, absolute URI, long segments, trailing slash, Windows device names
- [ ] Property test: `normalize(normalize(path)) == normalize(path)`
- [ ] Property test: for every accepted path, resolved file is under allowed root
- [ ] Fuzz test: `FuzzPathParser`

### Filesystem Resolver
- [ ] Implement `internal/filesystem/resolver/`
- [ ] Pre-opened document root approach
- [ ] Symlink policy: deny, within-root, allow-listed
- [ ] Protected file deny patterns (§11.5)
- [ ] Verify regular file after open
- [ ] Deny devices, sockets, pipes
- [ ] Unit tests: file inside root, missing file, directory, symlink inside/outside root, protected file, special file, permission denied
- [ ] Fuzz test: `FuzzPathResolverInputs`

### FastCGI Client
- [ ] Implement `internal/php/fastcgi/` — protocol encoder/decoder
- [ ] 8-byte header encode/decode
- [ ] Name/value length encodings
- [ ] FCGI_BEGIN_REQUEST, FCGI_PARAMS, FCGI_STDIN records
- [ ] FCGI_STDOUT, FCGI_STDERR, FCGI_END_REQUEST parsing
- [ ] Empty PARAMS and STDIN terminators
- [ ] Enforce 16-bit content length
- [ ] Split streams into multiple records
- [ ] Validate request IDs
- [ ] Unit tests: header encode/decode, all length combinations, empty params, stream fragmentation, max record size, unexpected request IDs, truncated headers/records, invalid padding, STDERR interleaving, multiple STDOUT, empty STDOUT, END_REQUEST parsing
- [ ] Fuzz tests: `FuzzDecodeRecord`, `FuzzDecodeNameValue`

### CGI Response Parser
- [ ] Parse CGI-style headers from PHP FastCGI output
- [ ] Handle Status, Location, Content-Type, Set-Cookie (repeated)
- [ ] LF and CRLF separation
- [ ] Reject invalid header names/values
- [ ] Enforce header size limit
- [ ] Handle body-only responses
- [ ] Handle STDERR independently from STDOUT
- [ ] Unit tests: normal headers, status, location, repeated set-cookie, oversized header, missing separator, CR/LF injection, body-only, empty response, binary body
- [ ] Fuzz test: `FuzzCGIResponseHeaders`

### PHP Request Builder
- [ ] Implement CGI/FastCGI variable mapping (§14.4)
- [ ] Build `PHPRequest` struct
- [ ] Script resolution: route → release root → verify file → verify extension → verify symlink → canonical path
- [ ] Client must never supply `SCRIPT_FILENAME`
- [ ] Unit tests for all CGI variables

### Minimal HTTP Server
- [ ] Create `cmd/gateway/` entrypoint
- [ ] Use `net/http.Server` with required timeouts
- [ ] Connection-level deadlines
- [ ] Request ID assignment
- [ ] Basic request normalization
- [ ] Graceful shutdown

### FPM Supervisor (Minimal)
- [ ] Start PHP-FPM with private Unix socket
- [ ] Generate minimal FPM config
- [ ] Health check: socket exists + minimal FastCGI request
- [ ] Stop FPM on shutdown
- [ ] Process lifecycle: Starting → Ready → Stopping → Stopped
- [ ] Unit tests for config generation

### Request Pipeline (Minimal)
- [ ] Accept → parse → normalize → select project → resolve target → execute handler → respond
- [ ] Static file handler
- [ ] PHP handler (route to FPM)
- [ ] Deny `.env` files
- [ ] Deny path traversal
- [ ] Log request ID and timings

### Exit Criteria — Phase 0
- [ ] Plain PHP request works end-to-end
- [ ] Cancellation works (client disconnect kills PHP)
- [ ] Path traversal tests pass
- [ ] FPM process can start, health-check, and stop
- [ ] Technical risks documented

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
| Phase 0: Architecture Prototypes | Not started | |
| Phase 1: Local Dev Server | Not started | |
| Phase 2: Runtime Manager | Not started | |
| Phase 3: Production Server | Not started | |
| Phase 4: Deployment Manager | Not started | |
| Phase 5: Advanced Security | Not started | |
| Phase 6: Differentiators | Not started | |

---

## Notes

- Start with the First Vertical Slice (§60): `gateway serve ./example --php-fpm /usr/sbin/php-fpm`
- Build path handling and FastCGI correctness first.
- Add runtime distribution only after local FPM supervision is reliable.
- Add production features only after config reload and failure recovery are proven.
- Add WAF/plugins only after core can be tested and observed clearly.
