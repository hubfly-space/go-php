package observability

import (
	"log/slog"
	"net/http"
	"time"
)

// ResponseWriter wraps http.ResponseWriter to capture status and bytes.
type ResponseWriter struct {
	http.ResponseWriter
	status      int
	written     int64
	wroteHeader bool
}

// NewResponseWriter wraps a ResponseWriter.
func NewResponseWriter(w http.ResponseWriter) *ResponseWriter {
	return &ResponseWriter{ResponseWriter: w, status: http.StatusOK}
}

func (rw *ResponseWriter) WriteHeader(code int) {
	if !rw.wroteHeader {
		rw.status = code
		rw.wroteHeader = true
	}
	rw.ResponseWriter.WriteHeader(code)
}

func (rw *ResponseWriter) Write(b []byte) (int, error) {
	if !rw.wroteHeader {
		rw.ResponseWriter.WriteHeader(rw.status)
		rw.wroteHeader = true
	}
	n, err := rw.ResponseWriter.Write(b)
	rw.written += int64(n)
	return n, err
}

// Status returns the response status code.
func (rw *ResponseWriter) Status() int {
	return rw.status
}

// BytesWritten returns bytes written.
func (rw *ResponseWriter) BytesWritten() int64 {
	return rw.written
}

// AccessLog logs a structured access entry.
func AccessLog(logger *slog.Logger, r *http.Request, rw *ResponseWriter, start time.Time) {
	logger.Info("request",
		"method", r.Method,
		"path", r.URL.Path,
		"remote", r.RemoteAddr,
		"host", r.Host,
		"status", rw.Status(),
		"bytes", rw.BytesWritten(),
		"duration_ms", time.Since(start).Milliseconds(),
		"user_agent", r.UserAgent(),
		"request_id", r.Header.Get("X-Request-ID"),
	)
}

// Middleware returns an HTTP middleware that logs requests.
func Middleware(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			rw := NewResponseWriter(w)
			next.ServeHTTP(rw, r)
			AccessLog(logger, r, rw, start)
		})
	}
}
