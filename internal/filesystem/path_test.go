package filesystem

import (
	"strings"
	"testing"
)

func TestParsePath_Normal(t *testing.T) {
	tests := []struct {
		input      string
		segments   []string
		normalized string
	}{
		{"/", nil, "/"},
		{"/foo", []string{"foo"}, "/foo"},
		{"/foo/bar", []string{"foo", "bar"}, "/foo/bar"},
		{"/foo/bar/baz", []string{"foo", "bar", "baz"}, "/foo/bar/baz"},
		{"/foo?q=1", []string{"foo"}, "/foo"},
		{"/foo/bar?q=1#frag", []string{"foo", "bar"}, "/foo/bar"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			pp, err := ParsePath(tt.input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if pp.NormalizedPath != tt.normalized {
				t.Errorf("NormalizedPath = %q, want %q", pp.NormalizedPath, tt.normalized)
			}
			if len(pp.DecodedSegments) != len(tt.segments) {
				t.Fatalf("segments len = %d, want %d", len(pp.DecodedSegments), len(tt.segments))
			}
			for i, s := range pp.DecodedSegments {
				if s != tt.segments[i] {
					t.Errorf("segment[%d] = %q, want %q", i, s, tt.segments[i])
				}
			}
		})
	}
}

func TestParsePath_DotSegments(t *testing.T) {
	tests := []struct {
		input      string
		normalized string
	}{
		{"/foo/../bar", "/bar"},
		{"/foo/./bar", "/foo/bar"},
		{"/a/b/../c/./d", "/a/c/d"},
		{"/..", "/"},
		{"/../..", "/"},
		{"/foo/../../bar", "/bar"},
		{"/a/b/c/../../..", "/"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			pp, err := ParsePath(tt.input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if pp.NormalizedPath != tt.normalized {
				t.Errorf("NormalizedPath = %q, want %q", pp.NormalizedPath, tt.normalized)
			}
		})
	}
}

func TestParsePath_RepeatedSlashes(t *testing.T) {
	pp, err := ParsePath("//foo///bar//")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pp.NormalizedPath != "/foo/bar" {
		t.Errorf("NormalizedPath = %q, want %q", pp.NormalizedPath, "/foo/bar")
	}
}

func TestParsePath_EncodedDots(t *testing.T) {
	// %2e = '.', %2f = '/' (rejected)
	pp, err := ParsePath("/%2e%2e/%66%6f%6f")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should resolve to /foo after collapsing
	if pp.NormalizedPath != "/foo" {
		t.Errorf("NormalizedPath = %q, want %q", pp.NormalizedPath, "/foo")
	}
}

func TestParsePath_InvalidPercentEncoding(t *testing.T) {
	bad := []string{
		"/foo%ZZ/bar",
		"/foo%2/bar",
		"/foo%2g/bar",
		"/foo%2",
		"/foo%",
	}
	for _, input := range bad {
		t.Run(input, func(t *testing.T) {
			_, err := ParsePath(input)
			if err == nil {
				t.Error("expected error for invalid percent encoding")
			}
		})
	}
}

func TestParsePath_NULByte(t *testing.T) {
	_, err := ParsePath("/foo\x00/bar")
	if err != ErrNULInPath {
		t.Errorf("got %v, want ErrNULInPath", err)
	}
}

func TestParsePath_ControlChars(t *testing.T) {
	bad := []string{
		"/foo\x01/bar",
		"/foo\x1f/bar",
		"/foo\x7f/bar",
		"/foo\t/bar",
	}
	for _, input := range bad {
		t.Run(strings.ReplaceAll(input, "\t", "TAB"), func(t *testing.T) {
			_, err := ParsePath(input)
			if err != ErrControlCharInPath {
				t.Errorf("got %v, want ErrControlCharInPath", err)
			}
		})
	}
}

func TestParsePath_Backslash(t *testing.T) {
	_, err := ParsePath(`/foo\bar`)
	if err != ErrBackslashInPath {
		t.Errorf("got %v, want ErrBackslashInPath", err)
	}
}

func TestParsePath_EncodedBackslash(t *testing.T) {
	_, err := ParsePath("/foo%5cbar")
	if err != ErrBackslashInPath {
		t.Errorf("got %v, want ErrBackslashInPath", err)
	}
	_, err = ParsePath("/foo%5Cbar")
	if err != ErrBackslashInPath {
		t.Errorf("got %v, want ErrBackslashInPath for %%5C", err)
	}
}

func TestParsePath_EncodedSlash(t *testing.T) {
	_, err := ParsePath("/foo%2fbar")
	if err != ErrEncodedSlashInPath {
		t.Errorf("got %v, want ErrEncodedSlashInPath", err)
	}
}

func TestParsePath_EmptyPath(t *testing.T) {
	pp, err := ParsePath("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pp.NormalizedPath != "/" {
		t.Errorf("NormalizedPath = %q, want %q", pp.NormalizedPath, "/")
	}
}

func TestParsePath_DoubleEncoding(t *testing.T) {
	bad := []string{
		"/%252e%252e/etc/passwd",
		"/%252e%252e/%2565tc/passwd",
	}
	for _, input := range bad {
		t.Run(input, func(t *testing.T) {
			_, err := ParsePath(input)
			if err != ErrDoubleEncoding {
				t.Errorf("got %v, want ErrDoubleEncoding", err)
			}
		})
	}
}

func TestParsePath_PercentDecoding(t *testing.T) {
	pp, err := ParsePath("/foo%20bar/baz%7Eq")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pp.NormalizedPath != "/foo bar/baz~q" {
		t.Errorf("NormalizedPath = %q, want %q", pp.NormalizedPath, "/foo bar/baz~q")
	}
}

func TestParsePath_PlusAsSpace(t *testing.T) {
	pp, err := ParsePath("/foo+bar")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pp.NormalizedPath != "/foo+bar" {
		t.Errorf("NormalizedPath = %q, want %q", pp.NormalizedPath, "/foo+bar")
	}
}

func TestParsePath_LongSegments(t *testing.T) {
	long := "/" + strings.Repeat("a", 1000)
	pp, err := ParsePath(long)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pp.NormalizedPath != long {
		t.Errorf("long segment preserved")
	}
}

func TestParsePath_TrailingSlash(t *testing.T) {
	pp, err := ParsePath("/foo/")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pp.NormalizedPath != "/foo" {
		t.Errorf("NormalizedPath = %q, want %q", pp.NormalizedPath, "/foo")
	}
}

func TestParsePath_RawTargetPreserved(t *testing.T) {
	pp, err := ParsePath("/foo/bar?q=1#frag")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pp.RawTarget != "/foo/bar?q=1#frag" {
		t.Errorf("RawTarget = %q, want %q", pp.RawTarget, "/foo/bar?q=1#frag")
	}
	if pp.RawPath != "/foo/bar" {
		t.Errorf("RawPath = %q, want %q", pp.RawPath, "/foo/bar")
	}
}

func TestParsePath_Idempotence(t *testing.T) {
	inputs := []string{
		"/foo/bar",
		"/a/b/c",
		"/",
		"/assets/app.js",
	}
	for _, input := range inputs {
		t.Run(input, func(t *testing.T) {
			pp1, err := ParsePath(input)
			if err != nil {
				t.Fatalf("first parse: %v", err)
			}
			pp2, err := ParsePath(pp1.NormalizedPath)
			if err != nil {
				t.Fatalf("second parse: %v", err)
			}
			if pp1.NormalizedPath != pp2.NormalizedPath {
				t.Errorf("not idempotent: %q -> %q -> %q",
					input, pp1.NormalizedPath, pp2.NormalizedPath)
			}
		})
	}
}

func FuzzPathParser(f *testing.F) {
	// Seed corpus with known attack strings and valid paths.
	f.Add("/foo/bar")
	f.Add("/foo/../bar")
	f.Add("/%2e%2e/etc/passwd")
	f.Add("/foo%2fbar")
	f.Add("/foo\x00bar")
	f.Add("/foo%ZZbar")
	f.Add(`/foo\bar`)
	f.Add("/foo%5cbar")
	f.Add("////foo///bar///")
	f.Add("/a/b/c/../../d")
	f.Add("/foo%00bar")
	f.Add("/foo%01bar")
	f.Add("/foo%7fbar")
	f.Add("/foo+bar")
	f.Add("")
	f.Add("/..")
	f.Add("/../..")
	f.Add(strings.Repeat("/a", 100))

	f.Fuzz(func(t *testing.T, input string) {
		pp, err := ParsePath(input)
		if err != nil {
			return // rejected — that's fine
		}
		// Invariant 1: NormalizedPath must not be empty.
		if pp.NormalizedPath == "" {
			t.Errorf("empty NormalizedPath for input %q", input)
		}
		// Invariant 2: must start with '/'.
		if !strings.HasPrefix(pp.NormalizedPath, "/") {
			t.Errorf("NormalizedPath %q does not start with /", pp.NormalizedPath)
		}
		// Invariant 3: must not contain ".." as a segment.
		for _, seg := range strings.Split(pp.NormalizedPath, "/") {
			if seg == ".." {
				t.Errorf("unresolved dot segment in %q", pp.NormalizedPath)
			}
		}
		// Invariant 4: must not contain percent-encoded characters
		// (would indicate double-decoding risk).
		if containsPercentEncoding(pp.NormalizedPath) {
			t.Errorf("percent-encoded chars in NormalizedPath %q", pp.NormalizedPath)
		}
		// Invariant 5: double-parse idempotence.
		pp2, err := ParsePath(pp.NormalizedPath)
		if err != nil {
			t.Errorf("second parse of %q failed: %v", pp.NormalizedPath, err)
		} else if pp.NormalizedPath != pp2.NormalizedPath {
			t.Errorf("not idempotent: %q -> %q -> %q",
				input, pp.NormalizedPath, pp2.NormalizedPath)
		}
	})
}
