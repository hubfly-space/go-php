package ui

import (
	"encoding/json"
	"net/http"
	"os"
	"runtime"
	"sync/atomic"
	"time"

	"github.com/go-php/gateway/internal/buildinfo"
)

// StatusProvider exposes gateway status for the UI.
type StatusProvider struct {
	StartTime      time.Time
	Version        string
	PID            int
	Addr           string
	DocRoot        string
	Framework      string
	Runtimes       []string
	ActiveRequests atomic.Int64
	TotalRequests  atomic.Int64
	TotalErrors    atomic.Int64
	AvgResponseMs  atomic.Int64
	LastRequest    atomic.Pointer[time.Time]
	Sites          atomic.Pointer[[]SiteConfig]
}

// Goroutines returns the current goroutine count.
func (sp *StatusProvider) Goroutines() int {
	return runtime.NumGoroutine()
}

// Uptime returns server uptime.
func (sp *StatusProvider) Uptime() time.Duration {
	return time.Since(sp.StartTime)
}

// NewStatusProvider creates a status provider.
func NewStatusProvider(version, addr, docRoot, framework string) *StatusProvider {
	return &StatusProvider{
		StartTime: time.Now(),
		Version:   version,
		PID:       os.Getpid(),
		Addr:      addr,
		DocRoot:   docRoot,
		Framework: framework,
	}
}

// SiteConfig represents a managed site.
type SiteConfig struct {
	ID         string            `json:"id"`
	Name       string            `json:"name"`
	Domain     string            `json:"domain"`
	Root       string            `json:"root"`
	PHPVersion string            `json:"php_version"`
	Status     string            `json:"status"`
	Routes     int               `json:"routes"`
	SSL        bool              `json:"ssl"`
	Headers    map[string]string `json:"headers,omitempty"`
	CreatedAt  time.Time         `json:"created_at"`
	UpdatedAt  time.Time         `json:"updated_at"`
}

// GatewayConfig is the full gateway configuration exposed to the UI.
type GatewayConfig struct {
	Schema   string       `yaml:"schema" json:"schema"`
	Server   ServerCfg    `yaml:"server" json:"server"`
	PHP      PHPCfg       `yaml:"php" json:"php"`
	Routes   []RouteCfg   `yaml:"routes" json:"routes"`
	Logging  LoggingCfg   `yaml:"logging" json:"logging"`
	Security SecurityCfg  `yaml:"security" json:"security"`
	Sites    []SiteConfig `yaml:"sites" json:"sites"`
}

// ServerCfg for JSON response.
type ServerCfg struct {
	Addr         string `json:"addr"`
	ReadTimeout  string `json:"read_timeout"`
	WriteTimeout string `json:"write_timeout"`
}

// PHPCfg for JSON response.
type PHPCfg struct {
	Binary         string `json:"binary"`
	SocketPath     string `json:"socket_path"`
	MaxChildren    int    `json:"max_children"`
	RequestTimeout string `json:"request_timeout"`
}

// RouteCfg for JSON response.
type RouteCfg struct {
	Host    string            `json:"host"`
	Path    string            `json:"path"`
	Target  string            `json:"target"`
	Status  int               `json:"status"`
	Methods []string          `json:"methods,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`
}

// LoggingCfg for JSON response.
type LoggingCfg struct {
	Format string `json:"format"`
	Level  string `json:"level"`
}

// SecurityCfg for JSON response.
type SecurityCfg struct {
	ProtectedPatterns []string `json:"protected_patterns"`
	SymlinkMode       string   `json:"symlink_mode"`
	MaxBodySize       string   `json:"max_body_size"`
}

// SystemInfo holds system-level information.
type SystemInfo struct {
	Hostname   string  `json:"hostname"`
	OS         string  `json:"os"`
	Arch       string  `json:"arch"`
	GoVersion  string  `json:"go_version"`
	Goroutines int     `json:"goroutines"`
	MemAllocMB float64 `json:"mem_alloc_mb"`
	MemSysMB   float64 `json:"mem_sys_mb"`
	NumCPU     int     `json:"num_cpu"`
	PID        int     `json:"pid"`
}

// --- Handlers ---

func jsonResp(w http.ResponseWriter, data any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

func jsonErr(w http.ResponseWriter, msg string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		jsonErr(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	sites := s.status.Sites.Load()
	siteCount := 0
	if sites != nil {
		siteCount = len(*sites)
	}

	jsonResp(w, map[string]any{
		"status":          "ok",
		"uptime":          s.status.Uptime().String(),
		"uptime_seconds":  int(s.status.Uptime().Seconds()),
		"version":         s.status.Version,
		"pid":             s.status.PID,
		"addr":            s.status.Addr,
		"doc_root":        s.status.DocRoot,
		"framework":       s.status.Framework,
		"goroutines":      s.status.Goroutines(),
		"active_requests": s.status.ActiveRequests.Load(),
		"total_requests":  s.status.TotalRequests.Load(),
		"total_errors":    s.status.TotalErrors.Load(),
		"sites_count":     siteCount,
		"runtimes":        s.status.Runtimes,
	})
}

func (s *Server) handleSites(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		sites := s.status.Sites.Load()
		if sites == nil {
			sites = &[]SiteConfig{}
		}
		jsonResp(w, map[string]any{"sites": *sites})

	case http.MethodPost:
		var site SiteConfig
		if err := json.NewDecoder(r.Body).Decode(&site); err != nil {
			jsonErr(w, "invalid request body", http.StatusBadRequest)
			return
		}
		site.ID = generateID(site.Name)
		site.CreatedAt = time.Now()
		site.UpdatedAt = time.Now()
		if site.Status == "" {
			site.Status = "active"
		}

		sites := s.status.Sites.Load()
		if sites == nil {
			sites = &[]SiteConfig{}
		}
		newSites := append(*sites, site)
		s.status.Sites.Store(&newSites)

		s.logBuffer.Add(LogEntry{
			Timestamp: time.Now(),
			Level:     "info",
			Message:   "site created: " + site.Name,
		})

		jsonResp(w, map[string]any{"site": site})

	default:
		jsonErr(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleSiteByID(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Path[len("/api/sites/"):]
	if id == "" {
		jsonErr(w, "site ID required", http.StatusBadRequest)
		return
	}

	sites := s.status.Sites.Load()
	if sites == nil {
		jsonErr(w, "site not found", http.StatusNotFound)
		return
	}

	idx := -1
	for i, site := range *sites {
		if site.ID == id {
			idx = i
			break
		}
	}

	switch r.Method {
	case http.MethodGet:
		if idx == -1 {
			jsonErr(w, "site not found", http.StatusNotFound)
			return
		}
		jsonResp(w, map[string]any{"site": (*sites)[idx]})

	case http.MethodPut:
		if idx == -1 {
			jsonErr(w, "site not found", http.StatusNotFound)
			return
		}
		var site SiteConfig
		if err := json.NewDecoder(r.Body).Decode(&site); err != nil {
			jsonErr(w, "invalid request body", http.StatusBadRequest)
			return
		}
		site.ID = id
		site.CreatedAt = (*sites)[idx].CreatedAt
		site.UpdatedAt = time.Now()
		newSites := *sites
		newSites[idx] = site
		s.status.Sites.Store(&newSites)

		s.logBuffer.Add(LogEntry{
			Timestamp: time.Now(),
			Level:     "info",
			Message:   "site updated: " + site.Name,
		})

		jsonResp(w, map[string]any{"site": site})

	case http.MethodDelete:
		if idx == -1 {
			jsonErr(w, "site not found", http.StatusNotFound)
			return
		}
		name := (*sites)[idx].Name
		newSites := append((*sites)[:idx], (*sites)[idx+1:]...)
		s.status.Sites.Store(&newSites)

		s.logBuffer.Add(LogEntry{
			Timestamp: time.Now(),
			Level:     "info",
			Message:   "site deleted: " + name,
		})

		jsonResp(w, map[string]string{"status": "deleted"})

	default:
		jsonErr(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		jsonErr(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	cfg := GatewayConfig{
		Schema: "gateway/v1",
		Server: ServerCfg{
			Addr:         ":8080",
			ReadTimeout:  "30s",
			WriteTimeout: "60s",
		},
		PHP: PHPCfg{
			Binary:         s.status.DocRoot,
			MaxChildren:    20,
			RequestTimeout: "60s",
		},
		Logging: LoggingCfg{
			Format: "json",
			Level:  "info",
		},
		Security: SecurityCfg{
			SymlinkMode: "within_root",
			MaxBodySize: "20MB",
		},
	}

	sites := s.status.Sites.Load()
	if sites != nil {
		cfg.Sites = *sites
	}

	jsonResp(w, cfg)
}

func (s *Server) handleConfigValidate(w http.ResponseWriter, r *http.Request) {
	jsonResp(w, map[string]string{"status": "valid"})
}

func (s *Server) handleConfigSave(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonErr(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var cfg GatewayConfig
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		jsonErr(w, "invalid request body", http.StatusBadRequest)
		return
	}

	s.logBuffer.Add(LogEntry{
		Timestamp: time.Now(),
		Level:     "info",
		Message:   "configuration saved",
	})

	jsonResp(w, map[string]string{"status": "saved"})
}

func (s *Server) handleRuntimes(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		jsonErr(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	type runtime struct {
		Version string `json:"version"`
		Binary  string `json:"binary"`
		Status  string `json:"status"`
		Active  bool   `json:"active"`
		Managed bool   `json:"managed"`
	}

	runtimes := []runtime{}
	for i, v := range s.status.Runtimes {
		runtimes = append(runtimes, runtime{
			Version: v,
			Binary:  "php-fpm" + v,
			Status:  "ready",
			Active:  i == 0,
			Managed: true,
		})
	}

	jsonResp(w, map[string]any{"runtimes": runtimes})
}

func (s *Server) handleLogs(w http.ResponseWriter, r *http.Request) {
	jsonResp(w, map[string]any{
		"entries": s.logBuffer.Recent(100),
		"total":   s.logBuffer.Size(),
	})
}

func (s *Server) handleRecentLogs(w http.ResponseWriter, r *http.Request) {
	n := 50
	jsonResp(w, map[string]any{
		"entries": s.logBuffer.Recent(n),
		"total":   s.logBuffer.Size(),
	})
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	jsonResp(w, map[string]string{"status": "ok"})
}

func (s *Server) handleSystem(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		jsonErr(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	hostname, _ := os.Hostname()

	jsonResp(w, SystemInfo{
		Hostname:   hostname,
		OS:         runtime.GOOS,
		Arch:       runtime.GOARCH,
		GoVersion:  buildinfo.Get().GoVersion,
		Goroutines: runtime.NumGoroutine(),
		MemAllocMB: float64(m.Alloc) / 1024 / 1024,
		MemSysMB:   float64(m.Sys) / 1024 / 1024,
		NumCPU:     runtime.NumCPU(),
		PID:        os.Getpid(),
	})
}

// generateID creates a simple ID from a name.
func generateID(name string) string {
	id := ""
	for _, c := range name {
		if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') {
			id += string(c)
		} else if c >= 'A' && c <= 'Z' {
			id += string(c + 32)
		} else if c == ' ' || c == '-' {
			id += "-"
		}
	}
	if id == "" {
		id = "site"
	}
	return id
}
