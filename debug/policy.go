package debug

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"slices"
	"strconv"
	"strings"
)

// Policy is a fail-closed target policy for server-side debug calls.
// A nil Policy allows everything; a non-nil Policy denies a target unless
// every resolved address matches AllowHosts or one of the IP-class flags,
// and the port (when AllowPorts is set) is listed. Resolution failures are
// denials. Adapters must still validate their own targets.
type Policy struct {
	AllowHosts    []string // host[:port] or CIDR
	AllowPrivate  bool
	AllowLoopback bool
	AllowPublic   bool
	AllowPorts    []int
}

// policyDefaultPorts fills in a port when the URL omits one.
var policyDefaultPorts = map[string]int{
	"http": 80, "https": 443,
	"ws": 80, "wss": 443,
	"mqtt": 1883, "mqtts": 8883,
	"amqp": 5672, "amqps": 5671,
	"kafka": 9092, "stomp": 61613,
	"grpc": 443, "grpcs": 443,
	"coap": 5683, "coaps": 5684,
	"lwm2m": 5683, "opcua": 4840, "opc.tcp": 4840,
	"modbus": 502, "dnp3": 20000, "iec104": 2404, "bacnet": 47808,
	"sip": 5060, "sips": 5061,
	"rtsp": 554, "rtsps": 322, "rtmp": 1935,
	"xmpp": 5222, "xmpps": 5223,
	"thrift": 9090, "dubbo": 20880,
	"hls": 80, "sse": 80, "socketio": 80, "mcp": 80, "a2a": 80, "webrtc": 80,
	"hessian": 80, "jsonrpc": 80, "xmlrpc": 80, "soap": 80, "graphql": 80, "odata": 80,
	"webhook": 80, "webhooks": 80,
}

// Check resolves rawURL and validates every resolved address. It returns
// the first denial; a target that does not resolve is denied.
func (p *Policy) Check(ctx context.Context, rawURL string) error {
	if p == nil {
		return nil
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("debug: policy: invalid target %q", redactURL(rawURL))
	}
	host := u.Hostname()
	if host == "" {
		return fmt.Errorf("debug: policy: target %q has no host", redactURL(rawURL))
	}
	port := policyDefaultPorts[strings.ToLower(u.Scheme)]
	if s := u.Port(); s != "" {
		if port, err = strconv.Atoi(s); err != nil || port <= 0 {
			return fmt.Errorf("debug: policy: target %q has an invalid port", redactURL(rawURL))
		}
	}
	if port == 0 && len(p.AllowPorts) > 0 {
		return fmt.Errorf("debug: policy: target %q requires an explicit port for scheme %q", redactURL(rawURL), u.Scheme)
	}
	if len(p.AllowPorts) > 0 && !slices.Contains(p.AllowPorts, port) {
		return fmt.Errorf("debug: policy: target %q denied (port %d)", redactURL(rawURL), port)
	}
	addrs, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return fmt.Errorf("debug: policy: resolve %q: %w", host, err)
	}
	if len(addrs) == 0 {
		return fmt.Errorf("debug: policy: %q resolved to no addresses", host)
	}
	for _, a := range addrs {
		if !p.allows(host, port, a.IP) {
			return fmt.Errorf("debug: policy: target %q denied (%s)", redactURL(rawURL), a.IP)
		}
	}
	return nil
}

// redactURL removes userinfo from a target before it appears in errors.
func redactURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.User == nil {
		return raw
	}
	u.User = nil
	return u.String()
}

func (p *Policy) allows(host string, port int, ip net.IP) bool {
	if p.allowHost(host, port, ip) {
		return true
	}
	switch {
	case ip.IsLoopback():
		return p.AllowLoopback
	case ip.IsPrivate():
		return p.AllowPrivate
	case ip.IsGlobalUnicast():
		return p.AllowPublic
	}
	return false
}

// allowHost matches one resolved IP against AllowHosts entries. A name
// entry allows every resolved address of that name; CIDRs match per IP.
func (p *Policy) allowHost(host string, port int, ip net.IP) bool {
	for _, entry := range p.AllowHosts {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		if strings.Contains(entry, "/") {
			if _, cidr, err := net.ParseCIDR(entry); err == nil && cidr.Contains(ip) {
				return true
			}
			continue
		}
		eh, ep := splitHostEntry(entry)
		if ep != 0 && ep != port {
			continue
		}
		if strings.EqualFold(eh, host) {
			return true
		}
		if pip := net.ParseIP(eh); pip != nil && pip.Equal(ip) {
			return true
		}
	}
	return false
}

// splitHostEntry splits host[:port]; a malformed entry returns an
// unreachable port so it cannot widen the policy.
func splitHostEntry(entry string) (string, int) {
	if h, s, err := net.SplitHostPort(entry); err == nil {
		n, err := strconv.Atoi(s)
		if err != nil || n <= 0 {
			return "", -1
		}
		return h, n
	}
	return strings.Trim(entry, "[]"), 0
}
