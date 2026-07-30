package filesystem

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// CachePolicy defines caching rules for static files.
type CachePolicy struct {
	DefaultMaxAge  time.Duration // default cache duration
	ImmutablePaths []string      // paths that never change (e.g., hashed assets)
	NoCachePaths   []string      // paths that should never be cached
	ETag           bool          // enable ETag generation
}

// DefaultCachePolicy returns sensible defaults.
func DefaultCachePolicy() *CachePolicy {
	return &CachePolicy{
		DefaultMaxAge: 1 * time.Hour,
		ImmutablePaths: []string{
			"/assets/", "/static/", "/dist/", "/build/",
		},
		NoCachePaths: []string{
			"/api/", "/admin/",
		},
		ETag: true,
	}
}

// CacheControlledFileServer wraps PrecompressedFileServer with cache headers.
type CacheControlledFileServer struct {
	*PrecompressedFileServer
	Cache *CachePolicy
}

// NewCacheControlledFileServer creates a static file server with cache control.
func NewCacheControlledFileServer(root, index string, cache *CachePolicy) *CacheControlledFileServer {
	if cache == nil {
		cache = DefaultCachePolicy()
	}
	return &CacheControlledFileServer{
		PrecompressedFileServer: &PrecompressedFileServer{
			Root:  root,
			Index: index,
		},
		Cache: cache,
	}
}

// CacheControl returns the Cache-Control header value for a request path.
//
// This is separated from the file server so a caller that resolves paths
// through Resolver — as the gateway does, and must — can reuse the policy
// without adopting CacheControlledFileServer's own, weaker path handling.
func (c *CachePolicy) CacheControl(path string) string {
	for _, p := range c.NoCachePaths {
		if strings.HasPrefix(path, p) {
			return "no-cache, no-store, must-revalidate"
		}
	}
	for _, p := range c.ImmutablePaths {
		if strings.HasPrefix(path, p) {
			return "public, max-age=31536000, immutable"
		}
	}
	return "public, max-age=" + itoa(int(c.DefaultMaxAge.Seconds()))
}

func (s *CacheControlledFileServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path

	w.Header().Set("Cache-Control", s.Cache.CacheControl(path))

	// Generate ETag if enabled.
	if s.Cache.ETag {
		s.setETag(w, r, path)
	}

	s.PrecompressedFileServer.ServeHTTP(w, r)
}

func (s *CacheControlledFileServer) setETag(w http.ResponseWriter, r *http.Request, path string) {
	fullPath := filepath.Join(s.Root, filepath.Clean(path))
	info, err := os.Stat(fullPath)
	if err != nil || info.IsDir() {
		return
	}

	// Simple ETag based on size + mtime.
	etag := `"` + itoa(int(info.Size())) + `-` + itoa(int(info.ModTime().UnixNano())) + `"`
	w.Header().Set("ETag", etag)

	// Check If-None-Match.
	if match := r.Header.Get("If-None-Match"); match == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
}

// itoa is a minimal int-to-string without allocating fmt.Sprintf.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	buf := [20]byte{}
	i := len(buf)
	neg := false
	if n < 0 {
		neg = true
		n = -n
	}
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
