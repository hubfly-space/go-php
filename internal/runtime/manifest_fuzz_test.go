package runtime

import (
	"encoding/json"
	"testing"
)

func FuzzRuntimeManifest(f *testing.F) {
	f.Add([]byte(`{"version":"8.3.0","platform":"linux","arch":"amd64","sha256":"abc"}`))
	f.Add([]byte(`{"runtimes":[{"version":"8.2.0","platform":"darwin","arch":"arm64"}]}`))

	f.Fuzz(func(t *testing.T, data []byte) {
		var m Manifest
		if err := json.Unmarshal(data, &m); err == nil {
			_ = m.Version
			_ = m.Platform
			_ = m.Arch
		}

		var idx Index
		if err := json.Unmarshal(data, &idx); err == nil {
			_ = len(idx.Runtimes)
		}
	})
}
