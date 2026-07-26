package tls

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// ACMEManager manages automatic TLS certificates via ACME (Let's Encrypt).
type ACMEManager struct {
	mu            sync.RWMutex
	certs         map[string]*tls.Certificate
	email         string
	cacheDir      string
	directoryURL  string
	httpChallenge *HTTPChallenge
}

// NewACMEManager creates an ACME certificate manager.
func NewACMEManager(email, cacheDir string) *ACMEManager {
	return &ACMEManager{
		certs:        make(map[string]*tls.Certificate),
		email:        email,
		cacheDir:     cacheDir,
		directoryURL: "https://acme-v02.api.letsencrypt.org/directory",
	}
}

// UseStaging switches to the Let's Encrypt staging environment.
func (m *ACMEManager) UseStaging() {
	m.directoryURL = "https://acme-staging-v02.api.letsencrypt.org/directory"
}

// Obtain attempts to obtain a certificate for the given domains.
// This is a simplified implementation — production would use an ACME library.
func (m *ACMEManager) Obtain(ctx context.Context, domains []string) (*tls.Certificate, error) {
	if len(domains) == 0 {
		return nil, fmt.Errorf("at least one domain required")
	}

	domain := domains[0]

	// Check cache first.
	cached, err := m.loadCached(domain)
	if err == nil && cached != nil {
		return cached, nil
	}

	// Generate a self-signed certificate as placeholder.
	// Real implementation would complete ACME HTTP-01 or DNS-01 challenge.
	cert, err := m.generateSelfSigned(domain)
	if err != nil {
		return nil, fmt.Errorf("generate cert: %w", err)
	}

	// Cache the certificate.
	if err := m.cacheCert(domain, cert); err != nil {
		// Non-fatal.
		return cert, nil
	}

	m.mu.Lock()
	m.certs[domain] = cert
	m.mu.Unlock()

	return cert, nil
}

// GetCertificate returns a cached certificate for the domain.
func (m *ACMEManager) GetCertificate(domain string) (*tls.Certificate, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	cert, ok := m.certs[domain]
	return cert, ok
}

// StartHTTPChallenge starts the HTTP-01 challenge server.
func (m *ACMEManager) StartHTTPChallenge(addr string) error {
	m.httpChallenge = NewHTTPChallenge(addr)
	return m.httpChallenge.Start()
}

// StopHTTPChallenge stops the challenge server.
func (m *ACMEManager) StopHTTPChallenge() {
	if m.httpChallenge != nil {
		m.httpChallenge.Stop()
	}
}

func (m *ACMEManager) generateSelfSigned(domain string) (*tls.Certificate, error) {
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}

	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: domain},
		NotBefore:    time.Now(),
		NotAfter:     time.Now().Add(90 * 24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:     []string{domain},
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &priv.PublicKey, priv)
	if err != nil {
		return nil, err
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	keyDER, _ := x509.MarshalECPrivateKey(priv)
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})

	certResult, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return nil, fmt.Errorf("create key pair: %w", err)
	}

	return &certResult, nil
}

func (m *ACMEManager) cacheCert(domain string, cert *tls.Certificate) error {
	dir := filepath.Join(m.cacheDir, domain)
	os.MkdirAll(dir, 0700)

	certFile := filepath.Join(dir, "cert.pem")
	keyFile := filepath.Join(dir, "key.pem")

	// Extract PEM from certificate.
	if len(cert.Certificate) == 0 {
		return fmt.Errorf("no certificate data")
	}

	certPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: cert.Certificate[0],
	})

	if err := os.WriteFile(certFile, certPEM, 0600); err != nil {
		return err
	}

	// Extract private key.
	if cert.PrivateKey == nil {
		return fmt.Errorf("no private key")
	}

	keyDER, err := x509.MarshalECPrivateKey(cert.PrivateKey.(*ecdsa.PrivateKey))
	if err != nil {
		return err
	}

	keyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "EC PRIVATE KEY",
		Bytes: keyDER,
	})

	return os.WriteFile(keyFile, keyPEM, 0600)
}

func (m *ACMEManager) loadCached(domain string) (*tls.Certificate, error) {
	if m.cacheDir == "" {
		return nil, fmt.Errorf("no cache dir")
	}

	certFile := filepath.Join(m.cacheDir, domain, "cert.pem")
	keyFile := filepath.Join(m.cacheDir, domain, "key.pem")

	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return nil, err
	}

	return &cert, nil
}

// HTTPChallenge handles ACME HTTP-01 challenges.
type HTTPChallenge struct {
	addr   string
	server *http.Server
	proofs map[string]string // token -> response
	mu     sync.RWMutex
}

// NewHTTPChallenge creates an HTTP challenge handler.
func NewHTTPChallenge(addr string) *HTTPChallenge {
	return &HTTPChallenge{
		addr:   addr,
		proofs: make(map[string]string),
	}
}

// AddProof adds a token/response pair for the challenge.
func (h *HTTPChallenge) AddProof(token, response string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.proofs[token] = response
}

// Start begins listening for challenge requests.
func (h *HTTPChallenge) Start() error {
	h.server = &http.Server{
		Addr:    h.addr,
		Handler: h,
	}

	go h.server.ListenAndServe()
	return nil
}

// Stop gracefully stops the challenge server.
func (h *HTTPChallenge) Stop() {
	if h.server != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		h.server.Shutdown(ctx)
	}
}

// ServeHTTP handles /.well-known/acme-challenge/ requests.
func (h *HTTPChallenge) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Path[len("/.well-known/acme-challenge/"):]

	h.mu.RLock()
	response, ok := h.proofs[token]
	h.mu.RUnlock()

	if !ok {
		http.NotFound(w, r)
		return
	}

	w.Header().Set("Content-Type", "text/plain")
	w.Write([]byte(response))
}
