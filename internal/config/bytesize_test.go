package config

import (
	"testing"
)

func TestParseByteSize(t *testing.T) {
	tests := []struct {
		input   string
		want    int64
		wantErr bool
	}{
		{"1024", 1024, false},
		{"1KB", 1024, false},
		{"1MB", 1024 * 1024, false},
		{"20MB", 20 * 1024 * 1024, false},
		{"1GB", 1024 * 1024 * 1024, false},
		{"0", 0, false},
		{"invalid", 0, true},
		{"100XB", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := ParseByteSize(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ParseByteSize(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("ParseByteSize(%q) = %d, want %d", tt.input, got, tt.want)
			}
		})
	}
}
