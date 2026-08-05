package observability

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func FuzzForwardedHeaderParser(f *testing.F) {
	f.Add("for=192.0.2.60;proto=http;by=203.0.113.43")
	f.Add("for=192.0.2.43, for=198.51.100.17")
	f.Add("192.168.1.1, 10.0.0.1")

	f.Fuzz(func(t *testing.T, headerVal string) {
		req := httptest.NewRequest("GET", "/", nil)
		req.Header.Set("Forwarded", headerVal)
		req.Header.Set("X-Forwarded-For", headerVal)

		// Parse forwarded values safely
		ff := req.Header.Get("Forwarded")
		parts := strings.Split(ff, ";")
		for _, part := range parts {
			kv := strings.SplitN(strings.TrimSpace(part), "=", 2)
			if len(kv) == 2 {
				_ = kv[0]
				_ = kv[1]
			}
		}

		xff := req.Header.Get("X-Forwarded-For")
		ips := strings.Split(xff, ",")
		for _, ip := range ips {
			_ = strings.TrimSpace(ip)
		}
	})
}
