package config

import (
	"testing"
	"time"
)

func FuzzConfigDecoder(f *testing.F) {
	f.Add([]byte(`schema: gateway/v1
server:
  listen: 127.0.0.1:8080
routes:
  - path: /
    target: index.php
`))
	f.Add([]byte(`schema: gateway/v1
server:
  max_body_size: 20MB
`))

	f.Fuzz(func(t *testing.T, data []byte) {
		cfg, err := Parse(data)
		if err == nil && cfg != nil {
			_ = Validate(cfg)
		}
	})
}

func FuzzByteSizeParser(f *testing.F) {
	seeds := []string{
		"10MB", "512KB", "1GB", "100B", "0", "-5MB", "invalidMB", "1.5GB", "99999999999999999999999999GB",
	}
	for _, s := range seeds {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, input string) {
		_, _ = ParseByteSize(input)
	})
}

func FuzzDurationParser(f *testing.F) {
	seeds := []string{
		"10s", "5m", "1h", "500ms", "0", "-1s", "invalid", "9999999999999999999999h",
	}
	for _, s := range seeds {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, input string) {
		_, _ = time.ParseDuration(input)
	})
}
