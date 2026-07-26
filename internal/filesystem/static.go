package filesystem

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// PrecompressedFileServer serves static files, preferring precompressed
// versions (.gz, .br, .zst) when the client supports them.
type PrecompressedFileServer struct {
	Root    string
	Index   string
	LogFunc func(string, ...interface{})
}

func (s *PrecompressedFileServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	if path == "/" {
		path = "/" + s.Index
	}

	// Reject traversal before cleaning.
	if strings.Contains(path, "..") {
		http.Error(w, "400 Bad Request", http.StatusBadRequest)
		return
	}

	path = filepath.Clean(path)

	fullPath := filepath.Join(s.Root, path)

	// Try precompressed first.
	if encodings := r.Header.Get("Accept-Encoding"); encodings != "" {
		for _, ext := range []string{".gz", ".br", ".zst"} {
			if strings.Contains(encodings, encodingForExt(ext)) {
				compressed := fullPath + ext
				if info, err := os.Stat(compressed); err == nil && info.Mode().IsRegular() {
					f, err := os.Open(compressed)
					if err != nil {
						http.Error(w, "500 Internal Server Error", http.StatusInternalServerError)
						return
					}
					defer f.Close()
					w.Header().Set("Content-Encoding", encodingForExt(ext))
					w.Header().Set("Vary", "Accept-Encoding")
					http.ServeContent(w, r, info.Name(), info.ModTime(), f)
					return
				}
			}
		}
	}

	// Serve original file.
	info, err := os.Stat(fullPath)
	if os.IsNotExist(err) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		http.Error(w, "500 Internal Server Error", http.StatusInternalServerError)
		return
	}
	if info.IsDir() {
		// Try index file.
		fullPath = filepath.Join(fullPath, s.Index)
		info, err = os.Stat(fullPath)
		if os.IsNotExist(err) {
			http.NotFound(w, r)
			return
		}
		if err != nil {
			http.Error(w, "500 Internal Server Error", http.StatusInternalServerError)
			return
		}
	}

	f, err := os.Open(fullPath)
	if err != nil {
		http.Error(w, "500 Internal Server Error", http.StatusInternalServerError)
		return
	}
	defer f.Close()

	http.ServeContent(w, r, info.Name(), info.ModTime(), f)
}

func encodingForExt(ext string) string {
	switch ext {
	case ".gz":
		return "gzip"
	case ".br":
		return "br"
	case ".zst":
		return "zstd"
	default:
		return ""
	}
}
