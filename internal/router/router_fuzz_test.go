package router

import (
	"net/http/httptest"
	"net/url"
	"testing"
)

func FuzzRewriteEngine(f *testing.F) {
	f.Add("^/api/(.*)$", "/index.php/$1", "/api/users/42")
	f.Add("^/old/(.*)$", "/new/$1", "/old/doc")

	f.Fuzz(func(t *testing.T, regex, target, inputPath string) {
		r := &Route{
			Regex:  regex,
			Target: target,
		}
		_ = r.Rewrite(inputPath)
	})
}

func FuzzHostParser(f *testing.F) {
	f.Add("example.com")
	f.Add("sub.domain.co.uk:8080")
	f.Add("127.0.0.1:3000")

	f.Fuzz(func(t *testing.T, rawHost string) {
		req := httptest.NewRequest("GET", "http://"+rawHost+"/", nil)
		req.Host = rawHost

		engine, err := NewEngine([]Route{{Host: rawHost, PathPrefix: "/"}})
		if err != nil {
			return
		}
		_ = engine.Match(req)

		// Parse URL host
		u, err := url.Parse("http://" + rawHost + "/")
		if err == nil {
			_ = u.Hostname()
			_ = u.Port()
		}
	})
}
