package observability

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestResponseWriterCapture(t *testing.T) {
	rec := httptest.NewRecorder()
	rw := NewResponseWriter(rec)

	rw.WriteHeader(http.StatusCreated)
	rw.Write([]byte("hello"))

	if rw.Status() != http.StatusCreated {
		t.Errorf("status = %d, want %d", rw.Status(), http.StatusCreated)
	}
	if rw.BytesWritten() != 5 {
		t.Errorf("bytes = %d, want 5", rw.BytesWritten())
	}
}

func TestResponseWriterDefaultStatus(t *testing.T) {
	rec := httptest.NewRecorder()
	rw := NewResponseWriter(rec)

	rw.Write([]byte("ok"))

	if rw.Status() != http.StatusOK {
		t.Errorf("status = %d, want %d", rw.Status(), http.StatusOK)
	}
}

func TestResponseWriterWriteHeaderOnce(t *testing.T) {
	rec := httptest.NewRecorder()
	rw := NewResponseWriter(rec)

	rw.WriteHeader(http.StatusNotFound)
	rw.WriteHeader(http.StatusOK) // should be ignored

	if rw.Status() != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rw.Status(), http.StatusNotFound)
	}
}
