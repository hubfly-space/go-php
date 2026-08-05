package proxy

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"
)

// ProxyConfig defines reverse proxy parameters.
type ProxyConfig struct {
	Target           string        // e.g. "http://127.0.0.1:8081"
	Timeout          time.Duration // timeout for upstream requests
	MaxIdleConns     int
	KeepAlive        time.Duration
	PreserveHost     bool
	PassXForwardedFor bool
}

// Proxy wraps httputil.ReverseProxy with health checking and WebSocket support.
type Proxy struct {
	targetURL *url.URL
	revProxy  *httputil.ReverseProxy
	config    ProxyConfig
}

// NewProxy creates a new Reverse Proxy for target.
func NewProxy(cfg ProxyConfig) (*Proxy, error) {
	if cfg.Target == "" {
		return nil, fmt.Errorf("proxy: target URL is required")
	}

	targetURL, err := url.Parse(cfg.Target)
	if err != nil {
		return nil, fmt.Errorf("proxy: parse target URL %q: %w", cfg.Target, err)
	}

	if cfg.Timeout <= 0 {
		cfg.Timeout = 30 * time.Second
	}
	if cfg.MaxIdleConns <= 0 {
		cfg.MaxIdleConns = 100
	}

	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   cfg.Timeout,
			KeepAlive: cfg.KeepAlive,
		}).DialContext,
		MaxIdleConns:        cfg.MaxIdleConns,
		IdleConnTimeout:     90 * time.Second,
		TLSHandshakeTimeout: 10 * time.Second,
	}

	revProxy := httputil.NewSingleHostReverseProxy(targetURL)
	revProxy.Transport = transport

	originalDirector := revProxy.Director
	revProxy.Director = func(req *http.Request) {
		originalDirector(req)
		if cfg.PreserveHost {
			req.Host = req.URL.Host
		}
		if !cfg.PassXForwardedFor {
			req.Header.Del("X-Forwarded-For")
		}
	}

	return &Proxy{
		targetURL: targetURL,
		revProxy:  revProxy,
		config:    cfg,
	}, nil
}

// ServeHTTP handles proxying HTTP and WebSocket requests.
func (p *Proxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if isWebSocketUpgrade(r) {
		p.serveWebSocket(w, r)
		return
	}

	p.revProxy.ServeHTTP(w, r)
}

// Target returns the target URL string.
func (p *Proxy) Target() string {
	return p.targetURL.String()
}

func isWebSocketUpgrade(r *http.Request) bool {
	containsUpgrade := false
	for _, h := range r.Header.Values("Connection") {
		if strings.EqualFold(strings.TrimSpace(h), "Upgrade") || strings.Contains(strings.ToLower(h), "upgrade") {
			containsUpgrade = true
			break
		}
	}
	return containsUpgrade && strings.EqualFold(r.Header.Get("Upgrade"), "websocket")
}

func (p *Proxy) serveWebSocket(w http.ResponseWriter, r *http.Request) {
	hijacker, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "webserver does not support hijacking", http.StatusInternalServerError)
		return
	}

	clientConn, _, err := hijacker.Hijack()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer clientConn.Close()

	outReq := r.Clone(r.Context())
	outReq.URL.Scheme = "http"
	if p.targetURL.Scheme == "https" {
		outReq.URL.Scheme = "https"
	}
	outReq.URL.Host = p.targetURL.Host
	outReq.RequestURI = ""

	targetConn, err := net.DialTimeout("tcp", p.targetURL.Host, p.config.Timeout)
	if err != nil {
		_ = outReq.Write(clientConn)
		return
	}
	defer targetConn.Close()

	if err := outReq.Write(targetConn); err != nil {
		return
	}

	errChan := make(chan error, 2)
	go func() {
		_, err := io.Copy(targetConn, clientConn)
		errChan <- err
	}()
	go func() {
		_, err := io.Copy(clientConn, targetConn)
		errChan <- err
	}()

	<-errChan
}
