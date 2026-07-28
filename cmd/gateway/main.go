package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/go-php/gateway/internal/buildinfo"
	"github.com/go-php/gateway/internal/config"
	"github.com/go-php/gateway/internal/errors"
	"github.com/go-php/gateway/internal/filesystem"
	"github.com/go-php/gateway/internal/observability"
	"github.com/go-php/gateway/internal/php/cgi"
	"github.com/go-php/gateway/internal/php/fastcgi"
	"github.com/go-php/gateway/internal/router"
	"github.com/go-php/gateway/internal/supervisor"
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
		serveCmd.Parse(args)
		err = runServe(*addr, *phpFPM, *configPath, serveCmd.Args(), *uiAddrFlag)
	case "init":
		initCmd.Parse(args)
		err = runInit(*initFramework, *initPHP, initCmd.Args())
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

	// Build routes from config.
	var routes []router.Route
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
	routingEngine, err := router.NewEngine(routes)
	if err != nil {
		return fmt.Errorf("build routes: %w", err)
	}

	// Start PHP-FPM.
	var fpm *supervisor.Supervisor
	sockPath := cfg.PHP.SocketPath
	if cfg.PHP.Binary != "" {
		if sockPath == "" {
			sockPath = filepath.Join(os.TempDir(), fmt.Sprintf("gateway-%d.sock", os.Getpid()))
		}
		pidPath := filepath.Join(os.TempDir(), fmt.Sprintf("gateway-%d.pid", os.Getpid()))

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
		})

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := fpm.Start(ctx); err != nil {
			slog.Warn("could not start php-fpm, PHP requests will fail", "error", err)
		} else {
			slog.Info("php-fpm started", "socket", sockPath)
			defer fpm.Stop(context.Background())
		}
	}

	// Start management UI server.
	uiCfg := ui.DefaultConfig()
	if uiAddr != "" {
		uiCfg.Addr = uiAddr
	}
	uiCfg.SockPath = sockPath
	statusProvider := ui.NewStatusProvider(buildinfo.Get().Version, cfg.Server.Addr, absRoot, framework)
	statusProvider.Runtimes = detectRuntimes(cfg.PHP.Binary)

	uiServer := ui.NewServer(uiCfg, slog.Default(), statusProvider)
	if err := uiServer.Start(); err != nil {
		slog.Warn("could not start management UI", "error", err)
	} else {
		slog.Info("management UI started", "addr", ui.FormatAddr(uiCfg.Addr))
		defer uiServer.Stop()
	}

	logger := slog.Default()

	handler := &gatewayHandler{
		docRoot:       absRoot,
		resolver:      resolver,
		fpm:           fpm,
		sockPath:      sockPath,
		routingEngine: routingEngine,
		logger:        logger,
		cfg:           cfg,
	}

	var h http.Handler = handler
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

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-quit
		slog.Info("shutting down gracefully...")
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		server.Shutdown(ctx)
	}()

	slog.Info("listening", "addr", cfg.Server.Addr)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("listen: %w", err)
	}

	slog.Info("server stopped")
	return nil
}

type gatewayHandler struct {
	docRoot       string
	resolver      *filesystem.Resolver
	fpm           *supervisor.Supervisor
	sockPath      string
	routingEngine *router.Engine
	logger        *slog.Logger
	cfg           *config.Config
}

func (h *gatewayHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	reqID := fmt.Sprintf("req_%d", start.UnixNano())
	w.Header().Set("X-Request-ID", reqID)

	path := r.URL.Path

	pp, err := filesystem.ParsePath(path)
	if err != nil {
		h.logger.Warn("path rejected", "request_id", reqID, "path", path, "error", err)
		h.devError(w, r, 400, "Bad Request", fmt.Sprintf("Path rejected: %v", err), reqID, start)
		return
	}
	normalized := pp.NormalizedPath

	// Check routing rules.
	if route := h.routingEngine.Match(r); route != nil {
		if route.IsRedirect() {
			target := route.Rewrite(normalized)
			http.Redirect(w, r, target, route.Status)
			return
		}
		// Rewrite the request path.
		normalized = route.Rewrite(normalized)
	}

	// Check for static file.
	rf, err := h.resolver.Resolve(normalized)
	if err == nil {
		defer rf.Close()
		h.serveStatic(w, r, rf, reqID, start)
		return
	}

	// Try PHP.
	if h.fpm != nil && h.fpm.State() == supervisor.StateReady {
		h.servePHP(w, r, normalized, reqID, start)
		return
	}

	// Try directory index.
	indexPath := normalized
	if indexPath == "/" {
		indexPath = "/index.html"
	} else {
		indexPath = indexPath + "/index.html"
	}
	rf2, err := h.resolver.Resolve(indexPath)
	if err == nil {
		defer rf2.Close()
		h.serveStatic(w, r, rf2, reqID, start)
		return
	}

	h.devError(w, r, 404, "Not Found", "The requested resource was not found.", reqID, start)
}

func (h *gatewayHandler) serveStatic(w http.ResponseWriter, r *http.Request, rf *filesystem.ResolvedFile, reqID string, start time.Time) {
	if h.resolver.IsProtected(r.URL.Path) {
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

func (h *gatewayHandler) servePHP(w http.ResponseWriter, r *http.Request, normalized, reqID string, start time.Time) {
	w.Header().Set("X-Request-ID", reqID)

	scriptName, scriptPath := resolveScript(h.docRoot, normalized)
	if scriptPath == "" {
		h.devError(w, r, 404, "Not Found", "No PHP entry point found.", reqID, start)
		return
	}

	resolved, err := h.resolver.ResolveInfo(scriptName)
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

	var stdin io.Reader
	if r.Body != nil {
		defer r.Body.Close()
		stdin = io.LimitReader(r.Body, 20<<20)
	}

	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
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
