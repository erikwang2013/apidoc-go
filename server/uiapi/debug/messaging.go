//go:build apidoc_debug_messaging || apidoc_debug_mqtt || apidoc_debug_stomp || apidoc_debug_amqp || apidoc_debug_kafka || apidoc_debug_xmpp || apidoc_debug_all

package debug

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// parseMessagingTarget parses host:port targets. Credentials, paths and query
// strings stay server-side options, never part of the target URL.
func parseMessagingTarget(target, defaultScheme, defaultPort string, schemes map[string]bool) (host, port, scheme string, err error) {
	raw := target
	if !strings.Contains(raw, "://") {
		raw = defaultScheme + "://" + raw
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return "", "", "", fmt.Errorf("invalid target `%s`: %v", target, err)
	}
	scheme = strings.ToLower(parsed.Scheme)
	if !schemes[scheme] {
		allowed := make([]string, 0, len(schemes))
		for name := range schemes {
			allowed = append(allowed, name+"://")
		}
		return "", "", "", fmt.Errorf("unsupported scheme `%s`; use %s", parsed.Scheme, strings.Join(allowed, " or "))
	}
	if parsed.User != nil {
		return "", "", "", fmt.Errorf("credentials must be provided via opts, not the target URL")
	}
	if parsed.Path != "" && parsed.Path != "/" {
		return "", "", "", fmt.Errorf("target must not contain a path; connection options belong in opts")
	}
	if parsed.RawQuery != "" {
		return "", "", "", fmt.Errorf("target must not contain a query string; connection options belong in opts")
	}
	host, port = parsed.Hostname(), parsed.Port()
	if host == "" {
		return "", "", "", fmt.Errorf("target requires a host")
	}
	if port == "" {
		port = defaultPort
	}
	return host, port, scheme, nil
}

// dialMessaging opens a TCP connection pinned to a checked address.
func dialMessaging(cfg Config, host, port string, timeout time.Duration) (net.Conn, error) {
	addrs, err := CheckHost(cfg, host, port)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	conn, err := (&net.Dialer{}).DialContext(ctx, "tcp", net.JoinHostPort(addrs[0].String(), port))
	if err != nil {
		if ctx.Err() != nil {
			return nil, fmt.Errorf("connect timed out")
		}
		return nil, fmt.Errorf("connect failed: %v", err)
	}
	return conn, nil
}

// windowDuration is the message collection window: opts.duration_ms clamped to
// the request timeout, defaulting to 2s.
func windowDuration(cfg Config, req *Request) time.Duration {
	ms := 2000
	if value, err := strconv.Atoi(req.Opts["duration_ms"]); err == nil {
		ms = value
	}
	if ms < 1 {
		ms = 1
	}
	limit := int(req.Timeout(cfg) / time.Millisecond)
	if limit < 1 {
		limit = 1
	}
	if ms > limit {
		ms = limit
	}
	return time.Duration(ms) * time.Millisecond
}

// wantMessages is the bounded message count for one debug call (1..1000).
func wantMessages(req *Request) int {
	want := 1
	if value, err := strconv.Atoi(req.Opts["messages"]); err == nil {
		want = value
	}
	if want < 1 {
		want = 1
	}
	if want > 1000 {
		want = 1000
	}
	return want
}

func optBool(req *Request, key string, def bool) (bool, error) {
	value, found := req.Opts[key]
	if !found || value == "" {
		return def, nil
	}
	switch value {
	case "true":
		return true, nil
	case "false":
		return false, nil
	default:
		return false, fmt.Errorf("invalid %s `%s`; use \"true\" or \"false\"", key, value)
	}
}

// addMessageBytes enforces the cumulative response cap for one debug call.
func addMessageBytes(total *int, n, limit int) error {
	*total += n
	if *total > limit {
		return fmt.Errorf("messages exceed max_bytes (%d)", limit)
	}
	return nil
}

func payloadJSON(p Payload) map[string]string {
	encoding := p.Encoding
	if encoding == "" {
		encoding = "utf8"
	}
	return map[string]string{"encoding": encoding, "data": p.Data}
}

func redactSecrets(text string, secrets ...string) string {
	for _, secret := range secrets {
		if secret != "" {
			text = strings.ReplaceAll(text, secret, "***")
		}
	}
	return text
}

func redactedError(text string, secrets ...string) error {
	return fmt.Errorf("%s", redactSecrets(text, secrets...))
}

func redactErr(err error, secrets ...string) error {
	return redactedError(err.Error(), secrets...)
}

func randomClientID(prefix string) string {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		return prefix
	}
	return prefix + hex.EncodeToString(buf)
}
