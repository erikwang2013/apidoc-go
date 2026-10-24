package model

import "testing"

func TestProtocolOf(t *testing.T) {
	cases := []struct{ in, want string }{
		{"https://api.example.com/x", "http"},
		{"wss://api.example.com/ws", "ws"},
		{"grpcs://host/pkg.Svc/M", "grpc"},
		{"mqtts://broker:8883/topic", "mqtt"},
		{"amqps://broker/queue", "amqp"},
		{"tcp://127.0.0.1:9000", "tcp"},
		{"ssh://host", "ssh"},
		{"/api/users", "http"},
		{"localhost:50051", "http"}, // no "://": not a scheme
		{"", "http"},
	}
	for _, c := range cases {
		if got := ProtocolOf(c.in); got != c.want {
			t.Errorf("ProtocolOf(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
