//go:build apidoc_debug_iot || apidoc_debug_all

package debug

import (
	"encoding/hex"
	"encoding/json"
	"net"
	"strings"
	"testing"
	"time"
)

// lwm2mTestServer starts a UDP CoAP endpoint. The handler receives the decoded
// request, the raw request bytes, and the peer, and returns the response packet.
func lwm2mTestServer(t *testing.T, handler func(coapPacket, []byte, net.Addr) coapPacket) net.PacketConn {
	t.Helper()
	server, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { server.Close() })
	go func() {
		buf := make([]byte, 65535)
		n, peer, err := server.ReadFrom(buf)
		if err != nil {
			return
		}
		request, err := coapDecode(buf[:n])
		if err != nil {
			return
		}
		response := handler(request, buf[:n], peer)
		_, _ = server.WriteTo(coapEncode(response.packetType, response.code,
			response.messageID, response.token, response.options, response.payload), peer)
	}()
	return server
}

func lwm2mTestRequest(server net.PacketConn, operation string, opts map[string]string) *Request {
	if opts == nil {
		opts = map[string]string{}
	}
	opts["operation"] = operation
	return coapTestRequest("coap://"+server.LocalAddr().String(), opts)
}

func lwm2mTestBody(t *testing.T, response *Response) map[string]any {
	t.Helper()
	var body map[string]any
	if err := json.Unmarshal([]byte(response.Body.Data), &body); err != nil {
		t.Fatalf("body %q: %v", response.Body.Data, err)
	}
	return body
}

func TestLwM2MRegisterUpdateDeregister(t *testing.T) {
	t.Run("register", func(t *testing.T) {
		server := lwm2mTestServer(t, func(request coapPacket, raw []byte, _ net.Addr) coapPacket {
			if request.code != 2 || strings.Join(coapOptionStrings(request, 11), "/") != "rd" {
				t.Errorf("unexpected register request: %#v", request)
			}
			query := coapOptionStrings(request, 15)
			for _, expected := range []string{"ep=sensor-1", "lt=300", "lwm2m=1.1", "b=U"} {
				if !contains(query, expected) {
					t.Errorf("query %#v missing %q", query, expected)
				}
			}
			if string(request.payload) != "hello" {
				t.Errorf("payload = %q", request.payload)
			}
			return coapPacket{packetType: coapTypeAcknowledgement, code: 65,
				messageID: request.messageID, token: request.token,
				options: []coapOption{{number: 8, value: []byte("rd")}, {number: 8, value: []byte("abc")}}}
		})
		request := lwm2mTestRequest(server, "register", map[string]string{"endpoint": "sensor-1"})
		request.Body = &Payload{Encoding: "utf8", Data: "hello"}
		response, err := executeLwM2M(coapTestConfig(), request)
		if err != nil {
			t.Fatal(err)
		}
		body := lwm2mTestBody(t, response)
		if body["code"] != "2.01" || body["location"] != "/rd/abc" {
			t.Fatalf("body = %#v", body)
		}
	})

	t.Run("update", func(t *testing.T) {
		server := lwm2mTestServer(t, func(request coapPacket, _ []byte, _ net.Addr) coapPacket {
			if request.code != 2 || strings.Join(coapOptionStrings(request, 11), "/") != "rd/abc" {
				t.Errorf("unexpected update request: %#v", request)
			}
			return coapPacket{packetType: coapTypeAcknowledgement, code: 68,
				messageID: request.messageID, token: request.token}
		})
		response, err := executeLwM2M(coapTestConfig(), lwm2mTestRequest(server, "update",
			map[string]string{"location": "/rd/abc"}))
		if err != nil {
			t.Fatal(err)
		}
		if lwm2mTestBody(t, response)["code"] != "2.04" {
			t.Fatalf("body = %s", response.Body.Data)
		}
	})

	t.Run("deregister", func(t *testing.T) {
		server := lwm2mTestServer(t, func(request coapPacket, _ []byte, _ net.Addr) coapPacket {
			if request.code != 4 || strings.Join(coapOptionStrings(request, 11), "/") != "rd/abc" {
				t.Errorf("unexpected deregister request: %#v", request)
			}
			return coapPacket{packetType: coapTypeAcknowledgement, code: 66,
				messageID: request.messageID, token: request.token}
		})
		response, err := executeLwM2M(coapTestConfig(), lwm2mTestRequest(server, "deregister",
			map[string]string{"location": "/rd/abc"}))
		if err != nil {
			t.Fatal(err)
		}
		if lwm2mTestBody(t, response)["code"] != "2.02" {
			t.Fatalf("body = %s", response.Body.Data)
		}
	})
}

func TestLwM2MReadWriteTLV(t *testing.T) {
	t.Run("read", func(t *testing.T) {
		server := lwm2mTestServer(t, func(request coapPacket, _ []byte, _ net.Addr) coapPacket {
			if request.code != 1 || strings.Join(coapOptionStrings(request, 11), "/") != "3/0/13" {
				t.Errorf("unexpected read request: %#v", request)
			}
			return coapPacket{packetType: coapTypeAcknowledgement, code: 69,
				messageID: request.messageID, token: request.token,
				options: []coapOption{{number: 12, value: []byte{0x2D, 0x16}}},
				payload: []byte{0xC1, 0x0D, 0x05}}
		})
		response, err := executeLwM2M(coapTestConfig(), lwm2mTestRequest(server, "read",
			map[string]string{"object_id": "3", "instance_id": "0", "resource_id": "13"}))
		if err != nil {
			t.Fatal(err)
		}
		value := lwm2mTestBody(t, response)["value"]
		if value != float64(5) {
			t.Fatalf("value = %#v", value)
		}
	})

	t.Run("write", func(t *testing.T) {
		server := lwm2mTestServer(t, func(request coapPacket, _ []byte, _ net.Addr) coapPacket {
			if request.code != 3 || strings.Join(coapOptionStrings(request, 11), "/") != "3/0/13" {
				t.Errorf("unexpected write request: %#v", request)
			}
			if hex.EncodeToString(request.payload) != "c10d05" {
				t.Errorf("write payload = %x", request.payload)
			}
			return coapPacket{packetType: coapTypeAcknowledgement, code: 68,
				messageID: request.messageID, token: request.token}
		})
		response, err := executeLwM2M(coapTestConfig(), lwm2mTestRequest(server, "write",
			map[string]string{"object_id": "3", "instance_id": "0", "resource_id": "13", "value": "5"}))
		if err != nil {
			t.Fatal(err)
		}
		if lwm2mTestBody(t, response)["code"] != "2.04" {
			t.Fatalf("body = %s", response.Body.Data)
		}
	})
}

func TestLwM2MUnexpectedCodeValidationAndGuard(t *testing.T) {
	server := lwm2mTestServer(t, func(request coapPacket, _ []byte, _ net.Addr) coapPacket {
		return coapPacket{packetType: coapTypeAcknowledgement, code: 128,
			messageID: request.messageID, token: request.token, payload: []byte("bad")}
	})
	_, err := executeLwM2M(coapTestConfig(), lwm2mTestRequest(server, "register",
		map[string]string{"endpoint": "x"}))
	if err == nil || !strings.Contains(err.Error(), "expected 2.01, got 4.00: bad") {
		t.Fatalf("unexpected code error = %v", err)
	}

	features := coapTestConfig()
	if _, err := executeLwM2M(features, lwm2mTestRequest(server, "register", nil)); err == nil ||
		!strings.Contains(err.Error(), "endpoint is required") {
		t.Fatalf("endpoint error = %v", err)
	}
	if _, err := executeLwM2M(features, lwm2mTestRequest(server, "register",
		map[string]string{"endpoint": "x", "lifetime": "soon"})); err == nil ||
		!strings.Contains(err.Error(), "invalid lifetime") {
		t.Fatalf("lifetime error = %v", err)
	}
	if _, err := executeLwM2M(features, lwm2mTestRequest(server, "update", nil)); err == nil ||
		!strings.Contains(err.Error(), "location is required") {
		t.Fatalf("location error = %v", err)
	}
	if _, err := executeLwM2M(features, lwm2mTestRequest(server, "subscribe", nil)); err == nil ||
		!strings.Contains(err.Error(), "unsupported lwm2m operation") {
		t.Fatalf("operation error = %v", err)
	}
	if _, err := executeLwM2M(Config{}, lwm2mTestRequest(server, "register",
		map[string]string{"endpoint": "x"})); err == nil ||
		!strings.Contains(err.Error(), "private/loopback") {
		t.Fatalf("guard error = %v", err)
	}
}

func TestLwM2MTLVHelpers(t *testing.T) {
	if encoded := lwm2mEncodeResource(300, "-2"); hex.EncodeToString(encoded) != "e1012cfe" {
		t.Fatalf("wide resource = %x", encoded)
	}
	if encoded := lwm2mEncodeResource(13, "5"); hex.EncodeToString(encoded) != "c10d05" {
		t.Fatalf("narrow resource = %x", encoded)
	}
	if value := lwm2mDecodeValue([]byte("22.5"), 0); value != "22.5" {
		t.Fatalf("text value = %#v", value)
	}
	if value := lwm2mDecodeValue([]byte{0xC1, 0x0D, 0x05}, coapContentFormatTLV); value != int64(5) {
		t.Fatalf("tlv value = %#v", value)
	}
	if value := lwm2mDecodeValue([]byte{0xC1}, coapContentFormatTLV); value == nil {
		t.Fatal("truncated TLV should report an error object")
	}
}

func TestLwM2MTimeout(t *testing.T) {
	server, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	started := time.Now()
	request := lwm2mTestRequest(server, "register", map[string]string{
		"endpoint": "x", "duration_ms": "50",
	})
	_, err = executeLwM2M(coapTestConfig(), request)
	if err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("timeout error = %v", err)
	}
	if time.Since(started) > 5*time.Second {
		t.Fatalf("timeout not bounded: %v", time.Since(started))
	}
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
