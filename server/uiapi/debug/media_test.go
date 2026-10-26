//go:build apidoc_debug_media || apidoc_debug_all

package debug

import (
	"encoding/json"
	"io"
	"net"
	"strings"
	"testing"
)

// mediaTestConfig allows loopback; every media test endpoint binds an
// ephemeral port, so no fixed port is required.
func mediaTestConfig() Config {
	return Config{AllowPrivateNetworks: true, TimeoutMS: 2000}
}

// mediaTCPOnce accepts one connection and hands it to handler.
func mediaTCPOnce(t *testing.T, handler func(net.Conn)) string {
	t.Helper()
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { listener.Close() })
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		handler(conn)
	}()
	return listener.Addr().String()
}

// mediaUDPOnce receives one datagram and hands it to handler.
func mediaUDPOnce(t *testing.T, handler func(*net.UDPConn, *net.UDPAddr, []byte)) string {
	t.Helper()
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	go func() {
		buf := make([]byte, 65535)
		n, peer, err := conn.ReadFromUDP(buf)
		if err != nil {
			return
		}
		handler(conn, peer, append([]byte{}, buf[:n]...))
	}()
	return conn.LocalAddr().String()
}

func mediaReadJSON(t *testing.T, response *Response) map[string]any {
	t.Helper()
	var body map[string]any
	if err := json.Unmarshal([]byte(response.Body.Data), &body); err != nil {
		t.Fatalf("body %q: %v", response.Body.Data, err)
	}
	return body
}

// mediaReadHeaders reads through the blank line and returns the header block.
func mediaReadHeaders(t *testing.T, conn net.Conn) string {
	t.Helper()
	buf := []byte{}
	chunk := make([]byte, 4096)
	for {
		n, err := conn.Read(chunk)
		if n > 0 {
			buf = append(buf, chunk[:n]...)
			if index := indexHeaderEnd(buf); index >= 0 {
				return string(buf[:index])
			}
		}
		if err != nil {
			if err != io.EOF {
				t.Errorf("read request: %v", err)
			}
			return string(buf)
		}
	}
}

func indexHeaderEnd(buf []byte) int {
	for i := 0; i+3 < len(buf); i++ {
		if buf[i] == '\r' && buf[i+1] == '\n' && buf[i+2] == '\r' && buf[i+3] == '\n' {
			return i
		}
	}
	return -1
}

// mediaRequestHeader returns a request header value by case-insensitive name.
func mediaRequestHeader(request, name string) string {
	for _, line := range strings.Split(request, "\r\n") {
		key, value, found := strings.Cut(line, ":")
		if found && strings.EqualFold(strings.TrimSpace(key), name) {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
