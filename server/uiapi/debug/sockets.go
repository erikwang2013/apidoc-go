//go:build apidoc_debug_sockets || apidoc_debug_all

package debug

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"
)

func init() {
	for _, kind := range []string{"tcp", "udp", "raw", "unix"} {
		RegisterExecutor(kind, executeSocket)
		RegisterStream(kind, openSocketStream)
	}
}

// socketTransport maps kind/opts to the actual transport. "raw" defaults to
// TCP but may select UDP via opts.transport.
func socketTransport(req *Request) (string, error) {
	switch req.Kind {
	case "tcp":
		return "tcp", nil
	case "udp":
		return "udp", nil
	case "unix":
		return "unix", nil
	case "raw":
		switch transport := req.Opts["transport"]; transport {
		case "", "tcp":
			return "tcp", nil
		case "udp":
			return "udp", nil
		default:
			return "", fmt.Errorf("unsupported raw socket transport `%s`", transport)
		}
	default:
		return "", fmt.Errorf("unsupported socket kind `%s`", req.Kind)
	}
}

func unixSocketPath(target string) string {
	return strings.TrimPrefix(target, "unix://")
}

// socketAddr parses "host:port" or "scheme://host:port" targets.
func socketAddr(target, transport string) (string, string, error) {
	raw := target
	if !strings.Contains(raw, "://") {
		raw = transport + "://" + raw
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return "", "", fmt.Errorf("invalid target `%s`: %v", target, err)
	}
	host, port := parsed.Hostname(), parsed.Port()
	if host == "" {
		return "", "", fmt.Errorf("target requires a host")
	}
	if port == "" {
		return "", "", fmt.Errorf("target requires a port")
	}
	return host, port, nil
}

// readWindow is the socket read budget: opts.read_ms clamped to the request
// timeout, defaulting to 500ms.
func readWindow(cfg Config, req *Request) time.Duration {
	ms := 500
	if value, err := strconv.Atoi(req.Opts["read_ms"]); err == nil {
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

func executeSocket(cfg Config, req *Request) (*Response, error) {
	started := time.Now()
	transport, err := socketTransport(req)
	if err != nil {
		return nil, err
	}
	if transport == "unix" {
		if !cfg.AllowUnixSockets {
			return nil, fmt.Errorf("unix sockets are disabled; set DebugConfig.allow_unix_sockets = true")
		}
		path := unixSocketPath(req.Target)
		conn, err := dialSocketUnary(cfg, req, "unix", path)
		if err != nil {
			return nil, err
		}
		defer conn.Close()
		body, err := exchangeSocket(cfg, req, conn)
		if err != nil {
			return nil, err
		}
		return socketResponse(started, body, path, 1), nil
	}

	host, port, err := socketAddr(req.Target, transport)
	if err != nil {
		return nil, err
	}
	addrs, err := CheckHost(cfg, host, port)
	if err != nil {
		return nil, err
	}
	if transport == "udp" {
		conn, err := dialSocketUnary(cfg, req, "udp", net.JoinHostPort(addrs[0].String(), port))
		if err != nil {
			return nil, err
		}
		defer conn.Close()
		body, count, err := udpExchange(cfg, req, conn)
		if err != nil {
			return nil, err
		}
		return socketResponse(started, body, conn.RemoteAddr().String(), count), nil
	}

	conn, err := dialSocketUnary(cfg, req, "tcp", net.JoinHostPort(addrs[0].String(), port))
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	body, err := exchangeSocket(cfg, req, conn)
	if err != nil {
		return nil, err
	}
	return socketResponse(started, body, conn.RemoteAddr().String(), 1), nil
}

func dialSocketUnary(cfg Config, req *Request, network, address string) (net.Conn, error) {
	ctx, cancel := context.WithTimeout(context.Background(), req.Timeout(cfg))
	defer cancel()
	conn, err := (&net.Dialer{}).DialContext(ctx, network, address)
	if err != nil {
		if ctx.Err() != nil {
			return nil, fmt.Errorf("%s connect timed out", network)
		}
		return nil, fmt.Errorf("%s connect failed: %v", network, err)
	}
	return conn, nil
}

// exchangeSocket writes the request body, then reads until the window closes,
// the peer closes, or expect_bytes is satisfied.
func exchangeSocket(cfg Config, req *Request, conn net.Conn) ([]byte, error) {
	payload, err := req.Body.Decode()
	if err != nil {
		return nil, err
	}
	if len(payload) > cfg.Limit() {
		return nil, fmt.Errorf("request body exceeds max_bytes (%d)", cfg.Limit())
	}
	if len(payload) > 0 {
		if _, err := conn.Write(payload); err != nil {
			return nil, fmt.Errorf("socket write failed: %v", err)
		}
	}
	expect := 0
	if value, err := strconv.Atoi(req.Opts["expect_bytes"]); err == nil && value > 0 {
		expect = value
	}
	deadline := time.Now().Add(readWindow(cfg, req))
	if err := conn.SetReadDeadline(deadline); err != nil {
		return nil, fmt.Errorf("socket deadline failed: %v", err)
	}
	out := []byte{}
	buf := make([]byte, 4096)
	for {
		n, err := conn.Read(buf)
		if n > 0 {
			out = append(out, buf[:n]...)
			if len(out) > cfg.Limit() {
				return nil, fmt.Errorf("response exceeds max_bytes (%d)", cfg.Limit())
			}
			if expect > 0 && len(out) >= expect {
				return out, nil
			}
		}
		if err != nil {
			if ne, ok := err.(net.Error); ok && ne.Timeout() {
				return out, nil
			}
			if err == io.EOF {
				return out, nil
			}
			return nil, fmt.Errorf("socket read failed: %v", err)
		}
	}
}

func udpExchange(cfg Config, req *Request, conn net.Conn) ([]byte, int, error) {
	payload, err := req.Body.Decode()
	if err != nil {
		return nil, 0, err
	}
	if len(payload) > cfg.Limit() {
		return nil, 0, fmt.Errorf("request body exceeds max_bytes (%d)", cfg.Limit())
	}
	if len(payload) > 0 {
		if _, err := conn.Write(payload); err != nil {
			return nil, 0, fmt.Errorf("udp send failed: %v", err)
		}
	}
	count := 1
	if value, err := strconv.Atoi(req.Opts["datagrams"]); err == nil {
		count = value
	}
	if count < 1 {
		count = 1
	}
	if count > 100 {
		count = 100
	}
	deadline := time.Now().Add(readWindow(cfg, req))
	if err := conn.SetReadDeadline(deadline); err != nil {
		return nil, 0, fmt.Errorf("udp deadline failed: %v", err)
	}
	datagrams := [][]byte{}
	total := 0
	for i := 0; i < count; i++ {
		buf := make([]byte, 65535)
		n, err := conn.Read(buf)
		if err != nil {
			if ne, ok := err.(net.Error); ok && ne.Timeout() {
				break
			}
			return nil, 0, fmt.Errorf("udp recv failed: %v", err)
		}
		total += n
		if total > cfg.Limit() {
			return nil, 0, fmt.Errorf("response exceeds max_bytes (%d)", cfg.Limit())
		}
		datagrams = append(datagrams, buf[:n])
	}
	if len(datagrams) <= 1 {
		if len(datagrams) == 0 {
			return []byte{}, 1, nil
		}
		return datagrams[0], 1, nil
	}
	items := make([]map[string]string, 0, len(datagrams))
	for _, datagram := range datagrams {
		items = append(items, map[string]string{
			"encoding": "base64",
			"data":     base64.StdEncoding.EncodeToString(datagram),
		})
	}
	body, err := json.Marshal(map[string]any{"datagrams": items})
	if err != nil {
		return nil, 0, fmt.Errorf("udp encode failed: %v", err)
	}
	return body, len(datagrams), nil
}

func socketResponse(started time.Time, body []byte, peer string, datagrams int) *Response {
	return &Response{
		Status: 0,
		MS:     time.Since(started).Milliseconds(),
		Headers: map[string]string{
			"x-peer":      peer,
			"x-datagrams": strconv.Itoa(datagrams),
		},
		Body: FromBytes(body),
	}
}

func openSocketStream(cfg Config, req *Request) (StreamSession, error) {
	transport, err := socketTransport(req)
	if err != nil {
		return nil, err
	}
	network, address := transport, ""
	if transport == "unix" {
		if !cfg.AllowUnixSockets {
			return nil, fmt.Errorf("unix sockets are disabled; set DebugConfig.allow_unix_sockets = true")
		}
		address = unixSocketPath(req.Target)
	} else {
		host, port, err := socketAddr(req.Target, transport)
		if err != nil {
			return nil, err
		}
		addrs, err := CheckHost(cfg, host, port)
		if err != nil {
			return nil, err
		}
		address = net.JoinHostPort(addrs[0].String(), port)
	}
	conn, err := dialSocketUnary(cfg, req, network, address)
	if err != nil {
		return nil, err
	}
	payload, err := req.Body.Decode()
	if err != nil {
		_ = conn.Close()
		return nil, err
	}
	if len(payload) > cfg.Limit() {
		_ = conn.Close()
		return nil, fmt.Errorf("request body exceeds max_bytes (%d)", cfg.Limit())
	}
	if len(payload) > 0 {
		if _, err := conn.Write(payload); err != nil {
			_ = conn.Close()
			return nil, fmt.Errorf("socket write failed: %v", err)
		}
	}
	return &socketStream{conn: conn, window: readWindow(cfg, req), udp: transport == "udp"}, nil
}

// socketStream is a long-lived TCP/unix or UDP session. ReadFrame waits for
// one chunk/datagram and returns io.EOF when the peer closes.
type socketStream struct {
	conn   net.Conn
	window time.Duration
	udp    bool
}

func (s *socketStream) ReadFrame() ([]byte, error) {
	buf := make([]byte, 32768)
	if s.udp {
		buf = make([]byte, 65535)
	}
	for {
		if err := s.conn.SetReadDeadline(time.Now().Add(s.window)); err != nil {
			return nil, err
		}
		n, err := s.conn.Read(buf)
		if n > 0 {
			return append([]byte{}, buf[:n]...), nil
		}
		if err != nil {
			if ne, ok := err.(net.Error); ok && ne.Timeout() {
				continue
			}
			return nil, err
		}
	}
}

func (s *socketStream) Close() error { return s.conn.Close() }
