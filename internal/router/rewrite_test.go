package router

import (
	"testing"
)

func TestRewriteLoop_TerminationAndIteration(t *testing.T) {
	routes := []Route{
		{
			Regex:  "^/old/(.*)$",
			Target: "/new/$1",
		},
		{
			Regex:  "^/new/(.*)$",
			Target: "/final/$1?rewritten=true",
		},
	}

	engine, err := NewEngine(routes)
	if err != nil {
		t.Fatalf("NewEngine failed: %v", err)
	}

	res, err := engine.RewriteLoop("/old/user", "id=123", 10)
	if err != nil {
		t.Fatalf("RewriteLoop failed: %v", err)
	}

	if res.Path != "/final/user" {
		t.Errorf("res.Path = %q, want /final/user", res.Path)
	}
	if res.RawQuery != "rewritten=true&id=123" {
		t.Errorf("res.RawQuery = %q, want rewritten=true&id=123", res.RawQuery)
	}
}

func TestRewriteLoop_MaxIterationsExceeded(t *testing.T) {
	// Infinite loop route
	routes := []Route{
		{
			Regex:  "^/loop/(.*)$",
			Target: "/loop/sub/$1",
		},
	}

	engine, err := NewEngine(routes)
	if err != nil {
		t.Fatalf("NewEngine failed: %v", err)
	}

	_, err = engine.RewriteLoop("/loop/test", "", 5)
	if err != ErrMaxIterationsExceeded {
		t.Errorf("expected ErrMaxIterationsExceeded, got %v", err)
	}
}

func TestRewriteLoop_RedirectTerminal(t *testing.T) {
	routes := []Route{
		{
			Path:   "/legacy",
			Target: "https://example.com/new",
			Status: 301,
		},
	}

	engine, err := NewEngine(routes)
	if err != nil {
		t.Fatalf("NewEngine failed: %v", err)
	}

	res, err := engine.RewriteLoop("/legacy", "", 10)
	if err != nil {
		t.Fatalf("RewriteLoop failed: %v", err)
	}

	if !res.IsTerminal || res.RedirectCode != 301 {
		t.Errorf("expected terminal redirect 301, got terminal=%v, code=%d", res.IsTerminal, res.RedirectCode)
	}
}
