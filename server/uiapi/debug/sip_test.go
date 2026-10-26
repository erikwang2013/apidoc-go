//go:build apidoc_debug_media || apidoc_debug_all

package debug

import (
	"fmt"
	"net"
	"strings"
	"syscall"
	"testing"
	"time"
)

const (
	sipTestCallID = "apidoc-test-call@example.com"
	sipTestBranch = "z9hG4bKapidoc"
)

func sipTestRequest(target string, opts map[string]string) *Request {
	if opts == nil {
		opts = map[string]string{}
	}
	opts["branch"] = sipTestBranch
	opts["call_id"] = sipTestCallID
	return &Request{Kind: "sip", Target: "sip://" + target, Opts: opts, TimeoutMS: 2000}
}

func sipTestReply(request, statusLine, extra string) string {
	return fmt.Sprintf("%s\r\nVia: %s\r\nFrom: %s\r\nTo: %s\r\nCall-ID: %s\r\nCSeq: %s\r\n%sContent-Length: 0\r\n\r\n",
		statusLine,
		mediaRequestHeader(request, "Via"),
		mediaRequestHeader(request, "From"),
		mediaRequestHeader(request, "To"),
		mediaRequestHeader(request, "Call-ID"),
		mediaRequestHeader(request, "CSeq"),
		extra)
}

func TestSIPUDPOptionsMatchesCallIDAndCSeq(t *testing.T) {
	address := mediaUDPOnce(t, func(conn *net.UDPConn, peer *net.UDPAddr, raw []byte) {
		request := string(raw)
		if !strings.HasPrefix(request, "OPTIONS sip:") ||
			!strings.Contains(request, "Call-ID: "+sipTestCallID) ||
			!strings.Contains(request, "CSeq: 5 OPTIONS") ||
			!strings.Contains(request, "branch="+sipTestBranch) {
			t.Errorf("request = %q", request)
		}
		conn.WriteToUDP([]byte(sipTestReply(request, "SIP/2.0 200 OK", "")), peer)
	})
	response, err := executeSIP(mediaTestConfig(), sipTestRequest(address,
		map[string]string{"method": "OPTIONS", "transport": "udp", "cseq": "5"}))
	if err != nil {
		t.Fatal(err)
	}
	if response.Status != 200 || response.Headers["Call-ID"] != sipTestCallID {
		t.Fatalf("response = %+v", response)
	}
	body := mediaReadJSON(t, response)
	if body["status"] != float64(200) || body["reason"] != "OK" {
		t.Fatalf("body = %+v", body)
	}
}

func TestSIPSkipsProvisionalUntilFinal(t *testing.T) {
	address := mediaUDPOnce(t, func(conn *net.UDPConn, peer *net.UDPAddr, raw []byte) {
		request := string(raw)
		conn.WriteToUDP([]byte(sipTestReply(request, "SIP/2.0 100 Trying", "")), peer)
		conn.WriteToUDP([]byte(sipTestReply(request, "SIP/2.0 200 OK", "")), peer)
	})
	response, err := executeSIP(mediaTestConfig(), sipTestRequest(address,
		map[string]string{"method": "INVITE", "transport": "udp", "cseq": "1"}))
	if err != nil {
		t.Fatal(err)
	}
	if response.Status != 200 || mediaReadJSON(t, response)["reason"] != "OK" {
		t.Fatalf("response = %+v", response)
	}
}

func TestSIPUnauthorizedFailsClosed(t *testing.T) {
	address := mediaUDPOnce(t, func(conn *net.UDPConn, peer *net.UDPAddr, raw []byte) {
		request := string(raw)
		conn.WriteToUDP([]byte(sipTestReply(request, "SIP/2.0 401 Unauthorized",
			"WWW-Authenticate: Digest realm=\"x\", nonce=\"y\"\r\n")), peer)
	})
	_, err := executeSIP(mediaTestConfig(), sipTestRequest(address,
		map[string]string{"method": "REGISTER", "transport": "udp", "cseq": "2"}))
	if err == nil || !strings.Contains(err.Error(), "401") || !strings.Contains(err.Error(), "authentication") {
		t.Fatalf("401: %v", err)
	}
}

func TestSIPTCPFramedResponse(t *testing.T) {
	address := mediaTCPOnce(t, func(conn net.Conn) {
		request := mediaReadHeaders(t, conn)
		if !strings.Contains(request, "Via: SIP/2.0/TCP") {
			t.Errorf("request = %q", request)
		}
		fmt.Fprint(conn, sipTestReply(request, "SIP/2.0 200 OK", ""))
	})
	response, err := executeSIP(mediaTestConfig(), sipTestRequest(address,
		map[string]string{"method": "OPTIONS", "transport": "tcp", "cseq": "3"}))
	if err != nil {
		t.Fatal(err)
	}
	if response.Status != 200 {
		t.Fatalf("response = %+v", response)
	}
}

func TestSIPMismatchedCallIDTimesOut(t *testing.T) {
	address := mediaUDPOnce(t, func(conn *net.UDPConn, peer *net.UDPAddr, raw []byte) {
		request := string(raw)
		reply := strings.ReplaceAll(sipTestReply(request, "SIP/2.0 200 OK", ""),
			sipTestCallID, "someone-else@example.com")
		conn.WriteToUDP([]byte(reply), peer)
	})
	started := time.Now()
	_, err := executeSIP(Config{AllowPrivateNetworks: true, TimeoutMS: 300}, &Request{
		Kind: "sip", Target: "sip://" + address, TimeoutMS: 300,
		Opts: map[string]string{
			"method": "OPTIONS", "transport": "udp", "cseq": "4",
			"branch": sipTestBranch, "call_id": sipTestCallID,
		},
	})
	if err == nil || !strings.Contains(err.Error(), "timed out") || time.Since(started) > 5*time.Second {
		t.Fatalf("mismatched call-id: %v after %s", err, time.Since(started))
	}
}

func TestSIPMalformedContentLengthDoesNotPanic(t *testing.T) {
	withLength := func(reply, length string) string {
		return strings.Replace(reply, "Content-Length: 0", "Content-Length: "+length, 1)
	}
	cases := []struct {
		name   string
		length string
		want   string
	}{
		{"negative", "-5", "invalid SIP Content-Length"},
		{"max_int", "9223372036854775807", "exceeds max_bytes"},
	}
	for _, tc := range cases {
		t.Run("tcp_"+tc.name, func(t *testing.T) {
			address := mediaTCPOnce(t, func(conn net.Conn) {
				request := mediaReadHeaders(t, conn)
				fmt.Fprint(conn, withLength(sipTestReply(request, "SIP/2.0 200 OK", ""), tc.length))
			})
			_, err := executeSIP(mediaTestConfig(), sipTestRequest(address,
				map[string]string{"method": "OPTIONS", "transport": "tcp", "cseq": "3"}))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("tcp %s: %v", tc.name, err)
			}
		})

		t.Run("udp_"+tc.name, func(t *testing.T) {
			address := mediaUDPOnce(t, func(conn *net.UDPConn, peer *net.UDPAddr, raw []byte) {
				conn.WriteToUDP([]byte(withLength(sipTestReply(string(raw), "SIP/2.0 200 OK", ""), tc.length)), peer)
			})
			started := time.Now()
			_, err := executeSIP(Config{AllowPrivateNetworks: true, TimeoutMS: 200}, &Request{
				Kind: "sip", Target: "sip://" + address, TimeoutMS: 200,
				Opts: map[string]string{
					"method": "OPTIONS", "transport": "udp", "cseq": "3",
					"branch": sipTestBranch, "call_id": sipTestCallID,
				},
			})
			if err == nil || !strings.Contains(err.Error(), "timed out") || time.Since(started) > 5*time.Second {
				t.Fatalf("udp %s: %v after %s", tc.name, err, time.Since(started))
			}
		})
	}
}

func TestSIPContentLengthBeyondBodyRejected(t *testing.T) {
	raw := []byte("SIP/2.0 200 OK\r\nCall-ID: x\r\nCSeq: 1 OPTIONS\r\nContent-Length: 10\r\n\r\nshort")
	if _, err := parseSIPResponse(raw); err == nil || !strings.Contains(err.Error(), "exceeds received body") {
		t.Fatalf("short body: %v", err)
	}
}

func TestSIPIPv6HeadersAreBracketed(t *testing.T) {
	request, _, _, err := buildSIPRequest(&Request{Opts: map[string]string{}}, "OPTIONS", "::1",
		&net.UDPAddr{IP: net.ParseIP("::1"), Port: 12345}, "tcp", mediaTestConfig())
	if err != nil {
		t.Fatal(err)
	}
	text := string(request)
	for _, want := range []string{
		"OPTIONS sip:[::1] SIP/2.0\r\n",
		"Via: SIP/2.0/TCP [::1]:12345;branch=",
		"From: <sip:apidoc@[::1]>;tag=",
		"Contact: <sip:apidoc@[::1]:12345;transport=tcp>",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("request missing %q:\n%s", want, text)
		}
	}
}

type sipStubConn struct {
	readData []byte
	readErr  error
}

func (c *sipStubConn) Read(buf []byte) (int, error) {
	if c.readErr != nil {
		return 0, c.readErr
	}
	return copy(buf, c.readData), nil
}

func (c *sipStubConn) Write(p []byte) (int, error) { return len(p), nil }
func (c *sipStubConn) Close() error                { return nil }
func (c *sipStubConn) LocalAddr() net.Addr {
	return &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)}
}
func (c *sipStubConn) RemoteAddr() net.Addr {
	return &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)}
}
func (c *sipStubConn) SetDeadline(time.Time) error      { return nil }
func (c *sipStubConn) SetReadDeadline(time.Time) error  { return nil }
func (c *sipStubConn) SetWriteDeadline(time.Time) error { return nil }

func TestSIPUDPRecvErrorFidelity(t *testing.T) {
	request := []byte("OPTIONS sip:x SIP/2.0\r\n\r\n")
	_, err := sipUDPExchange(Config{}, &sipStubConn{readErr: syscall.ENETUNREACH},
		request, "call", "OPTIONS", 1, time.Now().Add(time.Second))
	if err == nil || !strings.Contains(err.Error(), "SIP udp recv failed: network is unreachable") {
		t.Fatalf("non-refusal recv error: %v", err)
	}

	started := time.Now()
	_, err = sipUDPExchange(Config{}, &sipStubConn{readErr: syscall.ECONNREFUSED},
		request, "call", "OPTIONS", 1, started.Add(50*time.Millisecond))
	if err == nil || !strings.Contains(err.Error(), "timed out") || time.Since(started) > 5*time.Second {
		t.Fatalf("connection refused must retry then time out: %v after %s", err, time.Since(started))
	}
}

func TestSIPUDPOversizeDatagramRejected(t *testing.T) {
	_, err := sipUDPExchange(Config{MaxBytes: 64}, &sipStubConn{readData: make([]byte, 100)},
		[]byte("OPTIONS sip:x SIP/2.0\r\n\r\n"), "call", "OPTIONS", 1, time.Now().Add(time.Second))
	if err == nil || !strings.Contains(err.Error(), "exceeds max_bytes (64)") {
		t.Fatalf("oversize datagram: %v", err)
	}
}

func TestSIPRejectsCRLFAndSSRF(t *testing.T) {
	_, err := executeSIP(mediaTestConfig(), &Request{
		Kind: "sip", Target: "sip://127.0.0.1:1", TimeoutMS: 1000,
		Opts: map[string]string{"method": "OPTIONS", "call_id": "a\r\nInjected: 1"},
	})
	if err == nil || !strings.Contains(err.Error(), "must not contain CR/LF") {
		t.Fatalf("crlf injection: %v", err)
	}

	_, err = Execute(Config{}, &Request{
		Kind: "sip", Target: "sip://127.0.0.1:5060", TimeoutMS: 1000,
		Opts: map[string]string{"method": "OPTIONS"},
	})
	if err == nil || !strings.Contains(err.Error(), "private/loopback") {
		t.Fatalf("sip loopback must be denied: %v", err)
	}
}
