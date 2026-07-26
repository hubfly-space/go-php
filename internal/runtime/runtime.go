package runtime

import (
	"crypto/sha256"
	"fmt"
	"runtime"
	"sort"
	"strings"
)

// Runtime identifies a specific PHP build with extensions.
type Runtime struct {
	ID          RuntimeID
	Version     string // e.g. "8.3.6"
	Platform    string // e.g. "linux"
	Arch        string // e.g. "amd64"
	BuildFlavor string // e.g. "cli", "fpm"
	Extensions  []Extension
}

// RuntimeID is a deterministic identifier for a runtime.
// Format: php:<version>:<platform>:<arch>:<flavor>:<ext-hash>
type RuntimeID string

// Extension represents a PHP extension.
type Extension struct {
	Name    string // e.g. "redis", "mbstring"
	Version string // e.g. "6.0.2"
	Hash    string // sha256 of artifact
}

// GenerateID computes the RuntimeID from runtime fields.
func GenerateID(version, platform, arch, flavor string, exts []Extension) RuntimeID {
	sorted := make([]Extension, len(exts))
	copy(sorted, exts)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Name < sorted[j].Name
	})

	h := sha256.New()
	for _, e := range sorted {
		h.Write([]byte(e.Name))
		h.Write([]byte(e.Version))
		h.Write([]byte(e.Hash))
	}
	extHash := fmt.Sprintf("%x", h.Sum(nil)[:8])

	return RuntimeID(fmt.Sprintf("php:%s:%s:%s:%s:%s", version, platform, arch, flavor, extHash))
}

// ParseID parses a RuntimeID back into components.
func ParseID(id RuntimeID) (version, platform, arch, flavor, extHash string, ok bool) {
	parts := strings.SplitN(string(id), ":", 6)
	if len(parts) != 6 || parts[0] != "php" {
		return "", "", "", "", "", false
	}
	return parts[1], parts[2], parts[3], parts[4], parts[5], true
}

// Dir returns the directory name for this runtime.
func (r *Runtime) Dir() string {
	return string(r.ID)
}

// CurrentPlatform returns the current OS and arch.
func CurrentPlatform() (platform, arch string) {
	return runtime.GOOS, runtime.GOARCH
}

// DefaultFlavor returns the default build flavor.
func DefaultFlavor() string {
	return "fpm"
}
