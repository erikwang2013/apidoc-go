//go:build apidoc_debug_iot || apidoc_debug_industrial || apidoc_debug_all

package debug

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"
)

func protocolRandomUint16() uint16 {
	buf := make([]byte, 2)
	if _, err := rand.Read(buf); err != nil {
		return 0
	}
	return binary.BigEndian.Uint16(buf)
}

// parseProtocolTarget accepts `scheme://host:port` or `host:port` for the
// non-HTTP families. Credentials, paths and query strings are ignored: the
// target host/port is server-derived and the protocol path lives in opts.
func parseProtocolTarget(target, defaultScheme, defaultPort string, schemes ...string) (string, string, error) {
	raw := target
	if !strings.Contains(raw, "://") {
		raw = defaultScheme + "://" + raw
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return "", "", fmt.Errorf("invalid target `%s`: %v", target, err)
	}
	scheme := strings.ToLower(parsed.Scheme)
	allowed := false
	for _, candidate := range schemes {
		if scheme == candidate {
			allowed = true
			break
		}
	}
	if !allowed {
		return "", "", fmt.Errorf("unsupported %s target scheme `%s`", defaultScheme, parsed.Scheme)
	}
	host := parsed.Hostname()
	if host == "" {
		return "", "", fmt.Errorf("target requires a host")
	}
	port := parsed.Port()
	if port == "" {
		port = defaultPort
	}
	return host, port, nil
}

// dialPinned resolves through the SSRF guard and connects to the first checked
// address, so a DNS answer cannot change between validation and connect.
func dialPinned(cfg Config, network, host, port string, timeout time.Duration) (net.Conn, error) {
	addrs, err := CheckHost(cfg, host, port)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	conn, err := (&net.Dialer{}).DialContext(ctx, network, net.JoinHostPort(addrs[0].String(), port))
	if err != nil {
		if ctx.Err() != nil {
			return nil, fmt.Errorf("%s connect timed out", network)
		}
		return nil, fmt.Errorf("%s connect failed: %v", network, err)
	}
	return conn, nil
}

// protocolJSONFloat keeps non-finite floats as strings, matching Rust's
// f64::to_string() so JSON encoding never fails on NaN/Infinity.
func protocolJSONFloat(value float64) any {
	if math.IsNaN(value) {
		return "NaN"
	}
	if math.IsInf(value, 1) {
		return "inf"
	}
	if math.IsInf(value, -1) {
		return "-inf"
	}
	return value
}

// protocolJSONFloat32 formats through float32 precision, matching serde_json's
// shortest-f32 output instead of Go's float64 expansion (0.1 vs 0.10000000149).
func protocolJSONFloat32(value float32) any {
	if !math.IsNaN(float64(value)) && !math.IsInf(float64(value), 0) {
		return json.Number(strconv.FormatFloat(float64(value), 'g', -1, 32))
	}
	return protocolJSONFloat(float64(value))
}

func protocolOptUint(req *Request, key string, def uint64) uint64 {
	if value, err := strconv.ParseUint(req.Opts[key], 10, 64); err == nil {
		return value
	}
	return def
}

func protocolOptBool(req *Request, key string, def bool) (bool, error) {
	value, found := req.Opts[key]
	if !found || value == "" {
		return def, nil
	}
	switch strings.ToLower(value) {
	case "true", "1":
		return true, nil
	case "false", "0":
		return false, nil
	default:
		return false, fmt.Errorf("invalid %s `%s`; use \"true\" or \"false\"", key, value)
	}
}

// protocolWindow is the collection window: opts.duration_ms clamped to the
// configured timeout, defaulting to 3s for the IoT/industrial families.
func protocolWindow(cfg Config, req *Request) time.Duration {
	ms := protocolOptUint(req, "duration_ms", 3000)
	limit := uint64(cfg.Timeout() / time.Millisecond)
	if limit < 1 {
		limit = 1
	}
	if ms < 1 {
		ms = 1
	}
	if ms > limit {
		ms = limit
	}
	return time.Duration(ms) * time.Millisecond
}
