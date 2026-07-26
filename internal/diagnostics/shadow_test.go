package diagnostics

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestShadowTester_Compare(t *testing.T) {
	// Create two test servers.
	active := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Source", "active")
		w.WriteHeader(200)
		w.Write([]byte("active response"))
	}))
	defer active.Close()

	candidate := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Source", "candidate")
		w.WriteHeader(200)
		w.Write([]byte("active response")) // same body
	}))
	defer candidate.Close()

	tester := NewShadowTester(active.URL, candidate.URL)

	result, err := tester.Compare(context.Background(), "GET", "/test", nil)
	if err != nil {
		t.Fatal(err)
	}

	if !result.StatusMatch {
		t.Error("expected status match")
	}
	if !result.BodyMatch {
		t.Error("expected body match")
	}
	t.Logf("result: %+v", result)
}

func TestShadowTester_StatusMismatch(t *testing.T) {
	active := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte("ok"))
	}))
	defer active.Close()

	candidate := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		w.Write([]byte("error"))
	}))
	defer candidate.Close()

	tester := NewShadowTester(active.URL, candidate.URL)

	result, err := tester.Compare(context.Background(), "GET", "/test", nil)
	if err != nil {
		t.Fatal(err)
	}

	if result.StatusMatch {
		t.Error("expected status mismatch")
	}
	if result.BodyMatch {
		t.Error("expected body mismatch")
	}
}

func TestShadowTester_Summary(t *testing.T) {
	active := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte("ok"))
	}))
	defer active.Close()

	candidate := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte("ok"))
	}))
	defer candidate.Close()

	tester := NewShadowTester(active.URL, candidate.URL)

	for i := 0; i < 5; i++ {
		tester.Compare(context.Background(), "GET", "/test", nil)
	}

	summary := tester.Summary()
	if summary.Total != 5 {
		t.Errorf("expected 5 total, got %d", summary.Total)
	}
	if !summary.IsSafe() {
		t.Error("expected safe summary")
	}
	t.Logf("summary: %s", summary.String())
}

func TestShadowTester_ShadowHeader(t *testing.T) {
	var gotShadowHeader bool

	active := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotShadowHeader = r.Header.Get("X-Shadow-Request") == "true"
		w.WriteHeader(200)
		w.Write([]byte("ok"))
	}))
	defer active.Close()

	candidate := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte("ok"))
	}))
	defer candidate.Close()

	tester := NewShadowTester(active.URL, candidate.URL)
	tester.Compare(context.Background(), "GET", "/test", nil)

	if !gotShadowHeader {
		t.Error("expected X-Shadow-Request header to be set")
	}
}

func TestShadowTester_ConcurrentAccess(t *testing.T) {
	active := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte("ok"))
	}))
	defer active.Close()

	candidate := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte("ok"))
	}))
	defer candidate.Close()

	tester := NewShadowTester(active.URL, candidate.URL)

	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func() {
			tester.Compare(context.Background(), "GET", "/test", nil)
			done <- true
		}()
	}

	for i := 0; i < 10; i++ {
		<-done
	}

	if tester.Summary().Total != 10 {
		t.Errorf("expected 10 results, got %d", tester.Summary().Total)
	}
}
