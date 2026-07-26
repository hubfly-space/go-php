package router

import (
	"fmt"
	"net/http"
	"regexp"
	"strings"
)

// Route represents a single routing rule.
type Route struct {
	Host       string // exact host match (empty = any)
	Path       string // exact path match
	PathPrefix string // prefix match
	Regex      string // regex match (compiled at init)
	Target     string // rewrite target
	Status     int    // redirect status (301, 302, etc.) — 0 = proxy
	Methods    []string
	Headers    map[string]string
}

// Engine matches requests against routes.
type Engine struct {
	routes []Route
}

// NewEngine creates a routing engine from routes.
func NewEngine(routes []Route) (*Engine, error) {
	e := &Engine{routes: make([]Route, len(routes))}
	for i, r := range routes {
		if r.Regex != "" {
			_, err := regexp.Compile(r.Regex)
			if err != nil {
				return nil, fmt.Errorf("compile regex %q: %w", r.Regex, err)
			}
		}
		e.routes[i] = r
	}
	return e, nil
}

// Match finds the first matching route for a request, or nil.
func (e *Engine) Match(r *http.Request) *Route {
	for i := range e.routes {
		route := &e.routes[i]
		if e.matchRoute(route, r) {
			return route
		}
	}
	return nil
}

func (e *Engine) matchRoute(route *Route, r *http.Request) bool {
	// Host match.
	if route.Host != "" && route.Host != r.Host {
		return false
	}

	// Method match.
	if len(route.Methods) > 0 {
		found := false
		for _, m := range route.Methods {
			if strings.EqualFold(m, r.Method) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	// Header match.
	for hk, hv := range route.Headers {
		if r.Header.Get(hk) != hv {
			return false
		}
	}

	// Path matching.
	path := r.URL.Path

	switch {
	case route.Regex != "":
		re, _ := regexp.Compile(route.Regex)
		return re.MatchString(path)

	case route.PathPrefix != "":
		return strings.HasPrefix(path, route.PathPrefix)

	case route.Path != "":
		return route.Path == path

	default:
		return true
	}
}

// Rewrite applies the route's rewrite target to the request path.
func (route *Route) Rewrite(path string) string {
	if route.Target == "" {
		return path
	}

	target := route.Target

	// Simple substitution: $0 = original path.
	target = strings.ReplaceAll(target, "$0", path)

	// Regex capture groups: $1, $2, etc.
	if route.Regex != "" {
		re, _ := regexp.Compile(route.Regex)
		if matches := re.FindStringSubmatch(path); len(matches) > 0 {
			for i, m := range matches {
				target = strings.ReplaceAll(target, fmt.Sprintf("$%d", i), m)
			}
		}
	}

	return target
}

// IsRedirect returns true if the route is a redirect.
func (route *Route) IsRedirect() bool {
	return route.Status >= 300 && route.Status < 400
}
