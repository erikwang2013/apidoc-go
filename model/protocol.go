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
	switch s {
	case "http", "https":
		return "http"
	case "ws", "wss":
		return "ws"
	case "grpc", "grpcs":
		return "grpc"
	case "mqtt", "mqtts":
		return "mqtt"
	case "amqp", "amqps":
		return "amqp"
	case "stomp", "stomps":
		return "stomp"
	case "coap", "coaps":
		return "coap"
	default:
		return s
	}
}
