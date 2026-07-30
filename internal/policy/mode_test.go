package policy

import (
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestParseMode(t *testing.T) {
	tests := []struct {
		in      string
		want    Mode
		wantErr bool
	}{
		{"off", ModeOff, false},
		{"observe", ModeObserve, false},
		{"balanced", ModeBalanced, false},
		{"strict", ModeStrict, false},
		{"", ModeBalanced, false},
		{"paranoid", "", true},
	}

	for _, tt := range tests {
		got, err := ParseMode(tt.in)
		if (err != nil) != tt.wantErr {
			t.Errorf("ParseMode(%q) error = %v, wantErr %v", tt.in, err, tt.wantErr)
			continue
		}
		if !tt.wantErr && got != tt.want {
			t.Errorf("ParseMode(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func serveWithMode(t *testing.T, mode Mode, method, path string) *httptest.ResponseRecorder {
	t.Helper()

	reached := false
	h := ModeMiddleware(NewEngineForMode(mode), mode, slog.Default())(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			reached = true
			w.WriteHeader(http.StatusOK)
		}),
	)

	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(method, path, nil))
	w.Flush()
	if w.Code == http.StatusForbidden && reached {
		t.Fatal("handler ran despite a 403")
	}
	return w
}

func TestModeBalancedBlocksVCSMetadata(t *testing.T) {
	w := serveWithMode(t, ModeBalanced, "GET", "/.git/config")
	if w.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403", w.Code)
	}
	if !strings.Contains(w.Body.String(), "deny-vcs-metadata") {
		t.Errorf("denial is not attributable to a rule: %s", w.Body.String())
	}
}

func TestModeObserveDoesNotBlock(t *testing.T) {
	w := serveWithMode(t, ModeObserve, "GET", "/.git/config")
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200 (observe must not block)", w.Code)
	}
	if got := w.Header().Get("X-Policy-Observed"); got != "deny-vcs-metadata" {
		t.Errorf("X-Policy-Observed = %q, want the rule name", got)
	}
}

func TestModeOffEvaluatesNoRules(t *testing.T) {
	w := serveWithMode(t, ModeOff, "GET", "/.git/config")
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
	if w.Header().Get("X-Policy-Observed") != "" {
		t.Error("off mode should not annotate responses")
	}
}

func TestModeOffReturnsHandlerUnwrapped(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})
	got := ModeMiddleware(NewEngineForMode(ModeOff), ModeOff, nil)(inner)

	// Not merely equivalent — the same handler, so the disabled path costs
	// nothing per request.
	if fmt.Sprintf("%p", got) != fmt.Sprintf("%p", inner) {
		t.Error("off mode should return the handler unwrapped")
	}
}

func TestModeStrictAddsRules(t *testing.T) {
	// Legal under balanced, denied under strict.
	if w := serveWithMode(t, ModeBalanced, "GET", "/notes.bak"); w.Code != http.StatusOK {
		t.Errorf("balanced status = %d, want 200", w.Code)
	}
	if w := serveWithMode(t, ModeStrict, "GET", "/notes.bak"); w.Code != http.StatusForbidden {
		t.Errorf("strict status = %d, want 403", w.Code)
	}
}

func TestModeBlocksTraceMethods(t *testing.T) {
	for _, method := range []string{"TRACE", "TRACK"} {
		if w := serveWithMode(t, ModeBalanced, method, "/"); w.Code != http.StatusForbidden {
			t.Errorf("%s status = %d, want 403", method, w.Code)
		}
	}
}

func TestModeAllowsOrdinaryTraffic(t *testing.T) {
	// A ruleset that false-positives on normal application traffic is worse
	// than no ruleset (§26.1).
	paths := []string{
		"/", "/index.php", "/wp-admin/admin-ajax.php",
		"/api/users?page=2", "/assets/app.css", "/.well-known/acme-challenge/x",
		"/blog/2024/01/hello-world", "/environment", "/gitignore-docs",
	}
	for _, mode := range []Mode{ModeBalanced, ModeStrict} {
		for _, p := range paths {
			if w := serveWithMode(t, mode, "GET", p); w.Code != http.StatusOK {
				t.Errorf("%s %s = %d, want 200", mode, p, w.Code)
			}
		}
	}
}

func TestEvaluateRuleReportsMatchedRule(t *testing.T) {
	e := NewEngineForMode(ModeBalanced)

	decision, rule := e.EvaluateRule(&Context{
		Phase:  PhaseRequest,
		Method: "GET",
		Path:   "/.git/HEAD",
	})

	if decision != DecisionDeny {
		t.Fatalf("decision = %v, want deny", decision)
	}
	if rule == nil {
		t.Fatal("no rule reported; the denial would be unexplainable (§23.3)")
	}
	if rule.Name != "deny-vcs-metadata" {
		t.Errorf("rule = %q, want deny-vcs-metadata", rule.Name)
	}
}
