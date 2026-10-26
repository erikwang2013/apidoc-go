//go:build apidoc_debug_industrial || apidoc_debug_all

package debug

import (
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"io"
	"net"
	"strings"
	"testing"
	"time"
)

// startTCPServer accepts one connection and runs handler on it.
func startTCPServer(t *testing.T, handler func(net.Conn)) string {
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

func industrialTestConfig() Config {
	return Config{AllowPrivateNetworks: true, TimeoutMS: 2000}
}

// modbusTestServer answers one MBAP request with the supplied PDU. The mock
// asserts the request PDU and echoes transaction id and unit id.
func modbusTestServer(t *testing.T, expectedPDU, responsePDU []byte) string {
	t.Helper()
	return startTCPServer(t, func(conn net.Conn) {
		header := make([]byte, 7)
		if _, err := io.ReadFull(conn, header); err != nil {
			t.Errorf("modbus header: %v", err)
			return
		}
		if binary.BigEndian.Uint16(header[2:]) != 0 {
			t.Errorf("modbus protocol id = %x", header[2:4])
		}
		length := int(binary.BigEndian.Uint16(header[4:]))
		pdu := make([]byte, length-1)
		if _, err := io.ReadFull(conn, pdu); err != nil {
			t.Errorf("modbus pdu: %v", err)
			return
		}
		if hex.EncodeToString(pdu) != hex.EncodeToString(expectedPDU) {
			t.Errorf("modbus request pdu = %x, want %x", pdu, expectedPDU)
		}
		response := append([]byte{}, header[:4]...)
		response = binary.BigEndian.AppendUint16(response, uint16(len(responsePDU)+1))
		response = append(response, header[6])
		response = append(response, responsePDU...)
		if _, err := conn.Write(response); err != nil {
			t.Errorf("modbus write: %v", err)
		}
	})
}

func modbusTestRequest(target string, opts map[string]string) *Request {
	return &Request{Kind: "modbus", Target: "modbus://" + target, Opts: opts, TimeoutMS: 2000}
}

func decodeBody(t *testing.T, response *Response) map[string]any {
	t.Helper()
	var body map[string]any
	if err := json.Unmarshal([]byte(response.Body.Data), &body); err != nil {
		t.Fatalf("body %q: %v", response.Body.Data, err)
	}
	return body
}

func TestModbusReadsAndWrites(t *testing.T) {
	t.Run("read_holding", func(t *testing.T) {
		address := modbusTestServer(t,
			[]byte{0x03, 0x00, 0x00, 0x00, 0x02},
			[]byte{0x03, 0x04, 0x00, 0x64, 0x00, 0xC8})
		response, err := executeModbus(industrialTestConfig(), modbusTestRequest(address,
			map[string]string{"operation": "read_holding", "address": "0", "quantity": "2"}))
		if err != nil {
			t.Fatal(err)
		}
		values := decodeBody(t, response)["values"].([]any)
		if len(values) != 2 || values[0] != float64(100) || values[1] != float64(200) {
			t.Fatalf("values = %#v", values)
		}
		if response.Headers["x-unit-id"] != "1" || response.Headers["x-peer"] != address {
			t.Fatalf("headers = %#v", response.Headers)
		}
	})

	t.Run("read_coils", func(t *testing.T) {
		address := modbusTestServer(t,
			[]byte{0x01, 0x00, 0x00, 0x00, 0x02},
			[]byte{0x01, 0x01, 0x02})
		response, err := executeModbus(industrialTestConfig(), modbusTestRequest(address,
			map[string]string{"operation": "read_coils", "address": "0", "quantity": "2"}))
		if err != nil {
			t.Fatal(err)
		}
		values := decodeBody(t, response)["values"].([]any)
		if len(values) != 2 || values[0] != false || values[1] != true {
			t.Fatalf("values = %#v", values)
		}
	})

	t.Run("write_coil", func(t *testing.T) {
		address := modbusTestServer(t,
			[]byte{0x05, 0x00, 0x05, 0xFF, 0x00},
			[]byte{0x05, 0x00, 0x05, 0xFF, 0x00})
		response, err := executeModbus(industrialTestConfig(), modbusTestRequest(address,
			map[string]string{"operation": "write_coil", "address": "5", "value": "true"}))
		if err != nil {
			t.Fatal(err)
		}
		if values := decodeBody(t, response)["values"].([]any); len(values) != 0 {
			t.Fatalf("values = %#v", values)
		}
	})

	t.Run("write_register", func(t *testing.T) {
		address := modbusTestServer(t,
			[]byte{0x06, 0x00, 0x05, 0x01, 0x2C},
			[]byte{0x06, 0x00, 0x05, 0x01, 0x2C})
		if _, err := executeModbus(industrialTestConfig(), modbusTestRequest(address,
			map[string]string{"operation": "write_register", "address": "5", "value": "300"})); err != nil {
			t.Fatal(err)
		}
	})
}

func TestModbusExceptionsEchoAndValidation(t *testing.T) {
	address := modbusTestServer(t,
		[]byte{0x03, 0x00, 0x00, 0x00, 0x01},
		[]byte{0x83, 0x02})
	_, err := executeModbus(industrialTestConfig(), modbusTestRequest(address,
		map[string]string{"operation": "read_holding", "address": "0"}))
	if err == nil || !strings.Contains(err.Error(), "modbus exception: illegal data address") {
		t.Fatalf("exception error = %v", err)
	}

	badEcho := modbusTestServer(t,
		[]byte{0x06, 0x00, 0x05, 0x01, 0x2C},
		[]byte{0x06, 0x00, 0x05, 0x01, 0x2D})
	_, err = executeModbus(industrialTestConfig(), modbusTestRequest(badEcho,
		map[string]string{"operation": "write_register", "address": "5", "value": "300"}))
	if err == nil || !strings.Contains(err.Error(), "does not echo") {
		t.Fatalf("echo error = %v", err)
	}

	features := industrialTestConfig()
	cases := []struct {
		opts map[string]string
		want string
	}{
		{map[string]string{"operation": "read_holding"}, "`address` is required"},
		{map[string]string{"operation": "read_holding", "address": "0", "quantity": "0"},
			"quantity must be 1..=125"},
		{map[string]string{"operation": "read_holding", "address": "0", "quantity": "126"},
			"quantity must be 1..=125"},
		{map[string]string{"operation": "read_coils", "address": "65535", "quantity": "2"},
			"exceeds the 16-bit address space"},
		{map[string]string{"operation": "read_holding", "address": "0", "unit_id": "256"},
			"unit_id must fit u8"},
		{map[string]string{"operation": "read_holding", "address": "x"}, "invalid modbus address"},
		{map[string]string{"operation": "write_coil", "address": "1", "value": "maybe"},
			"invalid coil value"},
		{map[string]string{"operation": "subscribe", "address": "0"}, "unsupported modbus operation"},
	}
	for _, test := range cases {
		_, err := executeModbus(features, modbusTestRequest("127.0.0.1:502", test.opts))
		if err == nil || !strings.Contains(err.Error(), test.want) {
			t.Errorf("opts %#v: error %v, want %q", test.opts, err, test.want)
		}
	}

	if _, err := executeModbus(Config{}, modbusTestRequest("127.0.0.1:502",
		map[string]string{"operation": "read_holding", "address": "0"})); err == nil ||
		!strings.Contains(err.Error(), "private/loopback") {
		t.Fatalf("guard error = %v", err)
	}
}

func TestModbusTimeoutAndTransactionMismatch(t *testing.T) {
	t.Run("timeout", func(t *testing.T) {
		address := startTCPServer(t, func(conn net.Conn) {})
		request := modbusTestRequest(address, map[string]string{
			"operation": "read_holding", "address": "0",
		})
		request.TimeoutMS = 100
		started := time.Now()
		_, err := executeModbus(industrialTestConfig(), request)
		if err == nil || !strings.Contains(err.Error(), "modbus read failed") {
			t.Fatalf("timeout error = %v", err)
		}
		if time.Since(started) > 5*time.Second {
			t.Fatalf("timeout not bounded: %v", time.Since(started))
		}
	})

	t.Run("response cap", func(t *testing.T) {
		address := startTCPServer(t, func(conn net.Conn) {
			header := make([]byte, 7)
			if _, err := io.ReadFull(conn, header); err != nil {
				return
			}
			pdu := make([]byte, int(binary.BigEndian.Uint16(header[4:]))-1)
			if _, err := io.ReadFull(conn, pdu); err != nil {
				return
			}
			_, _ = conn.Write([]byte{header[0], header[1], 0, 0, 0, 200, header[6]})
		})
		cfg := industrialTestConfig()
		cfg.MaxBytes = 12 // exactly the request size, so the oversized reply trips the cap
		_, err := executeModbus(cfg, modbusTestRequest(address,
			map[string]string{"operation": "read_holding", "address": "0"}))
		if err == nil || !strings.Contains(err.Error(), "modbus response exceeds max_bytes (12)") {
			t.Fatalf("cap error = %v", err)
		}
	})

	t.Run("transaction", func(t *testing.T) {
		address := startTCPServer(t, func(conn net.Conn) {
			header := make([]byte, 7)
			if _, err := io.ReadFull(conn, header); err != nil {
				return
			}
			pdu := make([]byte, int(binary.BigEndian.Uint16(header[4:]))-1)
			_, _ = io.ReadFull(conn, pdu)
			response := []byte{0xFF, 0xFE, 0, 0, 0, 3, header[6], 0x03, 0x02, 0, 1}
			_, _ = conn.Write(response)
		})
		_, err := executeModbus(industrialTestConfig(), modbusTestRequest(address,
			map[string]string{"operation": "read_holding", "address": "0"}))
		if err == nil || !strings.Contains(err.Error(), "transaction mismatch") {
			t.Fatalf("transaction error = %v", err)
		}
	})
}
