package tls

import (
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// CertManager manages TLS certificates and SNI routing.
type CertManager struct {
	mu       sync.RWMutex
	certs    map[string]*tls.Certificate // hostname -> cert
	defaultCert *tls.Certificate
	domains  []string
	baseDir  string
}

// NewCertManager creates a new TLS certificate manager.
func NewCertManager(baseDir string) *CertManager {
	return &CertManager{
		certs:   make(map[string]*tls.Certificate),
		baseDir: baseDir,
	}
}

// LoadCert loads a certificate from PEM files.
func (m *CertManager) LoadCert(hostname, certFile, keyFile string) error {
	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return fmt.Errorf("load cert for %s: %w", hostname, err)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	m.certs[hostname] = &cert
	m.domains = append(m.domains, hostname)

	return nil
}

// LoadCertDir loads all cert/key pairs from a directory.
// Expects files named <hostname>.pem and <hostname>.key.
func (m *CertManager) LoadCertDir(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("read cert dir: %w", err)
	}

	// Group by base name (without extension).
	byBase := make(map[string]map[string]string)
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		ext := filepath.Ext(e.Name())
		base := strings.TrimSuffix(e.Name(), ext)

		if ext != ".pem" && ext != ".key" {
			continue
		}

		if byBase[base] == nil {
			byBase[base] = make(map[string]string)
		}
		byBase[base][ext] = filepath.Join(dir, e.Name())
	}

	for base, files := range byBase {
		certFile, hasCert := files[".pem"]
		keyFile, hasKey := files[".key"]
		if !hasCert || !hasKey {
			continue
		}
		if err := m.LoadCert(base, certFile, keyFile); err != nil {
			return err
		}
	}

	return nil
}

// SetDefault sets the fallback certificate for unknown hosts.
func (m *CertManager) SetDefault(certFile, keyFile string) error {
	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return fmt.Errorf("load default cert: %w", err)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	m.defaultCert = &cert
	return nil
}

// GetCertificate implements tls.Config.GetCertificate for SNI routing.
func (m *CertManager) GetCertificate(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Try exact match.
	if cert, ok := m.certs[hello.ServerName]; ok {
		return cert, nil
	}

	// Try wildcard match.
	for domain, cert := range m.certs {
		if strings.HasPrefix(domain, "*.") {
			wildcard := domain[1:] // e.g. ".example.com"
			if strings.HasSuffix(hello.ServerName, wildcard) {
				return cert, nil
			}
		}
	}

	// Return default.
	if m.defaultCert != nil {
		return m.defaultCert, nil
	}

	return nil, fmt.Errorf("no certificate for %q", hello.ServerName)
}

// GetConfigForClient returns a tls.Config that uses SNI routing.
func (m *CertManager) GetConfigForClient() *tls.Config {
	return &tls.Config{
		GetCertificate: m.GetCertificate,
		MinVersion:     tls.VersionTLS12,
		CipherSuites: []uint16{
			tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
		},
	}
}

// Domains returns all loaded domains.
func (m *CertManager) Domains() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]string, len(m.domains))
	copy(result, m.domains)
	return result
}

// HasCert checks if a certificate exists for the given hostname.
func (m *CertManager) HasCert(hostname string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	_, ok := m.certs[hostname]
	return ok
}

// HTTPRedirectServer returns a TCPAddr for the HTTP redirect listener.
func HTTPRedirectServer(addr string) *net.TCPAddr {
	host, _, _ := net.SplitHostPort(addr)
	if host == "" {
		host = "0.0.0.0"
	}
	return &net.TCPAddr{
		IP:   net.ParseIP(host),
		Port: 80,
	}
}

// RedirectHandler returns an http.Handler that redirects to HTTPS.
func RedirectHandler(httpsAddr string) *redirectHandler {
	return &redirectHandler{httpsAddr: httpsAddr}
}

type redirectHandler struct {
	httpsAddr string
}

func (h *redirectHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	target := "https://" + r.Host
	if h.httpsAddr != "" {
		_, port, _ := net.SplitHostPort(h.httpsAddr)
		if port != "" && port != "443" {
			host, _, _ := net.SplitHostPort(r.Host)
			if host == "" {
				host = r.Host
			}
			target = "https://" + host + ":" + port
		}
	}
	target += r.URL.RequestURI()
	http.Redirect(w, r, target, http.StatusMovedPermanently)
}
