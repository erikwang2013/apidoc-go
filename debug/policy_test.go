package debug

import (
	"context"
	"strings"
	"testing"
)

func TestPolicyLiteralHostAndPort(t *testing.T) {
	ctx := context.Background()
	p := &Policy{AllowHosts: []string{"127.0.0.1:9000"}}
	if err := p.Check(ctx, "tcp://127.0.0.1:9000/login"); err != nil {
		t.Fatalf("allowed target denied: %v", err)
	}
	if err := p.Check(ctx, "tcp://127.0.0.1:9001/login"); err == nil {
		t.Fatal("wrong port allowed")
	}
	if err := p.Check(ctx, "tcp://127.0.0.2:9000/login"); err == nil {
		t.Fatal("wrong host allowed")
	}
}

func TestPolicyCIDR(t *testing.T) {
	ctx := context.Background()
	p := &Policy{AllowHosts: []string{"10.0.0.0/8"}, AllowPorts: []int{1883}}
	if err := p.Check(ctx, "mqtt://10.1.2.3:1883/topic"); err != nil {
		t.Fatalf("CIDR member denied: %v", err)
	}
	if err := p.Check(ctx, "mqtt://192.168.1.1:1883/topic"); err == nil {
		t.Fatal("non-member allowed")
	}
	if err := p.Check(ctx, "mqtt://10.1.2.3:1884/topic"); err == nil {
		t.Fatal("port gate ignored")
	}
}

func TestPolicyIPClasses(t *testing.T) {
	ctx := context.Background()
	cases := []struct {
		name   string
		policy Policy
		url    string
		ok     bool
	}{
		{"loopback allowed", Policy{AllowLoopback: true}, "tcp://127.0.0.1:9000", true},
		{"loopback denied", Policy{AllowPublic: true}, "tcp://127.0.0.1:9000", false},
		{"private allowed", Policy{AllowPrivate: true}, "tcp://10.1.2.3:9000", true},
		{"private denied", Policy{AllowPublic: true}, "tcp://10.1.2.3:9000", false},
		{"public allowed", Policy{AllowPublic: true}, "tcp://8.8.8.8:9000", true},
		{"public denied", Policy{AllowPrivate: true}, "tcp://8.8.8.8:9000", false},
		{"empty policy", Policy{}, "tcp://127.0.0.1:9000", false},
		{"no host", Policy{AllowPublic: true}, "not-a-url", false},
		{"unix socket", Policy{AllowPrivate: true}, "unix:///tmp/app.sock", false},
		{"ipv6 loopback", Policy{AllowLoopback: true}, "tcp://[::1]:9000", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.policy.Check(ctx, tc.url)
			if tc.ok && err != nil {
				t.Fatalf("%s denied: %v", tc.url, err)
			}
			if !tc.ok && err == nil {
				t.Fatalf("%s allowed", tc.url)
			}
		})
	}
}

func TestPolicyLocalhost(t *testing.T) {
	p := &Policy{AllowLoopback: true, AllowPorts: []int{9000}}
	if err := p.Check(context.Background(), "tcp://localhost:9000"); err != nil {
		t.Fatalf("localhost denied: %v", err)
	}
}

func TestPolicyDefaultPort(t *testing.T) {
	p := &Policy{AllowHosts: []string{"127.0.0.1"}, AllowPorts: []int{443}}
	if err := p.Check(context.Background(), "wss://127.0.0.1/socket"); err != nil {
		t.Fatalf("default port denied: %v", err)
	}
}

func TestPolicyResolutionFailureIsDenial(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	p := &Policy{AllowPublic: true}
	if err := p.Check(ctx, "tcp://no-such-host.invalid:9000"); err == nil {
		t.Fatal("resolution failure was allowed")
	}
}

func TestPolicyNilAllows(t *testing.T) {
	var p *Policy
	if err := p.Check(context.Background(), "tcp://127.0.0.1:1"); err != nil {
		t.Fatalf("nil policy denied: %v", err)
	}
}

func TestPolicyRedactsUserinfo(t *testing.T) {
	p := &Policy{}
	err := p.Check(context.Background(), "amqp://user:secret@127.0.0.1:5672/q")
	if err == nil {
		t.Fatal("empty policy should deny")
	}
	if strings.Contains(err.Error(), "secret") {
		t.Fatalf("policy error leaked userinfo: %v", err)
	}
}
