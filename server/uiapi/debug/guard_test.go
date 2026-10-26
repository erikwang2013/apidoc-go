package debug

import (
	"net"
	"testing"
)

func cfg(private bool, hosts ...string) Config {
	return Config{AllowPrivateNetworks: private, AllowHosts: hosts}
}

func TestCheckIPPolicy(t *testing.T) {
	allowed := []string{"8.8.8.8", "2606:4700:4700::1111"}
	for _, ip := range allowed {
		if err := checkIP(cfg(false), parseIP(t, ip)); err != nil {
			t.Errorf("%s should be allowed: %v", ip, err)
		}
	}
	denied := []string{
		"127.0.0.1", "10.1.2.3", "172.16.0.1", "192.168.1.1", "169.254.1.1",
		"100.64.0.1", "192.0.0.9", "198.18.0.1", "240.0.0.1", "0.1.2.3",
		"::1", "fc00::1", "fe80::1", "ff02::1", "::",
	}
	for _, ip := range denied {
		if err := checkIP(cfg(false), parseIP(t, ip)); err == nil {
			t.Errorf("%s should be denied", ip)
		}
	}
	if err := checkIP(cfg(true), parseIP(t, "10.1.2.3")); err != nil {
		t.Errorf("private network should be allowed when configured: %v", err)
	}
}

func TestCloudMetadataAlwaysBlocked(t *testing.T) {
	for _, ip := range []string{
		"169.254.169.254", "169.254.170.2", "100.100.100.200",
		"fd00:ec2::254", "::ffff:169.254.169.254",
	} {
		if err := checkIP(cfg(true), parseIP(t, ip)); err == nil {
			t.Errorf("%s must stay blocked", ip)
		}
	}
}

func TestHostAllowlist(t *testing.T) {
	cases := []struct {
		allow []string
		host  string
		want  bool
	}{
		{[]string{"api.example.com"}, "api.example.com", true},
		{[]string{"api.example.com"}, "API.EXAMPLE.COM.", true},
		{[]string{"api.example.com"}, "evil.example.com", false},
		{[]string{"*.example.com"}, "api.example.com", true},
		{[]string{"*.example.com"}, "example.com", true},
		{[]string{"*.example.com"}, "example.com.evil", false},
		{nil, "anything.example", true},
	}
	for _, tc := range cases {
		if got := hostAllowed(cfg(false, tc.allow...), tc.host); got != tc.want {
			t.Errorf("hostAllowed(%v, %q) = %v, want %v", tc.allow, tc.host, got, tc.want)
		}
	}
}

func TestCheckTargetScheme(t *testing.T) {
	for _, raw := range []string{"file:///etc/passwd", "ftp://example.com/x", "gopher://example.com/"} {
		if _, err := CheckTarget(cfg(false), raw); err == nil {
			t.Errorf("%s should be rejected", raw)
		}
	}
	target, err := CheckTarget(cfg(false), "http://8.8.8.8:8080/x")
	if err != nil {
		t.Fatalf("public literal target rejected: %v", err)
	}
	if target.Port != "8080" || len(target.Addrs) != 1 {
		t.Fatalf("unexpected target: %+v", target)
	}
}

func parseIP(t *testing.T, s string) net.IP {
	t.Helper()
	ip := net.ParseIP(s)
	if ip == nil {
		t.Fatalf("bad test IP %q", s)
	}
	return ip
}
