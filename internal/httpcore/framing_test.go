package httpcore

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestValidateFraming_Valid(t *testing.T) {
	req := httptest.NewRequest("POST", "/api/upload", nil)
	req.Header.Set("Content-Length", "1024")

	if err := ValidateFraming(req); err != nil {
		t.Errorf("unexpected error for valid framing: %v", err)
	}
}

func TestValidateFraming_ConflictingFraming(t *testing.T) {
	req := httptest.NewRequest("POST", "/api/upload", nil)
	req.Header.Set("Content-Length", "1024")
	req.Header.Set("Transfer-Encoding", "chunked")

	err := ValidateFraming(req)
	if err == nil || err != ErrConflictingFraming {
		t.Errorf("expected ErrConflictingFraming, got %v", err)
	}
}

func TestValidateFraming_UnsupportedTransferEncoding(t *testing.T) {
	req := httptest.NewRequest("POST", "/api/upload", nil)
	req.Header.Set("Transfer-Encoding", "gzip, chunked")

	err := ValidateFraming(req)
	if err == nil {
		t.Error("expected error for unsupported Transfer-Encoding")
	}
}

func TestValidateFraming_ControlCharacters(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("X-Custom", "val\r\ninjected: true")

	err := ValidateFraming(req)
	if err == nil {
		t.Error("expected error for control characters in header")
	}
}

func TestFramingMiddleware(t *testing.T) {
	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	middleware := FramingMiddleware(nextHandler)

	// Valid request
	reqValid := httptest.NewRequest("GET", "/", nil)
	recValid := httptest.NewRecorder()
	middleware.ServeHTTP(recValid, reqValid)
	if recValid.Code != http.StatusOK {
		t.Errorf("expected 200 OK, got %d", recValid.Code)
	}

	// Invalid request
	reqInvalid := httptest.NewRequest("POST", "/", nil)
	reqInvalid.Header.Set("Content-Length", "100")
	reqInvalid.Header.Set("Transfer-Encoding", "chunked")
	recInvalid := httptest.NewRecorder()
	middleware.ServeHTTP(recInvalid, reqInvalid)
	if recInvalid.Code != http.StatusBadRequest {
		t.Errorf("expected 400 Bad Request, got %d", recInvalid.Code)
	}
}
