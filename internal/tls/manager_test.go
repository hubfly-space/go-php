package tls

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func selfSignedCert(t *testing.T, dir, name string) (certFile, keyFile string) {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}

	serial, _ := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))

	tmpl := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: name},
		NotBefore:    time.Now(),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:     []string{name},
	}

	certDER, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}

	certFile = filepath.Join(dir, name+".pem")
	keyFile = filepath.Join(dir, name+".key")

	certOut, _ := os.Create(certFile)
	pem.Encode(certOut, &pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	certOut.Close()

	keyDER, _ := x509.MarshalECPrivateKey(key)
	keyOut, _ := os.Create(keyFile)
	pem.Encode(keyOut, &pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
	keyOut.Close()

	return certFile, keyFile
}

func TestCertManagerLoadAndSNI(t *testing.T) {
	dir := t.TempDir()
	mgr := NewCertManager(dir)

	certFile, keyFile := selfSignedCert(t, dir, "example.com")

	if err := mgr.LoadCert("example.com", certFile, keyFile); err != nil {
		t.Fatal(err)
	}

	if !mgr.HasCert("example.com") {
		t.Error("expected cert for example.com")
	}
	if mgr.HasCert("other.com") {
		t.Error("should not have cert for other.com")
	}

	domains := mgr.Domains()
	if len(domains) != 1 || domains[0] != "example.com" {
		t.Errorf("domains = %v, want [example.com]", domains)
	}
}

func TestCertManagerWildcard(t *testing.T) {
	dir := t.TempDir()
	mgr := NewCertManager(dir)

	certFile, keyFile := selfSignedCert(t, dir, "*.example.com")
	mgr.LoadCert("*.example.com", certFile, keyFile)

	hello := &tls.ClientHelloInfo{ServerName: "sub.example.com"}
	cert, err := mgr.GetCertificate(hello)
	if err != nil {
		t.Fatal(err)
	}
	if cert == nil {
		t.Error("expected cert for sub.example.com")
	}
}

func TestCertManagerDefaultCert(t *testing.T) {
	dir := t.TempDir()
	mgr := NewCertManager(dir)

	certFile, keyFile := selfSignedCert(t, dir, "default")
	mgr.SetDefault(certFile, keyFile)

	hello := &tls.ClientHelloInfo{ServerName: "unknown.example.com"}
	cert, err := mgr.GetCertificate(hello)
	if err != nil {
		t.Fatal(err)
	}
	if cert == nil {
		t.Error("expected default cert")
	}
}

func TestCertManagerLoadDir(t *testing.T) {
	dir := t.TempDir()
	mgr := NewCertManager(dir)

	selfSignedCert(t, dir, "a.example.com")
	selfSignedCert(t, dir, "b.example.com")

	if err := mgr.LoadCertDir(dir); err != nil {
		t.Fatal(err)
	}

	domains := mgr.Domains()
	if len(domains) < 2 {
		t.Errorf("expected at least 2 domains, got %d", len(domains))
	}
}

func TestRedirectHandler(t *testing.T) {
	handler := RedirectHandler(":443")

	req := httptest.NewRequest("GET", "/page?q=1", nil)
	req.Host = "example.com"
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != 301 {
		t.Errorf("status = %d, want 301", w.Code)
	}
	loc := w.Header().Get("Location")
	if !strings.Contains(loc, "https://") {
		t.Errorf("Location = %q, expected https", loc)
	}
}
