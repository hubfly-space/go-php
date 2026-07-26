package observability

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"
)

// TraceID is a unique identifier for a request trace.
type TraceID string

// SpanID is a unique identifier for a span within a trace.
type SpanID string

// Span represents a unit of work within a trace.
type Span struct {
	TraceID    TraceID           `json:"trace_id"`
	SpanID     SpanID            `json:"span_id"`
	ParentID   SpanID            `json:"parent_id,omitempty"`
	Name       string            `json:"name"`
	StartTime  time.Time         `json:"start_time"`
	EndTime    time.Time         `json:"end_time,omitempty"`
	Duration   time.Duration     `json:"duration,omitempty"`
	Status     string            `json:"status"`
	Attributes map[string]string `json:"attributes,omitempty"`
	Events     []SpanEvent       `json:"events,omitempty"`
}

// SpanEvent is a timestamped event within a span.
type SpanEvent struct {
	Name       string            `json:"name"`
	Timestamp  time.Time         `json:"timestamp"`
	Attributes map[string]string `json:"attributes,omitempty"`
}

// Tracer manages trace and span creation.
type Tracer struct {
	serviceName string
	spans       map[TraceID][]*Span
	mu          sync.RWMutex
	logger      *slog.Logger
}

// NewTracer creates a new request tracer.
func NewTracer(serviceName string, logger *slog.Logger) *Tracer {
	return &Tracer{
		serviceName: serviceName,
		spans:       make(map[TraceID][]*Span),
		logger:      logger,
	}
}

// StartTrace creates a new trace with a root span.
func (t *Tracer) StartTrace(name string) (TraceID, *Span) {
	traceID := TraceID(generateID(16))
	spanID := SpanID(generateID(8))

	span := &Span{
		TraceID:    traceID,
		SpanID:     spanID,
		Name:       name,
		StartTime:  time.Now(),
		Status:     "ok",
		Attributes: make(map[string]string),
	}

	t.mu.Lock()
	t.spans[traceID] = []*Span{span}
	t.mu.Unlock()

	return traceID, span
}

// StartSpan creates a child span within an existing trace.
func (t *Tracer) StartSpan(traceID TraceID, parentSpanID SpanID, name string) *Span {
	spanID := SpanID(generateID(8))

	span := &Span{
		TraceID:    traceID,
		SpanID:     spanID,
		ParentID:   parentSpanID,
		Name:       name,
		StartTime:  time.Now(),
		Status:     "ok",
		Attributes: make(map[string]string),
	}

	t.mu.Lock()
	t.spans[traceID] = append(t.spans[traceID], span)
	t.mu.Unlock()

	return span
}

// FinishSpan marks a span as complete.
func (t *Tracer) FinishSpan(span *Span) {
	span.EndTime = time.Now()
	span.Duration = span.EndTime.Sub(span.StartTime)
}

// FinishSpanWithError marks a span as failed with an error.
func (t *Tracer) FinishSpanWithError(span *Span, err error) {
	span.EndTime = time.Now()
	span.Duration = span.EndTime.Sub(span.StartTime)
	if err != nil {
		span.Status = "error"
		span.Attributes["error"] = err.Error()
	}
}

// AddEvent adds a timestamped event to a span.
func (t *Tracer) AddEvent(span *Span, name string, attrs map[string]string) {
	span.Events = append(span.Events, SpanEvent{
		Name:       name,
		Timestamp:  time.Now(),
		Attributes: attrs,
	})
}

// GetTrace returns all spans for a trace.
func (t *Tracer) GetTrace(traceID TraceID) []*Span {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.spans[traceID]
}

// ActiveTraces returns the count of active traces.
func (t *Tracer) ActiveTraces() int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return len(t.spans)
}

// Cleanup removes completed traces older than the given duration.
func (t *Tracer) Cleanup(maxAge time.Duration) int {
	t.mu.Lock()
	defer t.mu.Unlock()

	removed := 0
	for traceID, spans := range t.spans {
		if len(spans) == 0 {
			continue
		}
		// Check if the root span is old enough.
		if time.Since(spans[0].StartTime) > maxAge {
			delete(t.spans, traceID)
			removed++
		}
	}
	return removed
}

// TraceContext holds trace information propagated through request context.
type traceContextKey struct{}

// TraceContext holds the trace and span IDs for a request.
type RequestTrace struct {
	TraceID TraceID
	SpanID  SpanID
}

// WithTraceContext adds trace context to a request context.
func WithTraceContext(ctx context.Context, traceID TraceID, spanID SpanID) context.Context {
	return context.WithValue(ctx, traceContextKey{}, &RequestTrace{
		TraceID: traceID,
		SpanID:  spanID,
	})
}

// GetTraceContext extracts trace context from a request context.
func GetTraceContext(ctx context.Context) *RequestTrace {
	tc, _ := ctx.Value(traceContextKey{}).(*RequestTrace)
	return tc
}

// TraceMiddleware creates HTTP middleware that traces requests.
func TraceMiddleware(tracer *Tracer) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Extract or create trace context.
			traceID := TraceID(r.Header.Get("X-Trace-ID"))
			if traceID == "" {
				traceID = TraceID(generateID(16))
			}

			spanID := SpanID(generateID(8))
			ctx := WithTraceContext(r.Context(), traceID, spanID)

			// Start span for this request.
			span := tracer.StartSpan(traceID, "", fmt.Sprintf("%s %s", r.Method, r.URL.Path))
			span.Attributes["method"] = r.Method
			span.Attributes["path"] = r.URL.Path
			span.Attributes["remote"] = r.RemoteAddr
			span.Attributes["host"] = r.Host

			// Propagate trace ID in response.
			w.Header().Set("X-Trace-ID", string(traceID))

			// Wrap response writer to capture status.
			rw := &traceResponseWriter{ResponseWriter: w, statusCode: 200}

			next.ServeHTTP(rw, r.WithContext(ctx))

			// Finish span.
			span.Attributes["status_code"] = fmt.Sprintf("%d", rw.statusCode)
			if rw.statusCode >= 500 {
				span.Status = "error"
			}
			tracer.FinishSpan(span)

			// Log the trace.
			if tracer.logger != nil {
				tracer.logger.Info("request traced",
					"trace_id", traceID,
					"span_id", spanID,
					"method", r.Method,
					"path", r.URL.Path,
					"status", rw.statusCode,
					"duration", span.Duration.String(),
				)
			}
		})
	}
}

type traceResponseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (w *traceResponseWriter) WriteHeader(code int) {
	w.statusCode = code
	w.ResponseWriter.WriteHeader(code)
}

func generateID(n int) string {
	b := make([]byte, n)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// InjectTraceHeaders adds trace headers to an outgoing request.
func InjectTraceHeaders(r *http.Request, traceID TraceID, spanID SpanID) {
	r.Header.Set("X-Trace-ID", string(traceID))
	r.Header.Set("X-Span-ID", string(spanID))
}

// ExtractTraceHeaders reads trace headers from an incoming request.
func ExtractTraceHeaders(r *http.Request) (TraceID, SpanID) {
	return TraceID(r.Header.Get("X-Trace-ID")),
		SpanID(r.Header.Get("X-Span-ID"))
}
