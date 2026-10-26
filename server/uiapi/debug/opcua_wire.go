//go:build apidoc_debug_industrial || apidoc_debug_all

package debug

import (
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"net"
	"strconv"
	"strings"
	"time"
)

const opcuaReceiveBuffer = 65535

type opcuaMessage struct {
	kind      [3]byte
	channel   uint32
	policy    string
	hasPolicy bool
	body      []byte
}

// opcuaReadMessage reads one binary message. Multi-chunk ('C') responses are
// rejected instead of being reassembled. The ERR body is decoded here.
func opcuaReadMessage(conn net.Conn, deadline time.Time, maxBytes int) (*opcuaMessage, error) {
	header := make([]byte, 8)
	if err := opcuaReadExact(conn, header, deadline); err != nil {
		return nil, err
	}
	var kind [3]byte
	copy(kind[:], header[:3])
	if header[3] == 'C' {
		return nil, fmt.Errorf("opcua multi-chunk responses are not supported")
	}
	if header[3] != 'F' {
		return nil, fmt.Errorf("opcua message was aborted by the server")
	}
	size := int(binary.LittleEndian.Uint32(header[4:]))
	if size < 8 || size > maxBytes {
		return nil, fmt.Errorf("opcua message size %d exceeds max_bytes (%d)", size, maxBytes)
	}
	rest := make([]byte, size-8)
	if err := opcuaReadExact(conn, rest, deadline); err != nil {
		return nil, err
	}
	if string(kind[:]) == "ERR" {
		dec := opcuaDecoder{data: rest}
		status, _ := dec.u32()
		reason, _, _ := dec.string()
		return nil, fmt.Errorf("opcua error %s %s", opcuaStatusLabel(status), reason)
	}
	if string(kind[:]) == "ACK" {
		return &opcuaMessage{kind: kind, body: rest}, nil
	}
	dec := opcuaDecoder{data: rest}
	channel, err := dec.u32()
	if err != nil {
		return nil, err
	}
	message := &opcuaMessage{kind: kind, channel: channel}
	if string(kind[:]) == "OPN" {
		policy, found, err := dec.string()
		if err != nil {
			return nil, err
		}
		message.policy = policy
		message.hasPolicy = found
		if _, _, err := dec.byteString(); err != nil {
			return nil, err
		}
		if _, _, err := dec.byteString(); err != nil {
			return nil, err
		}
	} else {
		if _, err := dec.u32(); err != nil { // token id
			return nil, err
		}
	}
	if _, err := dec.u32(); err != nil { // sequence
		return nil, err
	}
	if _, err := dec.u32(); err != nil { // request id
		return nil, err
	}
	message.body = dec.rest()
	return message, nil
}

func opcuaReadExact(conn net.Conn, buf []byte, deadline time.Time) error {
	if !time.Now().Before(deadline) {
		return fmt.Errorf("opcua read timed out")
	}
	if err := conn.SetReadDeadline(deadline); err != nil {
		return fmt.Errorf("opcua read failed: %v", err)
	}
	if _, err := io.ReadFull(conn, buf); err != nil {
		if ne, ok := err.(net.Error); ok && ne.Timeout() {
			return fmt.Errorf("opcua read timed out")
		}
		return fmt.Errorf("opcua read failed: %v", err)
	}
	return nil
}

func opcuaEncodeHEL(endpoint string, maxMessage uint32) []byte {
	if maxMessage < 8192 {
		maxMessage = 8192
	}
	bodyLen := 5*4 + 4 + len(endpoint)
	out := []byte{'H', 'E', 'L', 'F'}
	out = binary.LittleEndian.AppendUint32(out, uint32(8+bodyLen))
	out = binary.LittleEndian.AppendUint32(out, 0) // protocolVersion
	out = binary.LittleEndian.AppendUint32(out, opcuaReceiveBuffer)
	out = binary.LittleEndian.AppendUint32(out, opcuaReceiveBuffer)
	out = binary.LittleEndian.AppendUint32(out, maxMessage)
	out = binary.LittleEndian.AppendUint32(out, 1) // maxChunkCount: no multi-chunk
	return opcuaPutString(out, endpoint)
}

func opcuaEncodeOPN(channel uint32, policy string, seq, request uint32, body []byte) []byte {
	out := []byte{'O', 'P', 'N', 'F'}
	out = binary.LittleEndian.AppendUint32(out, uint32(32+len(policy)+len(body)))
	out = binary.LittleEndian.AppendUint32(out, channel)
	out = opcuaPutString(out, policy)
	out = opcuaPutBytes(out, nil) // senderCertificate
	out = opcuaPutBytes(out, nil) // receiverCertificateThumbprint
	out = binary.LittleEndian.AppendUint32(out, seq)
	out = binary.LittleEndian.AppendUint32(out, request)
	return append(out, body...)
}

func opcuaEncodeMSG(kind [3]byte, channel, token, seq, request uint32, body []byte) []byte {
	out := []byte{kind[0], kind[1], kind[2], 'F'}
	out = binary.LittleEndian.AppendUint32(out, uint32(24+len(body)))
	out = binary.LittleEndian.AppendUint32(out, channel)
	out = binary.LittleEndian.AppendUint32(out, token)
	out = binary.LittleEndian.AppendUint32(out, seq)
	out = binary.LittleEndian.AppendUint32(out, request)
	return append(out, body...)
}

func opcuaPutRequestHeader(out []byte, auth []byte, handle uint32) []byte {
	out = append(out, auth...)
	out = binary.LittleEndian.AppendUint64(out, 0) // timestamp
	out = binary.LittleEndian.AppendUint32(out, handle)
	out = binary.LittleEndian.AppendUint32(out, 0)     // returnDiagnostics
	out = opcuaPutBytes(out, nil)                      // auditEntryId
	out = binary.LittleEndian.AppendUint32(out, 10000) // timeoutHint
	return opcuaPutExtensionObject(out, 0, nil)        // additionalHeader null
}

func opcuaEncodeVariant(out []byte, value string) []byte {
	if value == "true" || value == "false" {
		boolean := byte(0)
		if value == "true" {
			boolean = 1
		}
		out = append(out, 1, boolean)
		return out
	}
	if integer, err := strconv.ParseInt(value, 10, 64); err == nil {
		if integer >= -2147483648 && integer <= 2147483647 {
			out = append(out, 6)
			out = binary.LittleEndian.AppendUint32(out, uint32(int32(integer)))
		} else {
			out = append(out, 8)
			out = binary.LittleEndian.AppendUint64(out, uint64(integer))
		}
		return out
	}
	if number, err := strconv.ParseFloat(value, 64); err == nil {
		out = append(out, 11)
		out = binary.LittleEndian.AppendUint64(out, math.Float64bits(number))
		return out
	}
	out = append(out, 12)
	return opcuaPutString(out, value)
}

func opcuaPutNodeID(out []byte, id uint32) []byte {
	if id <= 0xFF {
		return append(out, 0x00, byte(id))
	}
	if id <= 0xFFFF {
		out = append(out, 0x01, 0x00)
		return binary.LittleEndian.AppendUint16(out, uint16(id))
	}
	out = append(out, 0x02, 0x00, 0x00)
	return binary.LittleEndian.AppendUint32(out, id)
}

func opcuaPutNodeIDString(out []byte, value string) ([]byte, error) {
	namespace := uint16(0)
	rest := value
	if after, ok := strings.CutPrefix(value, "ns="); ok {
		namespaceText, remainder, found := strings.Cut(after, ";")
		if !found {
			return nil, fmt.Errorf("node_id `ns=` must be followed by `;`")
		}
		parsed, err := strconv.ParseUint(namespaceText, 10, 16)
		if err != nil {
			return nil, fmt.Errorf("invalid node_id namespace")
		}
		namespace = uint16(parsed)
		rest = remainder
	}
	if numeric, ok := strings.CutPrefix(rest, "i="); ok {
		id, err := strconv.ParseUint(numeric, 10, 32)
		if err != nil {
			return nil, fmt.Errorf("invalid numeric node_id")
		}
		switch {
		case namespace == 0 && id <= 0xFF:
			out = append(out, 0x00, byte(id))
		case namespace <= 0xFF && id <= 0xFFFF:
			out = append(out, 0x01, byte(namespace))
			out = binary.LittleEndian.AppendUint16(out, uint16(id))
		default:
			out = append(out, 0x02)
			out = binary.LittleEndian.AppendUint16(out, namespace)
			out = binary.LittleEndian.AppendUint32(out, uint32(id))
		}
		return out, nil
	}
	if text, ok := strings.CutPrefix(rest, "s="); ok {
		out = append(out, 0x03)
		out = binary.LittleEndian.AppendUint16(out, namespace)
		return opcuaPutString(out, text), nil
	}
	return nil, fmt.Errorf("node_id must look like `i=85`, `ns=2;i=5` or `s=name`")
}

func opcuaPutLocalizedText(out []byte, text string) []byte {
	out = append(out, 0x02) // only the text field
	return opcuaPutString(out, text)
}

func opcuaPutExtensionObject(out []byte, typeID uint32, body []byte) []byte {
	out = opcuaPutNodeID(out, typeID)
	if typeID == 0 {
		return append(out, 0x00)
	}
	out = append(out, 0x01) // Body
	out = binary.LittleEndian.AppendUint32(out, uint32(len(body)))
	return append(out, body...)
}

func opcuaPutString(out []byte, value string) []byte {
	return opcuaPutBytes(out, []byte(value))
}

func opcuaPutBytes(out []byte, value []byte) []byte {
	if value == nil {
		return binary.LittleEndian.AppendUint32(out, 0xFFFFFFFF)
	}
	out = binary.LittleEndian.AppendUint32(out, uint32(len(value)))
	return append(out, value...)
}
