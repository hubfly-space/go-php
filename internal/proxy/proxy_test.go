package proxy

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestProxy_HTTPForwarding(t *testing.T) {
	// Mock backend server
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Backend", "ok")
		fmt.Fprint(w, "Hello from Upstream!")
	}))
	defer backend.Close()

	p, err := NewProxy(ProxyConfig{
		Target: backend.URL,
	})
	if err != nil {
		t.Fatalf("NewProxy failed: %v", err)
	}

	req := httptest.NewRequest("GET", "/test", nil)
	rec := httptest.NewRecorder()

	p.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 OK, got %d", rec.Code)
	}
	if rec.Header().Get("X-Backend") != "ok" {
		t.Errorf("expected X-Backend: ok header")
	}
	body, _ := io.ReadAll(rec.Body)
	if string(body) != "Hello from Upstream!" {
		t.Errorf("got %q, want Hello from Upstream!", string(body))
	}
}

func TestProxy_InvalidTarget(t *testing.T) {
	_, err := NewProxy(ProxyConfig{
		Target: "",
	})
	if err == nil {
		t.Error("expected error for empty target URL")
	}
}

func TestIsWebSocketUpgrade(t *testing.T) {
	req := httptest.NewRequest("GET", "/ws", nil)
	req.Header.Set("Connection", "Upgrade")
	req.Header.Set("Upgrade", "websocket")

	if !isWebSocketUpgrade(req) {
		t.Error("expected isWebSocketUpgrade to be true")
	}

	reqNormal := httptest.NewRequest("GET", "/index.php", nil)
	if isWebSocketUpgrade(reqNormal) {
		t.Error("expected isWebSocketUpgrade to be false for normal HTTP request")
	}
}
