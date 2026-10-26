//go:build apidoc_debug_industrial || apidoc_debug_all

package debug

import (
	"encoding/binary"
	"io"
	"math"
	"net"
	"strings"
	"testing"
	"time"
)

func iec104TestRequest(target string, opts map[string]string) *Request {
	return &Request{Kind: "iec104", Target: "iec104://" + target, Opts: opts, TimeoutMS: 2000}
}

func iec104TestReadAPDU(conn net.Conn) ([]byte, error) {
	header := make([]byte, 2)
	if _, err := io.ReadFull(conn, header); err != nil {
		return nil, err
	}
	body := make([]byte, int(header[1]))
	if _, err := io.ReadFull(conn, body); err != nil {
		return nil, err
	}
	return body, nil
}

func iec104TestFrame(asdu []byte) []byte {
	out := []byte{0x68, byte(4 + len(asdu)), 0x00, 0x00, 0x02, 0x00} // I-frame ns=0, nr=1
	return append(out, asdu...)
}

// iec104TestServer performs the STARTDT handshake, asserts the request ASDU and
// writes one I-frame with responseASDU.
func iec104TestServer(t *testing.T, expectedASDU, responseASDU []byte) string {
	t.Helper()
	return startTCPServer(t, func(conn net.Conn) {
		body, err := iec104TestReadAPDU(conn)
		if err != nil || string(body) != string([]byte{0x07, 0x00, 0x00, 0x00}) {
			t.Errorf("STARTDT act = %x, %v", body, err)
			return
		}
		if _, err := conn.Write([]byte{0x68, 0x04, 0x0B, 0x00, 0x00, 0x00}); err != nil {
			t.Errorf("STARTDT con: %v", err)
			return
		}
		body, err = iec104TestReadAPDU(conn)
		if err != nil {
			t.Errorf("request APDU: %v", err)
			return
		}
		if string(body[:4]) != string([]byte{0x00, 0x00, 0x00, 0x00}) {
			t.Errorf("request control = %x", body[:4])
		}
		if string(body[4:]) != string(expectedASDU) {
			t.Errorf("request ASDU = %x, want %x", body[4:], expectedASDU)
		}
		if _, err := conn.Write(iec104TestFrame(responseASDU)); err != nil {
			t.Errorf("response: %v", err)
		}
	})
}

func TestIEC104InterrogationGolden(t *testing.T) {
	responseASDU := []byte{13, 0x01, 0x14, 0x00, 0x01, 0x00, 0x01, 0x00, 0x00}
	responseASDU = binary.LittleEndian.AppendUint32(responseASDU, math.Float32bits(10))
	responseASDU = append(responseASDU, 0x00) // quality
	responseASDU = append(responseASDU, 100, 0x01, 0x0A, 0x00, 0x01, 0x00, 0, 0, 0, 20)
	address := iec104TestServer(t,
		[]byte{100, 0x01, 0x06, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 20},
		responseASDU)
	result, err := executeIEC104(industrialTestConfig(), iec104TestRequest(address,
		map[string]string{"operation": "interrogation", "duration_ms": "500"}))
	if err != nil {
		t.Fatal(err)
	}
	rows := decodeBody(t, result)["asdu"].([]any)
	if len(rows) != 2 {
		t.Fatalf("rows = %#v", rows)
	}
	first := rows[0].(map[string]any)
	if first["type_id"] != float64(13) || first["cause"] != float64(20) ||
		first["ioa"] != float64(1) || first["value"] != float64(10) {
		t.Fatalf("first row = %#v", first)
	}
	second := rows[1].(map[string]any)
	if second["type_id"] != float64(100) || second["cause"] != float64(10) {
		t.Fatalf("second row = %#v", second)
	}
}

func TestIEC104SinglePointRead(t *testing.T) {
	address := iec104TestServer(t,
		[]byte{102, 0x01, 0x05, 0x00, 0x01, 0x00, 0x05, 0x00, 0x00},
		[]byte{1, 0x01, 0x14, 0x00, 0x01, 0x00, 0x05, 0x00, 0x00, 0x01})
	result, err := executeIEC104(industrialTestConfig(), iec104TestRequest(address,
		map[string]string{"operation": "read", "ioa": "5", "duration_ms": "500"}))
	if err != nil {
		t.Fatal(err)
	}
	rows := decodeBody(t, result)["asdu"].([]any)
	if len(rows) != 1 {
		t.Fatalf("rows = %#v", rows)
	}
	row := rows[0].(map[string]any)
	if row["type_id"] != float64(1) || row["ioa"] != float64(5) || row["value"] != true {
		t.Fatalf("row = %#v", row)
	}
}

func TestIEC104TimeoutCapMalformedAndValidation(t *testing.T) {
	t.Run("startdt timeout", func(t *testing.T) {
		address := startTCPServer(t, func(conn net.Conn) {
			if _, err := iec104TestReadAPDU(conn); err != nil {
				t.Errorf("STARTDT act: %v", err)
				return
			}
			time.Sleep(500 * time.Millisecond) // keep the link open past the read deadline
		})
		request := iec104TestRequest(address, nil)
		request.TimeoutMS = 100
		started := time.Now()
		_, err := executeIEC104(industrialTestConfig(), request)
		if err == nil || !strings.Contains(err.Error(), "STARTDT was not confirmed") {
			t.Fatalf("startdt error = %v", err)
		}
		if time.Since(started) > 5*time.Second {
			t.Fatalf("timeout not bounded: %v", time.Since(started))
		}
	})

	t.Run("cap", func(t *testing.T) {
		responseASDU := []byte{1, 0x01, 0x14, 0x00, 0x01, 0x00, 0x05, 0x00, 0x00, 0x01}
		address := iec104TestServer(t,
			[]byte{100, 0x01, 0x06, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 20},
			responseASDU)
		cfg := industrialTestConfig()
		cfg.MaxBytes = 10
		_, err := executeIEC104(cfg, iec104TestRequest(address,
			map[string]string{"duration_ms": "500"}))
		if err == nil || !strings.Contains(err.Error(), "exceeds max_bytes") {
			t.Fatalf("cap error = %v", err)
		}
	})

	t.Run("malformed start", func(t *testing.T) {
		address := startTCPServer(t, func(conn net.Conn) {
			body, err := iec104TestReadAPDU(conn)
			if err != nil || len(body) != 4 {
				return
			}
			_, _ = conn.Write([]byte{0x67, 0x04, 0x0B, 0x00, 0x00, 0x00})
		})
		_, err := executeIEC104(industrialTestConfig(), iec104TestRequest(address, nil))
		if err == nil || !strings.Contains(err.Error(), "invalid iec104 start byte") {
			t.Fatalf("malformed error = %v", err)
		}
	})

	t.Run("truncated ASDU", func(t *testing.T) {
		address := iec104TestServer(t,
			[]byte{100, 0x01, 0x06, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 20},
			[]byte{13, 0x01, 0x14, 0x00, 0x01, 0x00, 0x01, 0x00, 0x00})
		_, err := executeIEC104(industrialTestConfig(), iec104TestRequest(address,
			map[string]string{"duration_ms": "500"}))
		if err == nil || !strings.Contains(err.Error(), "truncated iec104 value") {
			t.Fatalf("truncated error = %v", err)
		}
	})

	features := industrialTestConfig()
	cases := []struct {
		opts map[string]string
		want string
	}{
		{map[string]string{"operation": "control"}, "unsupported iec104 operation"},
		{map[string]string{"operation": "read"}, "`ioa` is required"},
		{map[string]string{"operation": "read", "ioa": "16777216"}, "24 bits"},
		{map[string]string{"operation": "read", "ioa": "x"}, "invalid iec104 ioa"},
	}
	for _, test := range cases {
		_, err := executeIEC104(features, iec104TestRequest("127.0.0.1:2404", test.opts))
		if err == nil || !strings.Contains(err.Error(), test.want) {
			t.Errorf("opts %#v: error %v, want %q", test.opts, err, test.want)
		}
	}
	if _, err := executeIEC104(Config{}, iec104TestRequest("127.0.0.1:2404", nil)); err == nil ||
		!strings.Contains(err.Error(), "private/loopback") {
		t.Fatalf("guard error = %v", err)
	}
}
