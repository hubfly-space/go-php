package diagnostics

import (
	"testing"
)

func TestHtaccessTranslator_BasicRewrite(t *testing.T) {
	translator := NewHtaccessTranslator()

	content := `RewriteEngine On
RewriteRule ^(.*)$ index.php/$1 [L]`

	routes, _ := translator.Translate(content)

	if len(routes) != 1 {
		t.Fatalf("expected 1 route, got %d", len(routes))
	}

	if routes[0].Regex == "" {
		t.Error("expected regex to be set")
	}
	if routes[0].Target != "index.php/$1" {
		t.Errorf("expected target index.php/$1, got %s", routes[0].Target)
	}
	t.Logf("route: %+v", routes[0])
}

func TestHtaccessTranslator_Redirect(t *testing.T) {
	translator := NewHtaccessTranslator()

	content := `RewriteEngine On
RewriteRule ^old-page$ new-page [R=301,L]`

	routes, _ := translator.Translate(content)

	if len(routes) != 1 {
		t.Fatalf("expected 1 route, got %d", len(routes))
	}

	if routes[0].Status != 301 {
		t.Errorf("expected status 301, got %d", routes[0].Status)
	}
}

func TestHtaccessTranslator_RedirectDirective(t *testing.T) {
	translator := NewHtaccessTranslator()

	content := `Redirect 301 /old http://example.com/new`

	routes, warnings := translator.Translate(content)

	if len(routes) != 1 {
		t.Fatalf("expected 1 route, got %d (warnings: %v)", len(routes), warnings)
	}

	if routes[0].Status != 301 {
		t.Errorf("expected status 301, got %d", routes[0].Status)
	}
}

func TestHtaccessTranslator_ConditionsWarning(t *testing.T) {
	translator := NewHtaccessTranslator()

	content := `RewriteEngine On
RewriteCond %{HTTPS} off
RewriteRule ^(.*)$ https://%{HTTP_HOST}/$1 [R=301,L]`

	routes, warnings := translator.Translate(content)

	if len(routes) != 1 {
		t.Fatalf("expected 1 route, got %d", len(routes))
	}

	// Should have a warning about RewriteCond.
	foundWarning := false
	for _, w := range warnings {
		if contains(w, "RewriteCond") {
			foundWarning = true
			break
		}
	}
	if !foundWarning {
		t.Error("expected warning about RewriteCond")
	}
}

func TestHtaccessTranslator_EmptyContent(t *testing.T) {
	translator := NewHtaccessTranslator()

	routes, warnings := translator.Translate("")
	if len(routes) != 0 {
		t.Errorf("expected 0 routes, got %d", len(routes))
	}
	if len(warnings) != 0 {
		t.Errorf("expected 0 warnings, got %d", len(warnings))
	}
}

func TestHtaccessTranslator_CommentsOnly(t *testing.T) {
	translator := NewHtaccessTranslator()

	content := `# This is a comment
# Another comment`

	routes, _ := translator.Translate(content)
	if len(routes) != 0 {
		t.Errorf("expected 0 routes for comments, got %d", len(routes))
	}
}

func TestApacheToGoRegex(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"^(.*)$", "^(.*)$"},
		{"/blog/(.*)", "/blog/(.*)"},
		{"^old%1new$", "^old$1new$"},
	}

	for _, tt := range tests {
		result := apacheToGoRegex(tt.input)
		if result != tt.expected {
			t.Errorf("apacheToGoRegex(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstr(s, substr))
}

func containsSubstr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
