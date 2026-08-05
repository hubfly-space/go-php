package policy

import (
	"bytes"
	"mime/multipart"
	"testing"
)

func FuzzMultipartMetadata(f *testing.F) {
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, _ := mw.CreateFormFile("file", "test.txt")
	_, _ = fw.Write([]byte("hello"))
	_ = mw.Close()

	f.Add(buf.Bytes(), mw.Boundary())

	f.Fuzz(func(t *testing.T, payload []byte, boundary string) {
		if boundary == "" {
			boundary = "fuzzboundary"
		}
		mr := multipart.NewReader(bytes.NewReader(payload), boundary)
		for {
			part, err := mr.NextPart()
			if err != nil {
				break
			}
			_ = part.FileName()
			_ = part.FormName()
			_ = part.Header
			part.Close()
		}
	})
}
