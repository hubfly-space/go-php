package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/go-php/gateway/internal/buildinfo"
	"github.com/go-php/gateway/internal/config"
	"github.com/go-php/gateway/internal/errors"
	"github.com/go-php/gateway/internal/filesystem"
	"github.com/go-php/gateway/internal/observability"
	"github.com/go-php/gateway/internal/php/cgi"
	"github.com/go-php/gateway/internal/php/fastcgi"
	"github.com/go-php/gateway/internal/policy"
	"github.com/go-php/gateway/internal/router"
	"github.com/go-php/gateway/internal/runtime"
	"github.com/go-php/gateway/internal/supervisor"
	gatewaytls "github.com/go-php/gateway/internal/tls"
	"github.com/go-php/gateway/internal/ui"
)

func main() {
	serveCmd := flag.NewFlagSet("serve", flag.ExitOnError)
	phpFPM := serveCmd.String("php-fpm", "", "path to php-fpm binary")
	addr := serveCmd.String("addr", "", "listen address (overrides config)")
	configPath := serveCmd.String("config", "", "path to gateway.yaml config file")
	uiAddrFlag := serveCmd.String("ui-addr", "127.0.0.1:30200", "management UI address (empty to disable)")

	initCmd := flag.NewFlagSet("init", flag.ExitOnError)
	initFramework := initCmd.String("framework", "", "target framework (laravel, symfony, wordpress, plain)")
	initPHP := initCmd.String("php", "8.3", "PHP version constraint")

	if len(os.Args) < 2 {
		printUsage()
		os.Exit(2)
	}

	cmd := os.Args[1]
	args := os.Args[2:]

	var err error
	switch cmd {
	case "serve":
		var positional []string
		if positional, err = parseFlagsPermuted(serveCmd, args); err == nil {
			err = runServe(*addr, *phpFPM, *configPath, positional, *uiAddrFlag)
		}
	case "init":
		var positional []string
		if positional, err = parseFlagsPermuted(initCmd, args); err == nil {
			err = runInit(*initFramework, *initPHP, positional)
		}
	case "doctor":
		err = runDoctor()
	case "compat":
		err = runCompat(args)
	case "explain":
		err = runExplain(args)
	case "config":
		err = runConfig(args)
	case "deploy":
		err = runDeploy(args)
	case "php":
		err = runPHP(args)
	case "migrate":
		err = runMigrate(args)
	case "test":
		err = runTest(args)
	case "shadow":
		err = runShadow(args)
	case "incident":
		err = runIncident(args)
	case "service":
		err = runService(args)
	case "version":
		info := buildinfo.Get()
		fmt.Printf("gateway %s (commit %s, built %s, %s)\n",
			info.Version, info.Commit, info.BuildDate, info.GoVersion)
	case "help", "-h", "--help":
		printUsage()
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n\n", cmd)
		printUsage()
		os.Exit(2)
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "\033[31mError: %v\033[0m\n", err)
		os.Exit(1)
	}
}

// parseFlagsPermuted parses flags that may appear before, after, or interleaved
// with positional arguments, and returns the positional arguments.
//
// Go's flag package stops at the first non-flag argument, so the documented
// invocation `gateway serve . --php-fpm /usr/sbin/php-fpm8.3` silently ignored
// every flag after the ".". Silently is the problem: the server started with
// default settings and reported nothing.
func parseFlagsPermuted(fs *flag.FlagSet, args []string) ([]string, error) {
	var positional []string

	for {
		if err := fs.Parse(args); err != nil {
			return nil, err
		}
		rest := fs.Args()
		if len(rest) == 0 {
			return positional, nil
		}
		// Take one positional and resume parsing after it, so a flag that
		// follows still gets seen.
		positional = append(positional, rest[0])
		args = rest[1:]
	}
}

func printUsage() {
	fmt.Fprintf(os.Stderr, "\033[1;36mGo-PHP Gateway\033[0m — Secure PHP runtime manager & application gateway\n\n")
	fmt.Fprintf(os.Stderr, "Usage: gateway <command> [flags]\n\n")
	fmt.Fprintf(os.Stderr, "Commands:\n")
	fmt.Fprintf(os.Stderr, "  serve       Start gateway dev/production server\n")
	fmt.Fprintf(os.Stderr, "  init        Initialize a new Go-PHP Gateway project\n")
	fmt.Fprintf(os.Stderr, "  doctor      Run system readiness & environment checks\n")
	fmt.Fprintf(os.Stderr, "  compat      Scan project for framework & .htaccess compatibility\n")
	fmt.Fprintf(os.Stderr, "  explain     Trace a request through the decision pipeline\n")
	fmt.Fprintf(os.Stderr, "  config      Manage configuration (validate, init)\n")
	fmt.Fprintf(os.Stderr, "  deploy      Manage releases and deployments (create, activate, rollback, list)\n")
	fmt.Fprintf(os.Stderr, "  php         Manage PHP runtimes (list, install, use, remove)\n")
	fmt.Fprintf(os.Stderr, "  migrate     Translate Apache config to gateway routes (htaccess)\n")
	fmt.Fprintf(os.Stderr, "  test        Run route contract tests (routes)\n")
	fmt.Fprintf(os.Stderr, "  shadow      Compare an active runtime against a candidate\n")
	fmt.Fprintf(os.Stderr, "  incident    Capture diagnostic incident snapshot\n")
	fmt.Fprintf(os.Stderr, "  service     Install systemd service unit\n")
	fmt.Fprintf(os.Stderr, "  version     Show build and version metadata\n\n")
}

func runServe(flagAddr, phpFPMPath, configPath string, args []string, uiAddr string) error {
	// Load config.
	cfg := config.DefaultConfig()
	if configPath != "" {
		var err error
		cfg, err = config.Load(configPath)
		if err != nil {
			return fmt.Errorf("load config: %w", err)
		}
	}

	// Flags override config.
	if flagAddr != "" {
		cfg.Server.Addr = flagAddr
	}
	if phpFPMPath != "" {
		cfg.PHP.Binary = phpFPMPath
	}

	// Install log redaction before anything logs. §32.2 requires secrets to
	// stay out of the access log, and a redactor installed late is a redactor
	// that already leaked.
	if len(cfg.Observability.RedactKeys) > 0 {
		slog.SetDefault(slog.New(observability.StdoutRedactor(cfg.Observability.RedactKeys)))
	}

	// rootCtx bounds every background goroutine this command starts. Each one
	// returns a done channel; the single deferred closure below cancels first
	// and only then waits, so the process does not exit with work still in
	// flight (§62). Separate defers would deadlock — LIFO ordering would run
	// the waits before the cancel.
	rootCtx, stopBackground := context.WithCancel(context.Background())
	var backgroundDone []<-chan struct{}
	defer func() {
		stopBackground()
		for _, done := range backgroundDone {
			<-done
		}
	}()

	// Auto-detect PHP-FPM binary if the provided path doesn't exist.
	if cfg.PHP.Binary != "" {
		if _, err := os.Stat(cfg.PHP.Binary); err != nil {
			if detected := detectFPMBinary(); detected != "" {
				slog.Info("auto-detected PHP-FPM binary", "path", detected)
				cfg.PHP.Binary = detected
			}
		}
	} else {
		if detected := detectFPMBinary(); detected != "" {
			slog.Info("auto-detected PHP-FPM binary", "path", detected)
			cfg.PHP.Binary = detected
		}
	}

	docRoot := "."
	if len(args) > 0 {
		docRoot = args[0]
	}

	absRoot, err := filepath.Abs(docRoot)
	if err != nil {
		return fmt.Errorf("resolve root: %w", err)
	}

	slog.Info("starting gateway",
		"version", buildinfo.Get().Version,
		"root", absRoot,
		"addr", cfg.Server.Addr,
	)

	// Detect framework.
	framework, pubRoot := detectFramework(absRoot)
	if pubRoot != "" {
		absRoot = pubRoot
	}
	if framework != "" {
		slog.Info("framework detected", "framework", framework, "document_root", absRoot)
	}

	// Create filesystem resolver.
	symlinkMode := filesystem.SymlinkWithinRoot
	if cfg.Security.SymlinkMode == "deny" {
		symlinkMode = filesystem.SymlinkDeny
	}
	protected := cfg.Security.ProtectedPatterns
	if len(protected) == 0 {
		protected = filesystem.DefaultProtectedPatterns()
	}
	resolver := filesystem.NewResolver(absRoot, symlinkMode, protected)

	routingEngine, err := buildRouter(cfg)
	if err != nil {
		return err
	}

	// Start PHP-FPM.
	var fpm *supervisor.Supervisor
	var watchdog *supervisor.Watchdog
	sockPath := cfg.PHP.SocketPath
	if cfg.PHP.Binary != "" {
		if sockPath == "" {
			sockPath = filepath.Join(os.TempDir(), fmt.Sprintf("gateway-%d.sock", os.Getpid()))
		}
		pidPath := filepath.Join(os.TempDir(), fmt.Sprintf("gateway-%d.pid", os.Getpid()))

		// Resolve extensions from config.
		extensions, iniSettings, err := config.ResolveExtensions(&cfg.PHP)
		if err != nil {
			slog.Warn("could not resolve PHP extensions", "error", err)
		}

		// Auto-provision missing OS packages for extensions.
		extNames := make([]string, 0, len(extensions))
		for _, ext := range extensions {
			extNames = append(extNames, ext.Name)
		}
		provisioner := runtime.NewProvisioner(cfg.PHP.Binary)
		if missing := provisioner.MissingExtensions(extNames); len(missing) > 0 {
			slog.Info("provisioning OS packages for missing extensions", "missing", missing)
			installed, errs := provisioner.Provision(context.Background(), extNames)
			for _, err := range errs {
				slog.Warn("extension provisioning warning", "error", err)
			}
			if len(installed) > 0 {
				slog.Info("installed OS packages for extensions", "count", len(installed))
			}
		}

		supExtensions := make([]supervisor.Extension, 0, len(extensions))
		for _, ext := range extensions {
			supExtensions = append(supExtensions, supervisor.Extension{
				Name: ext.Name,
				Type: ext.Type,
			})
		}

		supIni := make([]supervisor.IniSetting, 0, len(iniSettings))
		for _, ini := range iniSettings {
			supIni = append(supIni, supervisor.IniSetting{
				Name:  ini.Name,
				Value: ini.Value,
			})
		}

		fpm = supervisor.New(supervisor.Config{
			PHPBinary:      cfg.PHP.Binary,
			SocketPath:     sockPath,
			PIDFile:        pidPath,
			MaxChildren:    cfg.PHP.MaxChildren,
			StartServers:   cfg.PHP.StartServers,
			MinSpare:       cfg.PHP.MinSpare,
			MaxSpare:       cfg.PHP.MaxSpare,
			MaxRequests:    cfg.PHP.MaxRequests,
			RequestTimeout: cfg.PHP.RequestTimeout,
			ErrorLog:       filepath.Join(os.TempDir(), "gateway-fpm-error.log"),
			Extensions:     supExtensions,
			PhpIni:         supIni,
		})

		// Apply OS-level isolation if configured. Tier 0 (no isolation) stays
		// the default so existing setups are unaffected — §28.1 requires the
		// claimed isolation level to be accurate, not maximal.
		if cfg.PHP.Isolation.Mode != "" && cfg.PHP.Isolation.Mode != "none" {
			isoCfg := supervisor.DefaultIsolationConfig()
			isoCfg.Enabled = true
			isoCfg.Mode = cfg.PHP.Isolation.Mode
			isoCfg.User = cfg.PHP.Isolation.User
			isoCfg.MemoryLimit = cfg.PHP.Isolation.MemoryLimit
			isoCfg.PIDLimit = cfg.PHP.Isolation.PIDLimit
			fpm.SetIsolator(supervisor.NewIsolator(isoCfg))
			slog.Info("php-fpm isolation enabled", "mode", cfg.PHP.Isolation.Mode)
		}

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := fpm.Start(ctx); err != nil {
			slog.Warn("could not start php-fpm, PHP requests will fail", "error", err)
		} else {
			slog.Info("php-fpm started", "socket", sockPath)
			defer fpm.Stop(context.Background())

			// Supervise it. Without this loop, a php-fpm that dies leaves the
			// state at StateReady and every PHP request fails forever with no
			// attempt to recover (§16.5).
			watchdog = supervisor.NewWatchdog(fpm, supervisor.DefaultWatchdogConfig(), slog.Default())
			backgroundDone = append(backgroundDone, watchdog.Run(rootCtx))
		}
	}

	// Metrics are constructed before the management server so the collector can
	// be shared: the middleware records into it and the management listener
	// exposes it.
	var metrics *observability.Metrics
	if cfg.Observability.Metrics.Enabled {
		metrics = observability.NewMetrics()
	}

	// Start management UI server.
	uiCfg := ui.DefaultConfig()
	if uiAddr != "" {
		uiCfg.Addr = uiAddr
	}
	uiCfg.SockPath = sockPath
	if metrics != nil {
		uiCfg.MetricsPath = cfg.Observability.Metrics.Path
		uiCfg.MetricsHandler = metrics.PrometheusHandler()
	}
	statusProvider := ui.NewStatusProvider(buildinfo.Get().Version, cfg.Server.Addr, absRoot, framework)
	statusProvider.Runtimes = detectRuntimes(cfg.PHP.Binary)

	uiServer := ui.NewServer(uiCfg, slog.Default(), statusProvider)
	if err := uiServer.Start(); err != nil {
		slog.Warn("could not start management UI", "error", err)
	} else {
		slog.Info("management UI started", "addr", ui.FormatAddr(uiCfg.Addr))
		if metrics != nil {
			slog.Info("metrics endpoint",
				"url", ui.FormatAddr(uiCfg.Addr)+cfg.Observability.Metrics.Path)
		}

		// The management API can create directories, start listeners on
		// arbitrary ports, and activate releases. Binding it off loopback
		// exposes all of that to the network, so say so loudly rather than
		// letting a flag typo become a silent exposure.
		if !isLoopbackAddr(uiCfg.Addr) {
			slog.Warn("management UI is NOT bound to loopback; it is reachable from the network",
				"addr", uiCfg.Addr)
		}

		// Print the token to stderr, not the structured log, so it is visible
		// to whoever started the process without being swept into log
		// aggregation.
		fmt.Fprintf(os.Stderr, "\nManagement dashboard: %s/?token=%s\n",
			ui.FormatAddr(uiCfg.Addr), uiServer.Token())
		fmt.Fprintf(os.Stderr, "Management API token: %s\n", uiServer.Token())
		fmt.Fprintf(os.Stderr, "  curl -H 'Authorization: Bearer %s' %s/api/status\n\n",
			uiServer.Token(), ui.FormatAddr(uiCfg.Addr))

		defer uiServer.Stop()
	}

	logger := slog.Default()

	handler := &gatewayHandler{
		docRoot:  absRoot,
		fpm:      fpm,
		watchdog: watchdog,
		sockPath: sockPath,
		logger:   logger,
	}
	handler.state.Store(&serveState{cfg: cfg, resolver: resolver, router: routingEngine})

	// The reloader owns the published config snapshot; the handler owns the
	// derived request-path components. SIGHUP updates both together.
	reloader := config.NewReloader(cfg, nil)

	// Build the middleware chain. Order matters and reads outermost-first:
	// access logging sees every request including ones later layers reject;
	// tracing wraps the work; metrics observe what the policy layer admits
	// plus what it rejects; the rate limiter runs before the policy engine so
	// a flood costs as little as possible (§3.3: "Minimal work for denied
	// requests").
	var h http.Handler = handler

	securityMode, err := policy.ParseMode(cfg.Security.Mode)
	if err != nil {
		return fmt.Errorf("security.mode: %w", err)
	}
	policyEngine := policy.NewEngineForMode(securityMode)
	h = policy.ModeMiddleware(policyEngine, securityMode, logger)(h)

	if cfg.Security.RateLimit.Enabled {
		limiter := policy.NewPerRouteLimiter(cfg.Security.RateLimit.RequestsPerMinute)
		backgroundDone = append(backgroundDone, limiter.StartCleanup(rootCtx, time.Minute))
		h = limiter.Middleware(h)
		slog.Info("rate limiting enabled",
			"requests_per_minute", cfg.Security.RateLimit.RequestsPerMinute,
			"burst", cfg.Security.RateLimit.Burst)
	}

	if metrics != nil {
		h = metrics.Middleware(h)
	}

	if cfg.Observability.Tracing.Enabled {
		tracer := observability.NewTracer("gateway", logger)
		// Without this the span map grows without bound.
		backgroundDone = append(backgroundDone, tracer.StartCleanup(rootCtx,
			cfg.Observability.Tracing.Retention/2, cfg.Observability.Tracing.Retention))
		h = observability.TraceMiddleware(tracer)(h)
	}

	h = observability.Middleware(logger)(h)

	server := &http.Server{
		Addr:              cfg.Server.Addr,
		Handler:           h,
		ReadHeaderTimeout: cfg.Server.ReadHeaderTimeout,
		ReadTimeout:       cfg.Server.ReadTimeout,
		WriteTimeout:      cfg.Server.WriteTimeout,
		IdleTimeout:       cfg.Server.IdleTimeout,
		MaxHeaderBytes:    cfg.Server.MaxHeaderBytes,
	}

	// Configure TLS if enabled. Validate has already rejected the acme mode and
	// checked that a usable certificate source is configured.
	var redirectServer *http.Server
	if cfg.TLS.Enabled() {
		certMgr := gatewaytls.NewCertManager("")

		if cfg.TLS.CertDir != "" {
			if err := certMgr.LoadCertDir(cfg.TLS.CertDir); err != nil {
				return fmt.Errorf("load tls.cert_dir: %w", err)
			}
		}
		if cfg.TLS.CertFile != "" {
			if err := certMgr.SetDefault(cfg.TLS.CertFile, cfg.TLS.KeyFile); err != nil {
				return fmt.Errorf("load tls.cert_file: %w", err)
			}
		}

		server.TLSConfig = certMgr.GetConfigForClient()
		slog.Info("TLS enabled", "domains", certMgr.Domains(),
			"default_cert", cfg.TLS.CertFile != "")

		if cfg.TLS.RedirectFrom != "" {
			redirectServer = &http.Server{
				Addr:              cfg.TLS.RedirectFrom,
				Handler:           gatewaytls.RedirectHandler(cfg.Server.Addr),
				ReadHeaderTimeout: cfg.Server.ReadHeaderTimeout,
			}
			go func() {
				slog.Info("HTTP redirect listener", "addr", cfg.TLS.RedirectFrom)
				if err := redirectServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
					slog.Error("redirect listener failed", "error", err)
				}
			}()
		}
	}

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	// SIGHUP reloads configuration. §5.2: "Never replace a known-good runtime
	// state with an unvalidated configuration" — a reload that fails validation
	// logs and leaves the running config in place.
	hup := make(chan os.Signal, 1)
	signal.Notify(hup, syscall.SIGHUP)

	go func() {
		<-quit
		slog.Info("shutting down gracefully...")
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if redirectServer != nil {
			redirectServer.Shutdown(ctx)
		}
		server.Shutdown(ctx)
	}()

	go func() {
		for {
			select {
			case <-rootCtx.Done():
				return
			case <-hup:
				if configPath == "" {
					slog.Warn("SIGHUP ignored: no config file was given at startup")
					continue
				}
				if err := handler.reload(configPath, reloader); err != nil {
					// The running configuration is untouched (§5.2).
					slog.Error("config reload rejected, keeping running configuration",
						"path", configPath, "error", err)
					continue
				}
				slog.Info("configuration reloaded", "path", configPath,
					"version", reloader.Version())
			}
		}
	}()

	scheme := "http"
	if cfg.TLS.Enabled() {
		scheme = "https"
	}
	slog.Info("listening", "addr", cfg.Server.Addr, "scheme", scheme)

	if cfg.TLS.Enabled() {
		// Certificates come from server.TLSConfig.GetCertificate, so the file
		// arguments are intentionally empty.
		err = server.ListenAndServeTLS("", "")
	} else {
		err = server.ListenAndServe()
	}
	if err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("listen: %w", err)
	}

	slog.Info("server stopped")
	return nil
}

// isLoopbackAddr reports whether a host:port binds only to loopback.
//
// An empty host means "all interfaces", which is the opposite of loopback — the
// case most likely to be a mistake.
func isLoopbackAddr(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return false
	}
	if host == "" {
		return false
	}
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// buildRouter converts config routes into a routing engine. It is shared by
// serve and explain so that a trace reflects the routing the server would
// actually perform.
func buildRouter(cfg *config.Config) (*router.Engine, error) {
	routes := make([]router.Route, 0, len(cfg.Routes))
	for _, rc := range cfg.Routes {
		routes = append(routes, router.Route{
			Host:       rc.Host,
			Path:       rc.Path,
			PathPrefix: rc.PathPrefix,
			Regex:      rc.Regex,
			Target:     rc.Target,
			Status:     rc.Status,
			Methods:    rc.Methods,
			Headers:    rc.Headers,
		})
	}

	engine, err := router.NewEngine(routes)
	if err != nil {
		return nil, fmt.Errorf("build routes: %w", err)
	}
	return engine, nil
}

// serveState is the set of request-path components derived from configuration.
// It is immutable once published; a reload builds a whole new one and swaps the
// pointer, so an in-flight request keeps the state it started with (§39.1).
type serveState struct {
	cfg      *config.Config
	resolver *filesystem.Resolver
	router   *router.Engine
}

type gatewayHandler struct {
	docRoot  string
	fpm      *supervisor.Supervisor
	watchdog *supervisor.Watchdog
	sockPath string
	logger   *slog.Logger

	// state is read once per request and never mutated in place.
	state atomic.Pointer[serveState]
}

// current returns the active state. Each request reads it exactly once, so a
// concurrent reload cannot change the configuration mid-request.
func (h *gatewayHandler) current() *serveState {
	return h.state.Load()
}

// phpAvailable reports whether PHP requests can be served right now.
//
// When the watchdog's restart circuit is open, php-fpm is known to be failing
// repeatedly. §16.5 wants static files to keep being served and PHP routes to
// return a stable status in that window, rather than each request rediscovering
// the outage as a fresh connection error.
func (h *gatewayHandler) phpAvailable() bool {
	if h.fpm == nil || h.fpm.State() != supervisor.StateReady {
		return false
	}
	if h.watchdog != nil && h.watchdog.CircuitOpen() {
		return false
	}
	return true
}

// reload rebuilds the request-path components from a config file and swaps them
// in atomically.
//
// Everything that can fail — parsing, validation, route compilation — happens
// before the swap. §5.2: "Never replace a known-good runtime state with an
// unvalidated configuration."
func (h *gatewayHandler) reload(configPath string, reloader *config.Reloader) error {
	newCfg, err := config.Load(configPath)
	if err != nil {
		return err
	}

	newRouter, err := buildRouter(newCfg)
	if err != nil {
		return err
	}

	symlinkMode := filesystem.SymlinkWithinRoot
	if newCfg.Security.SymlinkMode == "deny" {
		symlinkMode = filesystem.SymlinkDeny
	}
	protected := newCfg.Security.ProtectedPatterns
	if len(protected) == 0 {
		protected = filesystem.DefaultProtectedPatterns()
	}
	newResolver := filesystem.NewResolver(h.docRoot, symlinkMode, protected)

	// Capture the outgoing state before the swap, so the comparison below sees
	// what was actually running.
	old := h.current()

	// Everything below this line is infallible.
	if err := reloader.Reload(newCfg); err != nil {
		return err
	}
	h.state.Store(&serveState{cfg: newCfg, resolver: newResolver, router: newRouter})

	// Be explicit about what a reload does not cover, rather than letting an
	// operator believe a change took effect when it did not.
	if old != nil {
		if old.cfg.Server.Addr != newCfg.Server.Addr {
			h.logger.Warn("server.addr changed but requires a restart to take effect",
				"running", old.cfg.Server.Addr, "configured", newCfg.Server.Addr)
		}
		if old.cfg.TLS != newCfg.TLS {
			h.logger.Warn("tls settings changed but require a restart to take effect")
		}
		if old.cfg.PHP.Binary != newCfg.PHP.Binary {
			h.logger.Warn("php.binary changed but requires a restart to take effect",
				"running", old.cfg.PHP.Binary, "configured", newCfg.PHP.Binary)
		}
		if old.cfg.Security.Mode != newCfg.Security.Mode ||
			old.cfg.Security.RateLimit != newCfg.Security.RateLimit {
			h.logger.Warn("security.mode and security.rate_limit changes require a restart to take effect")
		}
	}

	return nil
}

func (h *gatewayHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	reqID := fmt.Sprintf("req_%d", start.UnixNano())
	w.Header().Set("X-Request-ID", reqID)

	// Read the configuration snapshot exactly once. A reload that lands
	// mid-request cannot change the rules this request is judged by (§39.1).
	st := h.current()

	path := r.URL.Path

	pp, err := filesystem.ParsePath(path)
	if err != nil {
		h.logger.Warn("path rejected", "request_id", reqID, "path", path, "error", err)
		h.devError(w, r, 400, "Bad Request", fmt.Sprintf("Path rejected: %v", err), reqID, start)
		return
	}
	normalized := pp.NormalizedPath

	// Check routing rules.
	if route := st.router.Match(r); route != nil {
		// Report the route's pattern, never the request path — the metrics
		// label set must be bounded by the route table (§32.3).
		observability.SetRouteLabel(r, route.Label())

		if route.IsRedirect() {
			target := route.Rewrite(normalized)
			http.Redirect(w, r, target, route.Status)
			return
		}
		// Rewrite the request path.
		normalized = route.Rewrite(normalized)
	}

	// Check if the path should be routed to PHP-FPM.
	if strings.HasSuffix(normalized, ".php") {
		if h.phpAvailable() {
			h.servePHP(w, r, st, normalized, reqID, start)
			return
		}
		// An explicit .php request with no backend is a backend outage, not a
		// missing file. Saying 404 here would send operators looking in the
		// wrong place.
		h.phpUnavailable(w, r, reqID, start)
		return
	}

	// Check for static file.
	rf, err := st.resolver.Resolve(normalized)
	if err == nil {
		defer rf.Close()
		h.serveStatic(w, r, st, rf, reqID, start)
		return
	}

	// Try PHP (front-controller fallback).
	if h.phpAvailable() {
		h.servePHP(w, r, st, normalized, reqID, start)
		return
	}

	// Try directory index.
	indexPath := normalized
	if indexPath == "/" {
		indexPath = "/index.html"
	} else {
		indexPath = indexPath + "/index.html"
	}
	rf2, err := st.resolver.Resolve(indexPath)
	if err == nil {
		defer rf2.Close()
		h.serveStatic(w, r, st, rf2, reqID, start)
		return
	}

	h.devError(w, r, 404, "Not Found", "The requested resource was not found.", reqID, start)
}

// phpUnavailable reports a PHP backend outage with a stable status and a
// Retry-After, so clients and proxies back off instead of hammering a backend
// that is known to be down (§16.5).
func (h *gatewayHandler) phpUnavailable(w http.ResponseWriter, r *http.Request, reqID string, start time.Time) {
	reason := "PHP backend is not available."
	if h.watchdog != nil && h.watchdog.CircuitOpen() {
		reason = "PHP backend is restarting repeatedly and has been temporarily taken out of service."
	} else if h.fpm != nil {
		if err := h.fpm.LastFailure(); err != nil {
			h.logger.Warn("php unavailable", "request_id", reqID, "error", err)
		}
	}

	w.Header().Set("Retry-After", "30")
	h.devError(w, r, http.StatusServiceUnavailable, "Service Unavailable", reason, reqID, start)
}

func (h *gatewayHandler) serveStatic(w http.ResponseWriter, r *http.Request, st *serveState, rf *filesystem.ResolvedFile, reqID string, start time.Time) {
	if st.resolver.IsProtected(r.URL.Path) {
		h.devError(w, r, 403, "Access Denied", "This file is protected.", reqID, start)
		return
	}

	ct := detectMIME(rf.RealPath)
	w.Header().Set("Content-Type", ct)
	w.Header().Set("X-Request-ID", reqID)

	etag := generateETag(rf.Info)
	w.Header().Set("ETag", etag)

	if match := r.Header.Get("If-Match"); match != "" && match != etag {
		w.WriteHeader(http.StatusPreconditionFailed)
		return
	}
	if match := r.Header.Get("If-None-Match"); match == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	if ims := r.Header.Get("If-Modified-Since"); ims != "" {
		if t, err := time.Parse(time.RFC1123, ims); err == nil {
			if !rf.Info.ModTime().After(t) {
				w.WriteHeader(http.StatusNotModified)
				return
			}
		}
	}

	w.Header().Set("Last-Modified", rf.Info.ModTime().UTC().Format(time.RFC1123))
	http.ServeContent(w, r, rf.Info.Name(), rf.Info.ModTime(), rf.F)
	h.logger.Info("static", "request_id", reqID, "path", r.URL.Path,
		"file", rf.RealPath, "duration_ms", time.Since(start).Milliseconds())
}

func (h *gatewayHandler) servePHP(w http.ResponseWriter, r *http.Request, st *serveState, normalized, reqID string, start time.Time) {
	w.Header().Set("X-Request-ID", reqID)

	scriptName, scriptPath := resolveScript(h.docRoot, normalized)
	if scriptPath == "" {
		h.devError(w, r, 404, "Not Found", "No PHP entry point found.", reqID, start)
		return
	}

	resolved, err := st.resolver.ResolveInfo(scriptName)
	if err != nil {
		h.devError(w, r, 404, "Not Found", "Script not found.", reqID, start)
		return
	}
	if resolved == nil || !resolved.Mode().IsRegular() {
		h.devError(w, r, 404, "Not Found", "Script is not a regular file.", reqID, start)
		return
	}

	params := cgi.BuildParams(r, scriptPath, scriptName, h.docRoot)

	client, err := fastcgi.NewClient(h.sockPath, 5*time.Second)
	if err != nil {
		h.logger.Error("fastcgi connect failed", "request_id", reqID, "error", err)
		h.devError(w, r, 502, "Bad Gateway", "Could not connect to PHP backend.", reqID, start)
		return
	}
	defer client.Close()

	// Enforce the configured body limit before handing the request to PHP
	// (§24.1). Content-Length is checked first so an oversized upload is
	// rejected without streaming it, and the reader is still capped in case the
	// header lies or is absent.
	maxBody := st.cfg.Security.MaxBodyBytes()
	if maxBody > 0 && r.ContentLength > maxBody {
		h.logger.Warn("request body too large", "request_id", reqID,
			"content_length", r.ContentLength, "limit", maxBody)
		h.devError(w, r, http.StatusRequestEntityTooLarge, "Payload Too Large",
			fmt.Sprintf("Request body exceeds the configured limit of %d bytes.", maxBody),
			reqID, start)
		return
	}

	var stdin io.Reader
	if r.Body != nil {
		defer r.Body.Close()
		if maxBody > 0 {
			stdin = io.LimitReader(r.Body, maxBody)
		} else {
			stdin = r.Body
		}
	}

	phpTimeout := st.cfg.PHP.RequestTimeout
	if phpTimeout <= 0 {
		phpTimeout = 60 * time.Second
	}
	ctx, cancel := context.WithTimeout(r.Context(), phpTimeout)
	defer cancel()

	type result struct {
		stdout []byte
		stderr []byte
		endReq *fastcgi.EndRequestData
		err    error
	}

	done := make(chan result, 1)
	go func() {
		stdout, stderr, endReq, err := client.Execute(params, stdin)
		done <- result{stdout, stderr, endReq, err}
	}()

	select {
	case <-ctx.Done():
		h.logger.Warn("php timeout", "request_id", reqID, "duration_ms", time.Since(start).Milliseconds())
		h.devError(w, r, 504, "Gateway Timeout", "PHP execution timed out.", reqID, start)
		return
	case res := <-done:
		if res.err != nil {
			h.logger.Error("php execution failed", "request_id", reqID, "error", res.err)
			h.devError(w, r, 502, "Bad Gateway", fmt.Sprintf("PHP error: %v", res.err), reqID, start)
			return
		}

		resp, err := cgi.ParseResponse(res.stdout, res.stderr)
		if err != nil {
			h.logger.Error("php response parse failed", "request_id", reqID, "error", err)
			h.devError(w, r, 502, "Bad Gateway", "Invalid PHP response.", reqID, start)
			return
		}

		if res.endReq != nil && res.endReq.ProtocolStatus != fastcgi.ProtocolRequestComplete {
			h.logger.Error("php protocol error", "request_id", reqID,
				"app_status", res.endReq.AppStatus,
				"proto_status", res.endReq.ProtocolStatus)
		}

		for k, vv := range resp.Headers {
			for _, v := range vv {
				w.Header().Add(k, v)
			}
		}
		w.WriteHeader(resp.StatusCode)
		io.Copy(w, resp.Body)

		h.logger.Info("php", "request_id", reqID, "path", r.URL.Path,
			"status", resp.StatusCode, "duration_ms", time.Since(start).Milliseconds())
	}
}

func (h *gatewayHandler) devError(w http.ResponseWriter, r *http.Request, status int, title, detail, reqID string, start time.Time) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)

	fmt.Fprintf(w, `<!DOCTYPE html>
<html><head><title>%d %s</title>
<style>body{font-family:monospace;margin:40px;background:#1a1a2e;color:#e0e0e0}
h1{color:#ff6b6b}h2{color:#ffd93d}pre{background:#16213e;padding:20px;border-radius:8px;overflow-x:auto}
.label{color:#a0a0a0}.val{color:#4ecdc4}</style></head>
<body>
<h1>%d %s</h1>
<p>%s</p>
<table>
<tr><td class="label">Request ID</td><td class="val">%s</td></tr>
<tr><td class="label">Path</td><td class="val">%s</td></tr>
<tr><td class="label">Method</td><td class="val">%s</td></tr>
<tr><td class="label">Duration</td><td class="val">%dms</td></tr>
</table>
<pre>gateway %s</pre>
</body></html>`,
		status, title, status, title, detail,
		reqID, r.URL.Path, r.Method,
		time.Since(start).Milliseconds(),
		buildinfo.Get().Version,
	)
}

func resolveScript(docRoot, normalized string) (scriptName, scriptPath string) {
	if strings.HasSuffix(normalized, ".php") {
		sp := filepath.Join(docRoot, normalized[1:])
		if _, err := os.Stat(sp); err == nil {
			return normalized, sp
		}
	}
	for _, entry := range []string{"public/index.php", "index.php"} {
		sp := filepath.Join(docRoot, entry)
		if _, err := os.Stat(sp); err == nil {
			return "/" + entry, sp
		}
	}
	return "", ""
}

func detectRuntimes(binary string) []string {
	runtimes := []string{}
	// Check common PHP-FPM binary names
	for _, v := range []string{"8.3", "8.2", "8.1", "8.0"} {
		path := "/usr/sbin/php-fpm" + v
		if _, err := os.Stat(path); err == nil {
			runtimes = append(runtimes, v)
		}
	}
	if len(runtimes) == 0 && binary != "" {
		runtimes = append(runtimes, "unknown")
	}
	return runtimes
}

// detectFPMBinary finds the PHP-FPM binary on the system.
func detectFPMBinary() string {
	candidates := []string{
		"/usr/sbin/php-fpm8.3",
		"/usr/sbin/php-fpm8.2",
		"/usr/sbin/php-fpm8.1",
		"/usr/sbin/php-fpm8.0",
		"/usr/sbin/php-fpm",
		"/usr/bin/php-fpm8.3",
		"/usr/bin/php-fpm8.2",
		"/usr/bin/php-fpm",
	}
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

func detectFramework(root string) (framework, docRoot string) {
	checks := []struct {
		file   string
		name   string
		pubDir string
	}{
		{"artisan", "Laravel", "public"},
		{"public/index.php", "Laravel", "public"},
		{"bin/console", "Symfony", "public"},
		{"wp-config.php", "WordPress", "."},
		{"composer.json", "PHP/Composer", "."},
	}
	for _, c := range checks {
		if _, err := os.Stat(filepath.Join(root, c.file)); err == nil {
			pub := filepath.Join(root, c.pubDir)
			if info, err := os.Stat(pub); err == nil && info.IsDir() {
				return c.name, pub
			}
			return c.name, ""
		}
	}
	return "", ""
}

func generateETag(info os.FileInfo) string {
	return fmt.Sprintf(`"w/%d-%d"`, info.Size(), info.ModTime().UnixNano())
}

func detectMIME(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".html", ".htm":
		return "text/html; charset=utf-8"
	case ".css":
		return "text/css; charset=utf-8"
	case ".js":
		return "application/javascript; charset=utf-8"
	case ".json":
		return "application/json; charset=utf-8"
	case ".xml":
		return "application/xml; charset=utf-8"
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".gif":
		return "image/gif"
	case ".svg":
		return "image/svg+xml"
	case ".ico":
		return "image/x-icon"
	case ".woff":
		return "font/woff"
	case ".woff2":
		return "font/woff2"
	case ".ttf":
		return "font/ttf"
	case ".txt":
		return "text/plain; charset=utf-8"
	case ".pdf":
		return "application/pdf"
	default:
		return "application/octet-stream"
	}
}

// Ensure errors package is used.
var _ = errors.IsCode
