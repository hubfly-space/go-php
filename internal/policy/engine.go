package policy

import (
	"fmt"
	"net"
	"net/http"
	"regexp"
	"strings"
	"sync"
)

// Phase represents the request processing phase.
type Phase string

const (
	PhaseRequest    Phase = "request"    // before proxying
	PhaseResponse   Phase = "response"   // after backend response
	PhasePreconnect Phase = "preconnect" // before connecting to backend
)

// Decision is the result of a policy evaluation.
type Decision int

const (
	DecisionAllow Decision = iota
	DecisionDeny
	DecisionObserve // log but don't block
)

func (d Decision) String() string {
	switch d {
	case DecisionAllow:
		return "allow"
	case DecisionDeny:
		return "deny"
	case DecisionObserve:
		return "observe"
	default:
		return "unknown"
	}
}

// Context holds request/response data for policy evaluation.
type Context struct {
	Phase     Phase
	Method    string
	Path      string
	Host      string
	Headers   http.Header
	Query     string
	BodySize  int64
	RemoteIP  net.IP
	TLS       bool
	RouteName string
}

// Rule is a single policy rule.
type Rule struct {
	Name       string
	Phase      Phase
	Mode       Decision // allow, deny, observe
	Priority   int      // lower = higher priority
	Conditions []Condition
	Exclusions []Exclusion
}

// Condition is a single match condition within a rule.
type Condition struct {
	Type    ConditionType
	Values  []string
	Negate  bool
}

// ConditionType determines what the condition matches.
type ConditionType string

const (
	CondMethod    ConditionType = "method"
	CondPath      ConditionType = "path"
	CondPathPrefix ConditionType = "path_prefix"
	CondPathRegex ConditionType = "path_regex"
	CondHost      ConditionType = "host"
	CondHeader    ConditionType = "header"
	CondQueryParam ConditionType = "query_param"
	CondBodySize  ConditionType = "body_size"
	CondIP        ConditionType = "ip"
	CondIPRange   ConditionType = "ip_range"
	CondScheme    ConditionType = "scheme"
)

// Exclusion excludes requests from a rule.
type Exclusion struct {
	Type  ConditionType
	Values []string
}

// Engine evaluates policy rules against requests.
type Engine struct {
	mu    sync.RWMutex
	rules []Rule
}

// NewEngine creates a policy engine.
func NewEngine() *Engine {
	return &Engine{}
}

// AddRule adds a rule to the engine.
func (e *Engine) AddRule(rule Rule) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.rules = append(e.rules, rule)
	// Sort by priority (lower = higher priority).
	for i := len(e.rules) - 1; i > 0; i-- {
		if e.rules[i].Priority < e.rules[i-1].Priority {
			e.rules[i], e.rules[i-1] = e.rules[i-1], e.rules[i]
		}
	}
}

// Clear removes all rules.
func (e *Engine) Clear() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.rules = nil
}

// Evaluate runs all rules against a context and returns the final decision.
func (e *Engine) Evaluate(ctx *Context) Decision {
	e.mu.RLock()
	defer e.mu.RUnlock()

	result := DecisionAllow

	for _, rule := range e.rules {
		if rule.Phase != ctx.Phase {
			continue
		}

		if !e.matchesConditions(&rule, ctx) {
			continue
		}

		if e.matchesExclusions(&rule, ctx) {
			continue
		}

		switch rule.Mode {
		case DecisionDeny:
			return DecisionDeny
		case DecisionObserve:
			result = DecisionObserve
		case DecisionAllow:
			if result == DecisionObserve {
				// Allow overrides observe.
				result = DecisionAllow
			}
		}
	}

	return result
}

func (e *Engine) matchesConditions(rule *Rule, ctx *Context) bool {
	for _, cond := range rule.Conditions {
		if !e.matchesCondition(&cond, ctx) {
			return false
		}
	}
	return true
}

func (e *Engine) matchesCondition(cond *Condition, ctx *Context) bool {
	result := false

	switch cond.Type {
	case CondMethod:
		for _, m := range cond.Values {
			if strings.EqualFold(m, ctx.Method) {
				result = true
				break
			}
		}

	case CondPath:
		for _, p := range cond.Values {
			if p == ctx.Path {
				result = true
				break
			}
		}

	case CondPathPrefix:
		for _, p := range cond.Values {
			if strings.HasPrefix(ctx.Path, p) {
				result = true
				break
			}
		}

	case CondPathRegex:
		for _, pattern := range cond.Values {
			re, err := regexp.Compile(pattern)
			if err != nil {
				continue
			}
			if re.MatchString(ctx.Path) {
				result = true
				break
			}
		}

	case CondHost:
		for _, h := range cond.Values {
			if strings.EqualFold(h, ctx.Host) {
				result = true
				break
			}
		}

	case CondHeader:
		for _, hv := range cond.Values {
			parts := strings.SplitN(hv, ":", 2)
			if len(parts) != 2 {
				continue
			}
			name := strings.TrimSpace(parts[0])
			value := strings.TrimSpace(parts[1])
			if ctx.Headers.Get(name) == value {
				result = true
				break
			}
		}

	case CondQueryParam:
		for _, qp := range cond.Values {
			if strings.Contains(ctx.Query, qp) {
				result = true
				break
			}
		}

	case CondBodySize:
		for _, sz := range cond.Values {
			var max int64
			fmt.Sscanf(sz, "%d", &max)
			if ctx.BodySize > max {
				result = true
				break
			}
		}

	case CondIP:
		for _, ipStr := range cond.Values {
			ip := net.ParseIP(ipStr)
			if ip != nil && ip.Equal(ctx.RemoteIP) {
				result = true
				break
			}
		}

	case CondIPRange:
		for _, cidr := range cond.Values {
			_, network, err := net.ParseCIDR(cidr)
			if err != nil {
				continue
			}
			if network.Contains(ctx.RemoteIP) {
				result = true
				break
			}
		}

	case CondScheme:
		for _, s := range cond.Values {
			if (s == "https" && ctx.TLS) || (s == "http" && !ctx.TLS) {
				result = true
				break
			}
		}
	}

	if cond.Negate {
		return !result
	}
	return result
}

func (e *Engine) matchesExclusions(rule *Rule, ctx *Context) bool {
	for _, excl := range rule.Exclusions {
		cond := Condition{
			Type:   excl.Type,
			Values: excl.Values,
			Negate: false,
		}
		if e.matchesCondition(&cond, ctx) {
			return true
		}
	}
	return false
}

// Rules returns a copy of all rules.
func (e *Engine) Rules() []Rule {
	e.mu.RLock()
	defer e.mu.RUnlock()
	result := make([]Rule, len(e.rules))
	copy(result, e.rules)
	return result
}

// Middleware returns an HTTP middleware that enforces policy.
func (e *Engine) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := &Context{
			Phase:   PhaseRequest,
			Method:  r.Method,
			Path:    r.URL.Path,
			Host:    r.Host,
			Headers: r.Header,
			Query:   r.URL.RawQuery,
			RemoteIP: parseIP(r.RemoteAddr),
			TLS:     r.TLS != nil,
		}

		decision := e.Evaluate(ctx)

		switch decision {
		case DecisionDeny:
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusForbidden)
			fmt.Fprintf(w, `{"error":"forbidden","rule":"policy"}`)
			return
		case DecisionObserve:
			w.Header().Set("X-Policy-Observed", "true")
		}

		next.ServeHTTP(w, r)
	})
}

func parseIP(addr string) net.IP {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return net.ParseIP(addr)
	}
	return net.ParseIP(host)
}
