// Package opt reads debug adapter options.
package opt

import (
	"strconv"
	"strings"
	"time"
)

// List splits a comma-separated option into trimmed, non-empty values.
func List(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// Duration parses m[key] as milliseconds, clamped to (0, max]. It returns
// def when the option is absent or not a positive integer.
func Duration(m map[string]string, key string, def, max time.Duration) time.Duration {
	ms, err := strconv.Atoi(m[key])
	if err != nil || ms <= 0 {
		return def
	}
	d := time.Duration(ms) * time.Millisecond
	if d > max {
		return max
	}
	return d
}
