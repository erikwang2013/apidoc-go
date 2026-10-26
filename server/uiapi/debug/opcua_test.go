//go:build apidoc_debug_industrial || apidoc_debug_all

package debug

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"io"
	"math"
	"net"
	"strconv"
	"strings"
	"testing"
	"time"
)

const opcuaTestPolicy = "http://opcfoundation.org/UA/SecurityPolicy#None"

func opcuaTestRequest(target string, opts map[string]string) *Request {
	return &Request{Kind: "opcua", Target: "opc.tcp://" + target, Opts: opts, TimeoutMS: 2000}
}

func opcuaTestNID(id uint32) []byte {
	if id <= 0xFF {
		return []byte{0x00, byte(id)}
	}
	out := []byte{0x01, 0x00}
	return binary.LittleEndian.AppendUint16(out, uint16(id))
}

func opcuaTestPutString(out []byte, text string) []byte {
	out = binary.LittleEndian.AppendUint32(out, uint32(len(text)))
	return append(out, text...)
}

func opcuaTestPutNull(out []byte) []byte {
	return binary.LittleEndian.AppendUint32(out, 0xFFFFFFFF)
}

func opcuaTestResponseHeader(status uint32) []byte {
	out := binary.LittleEndian.AppendUint64(nil, 0) // timestamp
	out = binary.LittleEndian.AppendUint32(out, 1)  // request handle
	out = binary.LittleEndian.AppendUint32(out, status)
	out = append(out, 0) // diagnostic info
	out = binary.LittleEndian.AppendUint32(out, 0xFFFFFFFF)
	return append(out, 0x00, 0x00, 0x00) // additional header null
}

// opcuaTestFrame builds MSG/OPN-style frames with channel, token, sequence and
// request id all 1.
func opcuaTestFrame(kind string, body []byte) []byte {
	out := []byte{kind[0], kind[1], kind[2], 'F'}
	out = binary.LittleEndian.AppendUint32(out, uint32(24+len(body)))
	out = binary.LittleEndian.AppendUint32(out, 1)
	out = binary.LittleEndian.AppendUint32(out, 1)
	out = binary.LittleEndian.AppendUint32(out, 1)
	out = binary.LittleEndian.AppendUint32(out, 1)
	return append(out, body...)
}

func opcuaTestACK() []byte {
	out := []byte{'A', 'C', 'K', 'F'}
	out = binary.LittleEndian.AppendUint32(out, 24)
	return append(out, make([]byte, 16)...)
}

func opcuaTestOpenChannelBody() []byte {
	body := opcuaTestNID(446)
	body = append(body, opcuaTestResponseHeader(0)...)
	body = binary.LittleEndian.AppendUint32(body, 0) // server protocol version
	body = binary.LittleEndian.AppendUint32(body, 1) // channel id
	body = binary.LittleEndian.AppendUint32(body, 1) // token id
	body = binary.LittleEndian.AppendUint64(body, 0) // created at
	return binary.LittleEndian.AppendUint32(body, 3600000)
}

func opcuaTestOPNResponse(body []byte) []byte {
	out := []byte{'O', 'P', 'N', 'F'}
	out = binary.LittleEndian.AppendUint32(out, uint32(32+len(opcuaTestPolicy)+len(body)))
	out = binary.LittleEndian.AppendUint32(out, 1)
	out = opcuaTestPutString(out, opcuaTestPolicy)
	out = opcuaTestPutNull(out) // sender certificate
	out = opcuaTestPutNull(out) // receiver thumbprint
	out = binary.LittleEndian.AppendUint32(out, 1)
	out = binary.LittleEndian.AppendUint32(out, 1)
	return append(out, body...)
}

func opcuaTestCreateSessionBody() []byte {
	body := opcuaTestNID(461)
	body = append(body, opcuaTestResponseHeader(0)...)
	body = append(body, opcuaTestNID(100)...)
	return append(body, opcuaTestNID(200)...)
}

func opcuaTestReadMessage(conn net.Conn) (string, []byte, error) {
	header := make([]byte, 8)
	if _, err := io.ReadFull(conn, header); err != nil {
		return "", nil, err
	}
	size := int(binary.LittleEndian.Uint32(header[4:]))
	rest := make([]byte, size-8)
	if _, err := io.ReadFull(conn, rest); err != nil {
		return "", nil, err
	}
	kind := string(header[:3])
	switch kind {
	case "HEL":
		return kind, rest, nil
	case "OPN":
		pos := 4
		policyLen := int(int32(binary.LittleEndian.Uint32(rest[pos:])))
		pos += 4 + max(policyLen, 0)
		for i := 0; i < 2; i++ {
			length := int(int32(binary.LittleEndian.Uint32(rest[pos:])))
			pos += 4 + max(length, 0)
		}
		return kind, rest[pos+8:], nil
	case "MSG":
		return kind, rest[16:], nil
	default:
		return kind, rest, nil
	}
}

// opcuaTestOpenSession performs the HEL/ACK, OPN, CreateSession and
// ActivateSession handshake, returning the ActivateSession request body.
func opcuaTestOpenSession(t *testing.T, conn net.Conn, activateStatus uint32) []byte {
	t.Helper()
	if kind, _, err := opcuaTestReadMessage(conn); err != nil || kind != "HEL" {
		t.Fatalf("HEL = %q %v", kind, err)
	}
	if _, err := conn.Write(opcuaTestACK()); err != nil {
		t.Fatalf("ACK: %v", err)
	}
	if kind, _, err := opcuaTestReadMessage(conn); err != nil || kind != "OPN" {
		t.Fatalf("OPN = %q %v", kind, err)
	}
	if _, err := conn.Write(opcuaTestOPNResponse(opcuaTestOpenChannelBody())); err != nil {
		t.Fatalf("OPN response: %v", err)
	}
	kind, body, err := opcuaTestReadMessage(conn)
	if err != nil || kind != "MSG" || !bytes.HasPrefix(body, opcuaTestNID(461)) {
		t.Fatalf("CreateSession = %q %x %v", kind, body, err)
	}
	if _, err := conn.Write(opcuaTestFrame("MSG", opcuaTestCreateSessionBody())); err != nil {
		t.Fatalf("CreateSession response: %v", err)
	}
	kind, body, err = opcuaTestReadMessage(conn)
	if err != nil || kind != "MSG" || !bytes.HasPrefix(body, opcuaTestNID(467)) {
		t.Fatalf("ActivateSession = %q %x %v", kind, body, err)
	}
	response := opcuaTestNID(467)
	response = append(response, opcuaTestResponseHeader(activateStatus)...)
	if _, err := conn.Write(opcuaTestFrame("MSG", response)); err != nil {
		t.Fatalf("ActivateSession response: %v", err)
	}
	return body
}

func TestOPCUAReadBrowseWrite(t *testing.T) {
	t.Run("read", func(t *testing.T) {
		address := startTCPServer(t, func(conn net.Conn) {
			opcuaTestOpenSession(t, conn, 0)
			kind, body, err := opcuaTestReadMessage(conn)
			if err != nil || kind != "MSG" || !bytes.HasPrefix(body, opcuaTestNID(631)) {
				t.Errorf("ReadRequest = %q %x %v", kind, body, err)
				return
			}
			response := opcuaTestNID(631)
			response = append(response, opcuaTestResponseHeader(0)...)
			response = binary.LittleEndian.AppendUint32(response, 1) // results
			response = append(response, 0x01, 11)                    // value-only Double
			response = binary.LittleEndian.AppendUint64(response, math.Float64bits(22.5))
			_, _ = conn.Write(opcuaTestFrame("MSG", response))
		})
		result, err := executeOPCUA(industrialTestConfig(), opcuaTestRequest(address,
			map[string]string{"operation": "read", "node_id": "ns=2;i=5"}))
		if err != nil {
			t.Fatal(err)
		}
		body := decodeBody(t, result)
		if body["value"] != 22.5 || body["status"] != "Good" ||
			result.Headers["x-opcua-status"] != "Good" {
			t.Fatalf("body = %#v headers = %#v", body, result.Headers)
		}
	})

	t.Run("browse", func(t *testing.T) {
		address := startTCPServer(t, func(conn net.Conn) {
			opcuaTestOpenSession(t, conn, 0)
			kind, body, err := opcuaTestReadMessage(conn)
			if err != nil || kind != "MSG" || !bytes.HasPrefix(body, opcuaTestNID(527)) {
				t.Errorf("BrowseRequest = %q %x %v", kind, body, err)
				return
			}
			response := opcuaTestNID(527)
			response = append(response, opcuaTestResponseHeader(0)...)
			response = binary.LittleEndian.AppendUint32(response, 1) // results
			response = binary.LittleEndian.AppendUint32(response, 0) // status
			response = opcuaTestPutNull(response)                    // continuation point
			response = binary.LittleEndian.AppendUint32(response, 1) // references
			response = append(response, opcuaTestNID(31)...)
			response = append(response, 1)                           // is forward
			response = append(response, 0x01, 0x02, 0x05, 0x00)      // ns=2;i=5
			response = binary.LittleEndian.AppendUint16(response, 2) // browse namespace
			response = opcuaTestPutString(response, "Demo")
			response = append(response, 0x02) // localized text
			response = opcuaTestPutString(response, "Demo Value")
			response = binary.LittleEndian.AppendUint32(response, 2) // Variable
			response = append(response, opcuaTestNID(0)...)
			_, _ = conn.Write(opcuaTestFrame("MSG", response))
		})
		result, err := executeOPCUA(industrialTestConfig(), opcuaTestRequest(address,
			map[string]string{"operation": "browse", "node_id": "i=85"}))
		if err != nil {
			t.Fatal(err)
		}
		references := decodeBody(t, result)["references"].([]any)
		if len(references) != 1 {
			t.Fatalf("references = %#v", references)
		}
		reference := references[0].(map[string]any)
		if reference["node_id"] != "ns=2;i=5" || reference["browse_name"] != "ns=2;Demo" ||
			reference["display_name"] != "Demo Value" || reference["node_class"] != float64(2) {
			t.Fatalf("reference = %#v", reference)
		}
	})

	t.Run("write", func(t *testing.T) {
		address := startTCPServer(t, func(conn net.Conn) {
			opcuaTestOpenSession(t, conn, 0)
			kind, body, err := opcuaTestReadMessage(conn)
			if err != nil || kind != "MSG" || !bytes.HasPrefix(body, opcuaTestNID(673)) {
				t.Errorf("WriteRequest = %q %x %v", kind, body, err)
				return
			}
			if !bytes.Contains(body, []byte{0x01, 0x02, 0x05, 0x00}) {
				t.Errorf("write target missing from %x", body)
			}
			variant := []byte{0x01, 11}
			variant = binary.LittleEndian.AppendUint64(variant, math.Float64bits(42.5))
			if !bytes.HasSuffix(body, variant) {
				t.Errorf("write value missing from %x", body)
			}
			response := opcuaTestNID(673)
			response = append(response, opcuaTestResponseHeader(0)...)
			response = binary.LittleEndian.AppendUint32(response, 1)
			response = binary.LittleEndian.AppendUint32(response, 0)
			_, _ = conn.Write(opcuaTestFrame("MSG", response))
		})
		result, err := executeOPCUA(industrialTestConfig(), opcuaTestRequest(address,
			map[string]string{"operation": "write", "node_id": "ns=2;i=5", "value": "42.5"}))
		if err != nil {
			t.Fatal(err)
		}
		if decodeBody(t, result)["status"] != "Good" {
			t.Fatalf("body = %s", result.Body.Data)
		}
	})
}

func TestOPCUAAuthFailureDoesNotLeakPassword(t *testing.T) {
	address := startTCPServer(t, func(conn net.Conn) {
		body := opcuaTestOpenSession(t, conn, 0x80210000)
		if !bytes.Contains(body, []byte("hunter2")) {
			t.Errorf("password was not sent: %x", body)
		}
	})
	_, err := executeOPCUA(industrialTestConfig(), opcuaTestRequest(address,
		map[string]string{"operation": "read", "username": "alice", "password": "hunter2"}))
	if err == nil || !strings.Contains(err.Error(), "BadIdentityTokenRejected") {
		t.Fatalf("auth error = %v", err)
	}
	if strings.Contains(err.Error(), "hunter2") {
		t.Fatalf("password leaked: %v", err)
	}
}

func TestOPCUAEndpointURLBracketsIPv6(t *testing.T) {
	listener, err := net.Listen("tcp6", "[::1]:0")
	if err != nil {
		t.Skipf("IPv6 loopback unavailable: %v", err)
	}
	defer listener.Close()
	port := strconv.Itoa(listener.Addr().(*net.TCPAddr).Port)
	endpoint := make(chan string, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		kind, body, err := opcuaTestReadMessage(conn)
		if err != nil || kind != "HEL" {
			return
		}
		pos := 20 // protocol version, receive/send buffers, max message, max chunks
		size := int(binary.LittleEndian.Uint32(body[pos:]))
		endpoint <- string(body[pos+4 : pos+4+size])
	}()
	_, _ = executeOPCUA(industrialTestConfig(), &Request{
		Kind: "opcua", Target: "opc.tcp://[::1]:" + port, TimeoutMS: 500,
	})
	select {
	case got := <-endpoint:
		if want := "opc.tcp://[::1]:" + port; got != want {
			t.Fatalf("HEL endpoint = %q, want %q", got, want)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("HEL endpoint was not captured")
	}
}

func TestOPCUANullSecurityPolicyRejected(t *testing.T) {
	address := startTCPServer(t, func(conn net.Conn) {
		if kind, _, err := opcuaTestReadMessage(conn); err != nil || kind != "HEL" {
			return
		}
		if _, err := conn.Write(opcuaTestACK()); err != nil {
			return
		}
		if kind, _, err := opcuaTestReadMessage(conn); err != nil || kind != "OPN" {
			return
		}
		body := opcuaTestOpenChannelBody()
		out := []byte{'O', 'P', 'N', 'F'}
		out = binary.LittleEndian.AppendUint32(out, uint32(32+len(body)))
		out = binary.LittleEndian.AppendUint32(out, 1)
		out = opcuaTestPutNull(out) // null SecurityPolicyUri
		out = opcuaTestPutNull(out) // sender certificate
		out = opcuaTestPutNull(out) // receiver thumbprint
		out = binary.LittleEndian.AppendUint32(out, 1)
		out = binary.LittleEndian.AppendUint32(out, 1)
		_, _ = conn.Write(append(out, body...))
	})
	_, err := executeOPCUA(industrialTestConfig(), opcuaTestRequest(address, nil))
	if err == nil || !strings.Contains(err.Error(), "only None is supported") {
		t.Fatalf("null policy error = %v", err)
	}
}

func TestOPCUAMultiChunkRejected(t *testing.T) {
	address := startTCPServer(t, func(conn net.Conn) {
		opcuaTestOpenSession(t, conn, 0)
		_, _, _ = opcuaTestReadMessage(conn)
		chunk := []byte{'M', 'S', 'G', 'C'}
		chunk = binary.LittleEndian.AppendUint32(chunk, 24)
		chunk = append(chunk, make([]byte, 16)...)
		_, _ = conn.Write(chunk)
	})
	_, err := executeOPCUA(industrialTestConfig(), opcuaTestRequest(address, nil))
	if err == nil || !strings.Contains(err.Error(), "multi-chunk") {
		t.Fatalf("multi-chunk error = %v", err)
	}
}

func TestOPCUAValidationAndGuard(t *testing.T) {
	features := industrialTestConfig()
	cases := []struct {
		opts map[string]string
		want string
	}{
		{map[string]string{"operation": "subscribe"}, "unsupported opcua operation"},
		{map[string]string{"operation": "write"}, "`value` is required"},
		{map[string]string{"operation": "read", "node_id": "x=1"}, "node_id must look like"},
		{map[string]string{"operation": "read", "node_id": "ns=x;i=1"}, "invalid node_id namespace"},
		{map[string]string{"operation": "read", "node_id": "ns=2;i=x"}, "invalid numeric node_id"},
	}
	for _, test := range cases {
		_, err := executeOPCUA(features, opcuaTestRequest("127.0.0.1:4840", test.opts))
		if err == nil || !strings.Contains(err.Error(), test.want) {
			t.Errorf("opts %#v: error %v, want %q", test.opts, err, test.want)
		}
	}
	if _, err := executeOPCUA(Config{}, opcuaTestRequest("127.0.0.1:4840", nil)); err == nil ||
		!strings.Contains(err.Error(), "private/loopback") {
		t.Fatalf("guard error = %v", err)
	}
}

func TestOPCUACodec(t *testing.T) {
	cases := []struct {
		nodeID string
		hex    string
	}{
		{"i=85", "0055"},
		{"ns=2;i=5", "01020500"},
		{"ns=2;i=70000", "020200" + "70110100"},
		{"ns=3;s=Demo", "0303000400000044656d6f"},
	}
	for _, test := range cases {
		encoded, err := opcuaPutNodeIDString(nil, test.nodeID)
		if err != nil || hex.EncodeToString(encoded) != test.hex {
			t.Errorf("node %q = %x, %v; want %s", test.nodeID, encoded, err, test.hex)
		}
	}

	for _, test := range []struct {
		value string
		hex   string
	}{
		{"true", "0101"},
		{"42", "062a000000"},
		{"hello", "0c0500000068656c6c6f"},
	} {
		if encoded := opcuaEncodeVariant(nil, test.value); hex.EncodeToString(encoded) != test.hex {
			t.Errorf("variant %q = %x, want %s", test.value, encoded, test.hex)
		}
	}

	array := []byte{0x01, 0x8B}
	array = binary.LittleEndian.AppendUint32(array, 2)
	array = binary.LittleEndian.AppendUint64(array, math.Float64bits(1.5))
	array = binary.LittleEndian.AppendUint64(array, math.Float64bits(2.5))
	value, err := opcuaDecodeDataValue(&opcuaDecoder{data: array})
	if err != nil {
		t.Fatal(err)
	}
	items := value.([]any)
	if len(items) != 2 || items[0] != 1.5 || items[1] != 2.5 {
		t.Fatalf("array = %#v", items)
	}

	_, err = opcuaDecodeDataValue(&opcuaDecoder{data: []byte{0x02, 0, 0, 0x35, 0x80}})
	if err == nil || !strings.Contains(err.Error(), "BadNotConnected") {
		t.Fatalf("status error = %v", err)
	}
	if err := opcuaCheckResponseHeader(&opcuaDecoder{data: opcuaTestResponseHeader(0x801F0000)}); err == nil || !strings.Contains(err.Error(), "BadUserAccessDenied") {
		t.Fatalf("header error = %v", err)
	}
}
