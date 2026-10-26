//go:build apidoc_debug_media || apidoc_debug_all

package debug

import (
	"fmt"
	"io"
	"net"
	"strings"
	"testing"
	"time"
)

func TestRTSPOptionsRoundtrip(t *testing.T) {
	address := mediaTCPOnce(t, func(conn net.Conn) {
		request := mediaReadHeaders(t, conn)
		if !strings.HasPrefix(request, "OPTIONS rtsp://") {
			t.Errorf("request = %q", request)
		}
		if !strings.Contains(request, "User-Agent: apidoc-go") {
			t.Errorf("missing user agent: %q", request)
		}
		fmt.Fprintf(conn, "RTSP/1.0 200 OK\r\nCSeq: %s\r\nPublic: OPTIONS, DESCRIBE\r\n\r\n",
			mediaRequestHeader(request, "CSeq"))
	})
	response, err := executeRTSP(mediaTestConfig(), &Request{
		Kind: "rtsp", Target: "rtsp://" + address + "/stream",
		Opts: map[string]string{"method": "OPTIONS"}, TimeoutMS: 2000,
	})
	if err != nil {
		t.Fatal(err)
	}
	if response.Status != 200 || !strings.Contains(response.Headers["Public"], "DESCRIBE") ||
		response.Body.Data != "" || response.Headers["x-reason"] != "OK" {
		t.Fatalf("response = %+v", response)
	}
}

func TestRTSPDescribeReturnsSDP(t *testing.T) {
	sdp := "v=0\r\no=- 1 1 IN IP4 127.0.0.1\r\ns=test\r\nm=video 0 RTP/AVP 96\r\n"
	address := mediaTCPOnce(t, func(conn net.Conn) {
		request := mediaReadHeaders(t, conn)
		if !strings.HasPrefix(request, "DESCRIBE rtsp://") || !strings.Contains(request, "CSeq: 7") {
			t.Errorf("request = %q", request)
		}
		fmt.Fprintf(conn, "RTSP/1.0 200 OK\r\nCSeq: %s\r\nContent-Type: application/sdp\r\nContent-Length: %d\r\n\r\n%s",
			mediaRequestHeader(request, "CSeq"), len(sdp), sdp)
	})
	response, err := executeRTSP(mediaTestConfig(), &Request{
		Kind: "rtsp", Target: "rtsp://" + address + "/stream",
		Opts: map[string]string{"method": "DESCRIBE", "cseq": "7"}, TimeoutMS: 2000,
	})
	if err != nil {
		t.Fatal(err)
	}
	if response.Body.Data != sdp || response.Headers["Content-Type"] != "application/sdp" {
		t.Fatalf("response = %+v", response)
	}
}

func TestRTSPSetupAndPlayHeaders(t *testing.T) {
	address := mediaTCPOnce(t, func(conn net.Conn) {
		request := mediaReadHeaders(t, conn)
		if !strings.Contains(request, "Transport: RTP/AVP/TCP;interleaved=0-1") {
			t.Errorf("setup transport: %q", request)
		}
		fmt.Fprintf(conn, "RTSP/1.0 200 OK\r\nCSeq: %s\r\n\r\n", mediaRequestHeader(request, "CSeq"))
	})
	if _, err := executeRTSP(mediaTestConfig(), &Request{
		Kind: "rtsp", Target: "rtsp://" + address + "/stream",
		Opts: map[string]string{"method": "SETUP"}, TimeoutMS: 2000,
	}); err != nil {
		t.Fatal(err)
	}

	address = mediaTCPOnce(t, func(conn net.Conn) {
		request := mediaReadHeaders(t, conn)
		if !strings.Contains(request, "Session: 12345678") {
			t.Errorf("play session: %q", request)
		}
		fmt.Fprintf(conn, "RTSP/1.0 200 OK\r\nCSeq: %s\r\nSession: 12345678\r\n\r\n",
			mediaRequestHeader(request, "CSeq"))
	})
	response, err := executeRTSP(mediaTestConfig(), &Request{
		Kind: "rtsp", Target: "rtsp://" + address + "/stream",
		Opts: map[string]string{"method": "PLAY", "session": "12345678"}, TimeoutMS: 2000,
	})
	if err != nil {
		t.Fatal(err)
	}
	if response.Headers["Session"] != "12345678" {
		t.Fatalf("response = %+v", response)
	}
}

func TestRTSPAuthFailsClosed(t *testing.T) {
	cases := []struct {
		status  string
		header  string
		scheme  string
		request string
	}{
		{"401 Unauthorized", "WWW-Authenticate: Digest realm=\"x\", nonce=\"y\"", "Digest", "401"},
		{"407 Proxy Authentication Required", "Proxy-Authenticate: Basic realm=\"x\"", "Basic", "407"},
	}
	for _, tc := range cases {
		address := mediaTCPOnce(t, func(conn net.Conn) {
			request := mediaReadHeaders(t, conn)
			fmt.Fprintf(conn, "RTSP/1.0 %s\r\nCSeq: %s\r\n%s\r\nContent-Length: 0\r\n\r\n",
				tc.status, mediaRequestHeader(request, "CSeq"), tc.header)
		})
		_, err := executeRTSP(mediaTestConfig(), &Request{
			Kind: "rtsp", Target: "rtsp://" + address + "/stream",
			Opts: map[string]string{"method": "DESCRIBE"}, TimeoutMS: 2000,
		})
		if err == nil || !strings.Contains(err.Error(), tc.request) ||
			!strings.Contains(err.Error(), tc.scheme+" authentication is required") {
			t.Fatalf("%s: %v", tc.status, err)
		}
	}
}

func TestRTSPRejectsMismatchedCSeq(t *testing.T) {
	address := mediaTCPOnce(t, func(conn net.Conn) {
		mediaReadHeaders(t, conn)
		fmt.Fprint(conn, "RTSP/1.0 200 OK\r\nCSeq: 99\r\n\r\n")
	})
	_, err := executeRTSP(mediaTestConfig(), &Request{
		Kind: "rtsp", Target: "rtsp://" + address + "/stream",
		Opts: map[string]string{"method": "OPTIONS"}, TimeoutMS: 2000,
	})
	if err == nil || !strings.Contains(err.Error(), "CSeq") {
		t.Fatalf("mismatched cseq: %v", err)
	}
}

func TestRTSPTimeoutAndHeaderInjection(t *testing.T) {
	address := mediaTCPOnce(t, func(conn net.Conn) {
		mediaReadHeaders(t, conn)
		_, _ = io.Copy(io.Discard, conn) // hold the connection until the client gives up
	})
	started := time.Now()
	_, err := executeRTSP(Config{AllowPrivateNetworks: true, TimeoutMS: 200}, &Request{
		Kind: "rtsp", Target: "rtsp://" + address + "/stream",
		Opts: map[string]string{"method": "OPTIONS"}, TimeoutMS: 200,
	})
	if err == nil || !strings.Contains(err.Error(), "timed out") || time.Since(started) > 5*time.Second {
		t.Fatalf("timeout: %v after %s", err, time.Since(started))
	}

	_, err = executeRTSP(mediaTestConfig(), &Request{
		Kind: "rtsp", Target: "rtsp://127.0.0.1:554/stream",
		Opts:    map[string]string{"method": "OPTIONS"},
		Headers: map[string]string{"X-Test": "a\r\nInjected: 1"}, TimeoutMS: 1000,
	})
	if err == nil || !strings.Contains(err.Error(), "must not contain CR/LF") {
		t.Fatalf("header injection: %v", err)
	}
}

func TestRTSPMalformedContentLengthDoesNotPanic(t *testing.T) {
	t.Run("negative", func(t *testing.T) {
		address := mediaTCPOnce(t, func(conn net.Conn) {
			request := mediaReadHeaders(t, conn)
			fmt.Fprintf(conn, "RTSP/1.0 200 OK\r\nCSeq: %s\r\nContent-Length: -5\r\n\r\n",
				mediaRequestHeader(request, "CSeq"))
		})
		response, err := executeRTSP(mediaTestConfig(), &Request{
			Kind: "rtsp", Target: "rtsp://" + address + "/stream",
			Opts: map[string]string{"method": "OPTIONS"}, TimeoutMS: 2000,
		})
		if err != nil {
			t.Fatal(err)
		}
		if response.Body.Data != "" {
			t.Fatalf("body = %q", response.Body.Data)
		}
	})

	t.Run("max_int", func(t *testing.T) {
		address := mediaTCPOnce(t, func(conn net.Conn) {
			request := mediaReadHeaders(t, conn)
			fmt.Fprintf(conn, "RTSP/1.0 200 OK\r\nCSeq: %s\r\nContent-Length: 9223372036854775807\r\n\r\n",
				mediaRequestHeader(request, "CSeq"))
		})
		_, err := executeRTSP(mediaTestConfig(), &Request{
			Kind: "rtsp", Target: "rtsp://" + address + "/stream",
			Opts: map[string]string{"method": "OPTIONS"}, TimeoutMS: 2000,
		})
		if err == nil || !strings.Contains(err.Error(), "exceeds max_bytes") {
			t.Fatalf("max-int Content-Length: %v", err)
		}
	})
}

func TestRTSPResponseCapAndSSRF(t *testing.T) {
	address := mediaTCPOnce(t, func(conn net.Conn) {
		request := mediaReadHeaders(t, conn)
		fmt.Fprintf(conn, "RTSP/1.0 200 OK\r\nCSeq: %s\r\nContent-Length: 100\r\n\r\n",
			mediaRequestHeader(request, "CSeq"))
	})
	_, err := executeRTSP(Config{AllowPrivateNetworks: true, TimeoutMS: 1000, MaxBytes: 64},
		&Request{Kind: "rtsp", Target: "rtsp://" + address + "/stream", TimeoutMS: 1000})
	if err == nil || !strings.Contains(err.Error(), "exceeds max_bytes (64)") {
		t.Fatalf("response cap: %v", err)
	}

	_, err = Execute(Config{}, &Request{
		Kind: "rtsp", Target: "rtsp://127.0.0.1:554/stream", TimeoutMS: 1000,
	})
	if err == nil || !strings.Contains(err.Error(), "private/loopback") {
		t.Fatalf("rtsp loopback must be denied: %v", err)
	}
}
