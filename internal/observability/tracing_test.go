package observability

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestTracer_StartAndFinishSpan(t *testing.T) {
	tracer := NewTracer("test", nil)

	traceID, span := tracer.StartTrace("test-request")
	if traceID == "" {
		t.Error("expected non-empty trace ID")
	}

	child := tracer.StartSpan(traceID, span.SpanID, "db-query")
	tracer.FinishSpan(child)
	tracer.FinishSpan(span)

	spans := tracer.GetTrace(traceID)
	if len(spans) != 2 {
		t.Errorf("expected 2 spans, got %d", len(spans))
	}

	if child.Duration == 0 {
		t.Error("expected non-zero duration")
	}
}

func TestTracer_FinishSpanWithError(t *testing.T) {
	tracer := NewTracer("test", nil)

	_, span := tracer.StartTrace("test-request")
	tracer.FinishSpanWithError(span, nil)

	if span.Status != "ok" {
		t.Errorf("expected ok status, got %s", span.Status)
	}
}

func TestTracer_AddEvent(t *testing.T) {
	tracer := NewTracer("test", nil)

	_, span := tracer.StartTrace("test-request")
	tracer.AddEvent(span, "cache_hit", map[string]string{"key": "user:1"})

	if len(span.Events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(span.Events))
	}
	if span.Events[0].Name != "cache_hit" {
		t.Errorf("expected event name cache_hit, got %s", span.Events[0].Name)
	}
}

func TestTracer_ActiveTraces(t *testing.T) {
	tracer := NewTracer("test", nil)

	tracer.StartTrace("req1")
	tracer.StartTrace("req2")

	if tracer.ActiveTraces() != 2 {
		t.Errorf("expected 2 active traces, got %d", tracer.ActiveTraces())
	}
}

func TestTracer_Cleanup(t *testing.T) {
	tracer := NewTracer("test", nil)

	_, span := tracer.StartTrace("old-request")
	span.StartTime = time.Now().Add(-2 * time.Hour) // make it old
	tracer.FinishSpan(span)

	tracer.StartTrace("new-request")

	removed := tracer.Cleanup(1 * time.Hour)
	if removed != 1 {
		t.Errorf("expected 1 removed, got %d", removed)
	}
}

func TestTracer_ActiveTracesAfterCleanup(t *testing.T) {
	tracer := NewTracer("test", nil)

	tracer.StartTrace("req1")
	tracer.StartTrace("req2")

	tracer.Cleanup(1 * time.Hour)

	if tracer.ActiveTraces() != 2 {
		t.Errorf("expected 2 active traces after cleanup, got %d", tracer.ActiveTraces())
	}
}

func TestTraceContext(t *testing.T) {
	ctx := WithTraceContext(context.Background(), "trace-123", "span-456")
	tc := GetTraceContext(ctx)

	if tc == nil {
		t.Fatal("expected trace context")
	}
	if tc.TraceID != "trace-123" {
		t.Errorf("expected trace-123, got %s", tc.TraceID)
	}
}

func TestTraceMiddleware(t *testing.T) {
	tracer := NewTracer("test", nil)

	handler := TraceMiddleware(tracer)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte("ok"))
	}))

	req := httptest.NewRequest("GET", "/api/test", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != 200 {
		t.Errorf("expected 200, got %d", rec.Code)
	}

	traceID := rec.Header().Get("X-Trace-ID")
	if traceID == "" {
		t.Error("expected X-Trace-ID header")
	}
}

func TestTraceMiddleware_ExistingTraceID(t *testing.T) {
	tracer := NewTracer("test", nil)

	handler := TraceMiddleware(tracer)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))

	req := httptest.NewRequest("GET", "/api/test", nil)
	req.Header.Set("X-Trace-ID", "custom-trace-id")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Header().Get("X-Trace-ID") != "custom-trace-id" {
		t.Error("expected custom trace ID to be preserved")
	}
}

func TestTraceMiddleware_ErrorStatus(t *testing.T) {
	tracer := NewTracer("test", nil)

	handler := TraceMiddleware(tracer)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
	}))

	req := httptest.NewRequest("GET", "/api/test", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	traceID := TraceID(rec.Header().Get("X-Trace-ID"))
	spans := tracer.GetTrace(traceID)
	if len(spans) == 0 {
		t.Fatal("expected spans")
	}

	if spans[0].Status != "error" {
		t.Errorf("expected error status, got %s", spans[0].Status)
	}
}

func TestGenerateID(t *testing.T) {
	id1 := generateID(16)
	id2 := generateID(16)

	if id1 == id2 {
		t.Error("expected different IDs")
	}
	if len(id1) != 32 { // 16 bytes = 32 hex chars
		t.Errorf("expected 32 hex chars, got %d", len(id1))
	}
}

func TestTracerStartCleanupExitsOnCancel(t *testing.T) {
	tracer := NewTracer("test", slog.Default())

	ctx, cancel := context.WithCancel(context.Background())
	done := tracer.StartCleanup(ctx, time.Millisecond, time.Hour)

	cancel()

	select {
	case <-done:
		// Goroutine exited, as §62 requires.
	case <-time.After(2 * time.Second):
		t.Fatal("cleanup goroutine did not exit after context cancellation")
	}
}

func TestTracerStartCleanupBoundsSpanMap(t *testing.T) {
	tracer := NewTracer("test", slog.Default())

	// Without a cleanup loop this map grows without limit.
	for i := 0; i < 50; i++ {
		traceID, span := tracer.StartTrace("req")
		tracer.FinishSpan(span)
		_ = traceID
	}
	if tracer.ActiveTraces() != 50 {
		t.Fatalf("setup: active traces = %d, want 50", tracer.ActiveTraces())
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	// maxAge of 0 makes every finished trace immediately eligible.
	done := tracer.StartCleanup(ctx, time.Millisecond, 0)

	deadline := time.After(2 * time.Second)
	for tracer.ActiveTraces() > 0 {
		select {
		case <-deadline:
			t.Fatalf("traces not reclaimed: %d still active", tracer.ActiveTraces())
		default:
			time.Sleep(time.Millisecond)
		}
	}

	cancel()
	<-done
}
