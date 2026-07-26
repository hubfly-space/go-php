package observability

import (
	"fmt"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Metrics collects gateway metrics for Prometheus export.
type Metrics struct {
	requestsTotal   atomic.Int64
	errorsTotal     atomic.Int64
	activeRequests  atomic.Int64
	responseTimeSum atomic.Int64 // microseconds
	responseCount   atomic.Int64

	startTime time.Time
	mu        sync.RWMutex
	histograms map[string]*Histogram
}

// Histogram is a simple histogram for response times.
type Histogram struct {
	buckets []float64
	counts  []int64
	sum     float64
	count   int64
}

// NewMetrics creates a metrics collector.
func NewMetrics() *Metrics {
	return &Metrics{
		startTime:  time.Now(),
		histograms: make(map[string]*Histogram),
	}
}

// RecordRequest records a completed request.
func (m *Metrics) RecordRequest(method, path string, status int, durationMs float64) {
	m.requestsTotal.Add(1)
	if status >= 500 {
		m.errorsTotal.Add(1)
	}

	m.responseTimeSum.Add(int64(durationMs * 1000))
	m.responseCount.Add(1)

	key := fmt.Sprintf("%s %s", method, path)
	m.mu.Lock()
	h, ok := m.histograms[key]
	if !ok {
		h = &Histogram{
			buckets: []float64{5, 10, 25, 50, 100, 250, 500, 1000, 2500, 5000},
			counts:  make([]int64, 11),
		}
		m.histograms[key] = h
	}
	m.mu.Unlock()

	h.record(durationMs)
}

// ActiveRequestInc increments active request count.
func (m *Metrics) ActiveRequestInc() {
	m.activeRequests.Add(1)
}

// ActiveRequestDec decrements active request count.
func (m *Metrics) ActiveRequestDec() {
	m.activeRequests.Add(-1)
}

func (h *Histogram) record(value float64) {
	h.sum += value
	h.count++
	for i, bucket := range h.buckets {
		if value <= bucket {
			h.counts[i]++
			return
		}
	}
	h.counts[len(h.buckets)]++ // +Inf bucket
}

// PrometheusHandler returns an HTTP handler that serves metrics in Prometheus format.
func (m *Metrics) PrometheusHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4")

		fmt.Fprintf(w, "# HELP gateway_requests_total Total HTTP requests\n")
		fmt.Fprintf(w, "# TYPE gateway_requests_total counter\n")
		fmt.Fprintf(w, "gateway_requests_total %d\n", m.requestsTotal.Load())

		fmt.Fprintf(w, "# HELP gateway_errors_total Total HTTP errors (5xx)\n")
		fmt.Fprintf(w, "# TYPE gateway_errors_total counter\n")
		fmt.Fprintf(w, "gateway_errors_total %d\n", m.errorsTotal.Load())

		fmt.Fprintf(w, "# HELP gateway_active_requests Currently active requests\n")
		fmt.Fprintf(w, "# TYPE gateway_active_requests gauge\n")
		fmt.Fprintf(w, "gateway_active_requests %d\n", m.activeRequests.Load())

		fmt.Fprintf(w, "# HELP gateway_uptime_seconds Gateway uptime in seconds\n")
		fmt.Fprintf(w, "# TYPE gateway_uptime_seconds gauge\n")
		fmt.Fprintf(w, "gateway_uptime_seconds %.0f\n", time.Since(m.startTime).Seconds())

		if count := m.responseCount.Load(); count > 0 {
			avg := float64(m.responseTimeSum.Load()) / float64(count) / 1000.0
			fmt.Fprintf(w, "# HELP gateway_response_time_avg_ms Average response time\n")
			fmt.Fprintf(w, "# TYPE gateway_response_time_avg_ms gauge\n")
			fmt.Fprintf(w, "gateway_response_time_avg_ms %.2f\n", avg)
		}

		// Per-route histograms.
		m.mu.RLock()
		for key, h := range m.histograms {
			parts := strings.SplitN(key, " ", 2)
			method, path := parts[0], parts[1]
			safeName := strings.ReplaceAll(path, "/", "_")
			if safeName == "" {
				safeName = "root"
			}

			fmt.Fprintf(w, "# HELP gateway_request_duration_ms Request duration histogram\n")
			fmt.Fprintf(w, "# TYPE gateway_request_duration_ms histogram\n")

			cumulative := int64(0)
			for i, bucket := range h.buckets {
				cumulative += h.counts[i]
				fmt.Fprintf(w, "gateway_request_duration_ms_bucket{method=\"%s\",path=\"%s\",le=\"%.0f\"} %d\n",
					method, path, bucket, cumulative)
			}
			cumulative += h.counts[len(h.buckets)]
			fmt.Fprintf(w, "gateway_request_duration_ms_bucket{method=\"%s\",path=\"%s\",le=\"+Inf\"} %d\n",
				method, path, cumulative)
			fmt.Fprintf(w, "gateway_request_duration_ms_sum{method=\"%s\",path=\"%s\"} %.2f\n",
				method, path, h.sum)
			fmt.Fprintf(w, "gateway_request_duration_ms_count{method=\"%s\",path=\"%s\"} %d\n",
				method, path, h.count)

			_ = safeName
		}
		m.mu.RUnlock()
	})
}

// Middleware returns an HTTP middleware that records metrics.
func (m *Metrics) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		m.ActiveRequestInc()
		defer m.ActiveRequestDec()

		rw := NewResponseWriter(w)
		next.ServeHTTP(rw, r)

		duration := float64(time.Since(start).Milliseconds())
		m.RecordRequest(r.Method, r.URL.Path, rw.Status(), duration)
	})
}
