package policy

import (
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPolicyEngineAllow(t *testing.T) {
	engine := NewEngine()

	engine.AddRule(Rule{
		Name:  "allow-get",
		Phase: PhaseRequest,
		Mode:  DecisionAllow,
		Conditions: []Condition{
			{Type: CondMethod, Values: []string{"GET"}},
		},
	})

	ctx := &Context{
		Phase:  PhaseRequest,
		Method: "GET",
		Path:   "/test",
	}

	if got := engine.Evaluate(ctx); got != DecisionAllow {
		t.Errorf("GET should be allowed, got %v", got)
	}
}

func TestPolicyEngineDeny(t *testing.T) {
	engine := NewEngine()

	engine.AddRule(Rule{
		Name:  "deny-delete",
		Phase: PhaseRequest,
		Mode:  DecisionDeny,
		Conditions: []Condition{
			{Type: CondMethod, Values: []string{"DELETE"}},
		},
	})

	ctx := &Context{
		Phase:  PhaseRequest,
		Method: "DELETE",
		Path:   "/test",
	}

	if got := engine.Evaluate(ctx); got != DecisionDeny {
		t.Errorf("DELETE should be denied, got %v", got)
	}
}

func TestPolicyEngineObserve(t *testing.T) {
	engine := NewEngine()

	engine.AddRule(Rule{
		Name:  "observe-admin",
		Phase: PhaseRequest,
		Mode:  DecisionObserve,
		Conditions: []Condition{
			{Type: CondPathPrefix, Values: []string{"/admin"}},
		},
	})

	ctx := &Context{
		Phase: PhaseRequest,
		Path:  "/admin/settings",
	}

	if got := engine.Evaluate(ctx); got != DecisionObserve {
		t.Errorf("/admin should be observed, got %v", got)
	}
}

func TestPolicyEnginePathRegex(t *testing.T) {
	engine := NewEngine()

	engine.AddRule(Rule{
		Name:  "block-sql-ext",
		Phase: PhaseRequest,
		Mode:  DecisionDeny,
		Conditions: []Condition{
			{Type: CondPathRegex, Values: []string{`\.sql$`}},
		},
	})

	tests := []struct {
		path   string
		expect Decision
	}{
		{"/dump.sql", DecisionDeny},
		{"/data.sql.bak", DecisionAllow},
		{"/page.php", DecisionAllow},
	}

	for _, tt := range tests {
		ctx := &Context{Phase: PhaseRequest, Path: tt.path}
		if got := engine.Evaluate(ctx); got != tt.expect {
			t.Errorf("path %q: got %v, want %v", tt.path, got, tt.expect)
		}
	}
}

func TestPolicyEngineHostMatch(t *testing.T) {
	engine := NewEngine()

	engine.AddRule(Rule{
		Name:  "admin-host",
		Phase: PhaseRequest,
		Mode:  DecisionDeny,
		Conditions: []Condition{
			{Type: CondHost, Values: []string{"admin.internal.com"}},
		},
	})

	ctx := &Context{
		Phase: PhaseRequest,
		Host:  "admin.internal.com",
		Path:  "/",
	}

	if got := engine.Evaluate(ctx); got != DecisionDeny {
		t.Errorf("admin host should be denied, got %v", got)
	}

	ctx.Host = "public.com"
	if got := engine.Evaluate(ctx); got != DecisionAllow {
		t.Errorf("public host should be allowed, got %v", got)
	}
}

func TestPolicyEngineExclusion(t *testing.T) {
	engine := NewEngine()

	engine.AddRule(Rule{
		Name:  "rate-limit-api",
		Phase: PhaseRequest,
		Mode:  DecisionDeny,
		Conditions: []Condition{
			{Type: CondPathPrefix, Values: []string{"/api/"}},
		},
		Exclusions: []Exclusion{
			{Type: CondPathPrefix, Values: []string{"/api/health"}},
		},
	})

	tests := []struct {
		path   string
		expect Decision
	}{
		{"/api/users", DecisionDeny},
		{"/api/health", DecisionAllow}, // excluded
		{"/other", DecisionAllow},
	}

	for _, tt := range tests {
		ctx := &Context{Phase: PhaseRequest, Path: tt.path}
		if got := engine.Evaluate(ctx); got != tt.expect {
			t.Errorf("path %q: got %v, want %v", tt.path, got, tt.expect)
		}
	}
}

func TestPolicyEngineNegation(t *testing.T) {
	engine := NewEngine()

	engine.AddRule(Rule{
		Name:  "block-non-get",
		Phase: PhaseRequest,
		Mode:  DecisionDeny,
		Conditions: []Condition{
			{Type: CondMethod, Values: []string{"GET"}, Negate: true},
		},
	})

	getCtx := &Context{Phase: PhaseRequest, Method: "GET"}
	if got := engine.Evaluate(getCtx); got != DecisionAllow {
		t.Errorf("GET should be allowed (negation), got %v", got)
	}

	postCtx := &Context{Phase: PhaseRequest, Method: "POST"}
	if got := engine.Evaluate(postCtx); got != DecisionDeny {
		t.Errorf("POST should be denied (negation), got %v", got)
	}
}

func TestPolicyEngineIPRange(t *testing.T) {
	engine := NewEngine()

	engine.AddRule(Rule{
		Name:  "block-internal",
		Phase: PhaseRequest,
		Mode:  DecisionDeny,
		Conditions: []Condition{
			{Type: CondIPRange, Values: []string{"10.0.0.0/8"}},
		},
	})

	internalCtx := &Context{Phase: PhaseRequest, RemoteIP: net.ParseIP("10.0.0.1")}
	if got := engine.Evaluate(internalCtx); got != DecisionDeny {
		t.Errorf("internal IP should be denied, got %v", got)
	}

	publicCtx := &Context{Phase: PhaseRequest, RemoteIP: net.ParseIP("8.8.8.8")}
	if got := engine.Evaluate(publicCtx); got != DecisionAllow {
		t.Errorf("public IP should be allowed, got %v", got)
	}
}

func TestPolicyEngineBodySize(t *testing.T) {
	engine := NewEngine()

	engine.AddRule(Rule{
		Name:  "limit-upload",
		Phase: PhaseRequest,
		Mode:  DecisionDeny,
		Conditions: []Condition{
			{Type: CondBodySize, Values: []string{"1048576"}}, // 1MB
		},
	})

	smallCtx := &Context{Phase: PhaseRequest, BodySize: 500}
	if got := engine.Evaluate(smallCtx); got != DecisionAllow {
		t.Errorf("small body should be allowed, got %v", got)
	}

	largeCtx := &Context{Phase: PhaseRequest, BodySize: 2000000}
	if got := engine.Evaluate(largeCtx); got != DecisionDeny {
		t.Errorf("large body should be denied, got %v", got)
	}
}

func TestPolicyEnginePriority(t *testing.T) {
	engine := NewEngine()

	// Higher priority (lower number) deny rule.
	engine.AddRule(Rule{
		Name:     "deny-admin",
		Phase:    PhaseRequest,
		Mode:     DecisionDeny,
		Priority: 10,
		Conditions: []Condition{
			{Type: CondPathPrefix, Values: []string{"/admin"}},
		},
	})

	// Lower priority (higher number) allow rule.
	engine.AddRule(Rule{
		Name:     "allow-admin-health",
		Phase:    PhaseRequest,
		Mode:     DecisionAllow,
		Priority: 20,
		Conditions: []Condition{
			{Type: CondPath, Values: []string{"/admin/health"}},
		},
	})

	ctx := &Context{Phase: PhaseRequest, Path: "/admin/settings"}
	if got := engine.Evaluate(ctx); got != DecisionDeny {
		t.Errorf("/admin should be denied by higher priority rule, got %v", got)
	}
}

func TestPolicyEngineClear(t *testing.T) {
	engine := NewEngine()
	engine.AddRule(Rule{
		Name:  "test",
		Phase: PhaseRequest,
		Mode:  DecisionDeny,
	})

	engine.Clear()

	ctx := &Context{Phase: PhaseRequest, Method: "GET"}
	if got := engine.Evaluate(ctx); got != DecisionAllow {
		t.Errorf("after clear, should default to allow, got %v", got)
	}
}

func TestPolicyEngineMiddleware(t *testing.T) {
	engine := NewEngine()
	engine.AddRule(Rule{
		Name:  "block-post",
		Phase: PhaseRequest,
		Mode:  DecisionDeny,
		Conditions: []Condition{
			{Type: CondMethod, Values: []string{"POST"}},
		},
	})

	handler := engine.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// GET should pass.
	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Errorf("GET: status = %d, want 200", w.Code)
	}

	// POST should be blocked.
	req = httptest.NewRequest("POST", "/test", nil)
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != 403 {
		t.Errorf("POST: status = %d, want 403", w.Code)
	}
}

func TestPolicyEngineSchemeMatch(t *testing.T) {
	engine := NewEngine()

	engine.AddRule(Rule{
		Name:  "require-https",
		Phase: PhaseRequest,
		Mode:  DecisionDeny,
		Conditions: []Condition{
			{Type: CondScheme, Values: []string{"http"}},
		},
	})

	httpCtx := &Context{Phase: PhaseRequest, TLS: false}
	if got := engine.Evaluate(httpCtx); got != DecisionDeny {
		t.Errorf("HTTP should be denied, got %v", got)
	}

	httpsCtx := &Context{Phase: PhaseRequest, TLS: true}
	if got := engine.Evaluate(httpsCtx); got != DecisionAllow {
		t.Errorf("HTTPS should be allowed, got %v", got)
	}
}

func TestPolicyEngineRules(t *testing.T) {
	engine := NewEngine()
	engine.AddRule(Rule{Name: "a", Phase: PhaseRequest, Mode: DecisionAllow})
	engine.AddRule(Rule{Name: "b", Phase: PhaseRequest, Mode: DecisionDeny})

	rules := engine.Rules()
	if len(rules) != 2 {
		t.Errorf("rules = %d, want 2", len(rules))
	}
}
