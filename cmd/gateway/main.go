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
	"github.com/go-php/gateway/internal/errors"
	"github.com/go-php/gateway/internal/filesystem"
	"github.com/go-php/gateway/internal/php/cgi"
	"github.com/go-php/gateway/internal/php/fastcgi"
	"github.com/go-php/gateway/internal/supervisor"
)

func main() {
	serveCmd := flag.NewFlagSet("serve", flag.ExitOnError)
	phpFPM := serveCmd.String("php-fpm", "", "path to php-fpm binary")
	addr := serveCmd.String("addr", ":8080", "listen address")

	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "Usage: gateway <command> [flags]\n")
		fmt.Fprintf(os.Stderr, "Commands: serve, version\n")
		os.Exit(2)
	}

	switch os.Args[1] {
	case "serve":
		serveCmd.Parse(os.Args[2:])
		if err := runServe(*addr, *phpFPM, serveCmd.Args()); err != nil {
			slog.Error("serve failed", "error", err)
			os.Exit(1)
		}
	case "version":
		info := buildinfo.Get()
		fmt.Printf("gateway %s (commit %s, built %s, %s)\n",
			info.Version, info.Commit, info.BuildDate, info.GoVersion)
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", os.Args[1])
		os.Exit(2)
	}
}

func runServe(addr, phpFPMPath string, args []string) error {
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
		"addr", addr,
	)

	// Detect framework and document root.
	framework, pubRoot := detectFramework(absRoot)
	if pubRoot != "" {
		absRoot = pubRoot
	}
	if framework != "" {
		slog.Info("framework detected", "framework", framework, "document_root", absRoot)
	}

	// Create filesystem resolver.
	resolver := filesystem.NewResolver(absRoot, filesystem.SymlinkWithinRoot, filesystem.DefaultProtectedPatterns())

	// Start PHP-FPM if binary provided.
	var fpm *supervisor.Supervisor
	var sockPath string
	if phpFPMPath != "" {
		sockPath = filepath.Join(os.TempDir(), fmt.Sprintf("gateway-%d.sock", os.Getpid()))
		pidPath := filepath.Join(os.TempDir(), fmt.Sprintf("gateway-%d.pid", os.Getpid()))

		fpm = supervisor.New(supervisor.Config{
			PHPBinary:      phpFPMPath,
			SocketPath:     sockPath,
			PIDFile:        pidPath,
			MaxChildren:    20,
			StartServers:   2,
			MinSpare:       2,
			MaxSpare:       6,
			MaxRequests:    500,
			RequestTimeout: 60 * time.Second,
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

	handler := &gatewayHandler{
		docRoot:   absRoot,
		resolver:  resolver,
		fpm:       fpm,
		sockPath:  sockPath,
		logger:    slog.Default(),
	}

	server := &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    1 << 20, // 1MB
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

	slog.Info("listening", "addr", addr)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("listen: %w", err)
	}

	slog.Info("server stopped")
	return nil
}

type gatewayHandler struct {
	docRoot  string
	resolver *filesystem.Resolver
	fpm      *supervisor.Supervisor
	sockPath string
	logger   *slog.Logger
}

func (h *gatewayHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	reqID := fmt.Sprintf("req_%d", start.UnixNano())

	path := r.URL.Path

	// Normalize path.
	pp, err := filesystem.ParsePath(path)
	if err != nil {
		h.logger.Warn("path rejected", "request_id", reqID, "path", path, "error", err)
		h.devError(w, r, 400, "Bad Request", fmt.Sprintf("Path rejected: %v", err), reqID, start)
		return
	}

	normalized := pp.NormalizedPath

	// Check if this is a static file.
	rf, err := h.resolver.Resolve(normalized)
	if err == nil {
		defer rf.Close()
		h.serveStatic(w, r, rf, reqID, start)
		return
	}

	// If file not found and we have PHP-FPM, try PHP.
	if h.fpm != nil && h.fpm.State() == supervisor.StateReady {
		h.servePHP(w, r, normalized, reqID, start)
		return
	}

	// Try directory index.
	if normalized == "/" {
		normalized = "/index.html"
	} else {
		normalized = normalized + "/index.html"
	}
	rf2, err := h.resolver.Resolve(normalized)
	if err == nil {
		defer rf2.Close()
		h.serveStatic(w, r, rf2, reqID, start)
		return
	}

	h.devError(w, r, 404, "Not Found", "The requested resource was not found.", reqID, start)
	h.logger.Info("not found", "request_id", reqID, "path", r.URL.Path,
		"duration_ms", time.Since(start).Milliseconds())
}

func (h *gatewayHandler) serveStatic(w http.ResponseWriter, r *http.Request, rf *filesystem.ResolvedFile, reqID string, start time.Time) {
	// Check for protected file access at resolver level.
	if h.resolver.IsProtected(r.URL.Path) {
		h.devError(w, r, 403, "Access Denied", "This file is protected.", reqID, start)
		return
	}

	ct := detectMIME(rf.RealPath)
	w.Header().Set("Content-Type", ct)
	w.Header().Set("X-Request-ID", reqID)

	// ETag
	etag := generateETag(rf.Info)
	w.Header().Set("ETag", etag)

	// If-Match check.
	if match := r.Header.Get("If-Match"); match != "" && match != etag {
		w.WriteHeader(http.StatusPreconditionFailed)
		return
	}

	// If-None-Match check.
	if match := r.Header.Get("If-None-Match"); match == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}

	// If-Modified-Since check.
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

	// Resolve script — try common entry points.
	scriptName, scriptPath := resolveScript(h.docRoot, normalized)
	if scriptPath == "" {
		h.devError(w, r, 404, "Not Found", "No PHP entry point found.", reqID, start)
		return
	}

	// Verify script is under doc root.
	resolved, err := h.resolver.ResolveInfo(scriptName)
	if err != nil {
		h.devError(w, r, 404, "Not Found", "Script not found.", reqID, start)
		return
	}
	if resolved == nil || !resolved.Mode().IsRegular() {
		h.devError(w, r, 404, "Not Found", "Script is not a regular file.", reqID, start)
		return
	}

	// Build CGI params.
	params := cgi.BuildParams(r, scriptPath, scriptName, h.docRoot)

	// Connect to FPM.
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
		stdin = io.LimitReader(r.Body, 20<<20) // 20MB
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

		// Check for PHP application errors.
		if res.endReq != nil && res.endReq.ProtocolStatus != fastcgi.ProtocolRequestComplete {
			h.logger.Error("php protocol error", "request_id", reqID,
				"app_status", res.endReq.AppStatus,
				"proto_status", res.endReq.ProtocolStatus)
		}

		// Write headers.
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

// devError writes a detailed development error page.
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

// resolveScript determines the PHP script to execute.
func resolveScript(docRoot, normalized string) (scriptName, scriptPath string) {
	// If the path points to a .php file directly.
	if strings.HasSuffix(normalized, ".php") {
		sp := filepath.Join(docRoot, normalized[1:]) // strip leading /
		if _, err := os.Stat(sp); err == nil {
			return normalized, sp
		}
	}

	// Try public/index.php (framework pattern).
	for _, entry := range []string{"public/index.php", "index.php"} {
		sp := filepath.Join(docRoot, entry)
		if _, err := os.Stat(sp); err == nil {
			return "/" + entry, sp
		}
	}

	return "", ""
}

// detectFramework identifies the framework and returns the correct document root.
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
