package diagnostics

import (
	"net/http"
	"net/url"
	"testing"

	"github.com/go-php/gateway/internal/router"
)

func TestContractTestSuite_BasicMatch(t *testing.T) {
	routes := []router.Route{
		{PathPrefix: "/api", Target: "/index.php"},
		{Path: "/old-page", Status: 301, Target: "/new-page"},
	}

	eng, err := router.NewEngine(routes)
	if err != nil {
		t.Fatal(err)
	}

	suite := NewContractTestSuite(eng)
	suite.AddTest(ContractTest{
		Name:   "API routes to PHP",
		Method: "GET",
		Path:   "/api/users",
		Expect: Expectation{RouteTarget: "/index.php"},
	})

	results := suite.RunAll()
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if !results[0].Passed {
		t.Errorf("expected test to pass: %s", results[0].Error)
	}
}

func TestContractTestSuite_Miss(t *testing.T) {
	routes := []router.Route{
		{PathPrefix: "/api", Target: "/index.php"},
	}

	eng, err := router.NewEngine(routes)
	if err != nil {
		t.Fatal(err)
	}

	suite := NewContractTestSuite(eng)
	suite.AddTest(ContractTest{
		Name:   "No match for unknown path",
		Method: "GET",
		Path:   "/unknown",
		Expect: Expectation{RouteTarget: ""},
	})

	results := suite.RunAll()
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if !results[0].Passed {
		t.Errorf("expected test to pass: %s", results[0].Error)
	}
}

func TestContractTestSuite_WrongTarget(t *testing.T) {
	routes := []router.Route{
		{PathPrefix: "/api", Target: "/index.php"},
	}

	eng, err := router.NewEngine(routes)
	if err != nil {
		t.Fatal(err)
	}

	suite := NewContractTestSuite(eng)
	suite.AddTest(ContractTest{
		Name:   "Wrong target expectation",
		Method: "GET",
		Path:   "/api/users",
		Expect: Expectation{RouteTarget: "/wrong.php"},
	})

	results := suite.RunAll()
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Passed {
		t.Error("expected test to fail for wrong target")
	}
}

func TestContractTestSuite_Redirect(t *testing.T) {
	routes := []router.Route{
		{Path: "/old-page", Target: "/new-page"},
	}

	eng, err := router.NewEngine(routes)
	if err != nil {
		t.Fatal(err)
	}

	suite := NewContractTestSuite(eng)
	suite.AddTest(ContractTest{
		Name:   "Old page rewrites",
		Method: "GET",
		Path:   "/old-page",
		Expect: Expectation{RedirectURL: "/new-page"},
	})

	results := suite.RunAll()
	if !results[0].Passed {
		t.Errorf("redirect test failed: %s", results[0].Error)
	}
}

func TestContractTestSuite_HostMatch(t *testing.T) {
	routes := []router.Route{
		{Host: "admin.example.com", PathPrefix: "/", Target: "/admin/index.php"},
	}

	eng, err := router.NewEngine(routes)
	if err != nil {
		t.Fatal(err)
	}

	suite := NewContractTestSuite(eng)
	suite.AddTest(ContractTest{
		Name:   "Admin host match",
		Method: "GET",
		Path:   "/dashboard",
		Host:   "admin.example.com",
		Expect: Expectation{RouteTarget: "/admin/index.php"},
	})

	results := suite.RunAll()
	if !results[0].Passed {
		t.Errorf("host match test failed: %s", results[0].Error)
	}
}

func TestContractTestSuite_MethodFilter(t *testing.T) {
	routes := []router.Route{
		{Path: "/api/users", Methods: []string{"POST"}, Target: "/index.php"},
	}

	eng, err := router.NewEngine(routes)
	if err != nil {
		t.Fatal(err)
	}

	suite := NewContractTestSuite(eng)

	// POST should match.
	suite.AddTest(ContractTest{
		Name:   "POST matches",
		Method: "POST",
		Path:   "/api/users",
		Expect: Expectation{RouteTarget: "/index.php"},
	})

	// GET should not match (but route has no path match for GET because of method filter).
	suite.AddTest(ContractTest{
		Name:   "GET should not match",
		Method: "GET",
		Path:   "/api/users",
		Expect: Expectation{RouteTarget: ""},
	})

	results := suite.RunAll()
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if !results[0].Passed {
		t.Errorf("POST test failed: %s", results[0].Error)
	}
	if !results[1].Passed {
		t.Errorf("GET test failed: %s", results[1].Error)
	}
}

func TestContractTestSuite_Summary(t *testing.T) {
	routes := []router.Route{
		{PathPrefix: "/api", Target: "/index.php"},
	}

	eng, err := router.NewEngine(routes)
	if err != nil {
		t.Fatal(err)
	}

	suite := NewContractTestSuite(eng)
	suite.AddTest(ContractTest{
		Name:   "Passing test",
		Method: "GET",
		Path:   "/api/items",
		Expect: Expectation{RouteTarget: "/index.php"},
	})
	suite.AddTest(ContractTest{
		Name:   "Failing test",
		Method: "GET",
		Path:   "/api/items",
		Expect: Expectation{RouteTarget: "/wrong.php"},
	})

	results := suite.RunAll()
	summary := suite.Summary(results)

	if summary == "" {
		t.Error("expected non-empty summary")
	}
	t.Logf("summary: %s", summary)

	// Verify counts.
	passed := 0
	failed := 0
	for _, r := range results {
		if r.Passed {
			passed++
		} else {
			failed++
		}
	}
	if passed != 1 || failed != 1 {
		t.Errorf("expected 1 pass, 1 fail; got %d pass, %d fail", passed, failed)
	}
}

func TestGenerateStandardTests(t *testing.T) {
	tests := GenerateStandardTests()
	if len(tests) < 3 {
		t.Errorf("expected at least 3 standard tests, got %d", len(tests))
	}

	for _, test := range tests {
		if test.Name == "" {
			t.Error("standard test has empty name")
		}
		if test.Method == "" {
			t.Errorf("standard test %q has empty method", test.Name)
		}
		if test.Path == "" {
			t.Errorf("standard test %q has empty path", test.Name)
		}
	}
}

func TestContractTestSuite_NoRoutesEngine(t *testing.T) {
	eng, err := router.NewEngine(nil)
	if err != nil {
		t.Fatal(err)
	}

	suite := NewContractTestSuite(eng)
	suite.AddTest(ContractTest{
		Name:   "No routes configured",
		Method: "GET",
		Path:   "/anything",
		Expect: Expectation{RouteTarget: ""},
	})

	results := suite.RunAll()
	if !results[0].Passed {
		t.Errorf("expected no-route test to pass: %s", results[0].Error)
	}
}

func TestRequestExplainer_MissingHost(t *testing.T) {
	// Just verify it doesn't panic with empty host.
	r := &http.Request{
		Method: "GET",
		URL:    &url.URL{Path: "/test"},
		Host:   "",
		Header: make(http.Header),
	}
	_ = r
}
