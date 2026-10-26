//go:build apidoc_debug_media || apidoc_debug_all

package debug

import (
	"bytes"
	"net"
	"strings"
	"testing"
)

func TestRTPHeaderGolden(t *testing.T) {
	packet := encodeRTPPacket(96, 7, 12345, 0xdeadbeef, []byte("hi"))
	want := []byte{0x80, 96, 0x00, 0x07, 0x00, 0x00, 0x30, 0x39, 0xde, 0xad, 0xbe, 0xef}
	if !bytes.Equal(packet[:12], want) {
		t.Fatalf("header = % x, want % x", packet[:12], want)
	}
	decoded, err := decodeRTPPacket(packet)
	if err != nil {
		t.Fatal(err)
	}
	if decoded.version != 2 || decoded.payloadType != 96 || decoded.sequence != 7 ||
		decoded.timestamp != 12345 || decoded.ssrc != 0xdeadbeef ||
		string(decoded.payload) != "hi" ||
		decoded.marker || decoded.padding || decoded.extension {
		t.Fatalf("decoded = %+v", decoded)
	}
}

func TestRTPCSRCExtensionAndPaddingOffsets(t *testing.T) {
	csrc := encodeRTPPacket(96, 1, 0, 1, []byte("hi"))
	csrc[0] |= 0x02
	csrc = append(csrc[:12], append([]byte{0, 0, 0, 10, 0, 0, 0, 11}, csrc[12:]...)...)
	decoded, err := decodeRTPPacket(csrc)
	if err != nil || len(decoded.csrc) != 2 || decoded.csrc[0] != 10 || decoded.csrc[1] != 11 ||
		string(decoded.payload) != "hi" {
		t.Fatalf("csrc = %+v, %v", decoded, err)
	}

	extension := encodeRTPPacket(96, 1, 0, 1, []byte("hi"))
	extension[0] |= 0x10
	extension = append(extension[:12], append([]byte{0xab, 0xac, 0, 1, 0xde, 0xad, 0xbe, 0xef}, extension[12:]...)...)
	decoded, err = decodeRTPPacket(extension)
	if err != nil || !decoded.extension || string(decoded.payload) != "hi" {
		t.Fatalf("extension = %+v, %v", decoded, err)
	}

	padded := encodeRTPPacket(96, 1, 0, 1, []byte("hi"))
	padded[0] |= 0x20
	padded = append(padded, 0, 0, 0, 4)
	decoded, err = decodeRTPPacket(padded)
	if err != nil || !decoded.padding || string(decoded.payload) != "hi" {
		t.Fatalf("padding = %+v, %v", decoded, err)
	}
}

func TestRTPRejectsMalformedPackets(t *testing.T) {
	if _, err := decodeRTPPacket([]byte{0x80, 96, 0, 1}); err == nil {
		t.Fatal("truncated header must fail")
	}
	badCSRC := encodeRTPPacket(96, 1, 0, 1, []byte("hi"))
	badCSRC[0] |= 0x0f
	if _, err := decodeRTPPacket(badCSRC); err == nil {
		t.Fatal("CC beyond packet must fail")
	}
	badPadding := encodeRTPPacket(96, 1, 0, 1, []byte("hi"))
	badPadding[0] |= 0x20
	badPadding = append(badPadding, 9)
	if _, err := decodeRTPPacket(badPadding); err == nil {
		t.Fatal("padding count beyond packet must fail")
	}
}

func TestRTPUDPRoundtrip(t *testing.T) {
	address := mediaUDPOnce(t, func(conn *net.UDPConn, peer *net.UDPAddr, raw []byte) {
		packet, err := decodeRTPPacket(raw)
		if err != nil {
			t.Errorf("server decode: %v", err)
			return
		}
		if string(packet.payload) != "hello" || packet.sequence != 10 || packet.payloadType != 96 {
			t.Errorf("server packet = %+v", packet)
		}
		reply := encodeRTPPacket(97, 99, 4242, 7, []byte("pong"))
		if _, err := conn.WriteToUDP(reply, peer); err != nil {
			t.Errorf("server reply: %v", err)
		}
	})
	response, err := executeRTP(mediaTestConfig(), &Request{
		Kind: "rtp", Target: "rtp://" + address,
		Body: &Payload{Encoding: "utf8", Data: "hello"},
		Opts: map[string]string{
			"payload_type": "96", "sequence": "10", "ssrc": "42",
			"recv": "true", "duration_ms": "1000",
		},
		TimeoutMS: 2000,
	})
	if err != nil {
		t.Fatal(err)
	}
	if response.Headers["x-sent"] != "1" || response.Headers["x-received"] != "1" {
		t.Fatalf("headers = %+v", response.Headers)
	}
	body := mediaReadJSON(t, response)
	packets := body["packets"].([]any)
	first := packets[0].(map[string]any)
	if first["payload_type"] != float64(97) {
		t.Fatalf("packet = %+v", first)
	}
	if payload := first["payload"].(map[string]any); payload["data"] != "pong" {
		t.Fatalf("payload = %+v", payload)
	}
}

func TestRTPMediaCapsAndSSRF(t *testing.T) {
	_, err := executeRTP(Config{AllowPrivateNetworks: true, TimeoutMS: 1000, MaxBytes: 13}, &Request{
		Kind: "rtp", Target: "rtp://127.0.0.1:5004",
		Body: &Payload{Encoding: "utf8", Data: "hello"}, TimeoutMS: 1000,
	})
	if err == nil || !strings.Contains(err.Error(), "udp send exceeds max_bytes (13)") {
		t.Fatalf("send cap: %v", err)
	}

	for _, kind := range []string{"rtp", "rtcp"} {
		_, err = Execute(Config{}, &Request{
			Kind: kind, Target: kind + "://127.0.0.1:5004", TimeoutMS: 1000,
		})
		if err == nil || !strings.Contains(err.Error(), "private/loopback") {
			t.Fatalf("%s loopback must be denied: %v", kind, err)
		}
	}
}
