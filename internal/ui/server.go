package ui

import (
	"embed"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"sync"
	"time"
)

//go:embed static/*
var staticFiles embed.FS

// ServerConfig holds UI server settings.
type ServerConfig struct {
	Addr     string `yaml:"addr"`
	SockPath string `yaml:"sock_path"`
}

// DefaultConfig returns sensible defaults.
func DefaultConfig() ServerConfig {
	return ServerConfig{
		Addr: "127.0.0.1:30200",
	}
}

// Server is the management UI server.
type Server struct {
	cfg       ServerConfig
	logger    *slog.Logger
	status    *StatusProvider
	mux       *http.ServeMux
	server    *http.Server
	logBuffer *LogBuffer
	siteMgr   *SiteManager
}

// NewServer creates a new UI server.
func NewServer(cfg ServerConfig, logger *slog.Logger, status *StatusProvider) *Server {
	s := &Server{
		cfg:       cfg,
		logger:    logger,
		status:    status,
		mux:       http.NewServeMux(),
		logBuffer: NewLogBuffer(500),
		siteMgr:   NewSiteManager(logger, cfg.SockPath),
	}
	s.routes()
	return s
}

func (s *Server) routes() {
	// API routes
	s.mux.HandleFunc("/api/status", s.handleStatus)
	s.mux.HandleFunc("/api/sites", s.handleSites)
	s.mux.HandleFunc("/api/sites/", s.handleSiteByID)
	s.mux.HandleFunc("/api/config", s.handleConfig)
	s.mux.HandleFunc("/api/config/validate", s.handleConfigValidate)
	s.mux.HandleFunc("/api/config/save", s.handleConfigSave)
	s.mux.HandleFunc("/api/runtimes", s.handleRuntimes)
	s.mux.HandleFunc("/api/logs", s.handleLogs)
	s.mux.HandleFunc("/api/logs/recent", s.handleRecentLogs)
	s.mux.HandleFunc("/api/health", s.handleHealth)
	s.mux.HandleFunc("/api/system", s.handleSystem)

	// Static files
	staticFS, err := fs.Sub(staticFiles, "static")
	if err != nil {
		s.logger.Error("failed to create static FS", "error", err)
		return
	}

	fileServer := http.FileServer(http.FS(staticFS))
	s.mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// Serve static files, but for SPA routes serve index.html
		path := r.URL.Path
		if path == "/" || path == "" {
			r.URL.Path = "/"
			fileServer.ServeHTTP(w, r)
			return
		}

		// Check if file exists in static
		f, err := staticFS.Open(path[1:]) // strip leading /
		if err != nil {
			// SPA fallback: serve index.html
			r.URL.Path = "/"
			fileServer.ServeHTTP(w, r)
			return
		}
		f.Close()
		fileServer.ServeHTTP(w, r)
	})
}

// Start begins listening.
func (s *Server) Start() error {
	s.server = &http.Server{
		Addr:              s.cfg.Addr,
		Handler:           s.mux,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	s.logger.Info("management UI starting", "addr", s.cfg.Addr)

	go func() {
		if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			s.logger.Error("UI server error", "error", err)
		}
	}()

	return nil
}

// Stop gracefully shuts down the UI server.
func (s *Server) Stop() error {
	if s.siteMgr != nil {
		s.siteMgr.StopAll()
	}
	if s.server == nil {
		return nil
	}
	s.logger.Info("management UI stopping")
	return s.server.Close()
}

// Addr returns the listening address.
func (s *Server) Addr() string {
	return s.cfg.Addr
}

// LogBuffer stores recent log entries for the UI.
type LogBuffer struct {
	entries []LogEntry
	max     int
	mu      sync.RWMutex
}

// LogEntry is a single log entry for the UI.
type LogEntry struct {
	Timestamp time.Time `json:"timestamp"`
	Level     string    `json:"level"`
	Message   string    `json:"message"`
	Extra     any       `json:"extra,omitempty"`
}

// NewLogBuffer creates a log buffer with max capacity.
func NewLogBuffer(max int) *LogBuffer {
	return &LogBuffer{
		entries: make([]LogEntry, 0, max),
		max:     max,
	}
}

// Add appends a log entry.
func (lb *LogBuffer) Add(entry LogEntry) {
	lb.mu.Lock()
	defer lb.mu.Unlock()

	lb.entries = append(lb.entries, entry)
	if len(lb.entries) > lb.max {
		lb.entries = lb.entries[len(lb.entries)-lb.max:]
	}
}

// Recent returns the last n entries.
func (lb *LogBuffer) Recent(n int) []LogEntry {
	lb.mu.RLock()
	defer lb.mu.RUnlock()

	total := len(lb.entries)
	if n > total {
		n = total
	}
	result := make([]LogEntry, n)
	copy(result, lb.entries[total-n:])
	return result
}

// Size returns the current number of entries.
func (lb *LogBuffer) Size() int {
	lb.mu.RLock()
	defer lb.mu.RUnlock()
	return len(lb.entries)
}

// FormatAddr returns a formatted address string.
func FormatAddr(addr string) string {
	return fmt.Sprintf("http://%s", addr)
}
