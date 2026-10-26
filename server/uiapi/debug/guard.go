package debug

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
)

// Target is a resolved and checked relay target. Addrs holds only addresses
// that passed the policy, and Dial pins connections to one of them so a DNS
// answer cannot change between validation and connect.
type Target struct {
	URL   *url.URL
	Host  string
	Port  string
	Addrs []net.IP
}

// CheckTarget validates an http/https relay target. Only server-derived
// targets reach this function; client input never controls the host.
func CheckTarget(cfg Config, raw string) (*Target, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("invalid target URL: %v", err)
	}
	switch strings.ToLower(u.Scheme) {
	case "http", "https":
	default:
		return nil, fmt.Errorf("unsupported target scheme `%s`; only http/https", u.Scheme)
	}
	host := u.Hostname()
	if host == "" {
		return nil, errors.New("target URL requires a host")
	}
	port := u.Port()
	if port == "" {
		if strings.EqualFold(u.Scheme, "https") {
			port = "443"
		} else {
			port = "80"
		}
	}
	addrs, err := CheckHost(cfg, host, port)
	if err != nil {
		return nil, err
	}
	return &Target{URL: u, Host: host, Port: port, Addrs: addrs}, nil
}

// CheckHost applies the allowlist, resolves the host, and rejects every
// address that is not explicitly permitted.
func CheckHost(cfg Config, host, port string) ([]net.IP, error) {
	if !hostAllowed(cfg, host) {
		return nil, fmt.Errorf("target host `%s` is not in DebugConfig.allow_hosts", host)
	}
	var addrs []net.IP
	if ip := net.ParseIP(host); ip != nil {
		addrs = []net.IP{ip}
	} else {
		ctx, cancel := context.WithTimeout(context.Background(), cfg.Timeout())
		defer cancel()
		found, err := net.DefaultResolver.LookupIPAddr(ctx, host)
		if err != nil {
			if ctx.Err() != nil {
				return nil, fmt.Errorf("DNS lookup timed out for `%s`", host)
			}
			return nil, fmt.Errorf("DNS lookup failed for `%s`: %v", host, err)
		}
		for _, addr := range found {
			addrs = append(addrs, addr.IP)
		}
	}
	if len(addrs) == 0 {
		return nil, fmt.Errorf("target host `%s` resolved to no addresses", host)
	}
	for _, ip := range addrs {
		if err := checkIP(cfg, ip); err != nil {
			return nil, err
		}
	}
	return addrs, nil
}

// DialContext connects to a checked address, ignoring the hostname passed by
// the HTTP transport.
func (t *Target) DialContext(ctx context.Context, network, _ string) (net.Conn, error) {
	var lastErr error
	for _, ip := range t.Addrs {
		conn, err := (&net.Dialer{}).DialContext(ctx, network, net.JoinHostPort(ip.String(), t.Port))
		if err == nil {
			return conn, nil
		}
		lastErr = err
	}
	if lastErr == nil {
		lastErr = errors.New("target resolved to no addresses")
	}
	return nil, lastErr
}

func hostAllowed(cfg Config, host string) bool {
	if len(cfg.AllowHosts) == 0 {
		return true
	}
	host = strings.ToLower(strings.TrimSuffix(host, "."))
	for _, entry := range cfg.AllowHosts {
		entry = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(entry), "."))
		if suffix, ok := strings.CutPrefix(entry, "*."); ok {
			if host == suffix || strings.HasSuffix(host, "."+suffix) {
				return true
			}
			continue
		}
		if host == entry {
			return true
		}
	}
	return false
}

func checkIP(cfg Config, ip net.IP) error {
	if alwaysBlocked(ip) {
		return fmt.Errorf("target address %s is blocked (cloud metadata)", ip)
	}
	if cfg.AllowPrivateNetworks {
		return nil
	}
	if privateIP(ip) {
		return fmt.Errorf("target address %s is private/loopback/link-local; set DebugConfig.allow_private_networks = true to allow it", ip)
	}
	return nil
}

func alwaysBlocked(ip net.IP) bool {
	if v4 := ip.To4(); v4 != nil {
		switch {
		case v4[0] == 169 && v4[1] == 254 && v4[2] == 169 && v4[3] == 254,
			v4[0] == 169 && v4[1] == 254 && v4[2] == 170 && v4[3] == 2,
			v4[0] == 100 && v4[1] == 100 && v4[2] == 100 && v4[3] == 200:
			return true
		}
		return false
	}
	ip16 := ip.To16()
	if ip16 == nil {
		return false
	}
	// AWS IMDS IPv6: fd00:ec2::254.
	return ip16[0] == 0xfd && ip16[1] == 0x00 && ip16[2] == 0x0e && ip16[3] == 0xc2 &&
		ip16[4] == 0 && ip16[5] == 0 && ip16[6] == 0 && ip16[7] == 0 &&
		ip16[8] == 0 && ip16[9] == 0 && ip16[10] == 0 && ip16[11] == 0 &&
		ip16[12] == 0 && ip16[13] == 0 && ip16[14] == 0x02 && ip16[15] == 0x54
}

func privateIP(ip net.IP) bool {
	if v4 := ip.To4(); v4 != nil {
		return v4.IsPrivate() ||
			v4.IsLoopback() ||
			v4.IsLinkLocalUnicast() ||
			v4.IsLinkLocalMulticast() ||
			v4.IsMulticast() ||
			v4.IsUnspecified() ||
			v4.Equal(net.IPv4bcast) ||
			(v4[0] == 100 && v4[1] >= 64 && v4[1] <= 127) || // 100.64.0.0/10 CGNAT
			(v4[0] == 192 && v4[1] == 0 && v4[2] == 0) || // 192.0.0.0/24
			(v4[0] == 198 && (v4[1] == 18 || v4[1] == 19)) || // 198.18.0.0/15
			v4[0] >= 240 || // 240.0.0.0/4
			v4[0] == 0 // 0.0.0.0/8
	}
	return ip.IsLoopback() ||
		ip.IsUnspecified() ||
		ip.IsMulticast() ||
		(ip[0]&0xfe) == 0xfc || // fc00::/7 ULA
		(ip[0]&0xff) == 0xfe && (ip[1]&0xc0) == 0x80 // fe80::/10
}
