package runtime

import (
	"fmt"
	"sort"
	"strings"
)

// VersionPolicy determines how a version constraint resolves.
type VersionPolicy string

const (
	PolicyExact  VersionPolicy = "exact"  // must match exactly
	PolicyPatch  VersionPolicy = "patch"  // match major.minor.patch
	PolicyMinor  VersionPolicy = "minor"  // match major.minor
	PolicyLocked VersionPolicy = "locked" // use lock file
)

// SelectVersion picks the best runtime from a list using the policy.
func SelectVersion(available []Runtime, constraint string, policy VersionPolicy) (*Runtime, error) {
	if len(available) == 0 {
		return nil, fmt.Errorf("no runtimes available")
	}

	switch policy {
	case PolicyExact:
		for i := range available {
			if available[i].Version == constraint {
				return &available[i], nil
			}
		}
		return nil, fmt.Errorf("exact match not found for %q", constraint)

	case PolicyPatch:
		return selectByPrefix(available, constraint, 3) // major.minor.patch

	case PolicyMinor:
		return selectByPrefix(available, constraint, 2) // major.minor

	case PolicyLocked:
		// Locked policy expects exact match (from lock file).
		for i := range available {
			if available[i].Version == constraint {
				return &available[i], nil
			}
		}
		return nil, fmt.Errorf("locked version %q not found", constraint)

	default:
		return nil, fmt.Errorf("unknown version policy: %s", policy)
	}
}

func selectByPrefix(available []Runtime, constraint string, parts int) (*Runtime, error) {
	prefix := versionPrefix(constraint, parts)

	var matches []Runtime
	for _, r := range available {
		if strings.HasPrefix(r.Version, prefix) {
			matches = append(matches, r)
		}
	}

	if len(matches) == 0 {
		return nil, fmt.Errorf("no runtime matching prefix %q", prefix)
	}

	// Sort by version descending, pick latest.
	sort.Slice(matches, func(i, j int) bool {
		return compareVersions(matches[i].Version, matches[j].Version) > 0
	})

	return &matches[0], nil
}

func versionPrefix(version string, parts int) string {
	p := strings.Split(version, ".")
	if len(p) >= parts {
		return strings.Join(p[:parts], ".")
	}
	return version
}

// compareVersions compares two semver strings (simple lexicographic for same-length).
func compareVersions(a, b string) int {
	aa := strings.Split(a, ".")
	bb := strings.Split(b, ".")

	maxLen := len(aa)
	if len(bb) > maxLen {
		maxLen = len(bb)
	}

	for i := 0; i < maxLen; i++ {
		var ai, bi int
		if i < len(aa) {
			fmt.Sscanf(aa[i], "%d", &ai)
		}
		if i < len(bb) {
			fmt.Sscanf(bb[i], "%d", &bi)
		}
		if ai > bi {
			return 1
		}
		if ai < bi {
			return -1
		}
	}
	return 0
}
