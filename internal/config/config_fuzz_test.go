package config

import (
	"testing"
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
