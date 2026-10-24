package model

import (
	"net/url"
	"strings"
)

// ProtocolOf maps a documented address to a protocol key. An address
// without "://" is an HTTP path or host:port, never a scheme.
func ProtocolOf(rawURL string) string {
	if !strings.Contains(rawURL, "://") {
		return "http"
	}
	u, err := url.Parse(rawURL)
	if err != nil || u.Scheme == "" {
		return "http"
	}
	s := strings.ToLower(u.Scheme)
	if p, ok := protocolScheme[s]; ok {
		return p
	}
	return s
}
