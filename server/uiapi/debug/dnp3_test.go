//go:build apidoc_debug_industrial || apidoc_debug_all

package debug

import (
	"encoding/hex"
	"encoding/json"
	"io"
	"net"
	"strings"
	"testing"
	"time"
)

const (
	dnp3GoldenRequest = "05640bc4010000004de9c0c0013c0106ff50"
	// app: C0 81 + IIN 00 00 + G30V1 objects {0: 42, 1: -7}
	dnp3GoldenResponse = "0564194400000100e9bfc0c08100001e01000001012a000000010c86f9ffffff0d6e"
)

func dnp3TestRequest(target string, opts map[string]string) *Request {
	return &Request{Kind: "dnp3", Target: "dnp3://" + target, Opts: opts, TimeoutMS: 2000}
}

func dnp3TestServer(t *testing.T, response []byte) string {
	t.Helper()
	return startTCPServer(t, func(conn net.Conn) {
		request := make([]byte, 18)
		if _, err := io.ReadFull(conn, request); err != nil {
			t.Errorf("dnp3 request: %v", err)
			return
		}
		if hex.EncodeToString(request) != dnp3GoldenRequest {
			t.Errorf("dnp3 request = %x, want %s", request, dnp3GoldenRequest)
		}
		if _, err := conn.Write(response); err != nil {
			t.Errorf("dnp3 response: %v", err)
		}
	})
}

func TestDNP3GoldenIntegrityPoll(t *testing.T) {
	response, err := hex.DecodeString(dnp3GoldenResponse)
	if err != nil {
		t.Fatal(err)
	}
	address := dnp3TestServer(t, response)
	result, err := executeDNP3(industrialTestConfig(), dnp3TestRequest(address,
		map[string]string{"operation": "integrity", "duration_ms": "500"}))
	if err != nil {
		t.Fatal(err)
	}
	points := decodeBody(t, result)["points"].([]any)
	if len(points) != 2 {
		t.Fatalf("points = %#v", points)
	}
	first := points[0].(map[string]any)
	second := points[1].(map[string]any)
	if first["group"] != float64(30) || first["index"] != float64(0) ||
		first["value"] != float64(42) || first["flags"] != float64(1) {
		t.Fatalf("first point = %#v", first)
	}
	if second["index"] != float64(1) || second["value"] != float64(-7) {
		t.Fatalf("second point = %#v", second)
	}
}

func TestDNP3Codec(t *testing.T) {
	if crc := dnp3CRC16([]byte("123456789")); crc != 0xEA82 {
		t.Fatalf("crc = %04x", crc)
	}
	frame, err := dnp3EncodeLinkFrame(0xC4, 1, 0, []byte{0xC0, 0xC0, 0x01, 0x3C, 0x01, 0x06})
	if err != nil {
		t.Fatal(err)
	}
	if hex.EncodeToString(frame) != dnp3GoldenRequest {
		t.Fatalf("encoded frame = %x", frame)
	}

	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()
	go func() {
		_, _ = server.Write(frame)
	}()
	decoded, err := dnp3ReadLinkFrame(client, time.Now().Add(time.Second))
	if err != nil || decoded == nil {
		t.Fatalf("decode frame: %v", err)
	}
	if decoded.ctrl != 0xC4 || decoded.dst != 1 || decoded.src != 0 ||
		hex.EncodeToString(decoded.data) != "c0c0013c0106" {
		t.Fatalf("decoded frame = %#v", decoded)
	}

	broken := append([]byte{}, frame...)
	broken[len(broken)-1] ^= 0xFF
	client2, server2 := net.Pipe()
	defer client2.Close()
	defer server2.Close()
	go func() {
		_, _ = server2.Write(broken)
	}()
	if _, err := dnp3ReadLinkFrame(client2, time.Now().Add(time.Second)); err == nil ||
		!strings.Contains(err.Error(), "data CRC mismatch") {
		t.Fatalf("crc error = %v", err)
	}

	packed, err := dnp3ParseObjects([]byte{1, 1, 0x00, 0x00, 0x02, 0x05})
	if err != nil {
		t.Fatal(err)
	}
	if len(packed) != 3 {
		t.Fatalf("packed points = %#v", packed)
	}
	for index, want := range []uint8{1, 0, 1} {
		point := packed[index].(map[string]any)
		if point["index"] != uint32(index) || point["value"] != want {
			t.Fatalf("packed point %d = %#v", index, point)
		}
	}
	// Group 30 vars 5/7 widen f32 to f64 before JSON, so 0.1 must serialize
	// with its full float64 expansion, matching Rust's f64::from.
	for _, objects := range [][]byte{
		{30, 5, 0x00, 0x00, 0x00, 0x01, 0xCD, 0xCC, 0xCC, 0x3D},
		{30, 7, 0x00, 0x00, 0x00, 0xCD, 0xCC, 0xCC, 0x3D},
	} {
		points, err := dnp3ParseObjects(objects)
		if err != nil {
			t.Fatal(err)
		}
		encoded, err := json.Marshal(points[0])
		if err != nil || !strings.Contains(string(encoded), `"value":0.10000000149011612`) {
			t.Fatalf("float point %x = %s (%v)", objects, encoded, err)
		}
	}
	// A packed object claiming more than the per-response cap must fail before
	// allocating its points.
	if _, err := dnp3ParseObjects([]byte{1, 1, 0x80, 0x11, 0x27}); err == nil ||
		!strings.Contains(err.Error(), "exceeds 10000 points") {
		t.Fatalf("point cap error = %v", err)
	}
	for _, malformed := range [][]byte{
		{30, 1, 0x00, 0x00, 0x01, 0x01},
		{30, 1, 0x06},
		{30, 1, 0x00, 0x02, 0x01},
	} {
		if _, err := dnp3ParseObjects(malformed); err == nil {
			t.Errorf("malformed objects %x parsed without error", malformed)
		}
	}
}

func TestDNP3TimeoutCapAndValidation(t *testing.T) {
	t.Run("timeout", func(t *testing.T) {
		address := startTCPServer(t, func(conn net.Conn) {
			request := make([]byte, 18)
			_, _ = io.ReadFull(conn, request)
		})
		started := time.Now()
		_, err := executeDNP3(industrialTestConfig(), dnp3TestRequest(address,
			map[string]string{"duration_ms": "50"}))
		if err == nil || !strings.Contains(err.Error(), "timed out after 50ms") {
			t.Fatalf("timeout error = %v", err)
		}
		if time.Since(started) > 5*time.Second {
			t.Fatalf("timeout not bounded: %v", time.Since(started))
		}
	})

	t.Run("cap", func(t *testing.T) {
		response, _ := hex.DecodeString(dnp3GoldenResponse)
		address := dnp3TestServer(t, response)
		cfg := industrialTestConfig()
		cfg.MaxBytes = 20
		_, err := executeDNP3(cfg, dnp3TestRequest(address,
			map[string]string{"duration_ms": "500"}))
		if err == nil || !strings.Contains(err.Error(), "exceeds max_bytes") {
			t.Fatalf("cap error = %v", err)
		}
	})

	t.Run("malformed", func(t *testing.T) {
		address := startTCPServer(t, func(conn net.Conn) {
			request := make([]byte, 18)
			_, _ = io.ReadFull(conn, request)
			_, _ = conn.Write([]byte{0x06, 0x64, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00})
		})
		_, err := executeDNP3(industrialTestConfig(), dnp3TestRequest(address,
			map[string]string{"duration_ms": "500"}))
		if err == nil || !strings.Contains(err.Error(), "invalid dnp3 start bytes") {
			t.Fatalf("malformed error = %v", err)
		}
	})

	t.Run("missing iin", func(t *testing.T) {
		address := startTCPServer(t, func(conn net.Conn) {
			request := make([]byte, 18)
			_, _ = io.ReadFull(conn, request)
			frame, err := dnp3EncodeLinkFrame(0x44, 0, 1, []byte{0xC0, 0xC0, 0x81, 0x00})
			if err != nil {
				t.Errorf("encode frame: %v", err)
				return
			}
			_, _ = conn.Write(frame)
		})
		_, err := executeDNP3(industrialTestConfig(), dnp3TestRequest(address,
			map[string]string{"duration_ms": "500"}))
		if err == nil || !strings.Contains(err.Error(), "missing IIN") {
			t.Fatalf("missing IIN error = %v", err)
		}
	})

	features := industrialTestConfig()
	cases := []struct {
		opts map[string]string
		want string
	}{
		{map[string]string{"link_address": "7", "source_address": "7"}, "must differ"},
		{map[string]string{"operation": "operate"}, "unsupported dnp3 operation"},
		{map[string]string{"operation": "read", "start": "5", "stop": "4"}, "start must be <= stop"},
		{map[string]string{"operation": "read", "stop": "65536"}, "stop must be <= 65535"},
	}
	for _, test := range cases {
		_, err := executeDNP3(features, dnp3TestRequest("127.0.0.1:20000", test.opts))
		if err == nil || !strings.Contains(err.Error(), test.want) {
			t.Errorf("opts %#v: error %v, want %q", test.opts, err, test.want)
		}
	}
	if _, err := executeDNP3(Config{}, dnp3TestRequest("127.0.0.1:20000", nil)); err == nil ||
		!strings.Contains(err.Error(), "private/loopback") {
		t.Fatalf("guard error = %v", err)
	}
}
