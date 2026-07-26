package cgi

import (
	"crypto/tls"
	"net/http/httptest"
	"testing"
)

func TestBuildParams(t *testing.T) {
	tests := []struct {
		name     string
		method   string
		path     string
		host     string
		headers  map[string]string
		wantKeys []string
		wantVals map[string]string
	}{
		{
			name:   "basic GET",
			method: "GET",
			path:   "/index.php",
			host:   "example.com",
			wantKeys: []string{
				"GATEWAY_INTERFACE", "SERVER_PROTOCOL", "REQUEST_METHOD",
				"REQUEST_URI", "SCRIPT_FILENAME", "SCRIPT_NAME",
				"DOCUMENT_ROOT", "REMOTE_ADDR", "SERVER_NAME",
			},
			wantVals: map[string]string{
				"GATEWAY_INTERFACE": "CGI/1.1",
				"REQUEST_METHOD":    "GET",
				"REQUEST_URI":       "/index.php",
				"SCRIPT_FILENAME":   "/app/public/index.php",
				"SCRIPT_NAME":       "/index.php",
				"DOCUMENT_ROOT":     "/app/public",
				"SERVER_NAME":       "example.com",
			},
		},
		{
			name:   "POST with content type",
			method: "POST",
			path:   "/api/submit",
			host:   "example.com:8080",
			headers: map[string]string{
				"Content-Type": "application/json",
			},
			wantVals: map[string]string{
				"REQUEST_METHOD":  "POST",
				"REQUEST_URI":     "/api/submit",
				"CONTENT_TYPE":    "application/json",
				"CONTENT_LENGTH":  "0",
				"SERVER_PORT":     "8080",
			},
		},
		{
			name:   "with PATH_INFO",
			method: "GET",
			path:   "/index.php/some/path",
			host:   "example.com",
			wantVals: map[string]string{
				"SCRIPT_NAME":   "/index.php",
				"PATH_INFO":     "/some/path",
				"PATH_TRANSLATED": "/app/public/some/path",
			},
		},
		{
			name:   "with custom headers",
			method: "GET",
			path:   "/",
			host:   "example.com",
			headers: map[string]string{
				"X-Custom-Header": "custom-value",
				"Authorization":   "Bearer token",
			},
			wantVals: map[string]string{
				"HTTP_X_CUSTOM_HEADER": "custom-value",
				"HTTP_AUTHORIZATION":   "Bearer token",
			},
		},
		{
			name:   "with query string",
			method: "GET",
			path:   "/search?q=test&page=1",
			host:   "example.com",
			wantVals: map[string]string{
				"QUERY_STRING": "q=test&page=1",
				"REQUEST_URI":  "/search?q=test&page=1",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, nil)
			req.Host = tt.host
			for k, v := range tt.headers {
				req.Header.Set(k, v)
			}

			params := BuildParams(req, "/app/public/index.php", "/index.php", "/app/public")

			for key, want := range tt.wantVals {
				if got := params[key]; got != want {
					t.Errorf("params[%q] = %q, want %q", key, got, want)
				}
			}

			for _, key := range tt.wantKeys {
				if _, ok := params[key]; !ok {
					t.Errorf("missing key %q", key)
				}
			}
		})
	}
}

func TestBuildParams_HTTPS(t *testing.T) {
	req := httptest.NewRequest("GET", "/secure", nil)
	req.TLS = &tls.ConnectionState{}

	params := BuildParams(req, "/app/index.php", "/", "/app")
	if params["HTTPS"] != "on" {
		t.Errorf("HTTPS = %q, want %q", params["HTTPS"], "on")
	}
}

func TestBuildParams_RedirectStatus(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	params := BuildParams(req, "/app/index.php", "/", "/app")
	if params["REDIRECT_STATUS"] != "200" {
		t.Errorf("REDIRECT_STATUS = %q, want %q", params["REDIRECT_STATUS"], "200")
	}
}
