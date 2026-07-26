package diagnostics

import (
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/go-php/gateway/internal/filesystem"
	"github.com/go-php/gateway/internal/policy"
	"github.com/go-php/gateway/internal/router"
)

// RequestExplainer traces a request through the gateway decision pipeline.
type RequestExplainer struct {
	resolver     *filesystem.Resolver
	router       *router.Engine
	policyEngine *policy.Engine
	docRoot      string
}

// NewRequestExplainer creates a request explainer.
func NewRequestExplainer(resolver *filesystem.Resolver, router *router.Engine, policyEngine *policy.Engine, docRoot string) *RequestExplainer {
	return &RequestExplainer{
		resolver:     resolver,
		router:       router,
		policyEngine: policyEngine,
		docRoot:      docRoot,
	}
}

// ensure policyEngine is never nil.
func (re *RequestExplainer) ensurePolicy() *policy.Engine {
	if re.policyEngine == nil {
		re.policyEngine = policy.NewEngine()
	}
	return re.policyEngine
}

// Explanation holds the full trace of a request through the gateway.
type Explanation struct {
	Request     RequestInfo  `json:"request"`
	PathNorm    PathNormStep `json:"path_normalization"`
	PolicyCheck PolicyStep   `json:"policy_check"`
	RouteMatch  RouteStep    `json:"route_match"`
	FileCheck   FileStep     `json:"file_check"`
	ScriptCheck ScriptStep   `json:"script_check"`
	Summary     string       `json:"summary"`
	Duration    string       `json:"duration"`
}

// RequestInfo holds the original request details.
type RequestInfo struct {
	Method  string            `json:"method"`
	Host    string            `json:"host"`
	Path    string            `json:"path"`
	Query   string            `json:"query"`
	Headers map[string]string `json:"headers"`
	TLS     bool              `json:"tls"`
	Remote  string            `json:"remote"`
}

// PathNormStep shows path normalization results.
type PathNormStep struct {
	Raw        string `json:"raw"`
	Decoded    string `json:"decoded"`
	Normalized string `json:"normalized"`
	Valid      bool   `json:"valid"`
	Error      string `json:"error,omitempty"`
}

// PolicyStep shows policy engine results.
type PolicyStep struct {
	Decision string   `json:"decision"`
	Rules    []string `json:"matched_rules"`
}

// RouteStep shows routing results.
type RouteStep struct {
	Matched   bool   `json:"matched"`
	RouteName string `json:"route_name,omitempty"`
	Target    string `json:"target,omitempty"`
	Rewritten string `json:"rewritten,omitempty"`
}

// FileStep shows filesystem resolution results.
type FileStep struct {
	Found     bool   `json:"found"`
	RealPath  string `json:"real_path,omitempty"`
	IsPHP     bool   `json:"is_php"`
	Protected bool   `json:"protected"`
	Error     string `json:"error,omitempty"`
}

// ScriptStep shows PHP script resolution results.
type ScriptStep struct {
	Found      bool   `json:"found"`
	ScriptName string `json:"script_name,omitempty"`
	ScriptPath string `json:"script_path,omitempty"`
	Error      string `json:"error,omitempty"`
}

// Explain traces a request and returns the full explanation.
func (re *RequestExplainer) Explain(r *http.Request) *Explanation {
	start := time.Now()
	explain := &Explanation{
		Request: RequestInfo{
			Method:  r.Method,
			Host:    r.Host,
			Path:    r.URL.Path,
			Query:   r.URL.RawQuery,
			TLS:     r.TLS != nil,
			Remote:  r.RemoteAddr,
			Headers: make(map[string]string),
		},
	}

	// Capture relevant headers.
	for _, h := range []string{"User-Agent", "Accept-Encoding", "If-None-Match", "Authorization"} {
		if v := r.Header.Get(h); v != "" {
			explain.Request.Headers[h] = v
		}
	}

	// Step 1: Path normalization.
	pp, err := filesystem.ParsePath(r.URL.Path)
	if err != nil {
		explain.PathNorm = PathNormStep{
			Raw:   r.URL.Path,
			Valid: false,
			Error: err.Error(),
		}
		explain.Summary = fmt.Sprintf("REJECTED: path normalization failed: %v", err)
		explain.Duration = time.Since(start).String()
		return explain
	}

	explain.PathNorm = PathNormStep{
		Raw:        r.URL.Path,
		Decoded:    pp.NormalizedPath,
		Normalized: pp.NormalizedPath,
		Valid:      true,
	}

	// Step 2: Policy check.
	pe := re.ensurePolicy()
	policyCtx := &policy.Context{
		Phase:    policy.PhaseRequest,
		Method:   r.Method,
		Path:     pp.NormalizedPath,
		Host:     r.Host,
		Headers:  r.Header,
		Query:    r.URL.RawQuery,
		RemoteIP: parseRemoteIP(r.RemoteAddr),
		TLS:      r.TLS != nil,
	}

	decision := pe.Evaluate(policyCtx)
	explain.PolicyCheck = PolicyStep{
		Decision: decision.String(),
	}

	if decision == policy.DecisionDeny {
		explain.Summary = fmt.Sprintf("DENIED by policy: method=%s path=%s", r.Method, pp.NormalizedPath)
		explain.Duration = time.Since(start).String()
		return explain
	}

	// Step 3: Route matching.
	if re.router != nil {
		if route := re.router.Match(r); route != nil {
			rewritten := route.Rewrite(pp.NormalizedPath)
			explain.RouteMatch = RouteStep{
				Matched:   true,
				RouteName: route.Path + route.PathPrefix + route.Regex,
				Target:    route.Target,
				Rewritten: rewritten,
			}
			pp.NormalizedPath = rewritten
		}
	}

	// Step 4: File resolution.
	rf, err := re.resolver.Resolve(pp.NormalizedPath)
	if err == nil {
		rf.Close()
		explain.FileCheck = FileStep{
			Found:    true,
			RealPath: rf.RealPath,
			IsPHP:    strings.HasSuffix(rf.RealPath, ".php"),
		}
	} else {
		explain.FileCheck = FileStep{
			Found: false,
			Error: err.Error(),
		}
	}

	// Step 4b: Check if protected.
	explain.FileCheck.Protected = re.resolver.IsProtected(pp.NormalizedPath)

	// Step 5: Script resolution.
	if !explain.FileCheck.Found || explain.FileCheck.IsPHP {
		scriptName, scriptPath := resolveScriptForExplain(re.docRoot, pp.NormalizedPath)
		if scriptPath != "" {
			explain.ScriptCheck = ScriptStep{
				Found:      true,
				ScriptName: scriptName,
				ScriptPath: scriptPath,
			}
		} else {
			explain.ScriptCheck = ScriptStep{
				Found: false,
				Error: "no PHP entry point found",
			}
		}
	}

	// Summary.
	explain.Summary = re.buildSummary(explain)
	explain.Duration = time.Since(start).String()
	return explain
}

func (re *RequestExplainer) buildSummary(e *Explanation) string {
	var parts []string

	if e.PolicyCheck.Decision == "observe" {
		parts = append(parts, "POLICY:observed")
	}

	if e.RouteMatch.Matched {
		parts = append(parts, fmt.Sprintf("ROUTE:%s->%s", e.RouteMatch.RouteName, e.RouteMatch.Target))
	}

	if e.FileCheck.Found {
		parts = append(parts, fmt.Sprintf("FILE:%s", e.FileCheck.RealPath))
	} else if e.ScriptCheck.Found {
		parts = append(parts, fmt.Sprintf("SCRIPT:%s", e.ScriptCheck.ScriptName))
	} else {
		parts = append(parts, "NOT_FOUND")
	}

	if len(parts) == 0 {
		return "OK"
	}
	return strings.Join(parts, " ")
}

func resolveScriptForExplain(docRoot, normalized string) (string, string) {
	// Same logic as in cmd/gateway/main.go
	if strings.HasSuffix(normalized, ".php") {
		return normalized, normalized
	}
	for _, entry := range []string{"public/index.php", "index.php"} {
		return "/" + entry, entry
	}
	return "", ""
}

func parseRemoteIP(addr string) net.IP {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return net.ParseIP(addr)
	}
	return net.ParseIP(host)
}
