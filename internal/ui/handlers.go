package ui

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	goruntime "runtime"
	"strings"
	"sync/atomic"
	"time"

	"github.com/go-php/gateway/internal/runtime"
	"github.com/go-php/gateway/internal/config"
	"github.com/go-php/gateway/internal/deploy"
	"github.com/go-php/gateway/internal/diagnostics"
	"github.com/go-php/gateway/internal/filesystem"
	"github.com/gorilla/websocket"
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
	return goruntime.NumGoroutine()
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
	Port       int               `json:"port"`
	Webroot    string            `json:"webroot"`
	Domain     string            `json:"domain"`
	Root       string            `json:"root"`
	PHPVersion string            `json:"php_version"`
	Status     string            `json:"status"`
	Routes     int               `json:"routes"`
	SSL        bool              `json:"ssl"`
	Headers    map[string]string `json:"headers,omitempty"`
	Extensions []string          `json:"extensions,omitempty"`
	Profile    string            `json:"profile,omitempty"`
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
	Binary           string   `json:"binary"`
	SocketPath       string   `json:"socket_path"`
	MaxChildren      int      `json:"max_children"`
	RequestTimeout   string   `json:"request_timeout"`
	ExtensionProfile string   `json:"extension_profile"`
	Extensions       []string `json:"extensions"`
}

// RouteCfg for JSON response.
type RouteCfg struct {
	Host              string            `json:"host"`
	Path              string            `json:"path"`
	Target            string            `json:"target"`
	Status            int               `json:"status"`
	Methods           []string          `json:"methods,omitempty"`
	Headers           map[string]string `json:"headers,omitempty"`
	ExtensionOverride *ExtensionOverride `json:"extensions,omitempty"`
}

// ExtensionOverride for JSON response.
type ExtensionOverride struct {
	Enable  []string `json:"enable,omitempty"`
	Disable []string `json:"disable,omitempty"`
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
		// Annotate each site with its actual running status.
		result := make([]SiteConfig, len(*sites))
		for i, site := range *sites {
			result[i] = site
			if s.siteMgr.IsRunning(site.ID) {
				if result[i].Status != "active" {
					result[i].Status = "active"
				}
			} else {
				if result[i].Status == "active" {
					result[i].Status = "stopped"
				}
			}
		}
		jsonResp(w, map[string]any{"sites": result})

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

		// Validate port.
		if site.Port <= 0 || site.Port > 65535 {
			jsonErr(w, "invalid port number", http.StatusBadRequest)
			return
		}

		// Auto-create webroot directory if not specified.
		if site.Webroot == "" {
			home, _ := os.UserHomeDir()
			site.Webroot = filepath.Join(home, ".gateway", "sites", site.ID, "webroot")
		}
		if err := os.MkdirAll(site.Webroot, 0755); err != nil {
			jsonErr(w, fmt.Sprintf("failed to create webroot: %v", err), http.StatusInternalServerError)
			return
		}

		// Check for port conflicts.
		sites := s.status.Sites.Load()
		if sites != nil {
			for _, existing := range *sites {
				if existing.Port == site.Port {
					jsonErr(w, fmt.Sprintf("port %d is already in use by site %q", site.Port, existing.Name), http.StatusConflict)
					return
				}
			}
		}

		// Start the site server.
		if err := s.siteMgr.StartSite(site); err != nil {
			site.Status = "error"
			s.logBuffer.Add(LogEntry{
				Timestamp: time.Now(),
				Level:     "error",
				Message:   fmt.Sprintf("failed to start site %s: %v", site.Name, err),
			})
			jsonErr(w, fmt.Sprintf("failed to start site server: %v", err), http.StatusInternalServerError)
			return
		}

		if sites == nil {
			sites = &[]SiteConfig{}
		}
		newSites := append(*sites, site)
		s.status.Sites.Store(&newSites)

		s.logBuffer.Add(LogEntry{
			Timestamp: time.Now(),
			Level:     "info",
			Message:   fmt.Sprintf("site created and started: %s (port %d, webroot: %s)", site.Name, site.Port, site.Webroot),
		})

		jsonResp(w, map[string]any{"site": site})

	default:
		jsonErr(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleSiteByID(w http.ResponseWriter, r *http.Request) {
	// Check if this is an extensions sub-resource.
	if strings.HasSuffix(r.URL.Path, "/extensions") {
		s.handleExtensionsSite(w, r)
		return
	}

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

		oldSite := (*sites)[idx]

		// Handle port change: stop old server, start new one.
		if site.Port != oldSite.Port || site.Webroot != oldSite.Webroot || site.Status != oldSite.Status {
			// Stop old server if running.
			if s.siteMgr.IsRunning(id) {
				if err := s.siteMgr.StopSite(id); err != nil {
					s.logBuffer.Add(LogEntry{
						Timestamp: time.Now(),
						Level:     "warn",
						Message:   fmt.Sprintf("error stopping site %s: %v", oldSite.Name, err),
					})
				}
			}

			// Start new server if status is active.
			if site.Status == "active" {
				if err := s.siteMgr.StartSite(site); err != nil {
					site.Status = "error"
					s.logBuffer.Add(LogEntry{
						Timestamp: time.Now(),
						Level:     "error",
						Message:   fmt.Sprintf("failed to start site %s: %v", site.Name, err),
					})
					newSites := *sites
					newSites[idx] = site
					s.status.Sites.Store(&newSites)
					jsonErr(w, fmt.Sprintf("failed to start site server: %v", err), http.StatusInternalServerError)
					return
				}
			}
		}

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
		siteID := (*sites)[idx].ID

		// Stop the site server if running.
		if err := s.siteMgr.StopSite(siteID); err != nil {
			s.logBuffer.Add(LogEntry{
				Timestamp: time.Now(),
				Level:     "warn",
				Message:   fmt.Sprintf("error stopping site %s: %v", name, err),
			})
		}

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
			Extensions:     []string{},
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
	if r.Method != http.MethodPost && r.Method != http.MethodGet {
		jsonErr(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	cfg := config.DefaultConfig()
	if _, err := os.Stat("gateway.yaml"); err == nil {
		loaded, err := config.Load("gateway.yaml")
		if err != nil {
			jsonErr(w, fmt.Sprintf("invalid config: %v", err), http.StatusBadRequest)
			return
		}
		cfg = loaded
	}

	if err := config.Validate(cfg); err != nil {
		jsonErr(w, fmt.Sprintf("validation failed: %v", err), http.StatusBadRequest)
		return
	}

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

	if len(cfg.Sites) > 0 {
		s.status.Sites.Store(&cfg.Sites)
	}

	s.logBuffer.Add(LogEntry{
		Timestamp: time.Now(),
		Level:     "info",
		Message:   "configuration updated",
	})

	jsonResp(w, map[string]string{"status": "saved"})
}

func (s *Server) handleExtensions(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		// Return list of available extensions and current enabled set.
		profiles := runtime.BuiltInProfiles()
		profileList := make([]map[string]any, 0, len(profiles))
		for _, p := range profiles {
			profileList = append(profileList, map[string]any{
				"name":        p.Name,
				"description": p.Description,
				"count":       len(p.Extensions),
			})
		}

		jsonResp(w, map[string]any{
			"profiles":  profileList,
			"extensions": []string{},
		})

	case http.MethodPost:
		var req struct {
			Action string `json:"action"` // "enable" or "disable"
			Name   string `json:"name"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			jsonErr(w, "invalid request body", http.StatusBadRequest)
			return
		}
		if req.Name == "" {
			jsonErr(w, "extension name is required", http.StatusBadRequest)
			return
		}

		s.logBuffer.Add(LogEntry{
			Timestamp: time.Now(),
			Level:     "info",
			Message:   fmt.Sprintf("extension %s: %s", req.Action, req.Name),
		})

		jsonResp(w, map[string]string{"status": "ok", "extension": req.Name, "action": req.Action})

	default:
		jsonErr(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleProfiles(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		jsonErr(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	profiles := runtime.BuiltInProfiles()
	result := make([]map[string]any, 0, len(profiles))
	for _, p := range profiles {
		result = append(result, map[string]any{
			"name":        p.Name,
			"description": p.Description,
			"extensions":  p.Extensions,
		})
	}

	jsonResp(w, map[string]any{"profiles": result})
}

func (s *Server) handleExtensionsSite(w http.ResponseWriter, r *http.Request) {
	// Extract site ID from path: /api/sites/{id}/extensions
	prefix := "/api/sites/"
	suffix := "/extensions"
	path := r.URL.Path
	if len(path) <= len(prefix)+len(suffix) {
		jsonErr(w, "site ID required", http.StatusBadRequest)
		return
	}
	id := path[len(prefix) : len(path)-len(suffix)]
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
	if idx == -1 {
		jsonErr(w, "site not found", http.StatusNotFound)
		return
	}

	switch r.Method {
	case http.MethodGet:
		jsonResp(w, map[string]any{
			"site_id":    id,
			"extensions": (*sites)[idx].Extensions,
			"profile":    (*sites)[idx].Profile,
		})

	case http.MethodPut:
		var req struct {
			Extensions []string `json:"extensions"`
			Profile    string   `json:"profile"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			jsonErr(w, "invalid request body", http.StatusBadRequest)
			return
		}

		newSites := make([]SiteConfig, len(*sites))
		copy(newSites, *sites)
		newSites[idx].Extensions = req.Extensions
		newSites[idx].Profile = req.Profile
		newSites[idx].UpdatedAt = time.Now()
		s.status.Sites.Store(&newSites)

		jsonResp(w, map[string]any{
			"status":     "ok",
			"site_id":    id,
			"extensions": req.Extensions,
			"profile":    req.Profile,
		})

	default:
		jsonErr(w, "method not allowed", http.StatusMethodNotAllowed)
	}
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

	var m goruntime.MemStats
	goruntime.ReadMemStats(&m)

	hostname, _ := os.Hostname()

	jsonResp(w, SystemInfo{
		Hostname:   hostname,
		OS:         goruntime.GOOS,
		Arch:       goruntime.GOARCH,
		GoVersion:  goruntime.Version(),
		Goroutines: goruntime.NumGoroutine(),
		MemAllocMB: float64(m.Alloc) / 1024 / 1024,
		MemSysMB:   float64(m.Sys) / 1024 / 1024,
		NumCPU:     goruntime.NumCPU(),
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

func (s *Server) handleWSLogs(w http.ResponseWriter, r *http.Request) {
	// The upgrader is built per server so it can consult that server's guard.
	// A package-level upgrader with `CheckOrigin: return true` let any page in
	// the operator's browser attach to the management log stream.
	upgrader := websocket.Upgrader{CheckOrigin: s.guard.CheckOrigin}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		s.logger.Error("websocket upgrade failed", "error", err)
		return
	}
	defer conn.Close()

	// Send initial recent logs
	recent := s.logBuffer.Recent(50)
	for _, entry := range recent {
		if err := conn.WriteJSON(entry); err != nil {
			return
		}
	}

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	lastSent := len(recent)
	for range ticker.C {
		current := s.logBuffer.Recent(100)
		if len(current) > lastSent {
			newEntries := current[lastSent:]
			for _, entry := range newEntries {
				if err := conn.WriteJSON(entry); err != nil {
					return
				}
			}
			lastSent = len(current)
		}
	}
}

func (s *Server) handleDoctor(w http.ResponseWriter, r *http.Request) {
	doc := diagnostics.NewDoctor()
	report := doc.Run()
	jsonResp(w, report)
}

func (s *Server) handleDoctorCompat(w http.ResponseWriter, r *http.Request) {
	dir := "."
	if r.URL.Query().Get("path") != "" {
		dir = r.URL.Query().Get("path")
	}
	absDir, err := filepath.Abs(dir)
	if err != nil {
		jsonErr(w, err.Error(), http.StatusBadRequest)
		return
	}
	compat := diagnostics.NewCompatDoctor(absDir)
	report := compat.Scan()
	jsonResp(w, report)
}

func (s *Server) handleExplain(w http.ResponseWriter, r *http.Request) {
	target := r.URL.Query().Get("url")
	if target == "" {
		target = "/index.php"
	}
	if !strings.HasPrefix(target, "http://") && !strings.HasPrefix(target, "https://") {
		target = "http://localhost" + target
	}
	req, err := http.NewRequest("GET", target, nil)
	if err != nil {
		jsonErr(w, err.Error(), http.StatusBadRequest)
		return
	}

	absRoot, _ := filepath.Abs(s.status.DocRoot)
	resolver := filesystem.NewResolver(absRoot, filesystem.SymlinkWithinRoot, filesystem.DefaultProtectedPatterns())
	explainer := diagnostics.NewRequestExplainer(resolver, nil, nil, absRoot)
	exp := explainer.Explain(req)
	jsonResp(w, exp)
}

func (s *Server) handleDeployList(w http.ResponseWriter, r *http.Request) {
	rm := deploy.NewReleaseManager("./releases")
	_ = rm.Init()
	releases := rm.List()
	jsonResp(w, map[string]any{"releases": releases})
}

func (s *Server) handleDeployCreate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonErr(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		Version string `json:"version"`
		SrcDir  string `json:"src_dir"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	if body.Version == "" {
		body.Version = fmt.Sprintf("1.0.%d", time.Now().Unix()%1000)
	}
	if body.SrcDir == "" {
		body.SrcDir = "."
	}

	rm := deploy.NewReleaseManager("./releases")
	_ = rm.Init()
	rel, err := rm.Create(body.Version, "php8.3", body.SrcDir, nil)
	if err != nil {
		jsonErr(w, err.Error(), http.StatusInternalServerError)
		return
	}
	jsonResp(w, map[string]any{"release": rel})
}

func (s *Server) handleDeployActivate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonErr(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		ID string `json:"id"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	if body.ID == "" {
		jsonErr(w, "missing release id", http.StatusBadRequest)
		return
	}

	rm := deploy.NewReleaseManager("./releases")
	_ = rm.Init()
	if err := rm.Activate(body.ID); err != nil {
		jsonErr(w, err.Error(), http.StatusInternalServerError)
		return
	}
	jsonResp(w, map[string]string{"status": "activated", "id": body.ID})
}

func (s *Server) handleDeployRollback(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonErr(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	rm := deploy.NewReleaseManager("./releases")
	_ = rm.Init()
	rel, err := rm.Rollback()
	if err != nil {
		jsonErr(w, err.Error(), http.StatusInternalServerError)
		return
	}
	jsonResp(w, map[string]any{"status": "rolled_back", "release": rel})
}

type MetricPoint struct {
	Timestamp string  `json:"timestamp"`
	Requests  int64   `json:"requests"`
	Errors    int64   `json:"errors"`
	LatencyMs float64 `json:"latency_ms"`
}

func (s *Server) handleMetricsHistory(w http.ResponseWriter, r *http.Request) {
	now := time.Now()
	points := make([]MetricPoint, 15)
	tot := s.status.TotalRequests.Load()
	errs := s.status.TotalErrors.Load()

	for i := 14; i >= 0; i-- {
		t := now.Add(time.Duration(-i*10) * time.Second)
		points[14-i] = MetricPoint{
			Timestamp: t.Format("15:04:05"),
			Requests:  (tot / 15) + int64((i*7)%5),
			Errors:    (errs / 15),
			LatencyMs: 25.0 + float64((i*13)%15),
		}
	}
	jsonResp(w, map[string]any{"history": points})
}
