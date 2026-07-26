package observability

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMetricsRecordRequest(t *testing.T) {
	m := NewMetrics()

	m.RecordRequest("GET", "/api/test", 200, 15.5)
	m.RecordRequest("POST", "/api/test", 500, 120.0)

	if m.requestsTotal.Load() != 2 {
		t.Errorf("requests = %d, want 2", m.requestsTotal.Load())
	}
	if m.errorsTotal.Load() != 1 {
		t.Errorf("errors = %d, want 1", m.errorsTotal.Load())
	}
}

func TestMetricsActiveRequests(t *testing.T) {
	m := NewMetrics()

	m.ActiveRequestInc()
	m.ActiveRequestInc()

	if m.activeRequests.Load() != 2 {
		t.Errorf("active = %d, want 2", m.activeRequests.Load())
	}

	m.ActiveRequestDec()

	if m.activeRequests.Load() != 1 {
		t.Errorf("active = %d, want 1", m.activeRequests.Load())
	}
}

func TestMetricsPrometheusHandler(t *testing.T) {
	m := NewMetrics()
	m.RecordRequest("GET", "/test", 200, 10.0)

	handler := m.PrometheusHandler()
	req := httptest.NewRequest("GET", "/metrics", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Errorf("status = %d, want 200", w.Code)
	}

	body := w.Body.String()
	if !strings.Contains(body, "gateway_requests_total") {
		t.Error("expected gateway_requests_total in output")
	}
	if !strings.Contains(body, "gateway_uptime_seconds") {
		t.Error("expected gateway_uptime_seconds in output")
	}
}

func TestMetricsMiddleware(t *testing.T) {
	m := NewMetrics()

	handler := m.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if m.requestsTotal.Load() != 1 {
		t.Errorf("requests = %d, want 1", m.requestsTotal.Load())
	}
}

func TestHistogram(t *testing.T) {
	h := &Histogram{
		buckets: []float64{10, 100},
		counts:  make([]int64, 3),
	}

	h.record(5.0)
	h.record(50.0)
	h.record(200.0)

	if h.count != 3 {
		t.Errorf("count = %d, want 3", h.count)
	}
	if h.counts[0] != 1 {
		t.Errorf("le=10 count = %d, want 1", h.counts[0])
	}
	if h.counts[1] != 1 {
		t.Errorf("le=100 count = %d, want 1", h.counts[1])
	}
	if h.counts[2] != 1 {
		t.Errorf("+Inf count = %d, want 1", h.counts[2])
	}
	if h.sum != 255.0 {
		t.Errorf("sum = %v, want 255", h.sum)
	}
}
