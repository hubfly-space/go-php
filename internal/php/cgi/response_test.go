package cgi

import (
	"io"
	"testing"
)

func TestParseResponse_Normal(t *testing.T) {
	tests := []struct {
		name       string
		stdout     []byte
		wantStatus int
		wantCT     string
	}{
		{
			name:       "simple 200",
			stdout:     []byte("Content-Type: text/html\r\n\r\n<h1>Hello</h1>"),
			wantStatus: 200,
			wantCT:     "text/html",
		},
		{
			name:       "with status",
			stdout:     []byte("Status: 404 Not Found\r\nContent-Type: text/plain\r\n\r\nnot found"),
			wantStatus: 404,
			wantCT:     "text/plain",
		},
		{
			name:       "redirect",
			stdout:     []byte("Location: /new\r\nStatus: 302\r\n\r\n"),
			wantStatus: 302,
			wantCT:     "",
		},
		{
			name:       "multiple set-cookie",
			stdout:     []byte("Set-Cookie: a=1\r\nSet-Cookie: b=2\r\nContent-Type: text/html\r\n\r\nok"),
			wantStatus: 200,
			wantCT:     "text/html",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, err := ParseResponse(tt.stdout, nil)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if resp.StatusCode != tt.wantStatus {
				t.Errorf("StatusCode = %d, want %d", resp.StatusCode, tt.wantStatus)
			}
			if tt.wantCT != "" && resp.Headers.Get("Content-Type") != tt.wantCT {
				t.Errorf("Content-Type = %q, want %q", resp.Headers.Get("Content-Type"), tt.wantCT)
			}
		})
	}
}

func TestParseResponse_EmptyStdout(t *testing.T) {
	resp, err := ParseResponse(nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Errorf("StatusCode = %d, want 200", resp.StatusCode)
	}
}

func TestParseResponse_BodyOnly(t *testing.T) {
	stdout := []byte("<h1>No headers</h1>")
	resp, err := ParseResponse(stdout, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Errorf("StatusCode = %d, want 200", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "<h1>No headers</h1>" {
		t.Errorf("Body = %q", string(body))
	}
}

func TestParseResponse_StderrPreserved(t *testing.T) {
	stdout := []byte("Content-Type: text/html\r\n\r\nok")
	stderr := []byte("PHP Warning: something")
	resp, err := ParseResponse(stdout, stderr)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(resp.Stderr) != "PHP Warning: something" {
		t.Errorf("Stderr = %q", string(resp.Stderr))
	}
}

func TestParseResponse_HeaderInjection(t *testing.T) {
	// Header name with control character.
	bad := []byte("X-Evil\x01: value\r\n\r\n")
	_, err := ParseResponse(bad, nil)
	if err != ErrHeaderInjection {
		t.Errorf("got %v, want ErrHeaderInjection", err)
	}

	// Header value with control character (not tab).
	bad = []byte("X-Evil: val\x01ue\r\n\r\n")
	_, err = ParseResponse(bad, nil)
	if err != ErrHeaderInjection {
		t.Errorf("got %v, want ErrHeaderInjection for value", err)
	}
}

func TestParseResponse_InvalidHeader(t *testing.T) {
	bad := []byte("NoColonHere\r\n\r\n")
	_, err := ParseResponse(bad, nil)
	if err != ErrInvalidHeader {
		t.Errorf("got %v, want ErrInvalidHeader", err)
	}
}

func TestParseResponse_EmptyHeaderName(t *testing.T) {
	bad := []byte(": value\r\n\r\n")
	_, err := ParseResponse(bad, nil)
	if err != ErrInvalidHeader {
		t.Errorf("got %v, want ErrInvalidHeader", err)
	}
}

func TestParseResponse_StatusOnly(t *testing.T) {
	stdout := []byte("Status: 201 Created\r\n\r\n")
	resp, err := ParseResponse(stdout, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != 201 {
		t.Errorf("StatusCode = %d, want 201", resp.StatusCode)
	}
}

func TestParseResponse_TooManyHeaders(t *testing.T) {
	var buf []byte
	for i := 0; i < maxHeaderCount+1; i++ {
		buf = append(buf, []byte("X-Header: value\r\n")...)
	}
	buf = append(buf, []byte("\r\n")...)
	_, err := ParseResponse(buf, nil)
	if err != ErrTooManyHeaders {
		t.Errorf("got %v, want ErrTooManyHeaders", err)
	}
}

func TestParseResponse_LargeHeader(t *testing.T) {
	// Single header line exceeding maxHeaderBytes.
	value := make([]byte, maxHeaderBytes+100)
	for i := range value {
		value[i] = 'x'
	}
	buf := []byte("X-Big: " + string(value) + "\r\n\r\n")
	_, err := ParseResponse(buf, nil)
	if err != ErrHeaderTooLarge {
		t.Errorf("got %v, want ErrHeaderTooLarge", err)
	}
}

func FuzzCGIResponseHeaders(f *testing.F) {
	f.Add([]byte("Content-Type: text/html\r\n\r\nhello"))
	f.Add([]byte("Status: 404\r\nContent-Type: text/plain\r\n\r\nnot found"))
	f.Add([]byte("\r\n"))
	f.Add([]byte(""))
	f.Add([]byte("No CRLF terminator"))
	f.Add([]byte("X\x01Bad: value\r\n\r\n"))
	f.Add([]byte("Status: 99999\r\n\r\n"))

	f.Fuzz(func(t *testing.T, data []byte) {
		resp, err := ParseResponse(data, nil)
		if err != nil {
			return
		}
		// Invariant: status code must be in valid range.
		if resp.StatusCode < 100 || resp.StatusCode > 599 {
			t.Errorf("StatusCode = %d, out of range", resp.StatusCode)
		}
		// Invariant: headers must not contain control characters in names.
		for name := range resp.Headers {
			for _, r := range name {
				if r < 32 || r == 127 {
					t.Errorf("control char in header name %q", name)
				}
			}
		}
	})
}
