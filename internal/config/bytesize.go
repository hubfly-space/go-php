package config

import (
	"fmt"
	"strconv"
	"strings"
)

// maxByteSize caps parsed sizes at 1 TiB. Anything larger is far more likely to
// be a typo than an intent, and §5.4 requires every limit to have a bound.
const maxByteSize int64 = 1 << 40

// byteUnits maps a normalized unit suffix to its multiplier.
//
// Note that KB/MB/GB are treated as binary multiples (1024-based), matching the
// convention used by php.ini and nginx rather than SI. The explicit KiB/MiB/GiB
// spellings are accepted as synonyms so a config can be unambiguous if it wants
// to be.
var byteUnits = map[string]int64{
	"":    1,
	"B":   1,
	"K":   1 << 10,
	"KB":  1 << 10,
	"KIB": 1 << 10,
	"M":   1 << 20,
	"MB":  1 << 20,
	"MIB": 1 << 20,
	"G":   1 << 30,
	"GB":  1 << 30,
	"GIB": 1 << 30,
	"T":   1 << 40,
	"TB":  1 << 40,
	"TIB": 1 << 40,
}

// ParseByteSize parses a human-readable size such as "20MB", "512K", or
// "1048576" into a byte count.
//
// Only whole numbers are accepted: "1.5MB" is an error rather than a silent
// truncation, per §5.6 (secure defaults with visible escape hatches — a config
// value the operator did not mean should fail loudly at load).
func ParseByteSize(s string) (int64, error) {
	trimmed := strings.TrimSpace(s)
	if trimmed == "" {
		return 0, fmt.Errorf("empty size")
	}

	// Split the leading digits from the trailing unit.
	i := 0
	for i < len(trimmed) && trimmed[i] >= '0' && trimmed[i] <= '9' {
		i++
	}
	digits, unit := trimmed[:i], strings.ToUpper(strings.TrimSpace(trimmed[i:]))

	if digits == "" {
		return 0, fmt.Errorf("size %q: missing numeric value", s)
	}

	multiplier, ok := byteUnits[unit]
	if !ok {
		return 0, fmt.Errorf("size %q: unknown unit %q (want B, KB, MB, GB, or TB)", s, unit)
	}

	n, err := strconv.ParseInt(digits, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("size %q: %w", s, err)
	}

	// Check the multiplication before performing it.
	if n > maxByteSize/multiplier {
		return 0, fmt.Errorf("size %q exceeds the maximum of 1TB", s)
	}

	return n * multiplier, nil
}
