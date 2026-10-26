//go:build apidoc_debug_media || apidoc_debug_all

package debug

import (
	"bytes"
	"net"
	"strings"
	"testing"
)

func TestRTCPSRRRSDESBYEVectors(t *testing.T) {
	rr := encodeRTCPPacket(201, 0x01020304, nil)
	if want := []byte{0x80, 201, 0x00, 0x01, 1, 2, 3, 4}; !bytes.Equal(rr, want) {
		t.Fatalf("rr = % x, want % x", rr, want)
	}
	decoded, err := decodeRTCPPacket(rr)
	if err != nil || decoded.version != 2 || decoded.packetType != 201 ||
		decoded.length != 8 || !decoded.hasSSRC || decoded.ssrc != 0x01020304 {
		t.Fatalf("rr decoded = %+v, %v", decoded, err)
	}

	sr := encodeRTCPPacket(200, 7, nil)
	if len(sr) != 8 {
		t.Fatalf("empty sr = % x", sr)
	}
	bigSR := encodeRTCPPacket(200, 7, bytes.Repeat([]byte{0x5a}, 300))
	if len(bigSR) != 308 {
		t.Fatalf("big sr length = %d", len(bigSR))
	}
	if decoded, err := decodeRTCPPacket(bigSR); err != nil || decoded.length != 308 || decoded.packetType != 200 {
		t.Fatalf("big sr = %+v, %v", decoded, err)
	}

	sdes := encodeRTCPPacket(202, 7, []byte("user@host"))
	decoded, err = decodeRTCPPacket(sdes)
	if err != nil || decoded.count != 1 || decoded.text != "user@host" {
		t.Fatalf("sdes = %+v, %v", decoded, err)
	}
	// "ab" makes the item 4 bytes long; the SDES END octet must still be
	// present and force one padding word.
	aligned := encodeRTCPPacket(202, 7, []byte("ab"))
	wantAligned := []byte{0x81, 202, 0x00, 0x03, 0, 0, 0, 7, 1, 2, 'a', 'b', 0, 0, 0, 0}
	if !bytes.Equal(aligned, wantAligned) {
		t.Fatalf("aligned sdes = % x, want % x", aligned, wantAligned)
	}
	bye := encodeRTCPPacket(203, 7, []byte("done"))
	decoded, err = decodeRTCPPacket(bye)
	if err != nil || decoded.count != 1 || decoded.text != "done" {
		t.Fatalf("bye = %+v, %v", decoded, err)
	}
	if _, err := decodeRTCPPacket([]byte{0x80, 201}); err == nil {
		t.Fatal("truncated rtcp packet must fail")
	}
}

func TestRTCPUDPRoundtripReceivesBYE(t *testing.T) {
	address := mediaUDPOnce(t, func(conn *net.UDPConn, peer *net.UDPAddr, raw []byte) {
		packet, err := decodeRTCPPacket(raw)
		if err != nil || packet.packetType != 201 {
			t.Errorf("server packet = %+v, %v", packet, err)
			return
		}
		if _, err := conn.WriteToUDP(encodeRTCPPacket(203, 42, []byte("bye")), peer); err != nil {
			t.Errorf("server reply: %v", err)
		}
	})
	response, err := executeRTCP(mediaTestConfig(), &Request{
		Kind: "rtcp", Target: "rtcp://" + address,
		Opts:      map[string]string{"packet_type": "rr", "ssrc": "42", "recv": "true", "duration_ms": "1000"},
		TimeoutMS: 2000,
	})
	if err != nil {
		t.Fatal(err)
	}
	body := mediaReadJSON(t, response)
	first := body["packets"].([]any)[0].(map[string]any)
	if first["packet_type"] != float64(203) || first["text"] != "bye" {
		t.Fatalf("packet = %+v", first)
	}
}

func TestRTCPUnknownPacketTypeFailsClosed(t *testing.T) {
	_, err := executeRTCP(mediaTestConfig(), &Request{
		Kind: "rtcp", Target: "rtcp://127.0.0.1:5005",
		Opts: map[string]string{"packet_type": "bogus"}, TimeoutMS: 500,
	})
	if err == nil || !strings.Contains(err.Error(), "unknown rtcp packet_type") {
		t.Fatalf("unknown type: %v", err)
	}
}
