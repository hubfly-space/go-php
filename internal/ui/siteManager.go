package ui

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/go-php/gateway/internal/filesystem"
	"github.com/go-php/gateway/internal/php/cgi"
	"github.com/go-php/gateway/internal/php/fastcgi"
)

// SiteManager manages per-site HTTP servers.
type SiteManager struct {
	servers  map[string]*runningSite
	mu       sync.RWMutex
	logger   *slog.Logger
	sockPath string
}

type runningSite struct {
	config SiteConfig
	server *http.Server
}

// NewSiteManager creates a new site manager.
func NewSiteManager(logger *slog.Logger, sockPath string) *SiteManager {
	// Auto-detect FPM socket if not provided or doesn't exist.
	if sockPath == "" || !socketExists(sockPath) {
		if detected := detectFPMSocket(); detected != "" {
			sockPath = detected
			logger.Info("auto-detected FPM socket", "path", sockPath)
		}
	}
	return &SiteManager{
		servers:  make(map[string]*runningSite),
		logger:   logger,
		sockPath: sockPath,
	}
}

func socketExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().Type()&os.ModeSocket != 0
}

func detectFPMSocket() string {
	candidates := []string{
		"/run/php/php-fpm.sock",
		"/run/php/php8.3-fpm.sock",
		"/run/php/php8.2-fpm.sock",
		"/run/php/php8.1-fpm.sock",
		"/run/php/php8.0-fpm.sock",
		"/var/run/php/php-fpm.sock",
		"/tmp/php-fpm.sock",
	}
	for _, p := range candidates {
		if socketExists(p) {
			return p
		}
	}
	return ""
}

// StartSite starts an HTTP server for the given site configuration.
func (sm *SiteManager) StartSite(cfg SiteConfig) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if _, exists := sm.servers[cfg.ID]; exists {
		return fmt.Errorf("site %s is already running", cfg.ID)
	}

	if cfg.Webroot == "" {
		return fmt.Errorf("webroot is required")
	}

	info, err := os.Stat(cfg.Webroot)
	if err != nil {
		return fmt.Errorf("webroot %s: %w", cfg.Webroot, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("webroot %s is not a directory", cfg.Webroot)
	}

	addr := fmt.Sprintf("127.0.0.1:%d", cfg.Port)
	handler := newSiteHandler(cfg.Webroot, cfg.ID, sm.sockPath, sm.logger)

	srv := &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		sm.logger.Info("site server starting", "site", cfg.ID, "addr", addr, "webroot", cfg.Webroot)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			sm.logger.Error("site server failed", "site", cfg.ID, "error", err)
			errCh <- err
		}
	}()

	// Give it a moment to confirm the listener opened.
	select {
	case err := <-errCh:
		return fmt.Errorf("start site %s: %w", cfg.ID, err)
	case <-time.After(200 * time.Millisecond):
	}

	sm.servers[cfg.ID] = &runningSite{config: cfg, server: srv}
	sm.logger.Info("site server started", "site", cfg.ID, "addr", addr)
	return nil
}

// StopSite stops the HTTP server for the given site ID.
func (sm *SiteManager) StopSite(siteID string) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	rs, exists := sm.servers[siteID]
	if !exists {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := rs.server.Shutdown(ctx); err != nil {
		sm.logger.Warn("site server shutdown error", "site", siteID, "error", err)
	}

	delete(sm.servers, siteID)
	sm.logger.Info("site server stopped", "site", siteID)
	return nil
}

// StopAll stops all running site servers.
func (sm *SiteManager) StopAll() {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	for id, rs := range sm.servers {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		if err := rs.server.Shutdown(ctx); err != nil {
			sm.logger.Warn("site server shutdown error", "site", id, "error", err)
		}
		cancel()
	}
	sm.servers = make(map[string]*runningSite)
}

// IsRunning checks if a site server is running.
func (sm *SiteManager) IsRunning(siteID string) bool {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	_, exists := sm.servers[siteID]
	return exists
}

// siteHandler serves static files from a site's webroot.
type siteHandler struct {
	docRoot  string
	siteID   string
	sockPath string
	logger   *slog.Logger

	// resolver enforces the same filesystem boundary as the main gateway.
	// This handler used to do its own filepath.Clean plus a prefix check, with
	// no protected-pattern list at all — so `.env` under a site webroot was
	// served, and a prefix check without a separator boundary let /var/wwwroot
	// pass for a docRoot of /var/www. Two request pipelines with two security
	// postures means every fix to one silently misses the other.
	resolver *filesystem.Resolver
}

// newSiteHandler builds a handler with the hardened resolver.
func newSiteHandler(docRoot, siteID, sockPath string, logger *slog.Logger) *siteHandler {
	return &siteHandler{
		docRoot:  docRoot,
		siteID:   siteID,
		sockPath: sockPath,
		logger:   logger,
		resolver: filesystem.NewResolver(docRoot,
			filesystem.SymlinkWithinRoot, filesystem.DefaultProtectedPatterns()),
	}
}

func (h *siteHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	start := time.Now()

	// Parse the path with the same rules the main gateway uses: single-pass
	// decoding, NUL and control rejection, encoded-slash rejection, dot-segment
	// collapsing.
	pp, err := filesystem.ParsePath(r.URL.Path)
	if err != nil {
		h.logger.Warn("site path rejected", "site", h.siteID, "path", r.URL.Path, "error", err)
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}
	normalized := pp.NormalizedPath

	if h.resolver.IsProtected(normalized) {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	// Resolve the request path, falling back to index files.
	candidates := []string{normalized}
	if strings.HasSuffix(normalized, "/") {
		candidates = []string{normalized + "index.php", normalized + "index.html"}
	} else {
		candidates = append(candidates,
			normalized+"/index.php", normalized+"/index.html")
	}

	for _, candidate := range candidates {
		resolved, resErr := h.resolver.ResolveInfo(candidate)
		if resErr != nil || resolved == nil || !resolved.Mode().IsRegular() {
			continue
		}
		if h.resolver.IsProtected(candidate) {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}

		absPath := filepath.Join(h.docRoot, strings.TrimPrefix(candidate, "/"))

		if strings.HasSuffix(candidate, ".php") && h.sockPath != "" {
			h.servePHP(w, r, absPath, start)
			return
		}

		f, openErr := os.Open(absPath)
		if openErr != nil {
			continue
		}
		defer f.Close()

		info, statErr := f.Stat()
		if statErr != nil || info.IsDir() {
			continue
		}

		w.Header().Set("Content-Type", detectSiteMIME(absPath))
		w.Header().Set("X-Request-ID", fmt.Sprintf("site_%d", time.Now().UnixNano()))
		http.ServeContent(w, r, info.Name(), info.ModTime(), f)

		h.logger.Info("site request", "site", h.siteID, "path", r.URL.Path,
			"file", absPath, "duration_ms", time.Since(start).Milliseconds())
		return
	}

	http.NotFound(w, r)
	h.logger.Info("site 404", "site", h.siteID, "path", r.URL.Path,
		"duration_ms", time.Since(start).Milliseconds())
}

func (h *siteHandler) servePHP(w http.ResponseWriter, r *http.Request, absPath string, start time.Time) {
	reqID := fmt.Sprintf("site_%d", time.Now().UnixNano())
	w.Header().Set("X-Request-ID", reqID)

	if h.sockPath == "" || !socketExists(h.sockPath) {
		h.logger.Error("php-fpm socket not available", "site", h.siteID, "sockPath", h.sockPath)
		http.Error(w, "Bad Gateway: PHP-FPM socket not available.", http.StatusBadGateway)
		return
	}

	// Compute SCRIPT_FILENAME and SCRIPT_NAME relative to docRoot.
	rel, _ := filepath.Rel(h.docRoot, absPath)
	scriptName := "/" + filepath.ToSlash(rel)
	scriptPath := absPath

	params := cgi.BuildParams(r, scriptPath, scriptName, h.docRoot)

	client, err := fastcgi.NewClient(h.sockPath, 5*time.Second)
	if err != nil {
		h.logger.Error("fastcgi connect failed", "site", h.siteID, "error", err)
		http.Error(w, "Bad Gateway: Could not connect to PHP backend.", http.StatusBadGateway)
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
		h.logger.Warn("php timeout", "site", h.siteID, "path", r.URL.Path, "duration_ms", time.Since(start).Milliseconds())
		http.Error(w, "Gateway Timeout", http.StatusGatewayTimeout)
		return
	case res := <-done:
		if res.err != nil {
			h.logger.Error("php execution failed", "site", h.siteID, "error", res.err)
			http.Error(w, fmt.Sprintf("Bad Gateway: %v", res.err), http.StatusBadGateway)
			return
		}

		resp, err := cgi.ParseResponse(res.stdout, res.stderr)
		if err != nil {
			h.logger.Error("php response parse failed", "site", h.siteID, "error", err)
			http.Error(w, "Bad Gateway: Invalid PHP response.", http.StatusBadGateway)
			return
		}

		if res.endReq != nil && res.endReq.ProtocolStatus != fastcgi.ProtocolRequestComplete {
			h.logger.Error("php protocol error", "site", h.siteID,
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

		h.logger.Info("php", "site", h.siteID, "path", r.URL.Path,
			"script", absPath, "status", resp.StatusCode, "duration_ms", time.Since(start).Milliseconds())
	}
}

func detectSiteMIME(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	if ct := mime.TypeByExtension(ext); ct != "" {
		return ct
	}

	switch ext {
	case ".php":
		return "text/html; charset=utf-8"
	case ".js":
		return "application/javascript; charset=utf-8"
	case ".json":
		return "application/json; charset=utf-8"
	case ".svg":
		return "image/svg+xml"
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".css":
		return "text/css; charset=utf-8"
	case ".html", ".htm":
		return "text/html; charset=utf-8"
	case ".txt":
		return "text/plain; charset=utf-8"
	case ".xml":
		return "application/xml; charset=utf-8"
	case ".pdf":
		return "application/pdf"
	case ".woff":
		return "font/woff"
	case ".woff2":
		return "font/woff2"
	case ".ttf":
		return "font/ttf"
	case ".ico":
		return "image/x-icon"
	case ".gif":
		return "image/gif"
	default:
		return "application/octet-stream"
	}
}

// Ensure io is used (for future PHP proxy support).
var _ = io.Copy
