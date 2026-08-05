package filesystem

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func FuzzPathResolverInputs(f *testing.F) {
	f.Add("/index.php")
	f.Add("/public/../.env")
	f.Add("/%2e%2e/etc/passwd")

	dir, err := os.MkdirTemp("", "fuzz-docroot-*")
	if err != nil {
		f.Fatal(err)
	}
	defer os.RemoveAll(dir)

	_ = os.WriteFile(filepath.Join(dir, "index.php"), []byte("<?php"), 0644)
	r := NewResolver(dir, SymlinkDeny, DefaultProtectedPatterns())

	f.Fuzz(func(t *testing.T, rawPath string) {
		pp, err := ParsePath(rawPath)
		if err != nil {
			return
		}
		rf, err := r.Resolve(pp.NormalizedPath)
		if err == nil && rf != nil {
			rf.Close()
			if !isUnderRoot(rf.RealPath, r.root) {
				t.Fatalf("path escaped root: %s", rf.RealPath)
			}
		}
	})
}

func FuzzArchiveExtractor(f *testing.F) {
	// Seed with a small valid tar.gz payload
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)
	_ = tw.WriteHeader(&tar.Header{Name: "test.txt", Mode: 0644, Size: 4})
	_, _ = tw.Write([]byte("test"))
	_ = tw.Close()
	_ = gw.Close()

	f.Add(buf.Bytes())

	f.Fuzz(func(t *testing.T, data []byte) {
		gr, err := gzip.NewReader(bytes.NewReader(data))
		if err != nil {
			return
		}
		defer gr.Close()

		tr := tar.NewReader(gr)
		for {
			hdr, err := tr.Next()
			if err != nil {
				break
			}
			if hdr.Typeflag == tar.TypeReg {
				_, _ = io.Copy(io.Discard, io.LimitReader(tr, 1024*1024))
			}
		}
	})
}
