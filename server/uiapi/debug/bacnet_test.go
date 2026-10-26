//go:build apidoc_debug_industrial || apidoc_debug_all

package debug

import (
	"encoding/binary"
	"encoding/hex"
	"math"
	"net"
	"strings"
	"testing"
	"time"
)

func bacnetTestRequest(target string, opts map[string]string) *Request {
	return &Request{Kind: "bacnet", Target: "bacnet://" + target, Opts: opts, TimeoutMS: 2000}
}

func bacnetTestServer(t *testing.T, handler func([]byte) []byte) string {
	t.Helper()
	server, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { server.Close() })
	go func() {
		buf := make([]byte, 1500)
		n, peer, err := server.ReadFromUDP(buf)
		if err != nil {
			return
		}
		if reply := handler(append([]byte{}, buf[:n]...)); reply != nil {
			_, _ = server.WriteToUDP(reply, peer)
		}
	}()
	return server.LocalAddr().String()
}

func bacnetTestBVLC(broadcast bool, apdu []byte) []byte {
	header := []byte{0x81, 0x0A}
	if broadcast {
		header[1] = 0x0B
	}
	out := binary.BigEndian.AppendUint16(header, uint16(4+2+len(apdu)))
	out = append(out, 0x01, 0x00)
	return append(out, apdu...)
}

func bacnetTestIAm(instance uint32) []byte {
	apdu := []byte{0x10, 0x00, 0xC4}
	apdu = binary.BigEndian.AppendUint32(apdu, 8<<22|instance)
	return append(apdu, 0x22, 0x01, 0x40, 0x91, 0x03, 0x21, 0x00)
}

func bacnetTestReadPropertyAck(instance uint32, value float32) []byte {
	apdu := []byte{0x30, 0x00, 0x0C, 0xC4}
	apdu = binary.BigEndian.AppendUint32(apdu, 8<<22|instance)
	apdu = append(apdu, 0x19, 0x55, 0x3E, 0x44)
	apdu = binary.BigEndian.AppendUint32(apdu, math.Float32bits(value))
	return append(apdu, 0x3F)
}

func TestBACnetWhoIsAndBroadcast(t *testing.T) {
	t.Run("unicast", func(t *testing.T) {
		address := bacnetTestServer(t, func(packet []byte) []byte {
			if hex.EncodeToString(packet) != "810a000801001008" {
				t.Errorf("who_is packet = %x", packet)
			}
			return bacnetTestBVLC(false, bacnetTestIAm(26000))
		})
		result, err := executeBACnet(industrialTestConfig(), bacnetTestRequest(address,
			map[string]string{"operation": "who_is", "duration_ms": "50"}))
		if err != nil {
			t.Fatal(err)
		}
		devices := decodeBody(t, result)["devices"].([]any)
		if len(devices) != 1 {
			t.Fatalf("devices = %#v", devices)
		}
		device := devices[0].(map[string]any)
		if device["instance"] != float64(26000) || device["address"] != address {
			t.Fatalf("device = %#v", device)
		}
	})

	t.Run("broadcast", func(t *testing.T) {
		address := bacnetTestServer(t, func(packet []byte) []byte {
			if len(packet) < 8 || packet[1] != 0x0B ||
				hex.EncodeToString(packet[4:8]) != "01001008" {
				t.Errorf("broadcast packet = %x", packet)
			}
			return bacnetTestBVLC(false, bacnetTestIAm(7))
		})
		result, err := executeBACnet(industrialTestConfig(), bacnetTestRequest(address,
			map[string]string{"broadcast": "true", "duration_ms": "50"}))
		if err != nil {
			t.Fatal(err)
		}
		devices := decodeBody(t, result)["devices"].([]any)
		if len(devices) != 1 || devices[0].(map[string]any)["instance"] != float64(7) {
			t.Fatalf("devices = %#v", devices)
		}
	})
}

func TestBACnetReadProperty(t *testing.T) {
	address := bacnetTestServer(t, func(packet []byte) []byte {
		expected := []byte{0x81, 0x0A, 0x00, 0x10, 0x01, 0x00, 0x00, 0x00, 0x0C, 0xC4}
		expected = binary.BigEndian.AppendUint32(expected, 8<<22|1)
		expected = append(expected, 0x19, 0x55)
		if hex.EncodeToString(packet) != hex.EncodeToString(expected) {
			t.Errorf("read_property packet = %x, want %x", packet, expected)
		}
		return bacnetTestBVLC(false, bacnetTestReadPropertyAck(1, 7.25))
	})
	result, err := executeBACnet(industrialTestConfig(), bacnetTestRequest(address,
		map[string]string{"operation": "read_property", "object_instance": "1", "duration_ms": "500"}))
	if err != nil {
		t.Fatal(err)
	}
	if value := decodeBody(t, result)["value"]; value != 7.25 {
		t.Fatalf("value = %#v", value)
	}
}

func TestBACnetBroadcastRulesFailClosed(t *testing.T) {
	cases := []struct {
		target string
		opts   map[string]string
		config Config
		want   string
	}{
		{"8.8.8.8:47808", map[string]string{"broadcast": "true"}, Config{},
			"allow_private_networks"},
		{"255.255.255.255:47808", map[string]string{"broadcast": "true"}, industrialTestConfig(),
			"255.255.255.255"},
		{"239.1.1.1:47808", map[string]string{"broadcast": "true"}, industrialTestConfig(),
			"multicast"},
		{"127.0.0.1:47808", map[string]string{"operation": "read_property",
			"object_instance": "1", "broadcast": "true"}, industrialTestConfig(),
			"only valid for who_is"},
		{"127.0.0.1:47808", map[string]string{"operation": "read_property"}, industrialTestConfig(),
			"object_instance is required"},
		{"127.0.0.1:47808", map[string]string{"operation": "read_property",
			"object_instance": "4194304"}, industrialTestConfig(), "22 bits"},
		{"127.0.0.1:47808", map[string]string{"operation": "read_property",
			"object_instance": "1", "object_type": "1024"}, industrialTestConfig(),
			"object_type <= 1023"},
		{"127.0.0.1:47808", map[string]string{"operation": "subscribe"}, industrialTestConfig(),
			"unsupported bacnet operation"},
	}
	for _, test := range cases {
		_, err := executeBACnet(test.config, bacnetTestRequest(test.target, test.opts))
		if err == nil || !strings.Contains(err.Error(), test.want) {
			t.Errorf("opts %#v: error %v, want %q", test.opts, err, test.want)
		}
	}
	if _, err := executeBACnet(Config{}, bacnetTestRequest("127.0.0.1:47808", nil)); err == nil ||
		!strings.Contains(err.Error(), "private/loopback") {
		t.Fatalf("guard error = %v", err)
	}
}

func TestBACnetTimeout(t *testing.T) {
	server, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	started := time.Now()
	_, err = executeBACnet(industrialTestConfig(), bacnetTestRequest(server.LocalAddr().String(),
		map[string]string{"operation": "who_is", "duration_ms": "50"}))
	if err == nil || !strings.Contains(err.Error(), "timed out after 50ms without I-Am") {
		t.Fatalf("who_is timeout error = %v", err)
	}
	if time.Since(started) > 5*time.Second {
		t.Fatalf("timeout not bounded: %v", time.Since(started))
	}
	_, err = executeBACnet(industrialTestConfig(), bacnetTestRequest(server.LocalAddr().String(),
		map[string]string{"operation": "read_property", "object_instance": "1",
			"duration_ms": "50"}))
	if err == nil || !strings.Contains(err.Error(), "read_property timed out") {
		t.Fatalf("read_property timeout error = %v", err)
	}
}

func TestBACnetCodec(t *testing.T) {
	if encoded := bacnetTestBVLC(false, []byte{0x10, 0x08}); hex.EncodeToString(encoded) != "810a000801001008" {
		t.Fatalf("encoded BVLC = %x", encoded)
	}
	if apdu, err := bacnetDecodeBVLC(bacnetTestBVLC(false, bacnetTestIAm(1))); err != nil ||
		bacnetParseIAmMust(t, apdu) != 1 {
		t.Fatalf("decode I-Am: %x %v", apdu, err)
	}
	// Routed NPDU (DNET/SNET present) is ignored, not misparsed.
	routed := []byte{0x81, 0x0A, 0x00, 0x08, 0x01, 0x20, 0x00, 0x00}
	if apdu, err := bacnetDecodeBVLC(routed); err != nil || apdu != nil {
		t.Fatalf("routed NPDU = %x %v", apdu, err)
	}
	if value, used, err := bacnetParseAppValue([]byte{0x44, 0x40, 0xE0, 0x00, 0x00}); err != nil || used != 5 || value.(interface{ String() string }).String() != "7" {
		t.Fatalf("REAL value = %#v %d %v", value, used, err)
	}
	if value, used, err := bacnetParseAppValue([]byte{0x11}); err != nil || used != 1 || value != true {
		t.Fatalf("boolean = %#v %d %v", value, used, err)
	}
	if value, _, err := bacnetParseAppValue([]byte{0x73, 0x00, 'h', 'i'}); err != nil || value != "hi" {
		t.Fatalf("string = %#v %v", value, err)
	}
	if value, _, err := bacnetParseAppValue([]byte{0x31, 0xFE}); err != nil || value != int32(-2) {
		t.Fatalf("signed = %#v %v", value, err)
	}
	if _, _, err := bacnetParseAppValue([]byte{0x45, 0x00}); err == nil {
		t.Fatal("truncated REAL parsed without error")
	}
}

func bacnetParseIAmMust(t *testing.T, apdu []byte) uint32 {
	t.Helper()
	instance, ok := bacnetParseIAm(apdu)
	if !ok {
		t.Fatalf("I-Am not parsed: %x", apdu)
	}
	return instance
}
