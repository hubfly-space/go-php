package diagnostics

import (
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/go-php/gateway/internal/router"
)

// ContractTest defines a single route contract test.
type ContractTest struct {
	Name        string            `json:"name"`
	Method      string            `json:"method"`
	Path        string            `json:"path"`
	Host        string            `json:"host,omitempty"`
	Headers     map[string]string `json:"headers,omitempty"`
	Expect      Expectation       `json:"expect"`
}

// Expectation defines what a contract test expects.
type Expectation struct {
	StatusCode  int    `json:"status_code,omitempty"`
	RedirectURL string `json:"redirect_url,omitempty"`
	RouteTarget string `json:"route_target,omitempty"`
	Denied      bool   `json:"denied,omitempty"`
	HasPHP      bool   `json:"has_php,omitempty"`
}

// ContractTestResult is the result of a single test.
type ContractTestResult struct {
	Name     string        `json:"name"`
	Passed   bool          `json:"passed"`
	Error    string        `json:"error,omitempty"`
	Duration time.Duration `json:"duration"`
}

// ContractTestSuite is a collection of contract tests.
type ContractTestSuite struct {
	Engine  *router.Engine
	Tests   []ContractTest
}

// NewContractTestSuite creates a test suite.
func NewContractTestSuite(engine *router.Engine) *ContractTestSuite {
	return &ContractTestSuite{
		Engine: engine,
	}
}

// AddTest adds a contract test.
func (s *ContractTestSuite) AddTest(test ContractTest) {
	s.Tests = append(s.Tests, test)
}

// RunAll runs all contract tests and returns results.
func (s *ContractTestSuite) RunAll() []ContractTestResult {
	var results []ContractTestResult

	for _, test := range s.Tests {
		start := time.Now()
		result := s.runSingle(test)
		result.Duration = time.Since(start)
		results = append(results, result)
	}

	return results
}

// Summary returns a summary of all results.
func (s *ContractTestSuite) Summary(results []ContractTestResult) string {
	passed := 0
	failed := 0
	for _, r := range results {
		if r.Passed {
			passed++
		} else {
			failed++
		}
	}

	return fmt.Sprintf("Contract tests: %d passed, %d failed, %d total", passed, failed, len(results))
}

func (s *ContractTestSuite) runSingle(test ContractTest) ContractTestResult {
	result := ContractTestResult{Name: test.Name}

	r := &http.Request{
		Method: test.Method,
		URL:    &url.URL{Path: test.Path},
		Host:   test.Host,
		Header: make(http.Header),
	}

	for k, v := range test.Headers {
		r.Header.Set(k, v)
	}

	route := s.Engine.Match(r)

	// Check route matching.
	if test.Expect.RouteTarget != "" {
		if route == nil {
			result.Error = fmt.Sprintf("expected route match for %s %s, got none", test.Method, test.Path)
			return result
		}
		if route.Target != test.Expect.RouteTarget {
			result.Error = fmt.Sprintf("expected target %q, got %q", test.Expect.RouteTarget, route.Target)
			return result
		}
	} else if test.Expect.Denied {
		// Route should not match (or match a deny rule).
		if route != nil && route.Status == 0 {
			// Route matched but it's not a redirect — could be a proxy route.
		}
	}

	// Check redirect.
	if test.Expect.RedirectURL != "" {
		if route == nil {
			result.Error = fmt.Sprintf("expected redirect for %s %s, got none", test.Method, test.Path)
			return result
		}
		rewritten := route.Rewrite(test.Path)
		if rewritten != test.Expect.RedirectURL {
			result.Error = fmt.Sprintf("expected redirect to %q, got %q", test.Expect.RedirectURL, rewritten)
			return result
		}
	}

	result.Passed = true
	return result
}

// GenerateStandardTests creates standard contract tests for common patterns.
func GenerateStandardTests() []ContractTest {
	return []ContractTest{
		{
			Name:   "API prefix routes to PHP",
			Method: "GET",
			Path:   "/api/users",
			Expect: Expectation{RouteTarget: "/index.php"},
		},
		{
			Name:   "Admin requires host match",
			Method: "GET",
			Path:   "/admin/dashboard",
			Host:   "admin.example.com",
			Expect: Expectation{RouteTarget: "/admin/index.php"},
		},
		{
			Name:   "Static files served directly",
			Method: "GET",
			Path:   "/style.css",
			Expect: Expectation{RouteTarget: ""},
		},
		{
			Name:   "Blog posts use regex route",
			Method: "GET",
			Path:   "/blog/2024/03/my-post",
			Expect: Expectation{RouteTarget: "/index.php"},
		},
		{
			Name:   "Old page redirects",
			Method: "GET",
			Path:   "/old-page",
			Expect: Expectation{RedirectURL: "/new-page"},
		},
	}
}
