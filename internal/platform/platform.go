package platform

import (
	"runtime"
)

// OSInfo holds operating system platform metadata.
type OSInfo struct {
	OS          string
	Arch        string
	NumCPU      int
	GoVersion   string
	HasUnixSock bool
}

// CurrentInfo returns metadata about the current running platform.
func CurrentInfo() OSInfo {
	return OSInfo{
		OS:          runtime.GOOS,
		Arch:        runtime.GOARCH,
		NumCPU:      runtime.NumCPU(),
		GoVersion:   runtime.Version(),
		HasUnixSock: runtime.GOOS != "windows",
	}
}
