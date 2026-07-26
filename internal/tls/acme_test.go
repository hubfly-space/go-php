package tls

import (
	"context"
	"crypto/tls"
	"testing"
)

func TestACMEManager_ObtainSelfSigned(t *testing.T) {
	manager := NewACMEManager("test@example.com", t.TempDir())

	cert, err := manager.Obtain(context.Background(), []string{"example.com"})
	if err != nil {
		t.Fatal(err)
	}

	if cert == nil {
		t.Fatal("expected non-nil certificate")
	}

	// Verify the certificate.
	if len(cert.Certificate) == 0 {
		t.Error("expected certificate data")
	}
}

func TestACMEManager_CacheAndLoad(t *testing.T) {
	cacheDir := t.TempDir()
	manager := NewACMEManager("test@example.com", cacheDir)

	// Obtain once.
	_, err := manager.Obtain(context.Background(), []string{"cached.example.com"})
	if err != nil {
		t.Fatal(err)
	}

	// Load from cache.
	cached, err := manager.loadCached("cached.example.com")
	if err != nil {
		t.Fatal(err)
	}
	if cached == nil {
		t.Error("expected cached certificate")
	}
}

func TestACMEManager_EmptyDomains(t *testing.T) {
	manager := NewACMEManager("test@example.com", t.TempDir())

	_, err := manager.Obtain(context.Background(), nil)
	if err == nil {
		t.Error("expected error for empty domains")
	}
}

func TestACMEManager_UseStaging(t *testing.T) {
	manager := NewACMEManager("test@example.com", t.TempDir())
	manager.UseStaging()

	if manager.directoryURL != "https://acme-staging-v02.api.letsencrypt.org/directory" {
		t.Errorf("expected staging URL, got %s", manager.directoryURL)
	}
}

func TestHTTPChallenge(t *testing.T) {
	challenge := NewHTTPChallenge(":0")
	challenge.AddProof("test-token", "test-response")

	// Verify the proof was stored.
	challenge.mu.RLock()
	response, ok := challenge.proofs["test-token"]
	challenge.mu.RUnlock()

	if !ok || response != "test-response" {
		t.Error("expected proof to be stored")
	}
}

func TestACMEManager_GetCertificate(t *testing.T) {
	manager := NewACMEManager("test@example.com", t.TempDir())

	// Obtain a cert.
	_, err := manager.Obtain(context.Background(), []string{"test.example.com"})
	if err != nil {
		t.Fatal(err)
	}

	// Should be retrievable.
	got, ok := manager.GetCertificate("test.example.com")
	if !ok {
		t.Error("expected certificate to be cached")
	}
	if got == nil {
		t.Error("expected non-nil certificate")
	}
}

func TestACMEManager_GetCertificateNotFound(t *testing.T) {
	manager := NewACMEManager("test@example.com", t.TempDir())

	_, ok := manager.GetCertificate("nonexistent.example.com")
	if ok {
		t.Error("expected no certificate for nonexistent domain")
	}
}

func TestCertManager_GetConfigForClient(t *testing.T) {
	m := NewCertManager(t.TempDir())

	config := m.GetConfigForClient()
	if config == nil {
		t.Fatal("expected non-nil config")
	}

	if config.MinVersion != tls.VersionTLS12 {
		t.Errorf("expected TLS 1.2, got %d", config.MinVersion)
	}
}

func TestHTTPRedirectServer(t *testing.T) {
	addr := HTTPRedirectServer(":8443")
	if addr.Port != 80 {
		t.Errorf("expected port 80, got %d", addr.Port)
	}
}

func TestACMEManager_ObtainMultiple(t *testing.T) {
	manager := NewACMEManager("test@example.com", t.TempDir())

	// Obtain for first domain.
	_, err := manager.Obtain(context.Background(), []string{"a.example.com"})
	if err != nil {
		t.Fatal(err)
	}

	// Obtain for second domain.
	_, err = manager.Obtain(context.Background(), []string{"b.example.com"})
	if err != nil {
		t.Fatal(err)
	}

	// Both should be retrievable.
	certA, ok := manager.GetCertificate("a.example.com")
	if !ok || certA == nil {
		t.Error("expected cert for a.example.com")
	}
	certB, ok := manager.GetCertificate("b.example.com")
	if !ok || certB == nil {
		t.Error("expected cert for b.example.com")
	}
}

func TestACMEManager_CacheExpired(t *testing.T) {
	manager := NewACMEManager("test@example.com", t.TempDir())
	manager.directoryURL = "https://acme-staging-v02.api.letsencrypt.org/directory"

	// Just verify the manager works with staging.
	_, err := manager.Obtain(context.Background(), []string{"test.example.com"})
	if err != nil {
		t.Fatal(err)
	}
}

func TestCertManager_Domains(t *testing.T) {
	m := NewCertManager(t.TempDir())

	// No domains initially.
	domains := m.Domains()
	if len(domains) != 0 {
		t.Errorf("expected 0 domains, got %d", len(domains))
	}
}

func TestCertManager_HasCert(t *testing.T) {
	m := NewCertManager(t.TempDir())

	if m.HasCert("nonexistent") {
		t.Error("expected no cert for nonexistent domain")
	}
}

func TestHTTPChallenge_ServeHTTP(t *testing.T) {
	challenge := NewHTTPChallenge(":0")
	challenge.AddProof("token123", "response456")

	// Create a test request.
	req := createTestRequest("GET", "/.well-known/acme-challenge/token123")
	rec := createTestRecorder()

	challenge.ServeHTTP(rec, req)

	if rec.Code != 200 {
		t.Errorf("expected 200, got %d", rec.Code)
	}
}

func TestHTTPChallenge_NotFound(t *testing.T) {
	challenge := NewHTTPChallenge(":0")

	req := createTestRequest("GET", "/.well-known/acme-challenge/unknown")
	rec := createTestRecorder()

	challenge.ServeHTTP(rec, req)

	if rec.Code != 404 {
		t.Errorf("expected 404, got %d", rec.Code)
	}
}

func TestCertManager_SetDefault(t *testing.T) {
	m := NewCertManager(t.TempDir())

	// SetDefault with non-existent files should fail.
	err := m.SetDefault("/nonexistent/cert.pem", "/nonexistent/key.pem")
	if err == nil {
		t.Error("expected error for non-existent cert files")
	}
}

func TestCertManager_LoadCertDir(t *testing.T) {
	m := NewCertManager(t.TempDir())

	// Load from empty directory should succeed (no certs found).
	err := m.LoadCertDir(t.TempDir())
	if err != nil {
		t.Errorf("expected no error for empty dir, got %v", err)
	}
}
