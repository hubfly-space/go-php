package fastcgi

import (
	"bytes"
	"testing"
)

func TestEncodeDecodeRecord(t *testing.T) {
	tests := []struct {
		name string
		rec  Record
	}{
		{
			name: "empty params",
			rec: Record{
				Version:       protoVersion,
				Type:          typeParams,
				RequestID:     1,
				ContentLength: 0,
			},
		},
		{
			name: "small content",
			rec: Record{
				Version:       protoVersion,
				Type:          typeStdout,
				RequestID:     1,
				ContentLength: 5,
				Content:       []byte("hello"),
			},
		},
		{
			name: "max record content",
			rec: Record{
				Version:       protoVersion,
				Type:          typeStdout,
				RequestID:     1,
				ContentLength: maxRecordContent,
				Content:       bytes.Repeat([]byte("x"), maxRecordContent),
			},
		},
		{
			name: "begin request",
			rec: Record{
				Version:       protoVersion,
				Type:          typeBeginReq,
				RequestID:     1,
				ContentLength: 8,
				Content:       []byte{0, 1, 0, 0, 0, 0, 0, 0},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := EncodeRecord(tt.rec)
			got, err := DecodeRecord(bytes.NewReader(data))
			if err != nil {
				t.Fatalf("DecodeRecord: %v", err)
			}
			if got.Version != tt.rec.Version {
				t.Errorf("Version = %d, want %d", got.Version, tt.rec.Version)
			}
			if got.Type != tt.rec.Type {
				t.Errorf("Type = %d, want %d", got.Type, tt.rec.Type)
			}
			if got.RequestID != tt.rec.RequestID {
				t.Errorf("RequestID = %d, want %d", got.RequestID, tt.rec.RequestID)
			}
			if got.ContentLength != tt.rec.ContentLength {
				t.Errorf("ContentLength = %d, want %d", got.ContentLength, tt.rec.ContentLength)
			}
			if !bytes.Equal(got.Content, tt.rec.Content) {
				t.Errorf("Content mismatch")
			}
		})
	}
}

func TestEncodeDecodeNameValue(t *testing.T) {
	tests := []struct {
		name  string
		value string
	}{
		{"REQUEST_METHOD", "GET"},
		{"SCRIPT_FILENAME", "/app/index.php"},
		{"QUERY_STRING", "foo=bar&baz=qux"},
		{"CONTENT_TYPE", "application/x-www-form-urlencoded"},
		{"HTTP_HOST", "example.com"},
		{"", ""},               // empty name and value
		{"key", ""},            // empty value
		{"", "value"},          // empty name
		{"long", string(make([]byte, 1000))}, // long value
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			err := encodeNameValuePair(&buf, tt.name, tt.value)
			if err != nil {
				t.Fatalf("encodeNameValuePair: %v", err)
			}

			data := buf.Bytes()
			// Read back.
			nLen, nBytes, err := DecodeLength(data)
			if err != nil {
				t.Fatalf("DecodeLength name: %v", err)
			}
			if nLen != len(tt.name) {
				t.Errorf("name length = %d, want %d", nLen, len(tt.name))
			}

			vLen, vBytes, err := DecodeLength(data[nBytes:])
			if err != nil {
				t.Fatalf("DecodeLength value: %v", err)
			}
			if vLen != len(tt.value) {
				t.Errorf("value length = %d, want %d", vLen, len(tt.value))
			}

			offset := nBytes + vBytes
			gotName := string(data[offset : offset+nLen])
			offset += nLen
			gotValue := string(data[offset : offset+vLen])

			if gotName != tt.name {
				t.Errorf("name = %q, want %q", gotName, tt.name)
			}
			if gotValue != tt.value {
				t.Errorf("value = %q, want %q", gotValue, tt.value)
			}
		})
	}
}

func TestDecodeLength(t *testing.T) {
	tests := []struct {
		name    string
		data    []byte
		length  int
		nBytes  int
		wantErr bool
	}{
		{"zero", []byte{0}, 0, 1, false},
		{"127", []byte{127}, 127, 1, false},
		{"128 four-byte", []byte{0x80, 0, 0, 128}, 128, 4, false},
		{"large four-byte", []byte{0x80, 0, 1, 0}, 256, 4, false},
		{"empty", nil, 0, 0, true},
		{"truncated four-byte", []byte{0x80, 0}, 0, 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			length, n, err := DecodeLength(tt.data)
			if (err != nil) != tt.wantErr {
				t.Errorf("err = %v, wantErr = %v", err, tt.wantErr)
			}
			if !tt.wantErr {
				if length != tt.length {
					t.Errorf("length = %d, want %d", length, tt.length)
				}
				if n != tt.nBytes {
					t.Errorf("nBytes = %d, want %d", n, tt.nBytes)
				}
			}
		})
	}
}

func TestEncodeRecord_PaddingAlignment(t *testing.T) {
	// Content of 5 bytes should be padded to 8.
	rec := Record{
		Version:       protoVersion,
		Type:          typeStdout,
		RequestID:     1,
		ContentLength: 5,
		Content:       []byte("hello"),
	}
	data := EncodeRecord(rec)
	// 8 (header) + 5 (content) + 3 (padding) = 16
	if len(data) != 16 {
		t.Errorf("encoded size = %d, want 16", len(data))
	}

	// Content of 8 bytes should have 0 padding.
	rec.ContentLength = 8
	rec.Content = []byte("hello123")
	data = EncodeRecord(rec)
	// 8 (header) + 8 (content) + 0 (padding) = 16
	if len(data) != 16 {
		t.Errorf("encoded size = %d, want 16", len(data))
	}
}

func TestRoundTrip_MultipleRecords(t *testing.T) {
	records := []Record{
		{Version: 1, Type: typeBeginReq, RequestID: 1, ContentLength: 8,
			Content: []byte{0, 1, 0, 0, 0, 0, 0, 0}},
		{Version: 1, Type: typeParams, RequestID: 1, ContentLength: 0},
		{Version: 1, Type: typeStdin, RequestID: 1, ContentLength: 0},
	}

	var buf bytes.Buffer
	for _, rec := range records {
		buf.Write(EncodeRecord(rec))
	}

	dec := bytes.NewReader(buf.Bytes())
	for i, want := range records {
		got, err := DecodeRecord(dec)
		if err != nil {
			t.Fatalf("record %d: %v", i, err)
		}
		if got.Type != want.Type {
			t.Errorf("record %d: Type = %d, want %d", i, got.Type, want.Type)
		}
		if got.RequestID != want.RequestID {
			t.Errorf("record %d: RequestID = %d, want %d", i, got.RequestID, want.RequestID)
		}
	}
}

func FuzzFastCGIRecordParser(f *testing.F) {
	// Seed with valid records.
	f.Add(EncodeRecord(Record{
		Version: 1, Type: typeBeginReq, RequestID: 1, ContentLength: 8,
		Content: []byte{0, 1, 0, 0, 0, 0, 0, 0},
	}))
	f.Add(EncodeRecord(Record{
		Version: 1, Type: typeStdout, RequestID: 1, ContentLength: 5,
		Content: []byte("hello"),
	}))
	f.Add([]byte{1, 6, 0, 1, 0, 5, 3, 0, 'h', 'e', 'l', 'l', 'o', 0, 0, 0})

	f.Fuzz(func(t *testing.T, data []byte) {
		r := bytes.NewReader(data)
		for r.Len() > 0 {
			_, err := DecodeRecord(r)
			if err != nil {
				return
			}
		}
	})
}

func FuzzDecodeLength(f *testing.F) {
	f.Add([]byte{0})
	f.Add([]byte{127})
	f.Add([]byte{0x80, 0, 0, 128})
	f.Add([]byte{0x80, 0, 1, 0})
	f.Add([]byte{})

	f.Fuzz(func(t *testing.T, data []byte) {
		length, n, err := DecodeLength(data)
		if err != nil {
			return
		}
		// Invariant: n must be 1 or 4.
		if n != 1 && n != 4 {
			t.Errorf("n = %d, want 1 or 4", n)
		}
		// Invariant: length must be non-negative.
		if length < 0 {
			t.Errorf("length = %d, want >= 0", length)
		}
		// Invariant: n must not exceed input length.
		if n > len(data) {
			t.Errorf("n = %d > len(data) = %d", n, len(data))
		}
	})
}

func FuzzFastCGIParams(f *testing.F) {
	f.Add("REQUEST_METHOD", "GET")
	f.Add("SCRIPT_FILENAME", "/app/index.php")
	f.Add("", "")
	f.Add("key", string([]byte{0, 0, 0, 128}))

	f.Fuzz(func(t *testing.T, name, value string) {
		var buf bytes.Buffer
		err := encodeNameValuePair(&buf, name, value)
		if err != nil {
			return
		}
		data := buf.Bytes()
		nLen, nBytes, err := DecodeLength(data)
		if err != nil {
			return
		}
		vLen, vBytes, err := DecodeLength(data[nBytes:])
		if err != nil {
			return
		}
		offset := nBytes + vBytes
		if offset+nLen+vLen > len(data) {
			return
		}
		gotName := string(data[offset : offset+nLen])
		gotValue := string(data[offset+nLen : offset+nLen+vLen])
		if gotName != name || gotValue != value {
			t.Errorf("round-trip failed: got name=%q value=%q, want name=%q value=%q",
				gotName, gotValue, name, value)
		}
	})
}
