//go:build apidoc_debug_industrial || apidoc_debug_all

package debug

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net"
	"strconv"
	"strings"
	"time"
)

const (
	bacnetDefaultPort = "47808"
	bacnetMaxFrames   = 1000
	bacnetPresentVal  = 85
)

func init() {
	RegisterExecutor("bacnet", executeBACnet)
}

var errBACnetUncorrelated = errors.New("bacnet response does not match the request")

// BACnet/IP minimal subset: Who-Is / I-Am and ReadProperty. Only direct
// BACnet/IP packets (no DNET/SNET routing) are parsed. Broadcast is limited to
// who_is, requires allow_private_networks and rejects 255.255.255.255 and
// multicast destinations.
func executeBACnet(cfg Config, req *Request) (*Response, error) {
	started := time.Now()
	host, port, err := parseProtocolTarget(req.Target, "bacnet", bacnetDefaultPort,
		"bacnet", "bacnet-ip", "bacnet/ip", "udp")
	if err != nil {
		return nil, err
	}
	addrs, err := CheckHost(cfg, host, port)
	if err != nil {
		return nil, err
	}
	broadcast := false
	if value, found := req.Opts["broadcast"]; found {
		broadcast = strings.EqualFold(value, "true") || value == "1"
	}
	if err := bacnetValidateDestination(cfg, broadcast, addrs); err != nil {
		return nil, err
	}
	portNumber, err := strconv.Atoi(port)
	if err != nil {
		return nil, fmt.Errorf("invalid target port `%s`", port)
	}
	addr := &net.UDPAddr{IP: addrs[0], Port: portNumber}
	operation := req.Opts["operation"]
	if operation == "" {
		operation = "who_is"
	}
	duration := protocolWindow(cfg, req)

	var apdu []byte
	var objectID uint32
	switch operation {
	case "who_is":
		apdu = []byte{0x10, 0x08}
	case "read_property":
		if broadcast {
			return nil, fmt.Errorf("bacnet broadcast is only valid for who_is")
		}
		objectType := protocolOptUint(req, "object_type", 8) // 8 = device
		objectInstance, err := bacnetRequiredU32(req, "object_instance")
		if err != nil {
			return nil, err
		}
		property := protocolOptUint(req, "property", bacnetPresentVal)
		if objectInstance > 0x3FFFFF {
			return nil, fmt.Errorf("bacnet object_instance must fit in 22 bits")
		}
		if objectType > 0x3FF || property > 0xFF {
			return nil, fmt.Errorf("bacnet object_type <= 1023 and property <= 255 are required")
		}
		objectID = uint32(objectType)<<22 | uint32(objectInstance)
		// Confirmed-Request: PDU header, invoke 0, ReadProperty, opening tag 0.
		apdu = []byte{0x00, 0x00, 0x00, 0x0C, 0xC4}
		apdu = binary.BigEndian.AppendUint32(apdu, objectID)
		apdu = append(apdu, 0x19, byte(property))
	default:
		return nil, fmt.Errorf("unsupported bacnet operation `%s`; use who_is or read_property", operation)
	}
	packet, err := bacnetEncodeBVLC(broadcast, apdu)
	if err != nil {
		return nil, err
	}

	socket, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4zero, Port: 0})
	if err != nil {
		return nil, fmt.Errorf("bacnet bind failed: %v", err)
	}
	defer socket.Close()
	// Unconnected socket: I-Am replies to a broadcast Who-Is come from each
	// device's unicast address.
	if _, err := socket.WriteToUDP(packet, addr); err != nil {
		return nil, fmt.Errorf("bacnet send failed: %v", err)
	}

	deadline := time.Now().Add(duration)
	total := 0
	frames := 0
	devices := []any{}
	var propertyValue any
	foundProperty := false
	for frames < bacnetMaxFrames {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			break
		}
		if err := socket.SetReadDeadline(time.Now().Add(remaining)); err != nil {
			return nil, fmt.Errorf("bacnet deadline failed: %v", err)
		}
		buf := make([]byte, 1500)
		n, peer, err := socket.ReadFromUDP(buf)
		if err != nil {
			if ne, ok := err.(net.Error); ok && ne.Timeout() {
				break
			}
			return nil, fmt.Errorf("bacnet recv failed: %v", err)
		}
		total += n
		if total > cfg.Limit() {
			return nil, fmt.Errorf("bacnet response exceeds max_bytes (%d)", cfg.Limit())
		}
		frames++
		responseAPDU, err := bacnetDecodeBVLC(buf[:n])
		if err != nil {
			return nil, err
		}
		if responseAPDU == nil {
			continue
		}
		if operation == "who_is" {
			if instance, ok := bacnetParseIAm(responseAPDU); ok {
				devices = append(devices, map[string]any{
					"instance": instance,
					"address":  peer.String(),
				})
			}
		} else if len(responseAPDU) > 0 {
			// Ignore non-response PDUs (e.g. other broadcast traffic on the port).
			switch responseAPDU[0] {
			case 0x30, 0x50, 0x60, 0x70:
				if len(responseAPDU) < 2 || responseAPDU[1] != 0 {
					continue // uncorrelated traffic on the shared BACnet port
				}
				propertyValue, err = bacnetParseReadPropertyAck(responseAPDU, objectID)
				if errors.Is(err, errBACnetUncorrelated) {
					continue
				}
				if err != nil {
					return nil, err
				}
				foundProperty = true
			}
		}
		if foundProperty {
			break
		}
	}

	var body any
	if operation == "who_is" {
		if len(devices) == 0 {
			return nil, fmt.Errorf("bacnet who_is timed out after %dms without I-Am", duration.Milliseconds())
		}
		body = map[string]any{"devices": devices}
	} else {
		if !foundProperty {
			return nil, fmt.Errorf("bacnet read_property timed out after %dms", duration.Milliseconds())
		}
		body = map[string]any{"value": propertyValue}
	}
	encoded, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("encode response: %v", err)
	}
	return &Response{
		Status:  0,
		MS:      time.Since(started).Milliseconds(),
		Headers: map[string]string{"x-peer": addr.String()},
		Body:    FromBytes(encoded),
	}, nil
}

func bacnetValidateDestination(cfg Config, broadcast bool, addrs []net.IP) error {
	if broadcast && !cfg.AllowPrivateNetworks {
		return fmt.Errorf("bacnet broadcast requires DebugConfig.allow_private_networks = true")
	}
	for _, ip := range addrs {
		if v4 := ip.To4(); v4 != nil && v4.Equal(net.IPv4bcast) {
			return fmt.Errorf("255.255.255.255 is never a valid BACnet target")
		}
		if ip.IsMulticast() {
			return fmt.Errorf("BACnet/IP multicast targets are not supported")
		}
	}
	return nil
}

// bacnetEncodeBVLC frames `81 0A/0B len(BE) NPDU APDU`; length includes the
// four-byte BVLC header.
func bacnetEncodeBVLC(broadcast bool, apdu []byte) ([]byte, error) {
	npdu := []byte{0x01, 0x00} // version 1, control 0 (direct, no routing info)
	npdu = append(npdu, apdu...)
	length := 4 + len(npdu)
	if length > 0xFFFF {
		return nil, fmt.Errorf("bacnet packet too large")
	}
	header := []byte{0x81, 0x0A}
	if broadcast {
		header[1] = 0x0B
	}
	out := binary.BigEndian.AppendUint16(header, uint16(length))
	return append(out, npdu...), nil
}

// bacnetDecodeBVLC validates the BVLC header and returns the APDU; non-data
// link frames and routed NPDUs return nil.
func bacnetDecodeBVLC(data []byte) ([]byte, error) {
	if len(data) < 6 || data[0] != 0x81 {
		return nil, nil
	}
	switch data[1] {
	case 0x04, 0x0A, 0x0B:
	default:
		return nil, nil
	}
	length := int(binary.BigEndian.Uint16(data[2:]))
	if length < 6 || length > len(data) {
		return nil, fmt.Errorf("invalid bacnet BVLC length")
	}
	if data[4] != 0x01 {
		return nil, fmt.Errorf("unsupported bacnet NPDU version")
	}
	if data[5]&0x30 != 0 {
		// DNET/SNET routing information: only direct BACnet/IP is handled.
		return nil, nil
	}
	return data[6:length], nil
}

func bacnetParseIAm(apdu []byte) (uint32, bool) {
	if len(apdu) < 7 || apdu[0] != 0x10 || apdu[1] != 0x00 {
		return 0, false
	}
	// Application tag context 0, length 4, then the object identifier.
	if apdu[2] != 0xC4 {
		return 0, false
	}
	objectID := binary.BigEndian.Uint32(apdu[3:7])
	if objectID>>22 != 8 { // 8 = device
		return 0, false
	}
	return objectID & 0x3FFFFF, true
}

func bacnetParseReadPropertyAck(apdu []byte, objectID uint32) (any, error) {
	if len(apdu) == 0 {
		return nil, fmt.Errorf("not a bacnet ComplexACK")
	}
	switch apdu[0] {
	case 0x30:
	case 0x50:
		return nil, fmt.Errorf("bacnet error PDU: %s", hexString(apdu))
	case 0x60:
		return nil, fmt.Errorf("bacnet reject PDU: %s", hexString(apdu))
	case 0x70:
		return nil, fmt.Errorf("bacnet abort PDU: %s", hexString(apdu))
	default:
		return nil, fmt.Errorf("not a bacnet ComplexACK")
	}
	// ComplexACK: 30 invoke 0C | C4 + object id | 19 property [| 29 array index] | 3E value 3F
	pos := 3
	if len(apdu) < 4 || apdu[2] != 0x0C || apdu[pos] != 0xC4 {
		return nil, fmt.Errorf("unsupported bacnet ComplexACK shape")
	}
	if len(apdu) < pos+5 || binary.BigEndian.Uint32(apdu[pos+1:pos+5]) != objectID {
		return nil, errBACnetUncorrelated
	}
	pos += 5
	if pos >= len(apdu) || apdu[pos] != 0x19 {
		return nil, fmt.Errorf("bacnet ComplexACK is missing property id")
	}
	pos += 2
	if pos < len(apdu) && apdu[pos] == 0x29 {
		pos += 2 // array index
	}
	if pos < len(apdu) && apdu[pos] == 0x3E {
		pos++ // opening tag
	}
	value, _, err := bacnetParseAppValue(apdu[pos:])
	if err != nil {
		return nil, err
	}
	return value, nil
}

// bacnetParseAppValue decodes one application-tagged value.
func bacnetParseAppValue(data []byte) (any, int, error) {
	if len(data) == 0 {
		return nil, 0, fmt.Errorf("empty bacnet value")
	}
	tag := data[0]
	tagNumber := tag >> 4
	if tagNumber == 0x0F {
		return nil, 0, fmt.Errorf("bacnet extended tag numbers are not supported")
	}
	header := 1
	length := int(tag & 0x0F)
	if length == 5 {
		if len(data) < 2 {
			return nil, 0, fmt.Errorf("truncated bacnet extended length")
		}
		length = int(data[1])
		header = 2
	} else if length > 5 {
		return nil, 0, fmt.Errorf("invalid bacnet tag length code")
	}
	// Boolean values live in the length nibble and have no data bytes.
	if tagNumber == 1 {
		return length == 1, header, nil
	}
	if header+length > len(data) {
		return nil, 0, fmt.Errorf("truncated bacnet value payload")
	}
	value := data[header : header+length]
	unsigned := func(bytes []byte) (uint32, error) {
		if len(bytes) > 4 {
			return 0, fmt.Errorf("bacnet unsigned value longer than 4 bytes")
		}
		var out uint32
		for _, b := range bytes {
			out = out<<8 | uint32(b)
		}
		return out, nil
	}
	var decoded any
	var err error
	switch tagNumber {
	case 0:
		decoded = nil
	case 2, 9:
		decoded, err = unsigned(value)
	case 3:
		if len(value) == 0 || len(value) > 4 {
			return nil, 0, fmt.Errorf("bacnet signed value must be 1..=4 bytes")
		}
		var raw uint32
		raw, err = unsigned(value)
		if err == nil {
			shift := uint(32 - 8*len(value))
			decoded = int32(raw<<shift) >> shift
		}
	case 4:
		if len(value) != 4 {
			return nil, 0, fmt.Errorf("bacnet REAL must be 4 bytes")
		}
		decoded = protocolJSONFloat32(math.Float32frombits(binary.BigEndian.Uint32(value)))
	case 5:
		if len(value) != 8 {
			return nil, 0, fmt.Errorf("bacnet DOUBLE must be 8 bytes")
		}
		decoded = protocolJSONFloat(math.Float64frombits(binary.BigEndian.Uint64(value)))
	case 6, 8:
		decoded = map[string]any{"encoding": "hex", "data": hexString(value)}
	case 7:
		charset := byte(0)
		if len(value) > 0 {
			charset = value[0]
		}
		if charset != 0 {
			return nil, 0, fmt.Errorf("unsupported bacnet string charset %d", charset)
		}
		text := []byte{}
		if len(value) > 1 {
			text = value[1:]
		}
		decoded = strings.ToValidUTF8(string(text), "�")
	default:
		decoded = map[string]any{"encoding": "hex", "data": hexString(value)}
	}
	if err != nil {
		return nil, 0, err
	}
	return decoded, header + length, nil
}

func bacnetRequiredU32(req *Request, key string) (uint64, error) {
	value, found := req.Opts[key]
	if !found {
		return 0, fmt.Errorf("bacnet %s is required", key)
	}
	parsed, err := strconv.ParseUint(value, 10, 32)
	if err != nil {
		return 0, fmt.Errorf("invalid bacnet %s", key)
	}
	return parsed, nil
}
