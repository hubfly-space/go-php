package diagnostics

import (
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/go-php/gateway/internal/filesystem"
	"github.com/go-php/gateway/internal/policy"
	"github.com/go-php/gateway/internal/router"
)

func setupExplainTestDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	// Create basic project structure.
	os.MkdirAll(filepath.Join(dir, "public"), 0755)
	os.WriteFile(filepath.Join(dir, "public", "index.php"), []byte(`<?php echo "hello"; ?>`), 0644)
	os.WriteFile(filepath.Join(dir, "style.css"), []byte(`body {}`), 0644)

	// Create .env (protected).
	os.WriteFile(filepath.Join(dir, ".env"), []byte(`SECRET=abc`), 0644)

	return dir
}

func TestRequestExplainer_PathRejected(t *testing.T) {
	dir := setupExplainTestDir(t)

	resolver := filesystem.NewResolver(dir, filesystem.SymlinkDeny, nil)
	routes := []router.Route{
		{PathPrefix: "/api", Target: "/index.php"},
	}
	eng, err := router.NewEngine(routes)
	if err != nil {
		t.Fatal(err)
	}
	policyEngine := policy.NewEngine()

	explainer := NewRequestExplainer(resolver, eng, policyEngine, dir)

	// Request with NUL byte — rejected by path parser.
	r := &http.Request{
		Method: "GET",
		URL:    &url.URL{Path: "/foo\x00bar"},
		Host:   "example.com",
		Header: make(http.Header),
	}

	explain := explainer.Explain(r)
	if explain.PathNorm.Valid {
		t.Error("expected path normalization to fail for NUL byte")
	}
	if explain.Summary == "" {
		t.Error("expected summary for rejected path")
	}
}

func TestRequestExplainer_PolicyDeny(t *testing.T) {
	dir := setupExplainTestDir(t)

	resolver := filesystem.NewResolver(dir, filesystem.SymlinkDeny, nil)

	// Route that matches /api.
	routes := []router.Route{
		{PathPrefix: "/api", Target: "/index.php"},
	}
	eng, err := router.NewEngine(routes)
	if err != nil {
		t.Fatal(err)
	}

	// Policy that denies DELETE.
	policyEngine := policy.NewEngine()
	policyEngine.AddRule(policy.Rule{
		Name:  "deny-delete",
		Phase: policy.PhaseRequest,
		Conditions: []policy.Condition{
			{Type: policy.CondMethod, Values: []string{"DELETE"}},
		},
		Mode: policy.DecisionDeny,
	})

	explainer := NewRequestExplainer(resolver, eng, policyEngine, dir)

	r := &http.Request{
		Method: "DELETE",
		URL:    &url.URL{Path: "/api/users/1"},
		Host:   "example.com",
		Header: make(http.Header),
	}

	explain := explainer.Explain(r)
	if explain.PolicyCheck.Decision != "deny" {
		t.Errorf("expected deny decision, got %s", explain.PolicyCheck.Decision)
	}
}

func TestRequestExplainer_RouteAndFile(t *testing.T) {
	dir := setupExplainTestDir(t)

	// Create a real PHP file to resolve.
	os.WriteFile(filepath.Join(dir, "index.php"), []byte(`<?php ?>`), 0644)

	resolver := filesystem.NewResolver(dir, filesystem.SymlinkDeny, nil)
	routes := []router.Route{
		{PathPrefix: "/api", Target: "/index.php"},
	}
	eng, err := router.NewEngine(routes)
	if err != nil {
		t.Fatal(err)
	}
	policyEngine := policy.NewEngine()

	explainer := NewRequestExplainer(resolver, eng, policyEngine, dir)

	r := &http.Request{
		Method: "GET",
		URL:    &url.URL{Path: "/api/users"},
		Host:   "example.com",
		Header: make(http.Header),
	}

	explain := explainer.Explain(r)

	if !explain.PathNorm.Valid {
		t.Error("expected valid path normalization")
	}
	if explain.PolicyCheck.Decision != "allow" {
		t.Errorf("expected allow, got %s", explain.PolicyCheck.Decision)
	}
	if !explain.RouteMatch.Matched {
		t.Error("expected route to match /api prefix")
	}
	if explain.Summary == "" {
		t.Error("expected summary")
	}
	t.Logf("explain: %+v", explain.Summary)
}

func TestRequestExplainer_ProtectedFile(t *testing.T) {
	dir := setupExplainTestDir(t)
	os.WriteFile(filepath.Join(dir, "index.php"), []byte(`<?php ?>`), 0644)

	resolver := filesystem.NewResolver(dir, filesystem.SymlinkDeny, []string{".env"})
	policyEngine := policy.NewEngine()

	explainer := NewRequestExplainer(resolver, nil, policyEngine, dir)

	r := &http.Request{
		Method: "GET",
		URL:    &url.URL{Path: "/.env"},
		Host:   "example.com",
		Header: make(http.Header),
	}

	explain := explainer.Explain(r)
	if !explain.FileCheck.Protected {
		t.Error("expected .env to be detected as protected")
	}
}

func TestRequestExplainer_Summary(t *testing.T) {
	dir := setupExplainTestDir(t)
	os.WriteFile(filepath.Join(dir, "index.php"), []byte(`<?php ?>`), 0644)

	resolver := filesystem.NewResolver(dir, filesystem.SymlinkDeny, nil)
	routes := []router.Route{
		{PathPrefix: "/api", Target: "/index.php"},
	}
	eng, err := router.NewEngine(routes)
	if err != nil {
		t.Fatal(err)
	}
	policyEngine := policy.NewEngine()

	explainer := NewRequestExplainer(resolver, eng, policyEngine, dir)

	r := &http.Request{
		Method: "GET",
		URL:    &url.URL{Path: "/api/items"},
		Host:   "example.com",
		Header: make(http.Header),
	}

	explain := explainer.Explain(r)

	// Summary should contain route info.
	if explain.Summary == "" {
		t.Error("expected non-empty summary")
	}
	// Duration should be set.
	if explain.Duration == "" {
		t.Error("expected duration to be set")
	}
}

func TestRequestExplainer_NilPolicy(t *testing.T) {
	dir := setupExplainTestDir(t)

	resolver := filesystem.NewResolver(dir, filesystem.SymlinkDeny, nil)

	// Pass nil policy engine — should auto-create.
	explainer := NewRequestExplainer(resolver, nil, nil, dir)

	r := &http.Request{
		Method: "GET",
		URL:    &url.URL{Path: "/test"},
		Host:   "example.com",
		Header: make(http.Header),
	}

	explain := explainer.Explain(r)
	if explain.PolicyCheck.Decision != "allow" {
		t.Errorf("expected allow with nil policy, got %s", explain.PolicyCheck.Decision)
	}
}
