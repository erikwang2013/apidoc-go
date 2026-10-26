//go:build apidoc_debug_media || apidoc_debug_all

package debug

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

func init() {
	RegisterExecutor("rtcp", executeRTCP)
}

// rtcpPacket is the RFC 3550 subset the debug executor reports. SDES/BYE text
// and SR/RR report blocks stay opaque.
type rtcpPacket struct {
	version    uint8
	padding    bool
	count      uint8
	packetType uint8
	length     int
	ssrc       uint32
	hasSSRC    bool
	text       string
	hasText    bool
}

// encodeRTCPPacket builds one packet. SDES/BYE carry a single SSRC and text;
// SR/RR and other types pass the body through untouched.
func encodeRTCPPacket(packetType uint8, ssrc uint32, payload []byte) []byte {
	count := byte(0)
	if packetType == 202 || packetType == 203 {
		count = 1
	}
	body := payload
	switch packetType {
	case 202: // SDES: one CNAME item (type 1, 8-bit length, text)
		text := payload
		if len(text) > 255 {
			text = text[:255]
		}
		body = append([]byte{1, byte(len(text))}, text...)
	case 203: // BYE: 8-bit reason length, then the reason
		if len(payload) > 0 {
			text := payload
			if len(text) > 255 {
				text = text[:255]
			}
			body = append([]byte{byte(len(text))}, text...)
		}
	}
	padded := (len(body) + 3) &^ 3
	// The header length field counts 32-bit words minus one and includes the
	// SSRC word.
	length := 1 + padded/4
	out := make([]byte, 8, 8+padded)
	out[0] = 0x80 | count
	out[1] = packetType
	out[2], out[3] = byte(length>>8), byte(length)
	out[4], out[5], out[6], out[7] = byte(ssrc>>24), byte(ssrc>>16), byte(ssrc>>8), byte(ssrc)
	out = append(out, body...)
	return append(out, make([]byte, padded-len(body))...)
}

func decodeRTCPPacket(bytes []byte) (rtcpPacket, error) {
	var packet rtcpPacket
	if len(bytes) < 4 {
		return packet, fmt.Errorf("rtcp packet too short: %d bytes", len(bytes))
	}
	length := ((int(bytes[2])<<8 | int(bytes[3])) + 1) * 4
	if length > len(bytes) {
		return packet, fmt.Errorf("rtcp length field says %d bytes but packet has %d", length, len(bytes))
	}
	packet.length = length
	packet.count = bytes[0] & 0x1f
	packet.packetType = bytes[1]
	packet.version = bytes[0] >> 6
	packet.padding = bytes[0]&0x20 != 0
	if length >= 8 {
		packet.ssrc = uint32(bytes[4])<<24 | uint32(bytes[5])<<16 | uint32(bytes[6])<<8 | uint32(bytes[7])
		packet.hasSSRC = true
	}
	switch {
	case packet.packetType == 202 && length >= 10:
		// SDES: after the SSRC, the first item is type, 8-bit length, text.
		textLen := int(bytes[9])
		if 10+textLen <= length {
			packet.text = string(bytes[10 : 10+textLen])
			packet.hasText = true
		}
	case packet.packetType == 203 && packet.hasSSRC:
		// BYE: after count SSRCs comes the reason length byte and text.
		at := 4 + int(packet.count)*4
		textLen := 0
		if at < len(bytes) {
			textLen = int(bytes[at])
		}
		if at+1+textLen <= length {
			packet.text = string(bytes[at+1 : at+1+textLen])
			packet.hasText = true
		}
	}
	return packet, nil
}

func executeRTCP(cfg Config, req *Request) (*Response, error) {
	started := time.Now()
	host, port, err := parseProtocolTarget(req.Target, "rtcp", "5005", "rtcp", "udp")
	if err != nil {
		return nil, err
	}
	packetType := byte(201)
	switch name := strings.ToLower(req.Opts["packet_type"]); name {
	case "", "rr":
	case "sr":
		packetType = 200
	case "sdes":
		packetType = 202
	case "bye":
		packetType = 203
	default:
		value, err := strconv.ParseUint(name, 10, 8)
		if err != nil {
			return nil, fmt.Errorf("unknown rtcp packet_type `%s`; use sr/rr/sdes/bye", name)
		}
		packetType = byte(value)
	}
	ssrc := uint32(protocolOptUint(req, "ssrc", 0))
	payload, err := req.Body.Decode()
	if err != nil {
		return nil, err
	}

	conn, err := dialPinned(cfg, "udp", host, port, req.Timeout(cfg))
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	sent, packets, err := mediaUDPExchange(cfg, req, conn, func(int) ([]byte, error) {
		return encodeRTCPPacket(packetType, ssrc, payload), nil
	})
	if err != nil {
		return nil, err
	}
	decoded := make([]any, 0, len(packets))
	for _, raw := range packets {
		packet, err := decodeRTCPPacket(raw)
		if err != nil {
			continue
		}
		item := map[string]any{
			"version":     packet.version,
			"padding":     packet.padding,
			"count":       packet.count,
			"packet_type": packet.packetType,
			"length":      packet.length,
			"ssrc":        nil,
			"text":        nil,
		}
		if packet.hasSSRC {
			item["ssrc"] = packet.ssrc
		}
		if packet.hasText {
			item["text"] = packet.text
		}
		decoded = append(decoded, item)
	}
	return mediaJSONResponse(started, sent, len(packets), map[string]any{"packets": decoded})
}
