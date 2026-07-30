package observability

import (
	"fmt"
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

func TestMetricsSeriesAreBounded(t *testing.T) {
	m := NewMetrics()

	// Simulate a client minting series by hitting distinct routes. §32.3
	// forbids unbounded cardinality; the collector must collapse the overflow.
	for i := 0; i < maxMetricSeries*3; i++ {
		m.RecordRequest("GET", fmt.Sprintf("/route/%d", i), 200, 1.0)
	}

	if got := m.Series(); got > maxMetricSeries+1 {
		t.Errorf("series = %d, want <= %d (overflow must collapse)", got, maxMetricSeries+1)
	}

	// Totals must stay correct even when series collapse.
	if got := m.requestsTotal.Load(); got != int64(maxMetricSeries*3) {
		t.Errorf("requestsTotal = %d, want %d", got, maxMetricSeries*3)
	}
}

func TestMetricsLabelEscaping(t *testing.T) {
	m := NewMetrics()
	// A route label containing a quote and a backslash would otherwise break
	// the exposition format for every series after it.
	m.RecordRequest("GET", `/a"}x{y="\`, 200, 1.0)

	w := httptest.NewRecorder()
	m.PrometheusHandler().ServeHTTP(w, httptest.NewRequest("GET", "/metrics", nil))

	body := w.Body.String()
	if !strings.Contains(body, `\"`) || !strings.Contains(body, `\\`) {
		t.Errorf("label value was not escaped:\n%s", body)
	}

	// Every histogram line must have balanced quotes. Counting substrings is
	// not good enough here — in `\\"` the `\"` is not an escaped quote — so
	// scan with backslash state.
	for _, line := range strings.Split(body, "\n") {
		if !strings.HasPrefix(line, "gateway_request_duration_ms") {
			continue
		}
		quotes, escaped := 0, false
		for i := 0; i < len(line); i++ {
			switch {
			case escaped:
				escaped = false
			case line[i] == '\\':
				escaped = true
			case line[i] == '"':
				quotes++
			}
		}
		if quotes%2 != 0 {
			t.Errorf("unbalanced quotes (%d) in %q", quotes, line)
		}
	}
}

func TestMetricsHelpEmittedOncePerFamily(t *testing.T) {
	m := NewMetrics()
	m.RecordRequest("GET", "/a", 200, 1.0)
	m.RecordRequest("GET", "/b", 200, 1.0)
	m.RecordRequest("POST", "/c", 200, 1.0)

	w := httptest.NewRecorder()
	m.PrometheusHandler().ServeHTTP(w, httptest.NewRequest("GET", "/metrics", nil))

	if n := strings.Count(w.Body.String(), "# HELP gateway_request_duration_ms"); n != 1 {
		t.Errorf("HELP emitted %d times, want 1 (duplicates are a parse error)", n)
	}
}

func TestSetRouteLabelIsNoOpWithoutMiddleware(t *testing.T) {
	// Handlers call this unconditionally, so it must tolerate the metrics
	// middleware being absent.
	req := httptest.NewRequest("GET", "/test", nil)
	SetRouteLabel(req, "/some/route")
}

func TestMetricsMiddlewareUsesRouteLabel(t *testing.T) {
	m := NewMetrics()

	handler := m.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		SetRouteLabel(r, "/api/:id")
		w.WriteHeader(http.StatusOK)
	}))

	// Distinct paths, one shared route label: one series, not three.
	for _, p := range []string{"/api/1", "/api/2", "/api/3"} {
		handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", p, nil))
	}

	if got := m.Series(); got != 1 {
		t.Errorf("series = %d, want 1 (paths must collapse onto the route label)", got)
	}

	w := httptest.NewRecorder()
	m.PrometheusHandler().ServeHTTP(w, httptest.NewRequest("GET", "/metrics", nil))
	if !strings.Contains(w.Body.String(), `route="/api/:id"`) {
		t.Errorf("expected route label in output:\n%s", w.Body.String())
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
