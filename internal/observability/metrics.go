package observability

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// maxMetricSeries bounds how many distinct method+route series are tracked.
// §32.3 forbids high-cardinality labels; this is the backstop for a caller that
// passes something unbounded anyway. Series beyond the cap collapse into
// overflowRoute so the totals stay correct.
const maxMetricSeries = 512

// overflowRoute is the route label used once maxMetricSeries is reached.
const overflowRoute = "__overflow__"

// unmatchedRoute is the route label for requests that matched no configured
// route. Using a constant rather than the request path is what keeps
// cardinality bounded.
const unmatchedRoute = "unmatched"

// Metrics collects gateway metrics for Prometheus export.
type Metrics struct {
	requestsTotal   atomic.Int64
	errorsTotal     atomic.Int64
	activeRequests  atomic.Int64
	responseTimeSum atomic.Int64 // microseconds
	responseCount   atomic.Int64

	startTime time.Time

	// mu guards histograms and the Histogram values inside it. Histogram is
	// not safe for concurrent use on its own.
	mu         sync.RWMutex
	histograms map[string]*Histogram
}

// Histogram is a simple histogram for response times. All access is guarded by
// the owning Metrics.mu.
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
//
// route must be a bounded label — a route identifier or pattern, never a raw
// request path. Passing raw paths would let a client mint unlimited series
// (§32.3). Values beyond maxMetricSeries collapse into a single overflow label.
func (m *Metrics) RecordRequest(method, route string, status int, durationMs float64) {
	m.requestsTotal.Add(1)
	if status >= 500 {
		m.errorsTotal.Add(1)
	}

	m.responseTimeSum.Add(int64(durationMs * 1000))
	m.responseCount.Add(1)

	if route == "" {
		route = unmatchedRoute
	}
	key := method + " " + route

	m.mu.Lock()
	defer m.mu.Unlock()

	h, ok := m.histograms[key]
	if !ok {
		if len(m.histograms) >= maxMetricSeries {
			key = method + " " + overflowRoute
			h, ok = m.histograms[key]
		}
		if !ok {
			h = &Histogram{
				buckets: []float64{5, 10, 25, 50, 100, 250, 500, 1000, 2500, 5000},
				counts:  make([]int64, 11),
			}
			m.histograms[key] = h
		}
	}

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

// Series returns the number of distinct histogram series currently tracked.
func (m *Metrics) Series() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.histograms)
}

// record adds a value. The caller must hold the owning Metrics.mu for writing.
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

// escapeLabelValue escapes a Prometheus label value per the text exposition
// format: backslash, double quote, and newline. Without this a label containing
// a quote corrupts the whole response.
func escapeLabelValue(s string) string {
	if !strings.ContainsAny(s, `\"`+"\n") {
		return s
	}
	var b strings.Builder
	b.Grow(len(s) + 8)
	for _, r := range s {
		switch r {
		case '\\':
			b.WriteString(`\\`)
		case '"':
			b.WriteString(`\"`)
		case '\n':
			b.WriteString(`\n`)
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
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

		// Snapshot under the lock, then write. HELP and TYPE are emitted once
		// for the metric family — repeating them per series is a parse error
		// in strict Prometheus parsers.
		type series struct {
			method, route string
			buckets       []float64
			counts        []int64
			sum           float64
			count         int64
		}

		m.mu.RLock()
		snapshot := make([]series, 0, len(m.histograms))
		for key, h := range m.histograms {
			method, route, _ := strings.Cut(key, " ")
			counts := make([]int64, len(h.counts))
			copy(counts, h.counts)
			snapshot = append(snapshot, series{
				method: method, route: route,
				buckets: h.buckets, counts: counts,
				sum: h.sum, count: h.count,
			})
		}
		m.mu.RUnlock()

		if len(snapshot) == 0 {
			return
		}

		// Stable output makes the endpoint diffable between scrapes.
		sort.Slice(snapshot, func(i, j int) bool {
			if snapshot[i].method != snapshot[j].method {
				return snapshot[i].method < snapshot[j].method
			}
			return snapshot[i].route < snapshot[j].route
		})

		fmt.Fprintf(w, "# HELP gateway_request_duration_ms Request duration histogram\n")
		fmt.Fprintf(w, "# TYPE gateway_request_duration_ms histogram\n")

		for _, s := range snapshot {
			method := escapeLabelValue(s.method)
			route := escapeLabelValue(s.route)

			cumulative := int64(0)
			for i, bucket := range s.buckets {
				cumulative += s.counts[i]
				fmt.Fprintf(w, "gateway_request_duration_ms_bucket{method=\"%s\",route=\"%s\",le=\"%.0f\"} %d\n",
					method, route, bucket, cumulative)
			}
			cumulative += s.counts[len(s.buckets)]
			fmt.Fprintf(w, "gateway_request_duration_ms_bucket{method=\"%s\",route=\"%s\",le=\"+Inf\"} %d\n",
				method, route, cumulative)
			fmt.Fprintf(w, "gateway_request_duration_ms_sum{method=\"%s\",route=\"%s\"} %.2f\n",
				method, route, s.sum)
			fmt.Fprintf(w, "gateway_request_duration_ms_count{method=\"%s\",route=\"%s\"} %d\n",
				method, route, s.count)
		}
	})
}

// Middleware returns an HTTP middleware that records metrics.
//
// The route label is read from the request context, so it must run outside the
// handler that sets it — the label is resolved after next.ServeHTTP returns.
func (m *Metrics) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		m.ActiveRequestInc()
		defer m.ActiveRequestDec()

		// The handler records its label here rather than on the context it
		// owns, because a context set downstream is not visible to us.
		label := &routeLabelCarrier{}
		r = r.WithContext(context.WithValue(r.Context(), routeCarrierKey{}, label))

		rw := NewResponseWriter(w)
		next.ServeHTTP(rw, r)

		duration := float64(time.Since(start).Milliseconds())
		m.RecordRequest(r.Method, label.Get(), rw.Status(), duration)
	})
}

// routeLabelCarrier lets a downstream handler report its matched route back up
// to the metrics middleware. A plain context value cannot do this, because
// values set downstream are invisible to the middleware that wrapped it.
type routeLabelCarrier struct {
	mu    sync.Mutex
	label string
}

type routeCarrierKey struct{}

// Set records the matched route label for this request.
func (c *routeLabelCarrier) Set(label string) {
	c.mu.Lock()
	c.label = label
	c.mu.Unlock()
}

// Get returns the recorded label, or "unmatched" if none was set.
func (c *routeLabelCarrier) Get() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.label == "" {
		return unmatchedRoute
	}
	return c.label
}

// SetRouteLabel records the matched route for metrics purposes. It is a no-op
// when the metrics middleware is not installed, so handlers can call it
// unconditionally.
func SetRouteLabel(r *http.Request, label string) {
	if c, ok := r.Context().Value(routeCarrierKey{}).(*routeLabelCarrier); ok {
		c.Set(label)
	}
}
