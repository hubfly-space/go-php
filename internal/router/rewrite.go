package router

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// ErrMaxIterationsExceeded is returned when rewrite evaluation exceeds the maximum iteration count.
var ErrMaxIterationsExceeded = errors.New("rewrite engine: max iterations exceeded")

// RewriteResult contains the outcome of evaluating a path through the rewrite rules.
type RewriteResult struct {
	Path         string
	RawQuery     string
	RedirectCode int
	IsTerminal   bool
	Iterations   int
}

// RewriteLoop evaluates the path through matching rules up to maxIterations.
func (e *Engine) RewriteLoop(initialPath string, initialQuery string, maxIterations int) (RewriteResult, error) {
	if maxIterations <= 0 {
		maxIterations = 10
	}

	currentPath := initialPath
	currentQuery := initialQuery
	res := RewriteResult{
		Path:     currentPath,
		RawQuery: currentQuery,
	}

	for i := 0; i < maxIterations; i++ {
		res.Iterations = i + 1
		matched := false

		for _, route := range e.routes {
			if route.Target == "" {
				continue
			}

			// Test regex match if regex is specified
			var captures []string
			if route.Regex != "" {
				re, err := regexp.Compile(route.Regex)
				if err != nil {
					continue
				}
				matches := re.FindStringSubmatch(currentPath)
				if len(matches) == 0 {
					continue
				}
				captures = matches
			} else if route.PathPrefix != "" {
				if !strings.HasPrefix(currentPath, route.PathPrefix) {
					continue
				}
			} else if route.Path != "" {
				if currentPath != route.Path {
					continue
				}
			}

			// Apply target rewrite
			target := route.Target
			if len(captures) > 0 {
				for idx, val := range captures {
					target = strings.ReplaceAll(target, fmt.Sprintf("$%d", idx), val)
				}
			}
			target = strings.ReplaceAll(target, "$0", currentPath)

			// Split target into path and query
			nextPath := target
			nextQuery := ""
			if idx := strings.IndexByte(target, '?'); idx >= 0 {
				nextPath = target[:idx]
				nextQuery = target[idx+1:]
			}

			// Query string append handling
			if nextQuery != "" {
				if currentQuery != "" {
					currentQuery = nextQuery + "&" + currentQuery
				} else {
					currentQuery = nextQuery
				}
			}

			matched = true
			if nextPath != currentPath {
				currentPath = nextPath
			}

			res.Path = currentPath
			res.RawQuery = currentQuery

			if route.IsRedirect() {
				res.RedirectCode = route.Status
				res.IsTerminal = true
				return res, nil
			}

			// Terminal flag check (e.g. if target doesn't allow further rewrites)
			break
		}

		if !matched {
			return res, nil
		}
	}

	return res, ErrMaxIterationsExceeded
}
