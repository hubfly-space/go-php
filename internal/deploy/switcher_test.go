package deploy

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestProbeRejectsMissingRelease(t *testing.T) {
	// Probe used to return true unconditionally, so the deploy pipeline's
	// health gate could never fail.
	p := &Prober{}

	if ok, err := p.Probe(context.Background(), nil); ok || err == nil {
		t.Errorf("Probe(nil) = (%v, %v), want (false, error)", ok, err)
	}

	if ok, err := p.Probe(context.Background(), &Release{Dir: ""}); ok || err == nil {
		t.Errorf("Probe(empty dir) = (%v, %v), want (false, error)", ok, err)
	}

	if ok, err := p.Probe(context.Background(), &Release{Dir: "/nonexistent/release"}); ok || err == nil {
		t.Errorf("Probe(missing dir) = (%v, %v), want (false, error)", ok, err)
	}
}

func TestProbeRejectsReleaseWithoutEntrypoint(t *testing.T) {
	dir := t.TempDir()
	// A directory with no index.php is not a servable release.
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}

	p := &Prober{}
	ok, err := p.Probe(context.Background(), &Release{Dir: dir})
	if ok || err == nil {
		t.Errorf("Probe = (%v, %v), want (false, error) for a release with no entrypoint", ok, err)
	}
}

func TestProbeAcceptsValidRelease(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "index.php"), []byte("<?php"), 0o644); err != nil {
		t.Fatal(err)
	}

	p := &Prober{}
	ok, err := p.Probe(context.Background(), &Release{Dir: dir})
	if !ok || err != nil {
		t.Errorf("Probe = (%v, %v), want (true, nil)", ok, err)
	}
}

func TestProbeAcceptsPublicSubdirectoryLayout(t *testing.T) {
	// Laravel and Symfony put the entrypoint under public/.
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "public"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "public", "index.php"), []byte("<?php"), 0o644); err != nil {
		t.Fatal(err)
	}

	p := &Prober{}
	if ok, err := p.Probe(context.Background(), &Release{Dir: dir}); !ok || err != nil {
		t.Errorf("Probe = (%v, %v), want (true, nil)", ok, err)
	}
}

func TestProbeHonoursHealthURL(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "index.php"), []byte("<?php"), 0o644); err != nil {
		t.Fatal(err)
	}

	failing := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer failing.Close()

	p := &Prober{HealthURL: failing.URL}
	if ok, err := p.Probe(context.Background(), &Release{Dir: dir}); ok || err == nil {
		t.Errorf("Probe = (%v, %v), want (false, error) when health returns 500", ok, err)
	}

	healthy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer healthy.Close()

	p2 := &Prober{HealthURL: healthy.URL}
	if ok, err := p2.Probe(context.Background(), &Release{Dir: dir}); !ok || err != nil {
		t.Errorf("Probe = (%v, %v), want (true, nil) when health returns 200", ok, err)
	}
}
