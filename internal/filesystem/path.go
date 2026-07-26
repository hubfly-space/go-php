// Package filesystem implements safe path parsing and resolution.
//
// This is a security-critical package. All path handling must decode percent
// encoding exactly once, reject NUL bytes and control characters, collapse
// dot segments, and never allow filesystem escapes.
package filesystem

import (
	"errors"
	"fmt"
	"strings"
	"unicode"
)

// ParsedPath holds multiple representations of a request path.
// The raw representations are preserved for audit; NormalizedPath is used
// for routing and filesystem mapping.
type ParsedPath struct {
	RawTarget       string   // Original request target (e.g. "/foo%2Fbar?q=1")
	RawPath         string   // Path portion of the target, before query
	EscapedPath     string   // Percent-encoded path (canonical)
	DecodedSegments []string // Path segments after decoding
	NormalizedPath  string   // Final safe path for routing
	HadEncodedSlash bool     // Whether %2f was present
	HadBackslash    bool     // Whether \ was present
}

var (
	ErrInvalidPercentEncoding = errors.New("invalid percent encoding")
	ErrNULInPath              = errors.New("NUL byte in path")
	ErrControlCharInPath      = errors.New("control character in path")
	ErrBackslashInPath        = errors.New("backslash in path")
	ErrEncodedSlashInPath     = errors.New("encoded slash in path")
	ErrDoubleEncoding         = errors.New("double percent encoding detected")
)

// ParsePath parses a raw request target into a ParsedPath.
// rawTarget is the full request target (e.g. "/foo/bar?q=1").
func ParsePath(rawTarget string) (*ParsedPath, error) {
	// Extract path portion (before query string).
	rawPath := rawTarget
	if idx := strings.IndexByte(rawTarget, '?'); idx >= 0 {
		rawPath = rawTarget[:idx]
	}

	pp := &ParsedPath{
		RawTarget: rawTarget,
		RawPath:   rawPath,
	}

	// Validate: reject NUL bytes.
	if strings.ContainsRune(rawPath, 0) {
		return nil, ErrNULInPath
	}

	// Validate: reject control characters (except TAB which is sometimes used).
	for _, r := range rawPath {
		if r == '\t' {
			return nil, ErrControlCharInPath
		}
		if unicode.IsControl(r) && r != 0 {
			return nil, ErrControlCharInPath
		}
	}

	// Detect backslashes.
	if strings.ContainsRune(rawPath, '\\') {
		pp.HadBackslash = true
		return nil, ErrBackslashInPath
	}

	// Detect encoded slashes.
	if strings.Contains(rawPath, "%2f") || strings.Contains(rawPath, "%2F") {
		pp.HadEncodedSlash = true
		return nil, ErrEncodedSlashInPath
	}

	// Check for encoded backslash.
	if strings.Contains(rawPath, "%5c") || strings.Contains(rawPath, "%5C") {
		pp.HadBackslash = true
		return nil, ErrBackslashInPath
	}

	// Decode percent encoding exactly once.
	decoded, err := percentDecode(rawPath)
	if err != nil {
		return nil, fmt.Errorf("percent decode: %w", err)
	}

	// Reject double-encoding: if the decoded result still contains %XX sequences,
	// an attacker is trying to bypass normalization.
	if containsPercentEncoding(decoded) {
		return nil, ErrDoubleEncoding
	}

	// After decoding, reject NUL and control characters again
	// (they may have been encoded).
	for _, r := range decoded {
		if r == 0 {
			return nil, ErrNULInPath
		}
		if unicode.IsControl(r) {
			return nil, ErrControlCharInPath
		}
	}

	// Split into segments, collapse dot segments.
	segments := splitAndCollapse(decoded)

	pp.DecodedSegments = segments
	pp.EscapedPath = rawPath
	pp.NormalizedPath = "/" + strings.Join(segments, "/")

	return pp, nil
}

// percentDecode performs a single pass of percent decoding.
// It rejects invalid sequences (e.g. %ZZ, %2 at end of string).
func percentDecode(s string) (string, error) {
	var b strings.Builder
	b.Grow(len(s))

	i := 0
	for i < len(s) {
		switch s[i] {
		case '%':
			if i+2 >= len(s) {
				return "", ErrInvalidPercentEncoding
			}
			h1 := hexDigit(s[i+1])
			h2 := hexDigit(s[i+2])
			if h1 < 0 || h2 < 0 {
				return "", ErrInvalidPercentEncoding
			}
			b.WriteByte(byte(h1<<4 | h2))
			i += 3
		case '+':
			b.WriteByte(' ')
			i++
		default:
			b.WriteByte(s[i])
			i++
		}
	}
	return b.String(), nil
}

func hexDigit(c byte) int {
	switch {
	case c >= '0' && c <= '9':
		return int(c - '0')
	case c >= 'a' && c <= 'f':
		return int(c-'a') + 10
	case c >= 'A' && c <= 'F':
		return int(c-'A') + 10
	default:
		return -1
	}
}

// splitAndCollapse splits a path on '/' and collapses dot segments per RFC 3986 §5.2.4.
func splitAndCollapse(path string) []string {
	if path == "" || path == "/" {
		return nil
	}

	segments := strings.Split(path, "/")
	var result []string

	for _, seg := range segments {
		switch seg {
		case ".", "":
			// skip
		case "..":
			if len(result) > 0 {
				result = result[:len(result)-1]
			}
		default:
			result = append(result, seg)
		}
	}

	out := make([]string, len(result))
	copy(out, result)
	return out
}

// containsPercentEncoding checks if a string contains %XX sequences
// (indicating double-encoding after a single decode pass).
func containsPercentEncoding(s string) bool {
	for i := 0; i < len(s)-2; i++ {
		if s[i] == '%' && hexDigit(s[i+1]) >= 0 && hexDigit(s[i+2]) >= 0 {
			return true
		}
	}
	return false
}
