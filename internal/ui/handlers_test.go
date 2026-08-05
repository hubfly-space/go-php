package ui

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestUIHandlers_Endpoints(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	status := NewStatusProvider("1.0.0", "127.0.0.1:8080", "/tmp", "Laravel")
	srv := NewServer(ServerConfig{Token: "test-token"}, logger, status)

	endpoints := []struct {
		path       string
		method     string
		wantStatus int
	}{
		{"/api/status", "GET", http.StatusOK},
		{"/api/health", "GET", http.StatusOK},
		{"/api/system", "GET", http.StatusOK},
		{"/api/doctor", "GET", http.StatusOK},
		{"/api/config", "GET", http.StatusOK},
		{"/api/sites", "GET", http.StatusOK},
		{"/api/extensions", "GET", http.StatusOK},
		{"/api/runtimes", "GET", http.StatusOK},
		{"/api/logs/recent", "GET", http.StatusOK},
		{"/api/profiles", "GET", http.StatusOK},
		{"/api/metrics/history", "GET", http.StatusOK},
	}

	for _, ep := range endpoints {
		t.Run(ep.path, func(t *testing.T) {
			req := httptest.NewRequest(ep.method, ep.path, nil)
			req.Header.Set("Authorization", "Bearer test-token")
			rec := httptest.NewRecorder()

			srv.mux.ServeHTTP(rec, req)

			if rec.Code != ep.wantStatus {
				t.Errorf("%s %s got status %d, want %d", ep.method, ep.path, rec.Code, ep.wantStatus)
			}
			if rec.Code == http.StatusOK && rec.Body.Len() == 0 {
				t.Errorf("%s %s returned empty body", ep.method, ep.path)
			}
		})
	}
}

func TestStatusProvider_GoroutinesAndUptime(t *testing.T) {
	sp := NewStatusProvider("1.0.0", "127.0.0.1:8080", "/tmp", "Laravel")

	if sp.Goroutines() <= 0 {
		t.Errorf("expected positive goroutine count, got %d", sp.Goroutines())
	}
	if sp.Uptime() <= 0 {
		t.Errorf("expected positive uptime, got %v", sp.Uptime())
	}
}

func TestUIHandlers_ConfigValidate(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	status := NewStatusProvider("1.0.0", "127.0.0.1:8080", "/tmp", "Laravel")
	srv := NewServer(ServerConfig{Token: "test-token"}, logger, status)

	req := httptest.NewRequest("GET", "/api/config/validate", nil)
	req.Header.Set("Authorization", "Bearer test-token")
	rec := httptest.NewRecorder()

	srv.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	var res map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
		t.Fatalf("invalid json response: %v", err)
	}
	if statusVal, ok := res["status"].(string); !ok || statusVal != "valid" {
		t.Errorf("expected status: valid in response, got %v", res["status"])
	}
}
