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
	"syscall"
	"time"

	"github.com/go-php/gateway/internal/buildinfo"
	"github.com/go-php/gateway/internal/filesystem"
	"github.com/go-php/gateway/internal/php/cgi"
	"github.com/go-php/gateway/internal/supervisor"
)

func main() {
	serveCmd := flag.NewFlagSet("serve", flag.ExitOnError)
	phpFPM := serveCmd.String("php-fpm", "", "path to php-fpm binary")
	addr := serveCmd.String("addr", ":8080", "listen address")

	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "Usage: gateway <command> [flags]\n")
		fmt.Fprintf(os.Stderr, "Commands: serve\n")
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

	// Create filesystem resolver.
	resolver := filesystem.NewResolver(absRoot, filesystem.SymlinkWithinRoot, filesystem.DefaultProtectedPatterns())

	// Start PHP-FPM if binary provided.
	var fpm *supervisor.Supervisor
	if phpFPMPath != "" {
		sockPath := filepath.Join(os.TempDir(), fmt.Sprintf("gateway-%d.sock", os.Getpid()))
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
		sockPath:  filepath.Join(os.TempDir(), fmt.Sprintf("gateway-%d.sock", os.Getpid())),
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

	// Graceful shutdown on SIGINT/SIGTERM.
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
		http.Error(w, "400 Bad Request", http.StatusBadRequest)
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

	http.NotFound(w, r)
	h.logger.Info("not found", "request_id", reqID, "path", r.URL.Path,
		"duration_ms", time.Since(start).Milliseconds())
}

func (h *gatewayHandler) serveStatic(w http.ResponseWriter, r *http.Request, rf *filesystem.ResolvedFile, reqID string, start time.Time) {
	w.Header().Set("Content-Type", detectMIME(rf.RealPath))
	http.ServeContent(w, r, rf.Info.Name(), rf.Info.ModTime(), rf.F)
	h.logger.Info("static", "request_id", reqID, "path", r.URL.Path,
		"file", rf.RealPath, "duration_ms", time.Since(start).Milliseconds())
}

func (h *gatewayHandler) servePHP(w http.ResponseWriter, r *http.Request, normalized, reqID string, start time.Time) {
	// Resolve script.
	scriptName := "/index.php"
	scriptPath := filepath.Join(h.docRoot, "public", "index.php")

	// Check if the script exists.
	if _, err := os.Stat(scriptPath); err != nil {
		// Try docRoot/index.php.
		scriptPath = filepath.Join(h.docRoot, "index.php")
		if _, err := os.Stat(scriptPath); err != nil {
			http.NotFound(w, r)
			return
		}
	}

	// Build CGI params.
	params := cgi.BuildParams(r, scriptPath, scriptName, filepath.Join(h.docRoot, "public"))

	// Connect to FPM and execute.
	client, err := h.connectFPM()
	if err != nil {
		h.logger.Error("fastcgi connect failed", "request_id", reqID, "error", err)
		http.Error(w, "502 Bad Gateway", http.StatusBadGateway)
		return
	}
	defer client.Close()

	var stdin io.Reader
	if r.Body != nil {
		defer r.Body.Close()
		// Limit body size.
		stdin = io.LimitReader(r.Body, 20<<20) // 20MB
	}

	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()

	type result struct {
		stdout []byte
		stderr []byte
		endReq interface{}
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
		http.Error(w, "504 Gateway Timeout", http.StatusGatewayTimeout)
		return
	case res := <-done:
		if res.err != nil {
			h.logger.Error("php execution failed", "request_id", reqID, "error", res.err)
			http.Error(w, "502 Bad Gateway", http.StatusBadGateway)
			return
		}

		resp, err := cgi.ParseResponse(res.stdout, res.stderr)
		if err != nil {
			h.logger.Error("php response parse failed", "request_id", reqID, "error", err)
			http.Error(w, "502 Bad Gateway", http.StatusBadGateway)
			return
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

func (h *gatewayHandler) connectFPM() (*fastcgiClient, error) {
	// Try to connect to the FPM socket.
	conn, err := net.DialTimeout("unix", h.sockPath, 5*time.Second)
	if err != nil {
		return nil, fmt.Errorf("dial FPM: %w", err)
	}
	return &fastcgiClient{conn: conn}, nil
}

// fastcgiClient is a minimal wrapper for the FPM connection.
// It uses the protocol from internal/php/fastcgi but inline for Phase 0 simplicity.
type fastcgiClient struct {
	conn net.Conn
}

func (c *fastcgiClient) Close() error {
	return c.conn.Close()
}

func (c *fastcgiClient) Execute(params map[string]string, stdin io.Reader) ([]byte, []byte, interface{}, error) {
	// For Phase 0, use a direct protocol implementation.
	// This will be replaced with the full fastcgi.Client later.
	return nil, nil, nil, fmt.Errorf("fastcgi: not yet wired (Phase 0 stub)")
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
