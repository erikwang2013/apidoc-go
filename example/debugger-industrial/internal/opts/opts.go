// Package opts parses debug-panel options shared by the industrial adapters.
package opts

import (
	"fmt"
	"strconv"
	"time"
)

// Uint returns option name as uint64, clamped to max. def is used when the
// option is absent; max <= 0 means unlimited.
func Uint(opts map[string]string, name string, def, max uint64) (uint64, error) {
	s := opts[name]
	if s == "" {
		return def, nil
	}
	v, err := strconv.ParseUint(s, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", name, err)
	}
	if max > 0 && v > max {
		v = max
	}
	return v, nil
}

// Timeout returns option name interpreted as milliseconds, in [1ms, max].
func Timeout(opts map[string]string, name string, def, max time.Duration) (time.Duration, error) {
	v, err := Uint(opts, name, uint64(def/time.Millisecond), uint64(max/time.Millisecond))
	if err != nil {
		return 0, err
	}
	if v == 0 {
		v = 1
	}
	return time.Duration(v) * time.Millisecond, nil
}
