//go:build apidoc_debug_media || apidoc_debug_all

package debug

import (
	"bytes"
	"io"
	"net"
	"strings"
	"testing"
	"time"
)

func TestAMF0GoldenVectors(t *testing.T) {
	var out []byte
	if err := writeAMF0(&out, amf0StringValue("connect")); err != nil {
		t.Fatal(err)
	}
	want := []byte{0x02, 0x00, 0x07, 'c', 'o', 'n', 'n', 'e', 'c', 't'}
	if !bytes.Equal(out, want) {
		t.Fatalf("string = % x, want % x", out, want)
	}

	out = nil
	if err := writeAMF0(&out, amf0NumberValue(1)); err != nil {
		t.Fatal(err)
	}
	if want := []byte{0x00, 0x3f, 0xf0, 0, 0, 0, 0, 0, 0}; !bytes.Equal(out, want) {
		t.Fatalf("number = % x, want % x", out, want)
	}

	out = nil
	if err := writeAMF0(&out, amf0ObjectValue(map[string]amf0Value{"app": amf0StringValue("live")})); err != nil {
		t.Fatal(err)
	}
	want = []byte{0x03, 0x00, 0x03, 'a', 'p', 'p', 0x02, 0x00, 0x04, 'l', 'i', 'v', 'e', 0x00, 0x00, 0x09}
	if !bytes.Equal(out, want) {
		t.Fatalf("object = % x, want % x", out, want)
	}
	values, err := decodeAMF0All(out)
	if err != nil || len(values) != 1 || values[0].kind != amf0Object {
		t.Fatalf("decoded = %+v, %v", values, err)
	}
	if app := values[0].fields["app"]; app.kind != amf0String || app.text != "live" {
		t.Fatalf("app = %+v", app)
	}

	if _, err := decodeAMF0All([]byte{0x99}); err == nil {
		t.Fatal("unsupported marker must fail")
	}
	position := 0
	if _, err := decodeAMF0([]byte{amf0Object}, &position, amf0MaxDepth); err == nil ||
		!strings.Contains(err.Error(), "nesting exceeds") {
		t.Fatalf("depth cap: %v", err)
	}
}

func TestRTMPChunkWriterSplitsPayload(t *testing.T) {
	payload := bytes.Repeat([]byte{0xab}, 130)
	var out []byte
	if err := writeRTMPMessage(&out, 3, 20, 1, payload, 128); err != nil {
		t.Fatal(err)
	}
	if len(out) != 12+128+1+2 {
		t.Fatalf("length = %d", len(out))
	}
	if out[12] != 0xab || out[12+128] != 0xc3 ||
		!bytes.Equal(out[12+128+1:], []byte{0xab, 0xab}) {
		t.Fatalf("chunks = % x", out[12:])
	}
	reader := newRTMPChunkReader(1 << 20)
	typeID, streamID, decoded, err := reader.readMessage(bytes.NewReader(out))
	if err != nil || typeID != 20 || streamID != 1 || !bytes.Equal(decoded, payload) {
		t.Fatalf("roundtrip = %d %d % x, %v", typeID, streamID, decoded, err)
	}

	truncated := []byte{3, 0, 0, 0, 0, 0, 100, 20, 1, 0, 0, 0}
	if _, _, _, err := newRTMPChunkReader(10).readChunkMessage(bytes.NewReader(truncated)); err == nil ||
		!strings.Contains(err.Error(), "exceeds max_bytes (10)") {
		t.Fatalf("chunk cap: %v", err)
	}
}

func rtmpTestHandshake(t *testing.T, conn net.Conn) {
	t.Helper()
	c0 := make([]byte, 1)
	if _, err := io.ReadFull(conn, c0); err != nil || c0[0] != rtmpVersion {
		t.Errorf("C0 = % x, %v", c0, err)
		return
	}
	c1 := make([]byte, rtmpHandshakeSize)
	if _, err := io.ReadFull(conn, c1); err != nil || !bytes.Equal(c1[:8], make([]byte, 8)) {
		t.Errorf("C1 time/zero = % x, %v", c1[:8], err)
		return
	}
	s1 := bytes.Repeat([]byte{0x5a}, rtmpHandshakeSize)
	out := append([]byte{rtmpVersion}, s1...)
	out = append(out, make([]byte, rtmpHandshakeSize)...)
	if _, err := conn.Write(out); err != nil {
		t.Errorf("handshake write: %v", err)
		return
	}
	c2 := make([]byte, rtmpHandshakeSize)
	if _, err := io.ReadFull(conn, c2); err != nil || !bytes.Equal(c2, s1) {
		t.Errorf("C2 must echo S1: %v", err)
	}
}

func rtmpTestSend(t *testing.T, conn net.Conn, csid uint8, streamID uint32, values ...amf0Value) {
	t.Helper()
	if err := sendRTMPCommand(conn, csid, streamID, values); err != nil {
		t.Errorf("send command: %v", err)
	}
}

func rtmpTestConnect(t *testing.T, conn net.Conn, reader *rtmpChunkReader) []amf0Value {
	t.Helper()
	values, err := readRTMPCommand(conn, reader)
	if err != nil {
		t.Errorf("read connect: %v", err)
		return nil
	}
	if rtmpCommandName(values) != "connect" {
		t.Errorf("first command = %+v", values)
	}
	rtmpTestSend(t, conn, 3, 0, amf0StringValue("_result"), amf0NumberValue(1), amf0NullValue,
		amf0ObjectValue(map[string]amf0Value{"code": amf0StringValue("NetConnection.Connect.Success")}))
	return values
}

func TestRTMPHandshakeAndConnect(t *testing.T) {
	address := mediaTCPOnce(t, func(conn net.Conn) {
		rtmpTestHandshake(t, conn)
		rtmpTestConnect(t, conn, newRTMPChunkReader(1<<20))
	})
	response, err := executeRTMP(mediaTestConfig(), &Request{
		Kind: "rtmp", Target: "rtmp://" + address + "/live",
		Opts: map[string]string{"app": "live"}, TimeoutMS: 2000,
	})
	if err != nil {
		t.Fatal(err)
	}
	body := mediaReadJSON(t, response)
	if body["connected"] != true || body["operation"] != "connect" || body["stream"] != nil {
		t.Fatalf("body = %+v", body)
	}
	if !strings.Contains(response.Headers["x-peer"], "127.0.0.1") {
		t.Fatalf("headers = %+v", response.Headers)
	}
}

func TestRTMPPlayCreatesStreamAndReadsOnStatus(t *testing.T) {
	address := mediaTCPOnce(t, func(conn net.Conn) {
		rtmpTestHandshake(t, conn)
		reader := newRTMPChunkReader(1 << 20)
		rtmpTestConnect(t, conn, reader)

		if _, err := readRTMPCommand(conn, reader); err != nil {
			t.Errorf("read createStream: %v", err)
			return
		}
		rtmpTestSend(t, conn, 3, 0, amf0StringValue("_result"), amf0NumberValue(2), amf0NullValue, amf0NumberValue(1))

		values, err := readRTMPCommand(conn, reader)
		if err != nil {
			t.Errorf("read play: %v", err)
			return
		}
		if rtmpCommandName(values) != "play" || len(values) < 4 ||
			values[3].kind != amf0String || values[3].text != "live" {
			t.Errorf("play command = %+v", values)
		}
		rtmpTestSend(t, conn, 5, 1, amf0StringValue("onStatus"), amf0NumberValue(0), amf0NullValue,
			amf0ObjectValue(map[string]amf0Value{
				"level": amf0StringValue("status"),
				"code":  amf0StringValue("NetStream.Play.Start"),
			}))
	})
	response, err := executeRTMP(mediaTestConfig(), &Request{
		Kind: "rtmp", Target: "rtmp://" + address + "/live",
		Opts: map[string]string{"app": "live", "operation": "play", "stream": "live"}, TimeoutMS: 2000,
	})
	if err != nil {
		t.Fatal(err)
	}
	body := mediaReadJSON(t, response)
	if body["stream"] != "live" || body["status"] != "NetStream.Play.Start" {
		t.Fatalf("body = %+v", body)
	}
}

func TestRTMPRequiresStream(t *testing.T) {
	address := mediaTCPOnce(t, func(conn net.Conn) {
		rtmpTestHandshake(t, conn)
		rtmpTestConnect(t, conn, newRTMPChunkReader(1<<20))
	})
	_, err := executeRTMP(mediaTestConfig(), &Request{
		Kind: "rtmp", Target: "rtmp://" + address + "/live",
		Opts: map[string]string{"operation": "publish"}, TimeoutMS: 2000,
	})
	if err == nil || !strings.Contains(err.Error(), "requires opts.stream") {
		t.Fatalf("missing stream: %v", err)
	}

	if _, err := executeRTMP(mediaTestConfig(), &Request{
		Kind: "rtmp", Target: "rtmp://127.0.0.1:1935", Opts: map[string]string{"operation": "bogus"},
	}); err == nil || !strings.Contains(err.Error(), "unsupported rtmp operation") {
		t.Fatalf("bad operation: %v", err)
	}
}

func TestRTMPTimeoutAndSSRF(t *testing.T) {
	address := mediaTCPOnce(t, func(conn net.Conn) {
		_, _ = io.Copy(io.Discard, conn) // never answer the handshake
	})
	started := time.Now()
	_, err := executeRTMP(Config{AllowPrivateNetworks: true, TimeoutMS: 200}, &Request{
		Kind: "rtmp", Target: "rtmp://" + address + "/live", TimeoutMS: 200,
	})
	if err == nil || !strings.Contains(err.Error(), "timed out") || time.Since(started) > 5*time.Second {
		t.Fatalf("rtmp timeout: %v after %s", err, time.Since(started))
	}

	_, err = Execute(Config{}, &Request{Kind: "rtmp", Target: "rtmp://127.0.0.1:1935", TimeoutMS: 1000})
	if err == nil || !strings.Contains(err.Error(), "private/loopback") {
		t.Fatalf("rtmp loopback must be denied: %v", err)
	}
}

func TestRTMPPlayWithoutOnStatusSucceeds(t *testing.T) {
	address := mediaTCPOnce(t, func(conn net.Conn) {
		rtmpTestHandshake(t, conn)
		reader := newRTMPChunkReader(1 << 20)
		rtmpTestConnect(t, conn, reader)
		if _, err := readRTMPCommand(conn, reader); err != nil {
			t.Errorf("read createStream: %v", err)
			return
		}
		rtmpTestSend(t, conn, 3, 0, amf0StringValue("_result"), amf0NumberValue(2), amf0NullValue, amf0NumberValue(1))
		if _, err := readRTMPCommand(conn, reader); err != nil {
			t.Errorf("read play: %v", err)
		}
		_, _ = io.Copy(io.Discard, conn) // stay silent: onStatus is optional
	})
	response, err := executeRTMP(Config{AllowPrivateNetworks: true, TimeoutMS: 500}, &Request{
		Kind: "rtmp", Target: "rtmp://" + address + "/live",
		Opts: map[string]string{
			"operation": "play", "stream": "live", "duration_ms": "100",
		},
		TimeoutMS: 500,
	})
	if err != nil {
		t.Fatal(err)
	}
	body := mediaReadJSON(t, response)
	if body["stream"] != "live" || body["status"] != nil {
		t.Fatalf("body = %+v", body)
	}
}
