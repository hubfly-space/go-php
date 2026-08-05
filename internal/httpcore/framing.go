package httpcore

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

var (
	ErrConflictingFraming = errors.New("framing: conflicting Content-Length and Transfer-Encoding")
	ErrInvalidTransferEnc = errors.New("framing: unsupported Transfer-Encoding")
	ErrControlCharHeader  = errors.New("framing: header contains control characters")
	ErrInvalidContentLen  = errors.New("framing: invalid Content-Length")
)

// ValidateFraming inspects an HTTP request for protocol framing anomalies (RFC 9112 §6.3).
func ValidateFraming(r *http.Request) error {
	hasContentLength := len(r.Header.Values("Content-Length")) > 0 || r.ContentLength > 0
	hasTransferEnc := len(r.Header.Values("Transfer-Encoding")) > 0

	// Reject conflicting framing (HTTP smuggling vector)
	if hasContentLength && hasTransferEnc {
		return ErrConflictingFraming
	}

	// Validate Transfer-Encoding values
	if hasTransferEnc {
		teValues := r.Header.Values("Transfer-Encoding")
		for _, te := range teValues {
			teLower := strings.ToLower(strings.TrimSpace(te))
			if teLower != "chunked" && teLower != "identity" {
				return fmt.Errorf("%w: %s", ErrInvalidTransferEnc, te)
			}
		}
	}

	// Validate Content-Length header integer format
	if clValues := r.Header.Values("Content-Length"); len(clValues) > 0 {
		firstVal := strings.TrimSpace(clValues[0])
		for _, v := range clValues {
			if strings.TrimSpace(v) != firstVal {
				return ErrInvalidContentLen // conflicting multiple Content-Length headers
			}
		}
		if _, err := strconv.ParseInt(firstVal, 10, 64); err != nil {
			return ErrInvalidContentLen
		}
	}

	// Validate headers for illegal control characters (\r, \n, \x00)
	for name, values := range r.Header {
		if hasControlChar(name) {
			return fmt.Errorf("%w in name %q", ErrControlCharHeader, name)
		}
		for _, v := range values {
			if hasControlChar(v) {
				return fmt.Errorf("%w in value of %q", ErrControlCharHeader, name)
			}
		}
	}

	return nil
}

// FramingMiddleware returns an http.Handler middleware that validates request framing before delegating.
func FramingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := ValidateFraming(r); err != nil {
			http.Error(w, fmt.Sprintf("Bad Request: %v", err), http.StatusBadRequest)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func hasControlChar(s string) bool {
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '\r' || c == '\n' || c == 0 {
			return true
		}
	}
	return false
}
