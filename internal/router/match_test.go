package router

import (
	"net/http"
	"net/url"
	"testing"
)

func TestEngineMatchExactPath(t *testing.T) {
	engine, err := NewEngine([]Route{
		{Path: "/about", Target: "/pages/about.html"},
		{Path: "/contact", Target: "/pages/contact.html"},
	})
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name   string
		path   string
		expect bool
	}{
		{"exact match", "/about", true},
		{"no match", "/other", false},
		{"prefix not match", "/about/team", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &http.Request{URL: &url.URL{Path: tt.path}, Host: "example.com"}
			route := engine.Match(r)
			got := route != nil
			if got != tt.expect {
				t.Errorf("Match(%q) = %v, want %v", tt.path, got, tt.expect)
			}
		})
	}
}

func TestEngineMatchPrefix(t *testing.T) {
	engine, err := NewEngine([]Route{
		{PathPrefix: "/api/", Target: "/index.php"},
	})
	if err != nil {
		t.Fatal(err)
	}

	r := &http.Request{URL: &url.URL{Path: "/api/users"}, Host: "example.com"}
	route := engine.Match(r)
	if route == nil {
		t.Fatal("expected match for /api/users")
	}
	if route.Target != "/index.php" {
		t.Errorf("target = %q, want %q", route.Target, "/index.php")
	}
}

func TestEngineMatchHost(t *testing.T) {
	engine, err := NewEngine([]Route{
		{Host: "admin.example.com", Path: "/", Target: "/admin/index.php"},
		{Host: "example.com", Path: "/", Target: "/index.php"},
	})
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		host   string
		expect string
	}{
		{"admin.example.com", "/admin/index.php"},
		{"example.com", "/index.php"},
		{"other.com", ""},
	}

	for _, tt := range tests {
		t.Run(tt.host, func(t *testing.T) {
			r := &http.Request{URL: &url.URL{Path: "/"}, Host: tt.host}
			route := engine.Match(r)
			if tt.expect == "" {
				if route != nil {
					t.Errorf("expected no match for %s", tt.host)
				}
			} else {
				if route == nil {
					t.Fatalf("expected match for %s", tt.host)
				}
				if route.Target != tt.expect {
					t.Errorf("target = %q, want %q", route.Target, tt.expect)
				}
			}
		})
	}
}

func TestEngineMatchMethod(t *testing.T) {
	engine, err := NewEngine([]Route{
		{Path: "/submit", Methods: []string{"POST"}, Target: "/handle.php"},
	})
	if err != nil {
		t.Fatal(err)
	}

	r := &http.Request{URL: &url.URL{Path: "/submit"}, Host: "example.com", Method: "GET"}
	if route := engine.Match(r); route != nil {
		t.Error("GET /submit should not match POST-only route")
	}

	r.Method = "POST"
	if route := engine.Match(r); route == nil {
		t.Error("POST /submit should match POST-only route")
	}
}

func TestRouteRewrite(t *testing.T) {
	route := &Route{Path: "/old", Target: "/new"}
	if got := route.Rewrite("/old"); got != "/new" {
		t.Errorf("Rewrite = %q, want %q", got, "/new")
	}

	// $0 substitution.
	route2 := &Route{PathPrefix: "/api/", Target: "/proxy$0"}
	if got := route2.Rewrite("/api/users"); got != "/proxy/api/users" {
		t.Errorf("Rewrite = %q, want %q", got, "/proxy/api/users")
	}

	// Regex captures.
	route3 := &Route{Regex: `/blog/(\d{4})/(\d{2})`, Target: "/archive/$1-$2.html"}
	if got := route3.Rewrite("/blog/2024/03"); got != "/archive/2024-03.html" {
		t.Errorf("Rewrite = %q, want %q", got, "/archive/2024-03.html")
	}
}

func TestRouteIsRedirect(t *testing.T) {
	if (&Route{Status: 301}).IsRedirect() != true {
		t.Error("301 should be redirect")
	}
	if (&Route{Status: 302}).IsRedirect() != true {
		t.Error("302 should be redirect")
	}
	if (&Route{Status: 0}).IsRedirect() != false {
		t.Error("0 should not be redirect")
	}
	if (&Route{Status: 200}).IsRedirect() != false {
		t.Error("200 should not be redirect")
	}
}

func TestEngineMatchRegex(t *testing.T) {
	engine, err := NewEngine([]Route{
		{Regex: `^/blog/\d{4}/\d{2}/[^/]+$`, Target: "/index.php"},
	})
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		path   string
		expect bool
	}{
		{"/blog/2024/03/post", true},
		{"/blog/2024/03", false},
		{"/blog/abc/def/post", false},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			r := &http.Request{URL: &url.URL{Path: tt.path}, Host: "example.com"}
			route := engine.Match(r)
			got := route != nil
			if got != tt.expect {
				t.Errorf("Match(%q) = %v, want %v", tt.path, got, tt.expect)
			}
		})
	}
}

func TestEngineNoRoutes(t *testing.T) {
	engine, err := NewEngine([]Route{})
	if err != nil {
		t.Fatal(err)
	}
	r := &http.Request{URL: &url.URL{Path: "/anything"}, Host: "example.com"}
	if route := engine.Match(r); route != nil {
		t.Error("empty engine should not match")
	}
}

func TestEngineInvalidRegex(t *testing.T) {
	_, err := NewEngine([]Route{
		{Regex: `[invalid`, Target: "/"},
	})
	if err == nil {
		t.Error("expected error for invalid regex")
	}
}
