# Roadmap

This document is a prioritized list of work for `go-php/gateway`, derived from a full read of
[the engineering spec](.material/go-php-gateway-complete-engineering-spec.md) against the code as it
actually exists. Section references like §15.3 point at the spec.

It is deliberately opinionated about ordering. The organizing observation is this:

> **The codebase is substantially more complete than the running binary.**
> Roughly a third of `internal/` is written, tested, and unreachable from `main`.

That makes "wire what exists" the highest-leverage category of work available, ahead of writing
anything new. The best-covered packages in the repo are the ones with zero production importers:
`internal/admin` at 90.4%, `internal/php/fpm` at 85.3%, `internal/tls` at 78.5% — while
`cmd/gateway`, which holds the entire request path, has no tests at all.

---

## Now — wire the dead subsystems

Verified dead (zero non-test importers) at the time of writing:

| Package / file | Coverage | What it does | Consequence of it being unwired |
|---|---|---|---|
| `internal/admin` | 90.4% | token auth, CSRF, audit log, rate limit | the *insecure* `internal/ui` runs instead |
| `internal/tls` | 78.5% | `CertManager`, SNI, wildcard, default cert | **the gateway cannot serve HTTPS at all** |
| `internal/policy` | 80.7% | WAF engine, token-bucket + per-route limiter | no WAF, no rate limiting |
| `internal/php/fpm` | 85.3% | pool config layering, directive classification | `supervisor.generateConfig` re-implements it worse, inline |
| `observability/metrics.go` | — | `Metrics`, `PrometheusHandler` | no `/metrics` endpoint exists |
| `observability/tracing.go` | — | `Tracer`, `TraceMiddleware` | no tracing |
| `observability/redact.go` | — | `SecretRedactor` slog handler | **the access log is unredacted** |
| `config/reload.go` | — | `Reloader`, `ReloadWithDrain` | no SIGHUP, no config reload |
| `supervisor/isolation.go` | — | cgroups, namespaces, credentials | the documented isolation tiers ship as unreachable code |
| `filesystem/{static,cache}.go` | — | precompressed serving, Cache-Control/ETag policy | `main.go` hand-rolls MIME and ETag, emits **no `Cache-Control` at all** |
| `deploy/{switcher,canary,lock}.go` | — | deploy pipeline, canary, lock file | only `ReleaseManager` is reachable |
| `diagnostics/{htaccess,shadow,contract}.go` | — | §33 differentiators | no CLI command surfaces them |
| `runtime/signed.go` | — | Ed25519 signed index, SHA-256 artifact verification | nothing ever downloads or verifies a runtime |

The middleware chain at `cmd/gateway/main.go:299-300` is currently **one** middleware
(`observability.Middleware`). Everything else in the list above is absent from the running process.

Ordering, so each step is independently shippable:

**A — request chain.** Redaction on the slog handler → access log → tracing → metrics → rate limit →
policy → handler. Add the config surface for each (`security.mode` per §23.4, `security.rate_limit`,
`observability.*`). Serve `PrometheusHandler()` on the admin listener, not the public one —
§5.5 requires control-plane/data-plane separation.

Two prerequisites inside this step, because wiring them as-is would be a regression:
- `metrics.go:51` keys histograms by raw request path (unbounded cardinality) and `:132` emits that
  path as an unescaped Prometheus label. §32.3 explicitly forbids high-cardinality labels; the
  escaping bug also means a request to `` /a"}x{y=" `` corrupts the exposition format.
- `tracing.go:44` is an unbounded `map[TraceID][]*Span`. `Cleanup` exists at `:140` and is never
  scheduled, so wiring `TraceMiddleware` without a cleanup ticker is an OOM.

Also in this step: honour the limits the config already declares. `servePHP` hardcodes a 20 MiB body
cap and a 60s timeout (`main.go:475,478`), ignoring `security.max_body_size` and
`php.request_timeout` — which today only reach the generated FPM conf. Note `security.max_body_size`
is a `string` field (`config.go:87`) that nothing ever parses; it needs a `ParseByteSize` helper
validated at config load, not at request time.

**B — lifecycle.** TLS (static certs), SIGHUP reload, and real php-fpm supervision.

On supervision: `Supervisor.Start` execs once. If php-fpm dies, `s.state` stays `StateReady`,
nothing restarts it, and `HealthCheck` (`supervisor.go:168`) is never called from anywhere in the
binary. `StateDraining` and `StateAbsent` are declared and never entered. §16.5 wants exponential
backoff with jitter, a max restart rate, and a circuit-open state — "Never loop rapidly and consume
the host" — with static kept serving and a stable 503 for PHP routes meanwhile.

On TLS: wire `mode: files` only. `acme.go:48` never contacts an ACME server — it checks a cache,
calls `generateSelfSigned`, and returns that, under a Let's Encrypt-shaped API. Its own header
comment admits it. Shipping self-signed certs while claiming automatic TLS is worse than not
offering the mode, so `mode: acme` should fail loudly at config load until real ACME lands.

**C — surfacing.** Admin auth on the management API, the orphaned diagnostics as CLI commands,
static caching, and the deploy pipeline. See [Management API security](#management-api-security)
below for why the first of these is urgent.

---

## Next — PHP core performance

All three items below have their scaffolding already in the tree. §3.3 frames performance as
"targets, not claims", and §62 makes one of them a hard coding standard.

**FastCGI connection pooling.** Every PHP request currently opens a fresh
`net.DialTimeout("unix", ...)` and tears it down (`main.go:464`). `flagKeepConn` is already set in
`BEGIN_REQUEST` at `protocol.go:134` and never exploited. §15.3 is explicit that pooling comes only
after fault testing, and specifies what a pooled strategy needs: max idle, idle timeout, health
validation, discard after protocol error, **no reuse while unread bytes remain**, per-connection
deadline reset, and reuse/discard metrics.

**Response streaming.** `Client.Execute` buffers the entire stdout into a `bytes.Buffer`
(`protocol.go:217-255`) before a single byte reaches the client. Large downloads and SSE are
impossible today. This violates §62's "No full-body buffering for ordinary proxied or PHP requests"
and §3.3's "no full-body buffering". `cgi.ParseResponseStream` already exists at
`internal/php/cgi/response.go:87` and is called from nowhere.

**Request abort.** On the timeout branch (`main.go:495`) the function returns while the goroutine
still holds the FastCGI connection. Nothing sends `FCGI_ABORT_REQUEST` — `typeAbortReq` is declared
at `protocol.go:24` and never used — and nothing closes the connection until the deferred `Close`
fires after return. Under timeout pressure this leaks sockets and lets FPM children keep working on
requests nobody is waiting for.

---

## Next — correctness bugs worth fixing regardless of roadmap order

Independent, small, and each one currently silent.

- **Route regexes recompile per request.** `router.NewEngine` validates with `regexp.Compile` at load
  and throws the result away (`match.go:32-36`); `matchRoute:85` and `Rewrite:112` each recompile on
  every call, discarding the error. Store the compiled `*regexp.Regexp` on the route. §13.2 requires
  regexes "compiled at config load".
- **The deploy health gate is a no-op.** `Prober.Probe` (`switcher.go:161`) always returns `true`;
  the comment at `:136-141` says it would verify the release structure, and it does not do that
  either. Activating on a gate that cannot fail is worse than having no gate.
- **Canary routing uses a clock LSB.** `randomInt` is `time.Now().UnixNano() % 100`
  (`canary.go:161`), not a uniform draw. Bursty traffic routes in correlated blocks. §21.5 also
  wants stable request hashing where session consistency matters, which this isn't.
- **Hand-rolled, incorrect SHA-256.** `admin/csrf.go:186` implements SHA-256 by hand — its own
  comment reads "placeholder", "use crypto/sha256 in production" — and encodes the length suffix in
  **bytes rather than bits**, which is wrong per FIPS 180-4. `newHMACSHA256` at `:145` likewise
  hand-rolls ipad/opad and doesn't hash down over-long keys. None of it matters today only because
  `Validate`/`Consume` (`:51`, `:72`) never verify the MAC at all — they do a map lookup on the token
  string. The package already imports `crypto/rand` and `crypto/subtle`, so there was never a
  dependency reason for this. It's a ~250-line deletion in favour of `crypto/hmac` + `crypto/sha256`.
- **A risky-file check that never fires.** `diagnostics/compat.go:262` uses
  `filepath.Glob("**/pattern")`, which does not recurse in Go. `checkRiskyFiles` silently finds
  nothing. Use `filepath.WalkDir`. Related: `compat.checkPHPFiles` reads every `.php` file in the
  tree into memory with no size or count limit, and `CompatibilityReport.ScannedAt` is the literal
  string `"now"` (`compat.go:45`).
- **A panic reachable from an unauthenticated request.** `tls/acme.go:235` slices
  `r.URL.Path[len(prefix):]` without checking the prefix is present; any request shorter than 28
  characters panics. Currently unreachable because nothing wires it — fix before it becomes
  reachable.
- **httpoxy.** `cgi/params.go:71-74` copies *all* client headers into `HTTP_*`, including `Proxy`.
  Also `SERVER_SOFTWARE` is hardcoded `"go-php-gateway/1.0"` instead of the build version.
- **Build metadata never reaches the binary.** `Makefile:12` sets `-X main.version=…` but the
  variables are declared in `internal/buildinfo`. The linker ignores unmatched `-X` silently, so
  `gateway version` always prints `dev (commit unknown, built unknown, unknown)`. `GoVersion` has no
  ldflag at all and should just be `runtime.Version()`.
- **`gateway explain` is half-blind.** `commands.go:128` passes a `nil` router and `nil` policy
  engine, so the route and policy steps of the trace are always empty — and §33.1 calls the Request
  Decision Explorer the feature that replaces "confusing rewrite behavior". `diagnostics/explain.go`
  has the same problem from the other side: it constructs an *empty* policy engine at `:34-39`, so
  the policy step always reports `allow`.
- **Deadlock-shaped locking.** `ReleaseManager.Rollback` (`release.go:194`) takes `mu.Lock()` with a
  deferred unlock, then manually unlocks mid-function to call `Activate` and re-locks (`:221-225`).
  It works today, but any early return added between those points double-unlocks or deadlocks.
- **`.gitignore` line 3 is a bare `gateway`.** This ignores the built binary *and* the
  `cmd/gateway/` source directory for every ignore-aware tool — meaning ripgrep-based search
  silently skips the entire request path, and `git add .` won't pick up new files there. Should be
  `/gateway`.

---

## Management API security

Calling this out separately because it is the most severe item in the document and it is not a
missing feature — it is a shipped one.

`internal/ui` registers 21 API routes plus an embedded SPA and is started unconditionally by
`runServe` (`main.go:271-285`). It has **no authentication, no CSRF, no origin check, and no
security headers**. `internal/admin`, which has all four and 90.4% coverage, is the dead one.

Reachable with no credentials:

- `POST /api/sites` (`handlers.go:194`) takes an attacker-supplied `webroot`, `os.MkdirAll`s it, and
  **starts a new HTTP listener on an attacker-chosen port serving that directory with PHP execution
  enabled**. `webroot: "/"` gives a remote filesystem browser plus arbitrary PHP execution.
- `POST /api/deploy/create` (`:812`) does a recursive copy of an arbitrary `src_dir`.
- `POST /api/deploy/activate` and `/rollback` (`:839`, `:862`).
- `GET /api/doctor/compat?path=` (`:769`) reads an arbitrary path with no allowlist.
- `GET /api/ws/logs` (`:729`) with `CheckOrigin: func(r) bool { return true }` (`:725-727`) — any web
  page open in the operator's browser can attach to the log stream, and the same origin-blindness
  applies to every POST above.

The only protection is the default `127.0.0.1` bind, and `--ui-addr` accepts `0.0.0.0` with no
warning. §36.4 additionally requires separate read and write scopes, token rotation, and origin
validation, none of which exist.

Fix order matters: replace the broken CSRF crypto **first**, then change
`admin.Server.authenticate` (`api.go:85-87`) from fail-open-when-no-token-configured to fail-closed,
then apply the middleware to the `internal/ui` mux.

Separately, `internal/ui/siteManager.go` is a second, parallel request pipeline — a near
line-for-line duplicate of `servePHP` — that uses an ad-hoc `filepath.Clean` + `strings.HasPrefix`
traversal check (`:189-202`) instead of the hardened `filesystem.Resolver`, and has **no
protected-pattern check at all**, so `.env` under a site webroot is served. Two divergent security
postures for the same job means every fix to one silently misses the other. It should route through
`filesystem.Resolver`.

---

## Later — spec'd subsystems with no code and no tracking

Each of these is in the spec, absent from the codebase, and absent from `TODO.md`. Listed roughly by
risk.

**Trusted proxies and forwarded headers (§10.3).** No `trusted_proxies`, no CIDR list, no
`forwarded_headers.mode`, none of the 7-step peer-trust algorithm. `docs/DEPLOYMENT.md` already
tells operators to run behind nginx or HAProxy with `X-Forwarded-For` — the deployment guide
documents a feature that does not exist. `FuzzForwardedHeaderParser` (§44) is likewise untracked.

**Request framing hardening (§10.4).** Conflicting `Content-Length`, unsupported transfer codings,
multiple Host headers, absolute-form targets, h2c upgrades, HTTP/2 pseudo-header inconsistencies.
§46.1 already defines a 15-case security matrix for exactly this. The README currently claims
HTTP/2 as shipped.

**Upload security pipeline (§25).** Entirely absent — zero mentions of "upload" in `TODO.md`.
Wanted: body size enforced before PHP, streamed multipart, part-count and part-header limits,
malformed-multipart rejection, optional extension allow-list, `security.executable_paths` with
`/public/uploads/**` denied, storage outside executable roots, and 17 named test cases.

**Resource limits and backpressure (§24).** Five of six boxes unchecked. Bounded PHP queue,
503/429 with `Retry-After`, queue-wait recording, health-endpoint prioritization, fair scheduling.
§24.3 warns "Do not use unbounded client-key maps" — relevant when the rate limiter is wired.

**Reverse proxy and WebSockets (§31).** `internal/proxy/` is an empty directory. §3.1 lists both as
functional goals; §45.4 defines nine integration tests. Wanted: HTTP/1.1 and HTTP/2 upstreams,
WebSocket upgrade, bidirectional streaming, health checks, load balancing, idempotent-only retry,
circuit breaker, hop-by-hop stripping, upstream URL validation at config load, and a hard rule
against arbitrary upstream selection from client input.

**State store (§38).** `internal/state/{sqlite,memory}/` is mandated by §7 and by `AGENTS.md`
("State store: SQLite (default, replaceable behind interface)") and **does not exist**. Ten tables
are specified, plus WAL mode, migrations, corruption detection, startup recovery, and lock timeout.
Note the unresolved architectural tension the spec never addresses: a SQLite default conflicts with
the §52.1 "standalone Go binary" packaging goal unless a CGO-free driver is chosen. Worth an ADR
before any code.

**Secrets management (§29).** Only "redact secrets from logs" is tracked. Missing: secret *sources*
(env, encrypted file, FD injection, named pipe, systemd credentials, container mounts, provider
plugin), the `php.environment.inherit/allow/set/secrets` schema, `clear_env` enforcement,
reload-on-secret-change, and secret-access auditing that records names but never values.

**Windows backend (§17).** `php-cgi.exe` worker supervision, Job Objects, restricted tokens, named
pipes, recycle counts, DLL and VC-runtime verification, ADS rejection. `internal/platform/{linux,
darwin,windows}/` all exist and are empty. §53 Phase 0 calls for a Windows feasibility experiment
that was never tracked. §17.3's rule is worth quoting: "Do not present different behavior as
identical."

**Missing §33 differentiators.** Per-route PHP runtime (§33.9) for staged legacy modernization —
`/legacy/** → 8.1`, `/** → 8.4`. Safe deployment replay (§33.8) — record sanitized request metadata,
replay against a candidate release before activation. Both are named differentiators with no
tracking anywhere.

**Public SDK.** `pkg/configapi`, `pkg/policyapi`, `pkg/pluginapi` are three empty directories. §26.4
specifies WebAssembly for untrusted policy plugins with a fuel budget, memory limit, versioned ABI,
and signed packages — alongside the hard rule "Do not use native Go plugins for untrusted
third-party extensions."

**Config schema divergence.** The shipped schema is flat: `schema: gateway/v1` with `server:`,
`php:`, `routes:`, `logging:`, `security:`. The spec's §34.1 canonical schema is `schema: 1` with
`listeners:`, `admin:`, `network:`, and `projects: [...]`. **The shipped schema cannot express the
multi-project, multi-listener production model** that Phase 3 is built on. Also unimplemented from
§34.2: unknown fields as errors, `--allow-unknown`, `gateway config migrate` with a diff, and "Never
silently reinterpret existing fields." `examples/gateway.yaml` is meanwhile inconsistent with the
loader — it uses `schema: "1.0"` and an integer `max_body_size` against a string field, so it would
fail to parse.

**Distribution.** `packaging/` and `scripts/` are empty directories. No Dockerfile anywhere. No
release workflow, no tag trigger, no goreleaser, no deb/rpm, no Homebrew formula, no Windows
installer, no checksums or signatures, no SBOM or provenance. `build-all` cross-compiles for
linux/amd64 and darwin/amd64+arm64 — no linux/arm64, no Windows — and CI is `ubuntu-latest` only, so
the darwin targets are never verified. Also missing from CI: `govulncheck`, `gosec`/CodeQL,
dependabot, a Node job for the dashboard, and a multi-PHP-version matrix (e2e pins 8.3).

**CLI surface gaps (§35).** `gateway start|stop|reload|status`, `config print-effective` /
`inspect config` (the enforcement mechanism for Principle #1, "showing the source of each final
setting"), `inspect request|routes|runtime`, `project list|inspect|enable|disable`,
`php extensions|check`, `test compatibility|security`, `benchmark`, and the §35.2 exit-code contract
(0–8). `gateway php install` is currently a stub that creates an empty temp dir and registers a
manifest for it (`commands.go:305-320`) — it downloads nothing, because `runtime/signed.go`, which
would verify a download, is dead.

---

## Later — testing and quality

- **`cmd/gateway` has no tests.** 677 lines holding the request path, script resolution, MIME/ETag,
  framework detection, and CLI dispatch, in a repo averaging 70.9%. This is the single largest
  coverage gap and it sits on the most security-sensitive code.
- **Three tests that assert nothing.** `test/security/security_test.go:172` re-implements the auth
  logic inline rather than calling `admin.Server`; `:215` is a no-op with a `t.Log`; `:223` asserts
  that `"POST" != "GET"`. A green security run currently proves less than it appears to.
- **9 of the 15 required fuzzers (§44) do not exist**: `FuzzRewriteEngine`, `FuzzHostParser`,
  `FuzzForwardedHeaderParser`, `FuzzMultipartMetadata`, `FuzzConfigDecoder`, `FuzzRuntimeManifest`,
  `FuzzArchiveExtractor`, `FuzzByteSizeParser`, `FuzzDurationParser`. Two more are tracked but
  unwritten. `make test-fuzz` currently **fails**, because it invokes `FuzzHtaccessTranslator`, which
  does not exist. No fuzz corpus is persisted (`testdata/fuzz` is absent), so §44's regression
  requirement isn't met.
- **`test/wordpress/` is 94 MB of a full WordPress checkout that no Go test references.**
  `test/compatibility/`, `test/fixtures/frameworks/`, and `test/fixtures/security/` are empty
  directories. The framework-compatibility suite — which §2.1 makes the precondition for any
  Apache-replacement claim — is scaffolded but unwritten.
- **Five of six property invariants (§43) untracked.** Only idempotence appears. Missing: the path
  invariant ("no filesystem open occurs" for rejected paths), FastCGI round-trip, config round-trip,
  rewrite termination, header safety.
- **Goroutine leak tests (§39.2).** Six named scenarios — canceled uploads, failed FastCGI
  connections, client disconnect, config reload, runtime crash, shutdown — with no entry. §62
  requires every goroutine have "a test proving it exits".
- **`coverage.out` (129 KB) is committed to the repo.**

---

## Later — documentation

- **`docs/adr/` is empty.** §58 pre-names ADR-0001 through ADR-0012, and `AGENTS.md` mandates an ADR
  per new package. Twelve decisions are already made and unrecorded — including the two most
  load-bearing ones, ADR-0012 "No embedded PHP in initial releases" (what separates this project from
  FrankenPHP) and ADR-0006 "SQLite default state store" (which conflicts with single-binary
  packaging).
- **All 20 documentation items in `TODO.md` are unchecked** and mirror §57 exactly. Three exist
  (README, quick start, deployment guide) and the README is inaccurate — see below.
- **§52.3 license and notice inventory** for redistributed runtimes, libraries, extensions, CA
  assets, dashboard assets, and Go modules.
- **§63's 20 security review questions** are a per-release gate and appear nowhere in the tracker.
  Sample: "Can the client influence a process argument?", "Is any queue unbounded?", "Can an old
  runtime be deleted while still used?", "Is the claimed isolation level accurate?"

---

## Housekeeping

- **`internal/php/fpm` should be adopted or deleted.** 203 lines at 85.3% coverage, zero importers,
  and `supervisor.generateConfig` (`supervisor.go:255-350`) re-implements a worse version of it
  inline. Adopting is the recommendation.
- **The dashboard build is not in CI** and is not a dependency of `make build`. The committed
  `internal/ui/static/` is the source of truth, so a stale checkout silently ships an old UI. Because
  `dashboard-embed` is `cp -r` and never cleans, `internal/ui/static/assets/` also holds orphaned
  bundles — `index-BlGGZGVg.js` (663 KB) and `index-TNzGGYIp.css` (28 KB) are dead and still
  compiled into the binary. ~690 KB of dead weight.
- **The dashboard has no tests and no lint job.** `dashboard/README.md` is still the stock Vite
  template text.
- **Several dashboard pages are backed by fiction.** `/api/metrics/history` (`handlers.go:884`)
  generates its data arithmetically — `25.0 + (i*13)%15` for latency. `/api/config` (`:415`) returns
  hardcoded literals and never reads `gateway.yaml`, and sets `PHP.Binary` to the docroot (`:429`).
  `/api/config/save` (`:476`) persists nothing to disk and returns `{"status":"saved"}`.
  `StatusProvider`'s request counters (`handlers.go:31-34`) are never incremented, so `/api/status`
  always reports zeros. The Logs page never shows requests, because `LogBuffer` is fed only by UI
  handlers and nothing bridges the gateway's `slog` output into it.
- **The dashboard is in neither the spec nor `TODO.md`**, has no ADR, and adds a Node toolchain to a
  project whose packaging goal is a standalone Go binary. §36.4 permits it but requires "the core
  server must not depend on a dashboard" — worth an explicit ADR either way.
- **`internal/runtime/provision.go`** — auto-running `sudo apt-get install` at boot for missing
  extensions (`main.go:206-217` → `provision.go:201-209`) — is in neither the spec nor `TODO.md`, and
  sits in tension with §5.3 immutable runtimes and §18.4's signed-artifact supply chain. It is a
  surprising amount of authority for a web server to take at startup. Worth an ADR and a config
  gate, defaulting to off.
- **`internal/errors` is complete, 100% covered, and used once** — as linter appeasement:
  `var _ = errors.IsCode` at `main.go:677`. Not a single `GatewayError` is constructed in the binary;
  all error handling is `fmt.Errorf`. §37 specifies ten stable codes as a compatibility contract.
- **`gorilla/websocket` is marked `// indirect` in `go.mod`** but is a direct import at
  `internal/ui/handlers.go:19`. `go mod tidy` fixes it.
- **`TODO.md` and `README.md` do not describe this repository.** See the note at the top of each.

---

## Spec inconsistencies worth resolving

Not code problems — places where the spec contradicts itself and a decision is needed.

- §15.4 names the FastCGI fuzzers `FuzzDecodeRecord`, `FuzzDecodeNameValue`, `FuzzParseCGIHeaders`,
  `FuzzFastCGIResponseSequence`; §44's canonical list names them differently. The implemented targets
  follow neither consistently.
- §38 makes SQLite the default state store; §52.1 makes "standalone Go binary" the packaging target.
  Reconcilable, but only by explicitly choosing a CGO-free driver or a non-SQLite default.
- `TODO.md`'s header defines `[-]` for "skipped/blocked" and **no `[-]` item exists anywhere in the
  file** — nothing has ever been formally deferred, including the four items marked "deferred to
  Phase 1" that were never done after Phase 1 was marked Complete.
