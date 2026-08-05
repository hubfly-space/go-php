package observability

import (
	"net/http/httptest"
	"testing"
)

func TestTrustedProxyConfig(t *testing.T) {
	cfg, err := NewTrustedProxyConfig([]string{"127.0.0.1", "10.0.0.0/8"}, 5)
	if err != nil {
		t.Fatalf("NewTrustedProxyConfig failed: %v", err)
	}

	if !cfg.IsTrusted("127.0.0.1") {
		t.Error("expected 127.0.0.1 to be trusted")
	}
	if !cfg.IsTrusted("10.1.2.3") {
		t.Error("expected 10.1.2.3 to be trusted")
	}
	if cfg.IsTrusted("203.0.113.19") {
		t.Error("expected 203.0.113.19 to be untrusted")
	}
}

func TestClientIP_UntrustedPeer(t *testing.T) {
	cfg, _ := NewTrustedProxyConfig([]string{"127.0.0.1"}, 5)

	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "203.0.113.5:12345"
	req.Header.Set("X-Forwarded-For", "1.1.1.1")

	// Since peer (203.0.113.5) is untrusted, X-Forwarded-For MUST be ignored!
	ip := cfg.ClientIP(req)
	if ip != "203.0.113.5" {
		t.Errorf("got %q, want untrusted peer IP 203.0.113.5", ip)
	}
}

func TestClientIP_TrustedPeerChain(t *testing.T) {
	cfg, _ := NewTrustedProxyConfig([]string{"127.0.0.1", "10.0.0.1"}, 5)

	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	req.Header.Set("X-Forwarded-For", "203.0.113.19, 10.0.0.1")

	// Immediate peer 127.0.0.1 is trusted. Chain has 10.0.0.1 (trusted) and 203.0.113.19 (untrusted).
	// Result must be 203.0.113.19!
	ip := cfg.ClientIP(req)
	if ip != "203.0.113.19" {
		t.Errorf("got %q, want client IP 203.0.113.19", ip)
	}
}
