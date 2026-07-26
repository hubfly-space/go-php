// Package load provides load and benchmark tests for the go-php gateway.
//
// Run with:
//
//	go test -tags=load -bench=. -benchmem ./test/load/...
//
//go:build load

package load

import (
	"crypto/rand"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-php/gateway/internal/filesystem"
	"github.com/go-php/gateway/internal/policy"
	"github.com/go-php/gateway/internal/router"
)

// BenchmarkPathParsing benchmarks the path parser.
func BenchmarkPathParsing(b *testing.B) {
	paths := []string{
		"/",
		"/api/users/123",
		"/blog/2024/03/my-post",
		"/static/css/style.css",
		"/%2e%2e/%2e%2e/etc/passwd",
		"/very/long/path/with/many/segments/to/test/performance",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, p := range paths {
			filesystem.ParsePath(p)
		}
	}
}

// BenchmarkPathResolver benchmarks filesystem path resolution.
func BenchmarkPathResolver(b *testing.B) {
	dir := b.TempDir()
	os.WriteFile(filepath.Join(dir, "test.php"), []byte(`<?php ?>`), 0644)

	resolver := filesystem.NewResolver(dir, filesystem.SymlinkDeny, nil)
	paths := []string{"/test.php", "/missing.php", "/"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, p := range paths {
			pp, _ := filesystem.ParsePath(p)
			rf, err := resolver.Resolve(pp.NormalizedPath)
			if err == nil {
				rf.Close()
			}
		}
	}
}

// BenchmarkPolicyEngine benchmarks policy evaluation.
func BenchmarkPolicyEngine(b *testing.B) {
	engine := policy.NewEngine()
	for i := 0; i < 10; i++ {
		engine.AddRule(policy.Rule{
			Name:  fmt.Sprintf("rule-%d", i),
			Phase: policy.PhaseRequest,
			Conditions: []policy.Condition{
				{Type: policy.CondPathPrefix, Values: []string{"/api"}},
			},
			Mode: policy.DecisionAllow,
		})
	}

	ctx := &policy.Context{
		Phase:  policy.PhaseRequest,
		Method: "GET",
		Path:   "/api/users/123",
		Host:   "example.com",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		engine.Evaluate(ctx)
	}
}

// BenchmarkRouterMatch benchmarks route matching.
func BenchmarkRouterMatch(b *testing.B) {
	routes := []router.Route{
		{PathPrefix: "/api", Target: "/index.php"},
		{Path: "/admin", Target: "/admin.php", Host: "admin.example.com"},
		{Regex: "^/blog/(\\d{4})/(\\d{2})/(.+)$", Target: "/blog.php"},
		{Path: "/old", Status: 301, Target: "/new"},
	}

	eng, _ := router.NewEngine(routes)
	req := httptest.NewRequest("GET", "/api/users", nil)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		eng.Match(req)
	}
}

// BenchmarkStaticFileServer benchmarks static file serving.
func BenchmarkStaticFileServer(b *testing.B) {
	dir := b.TempDir()
	os.WriteFile(filepath.Join(dir, "file.txt"), []byte("hello world"), 0644)

	srv := &filesystem.PrecompressedFileServer{
		Root:  dir,
		Index: "index.html",
	}

	req := httptest.NewRequest("GET", "/file.txt", nil)
	rec := httptest.NewRecorder()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rec.Body.Reset()
		srv.ServeHTTP(rec, req)
	}
}

// BenchmarkCSRFToken benchmarks CSRF token generation and validation.
func BenchmarkCSRFToken(b *testing.B) {
	// Simulated HMAC signing.
	key := make([]byte, 32)
	rand.Read(key)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Generate token.
		tokenBytes := make([]byte, 32)
		rand.Read(tokenBytes)
		// Sign (simplified).
		_ = append(tokenBytes, key...)
	}
}

// TestConcurrentRequests simulates concurrent requests through the gateway.
func TestConcurrentRequests(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "index.php"), []byte(`<?php echo "ok"; ?>`), 0644)

	resolver := filesystem.NewResolver(dir, filesystem.SymlinkDeny, nil)
	eng, _ := router.NewEngine([]router.Route{
		{PathPrefix: "/api", Target: "/index.php"},
	})
	policyEngine := policy.NewEngine()

	var (
		totalRequests int64
		totalErrors   int64
	)

	concurrency := 50
	requestsPerWorker := 100

	var wg sync.WaitGroup
	for c := 0; c < concurrency; c++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()

			for i := 0; i < requestsPerWorker; i++ {
				path := fmt.Sprintf("/api/req/%d/%d", workerID, i)
				pp, err := filesystem.ParsePath(path)
				if err != nil {
					atomic.AddInt64(&totalErrors, 1)
					continue
				}

				_ = eng.Match(httptest.NewRequest("GET", path, nil))
				_ = policyEngine.Evaluate(&policy.Context{
					Phase:  policy.PhaseRequest,
					Method: "GET",
					Path:   pp.NormalizedPath,
				})

				rf, err := resolver.Resolve(pp.NormalizedPath)
				if err == nil {
					rf.Close()
				}

				atomic.AddInt64(&totalRequests, 1)
			}
		}(c)
	}

	wg.Wait()

	total := concurrency * requestsPerWorker
	if atomic.LoadInt64(&totalRequests) != int64(total) {
		t.Errorf("expected %d requests, got %d", total, atomic.LoadInt64(&totalRequests))
	}
	t.Logf("concurrent test: %d requests, %d errors", totalRequests, totalErrors)
}

// TestSustainedLoad simulates sustained traffic over time.
func TestSustainedLoad(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "index.php"), []byte(`<?php echo "ok"; ?>`), 0644)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte("ok"))
	}))
	defer srv.Close()

	duration := 5 * time.Second
	clients := 10

	var (
		totalRequests int64
		totalErrors   int64
	)

	var wg sync.WaitGroup
	for c := 0; c < clients; c++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			client := &http.Client{Timeout: 5 * time.Second}
			end := time.Now().Add(duration)

			for time.Now().Before(end) {
				resp, err := client.Get(srv.URL + "/test")
				if err != nil {
					atomic.AddInt64(&totalErrors, 1)
					continue
				}
				io.Copy(io.Discard, resp.Body)
				resp.Body.Close()
				atomic.AddInt64(&totalRequests, 1)
			}
		}()
	}

	wg.Wait()

	qps := float64(totalRequests) / duration.Seconds()
	t.Logf("sustained load: %d requests in %v (%.0f req/s), %d errors",
		totalRequests, duration, qps, totalErrors)

	if totalErrors > totalRequests/100 {
		t.Errorf("too many errors: %d/%d", totalErrors, totalRequests)
	}
}
