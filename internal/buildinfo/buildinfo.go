package buildinfo

import (
	"runtime"
	"time"
)

// These are set at build time via -ldflags. See LDFLAGS in the Makefile — the
// symbol paths there must match this package, or the linker drops them without
// complaint.
var (
	Version   = "dev"
	Commit    = "unknown"
	BuildDate = "unknown"
)

// GoVersion is not an ldflag target: the toolchain already knows it.
var GoVersion = runtime.Version()

// Info holds build metadata.
type Info struct {
	Version   string
	Commit    string
	BuildDate string
	GoVersion string
}

// Get returns the build info.
func Get() Info {
	return Info{
		Version:   Version,
		Commit:    Commit,
		BuildDate: BuildDate,
		GoVersion: GoVersion,
	}
}

// BuildTime parses and returns the build date.
func BuildTime() time.Time {
	t, err := time.Parse(time.RFC3339, BuildDate)
	if err != nil {
		return time.Time{}
	}
	return t
}
