//go:build apidoc_debug_media || apidoc_debug_all

package debug

import (
	"fmt"
	"time"
)

func init() {
	RegisterExecutor("rtp", executeRTP)
}

// rtpPacket is the RFC 3550 header subset the debug executor reports.
type rtpPacket struct {
	version     uint8
	padding     bool
	extension   bool
	marker      bool
	payloadType uint8
	sequence    uint16
	timestamp   uint32
	ssrc        uint32
	csrc        []uint32
	payload     []byte
}

// encodeRTPPacket builds a minimal packet: fixed 12-byte header, no CSRC,
// extension or padding.
func encodeRTPPacket(payloadType uint8, sequence uint16, timestamp, ssrc uint32, payload []byte) []byte {
	out := make([]byte, 12, 12+len(payload))
	out[0] = 0x80 // V=2, P=0, X=0, CC=0
	out[1] = payloadType & 0x7f
	out[2], out[3] = byte(sequence>>8), byte(sequence)
	out[4], out[5], out[6], out[7] = byte(timestamp>>24), byte(timestamp>>16), byte(timestamp>>8), byte(timestamp)
	out[8], out[9], out[10], out[11] = byte(ssrc>>24), byte(ssrc>>16), byte(ssrc>>8), byte(ssrc)
	return append(out, payload...)
}

func decodeRTPPacket(bytes []byte) (rtpPacket, error) {
	var packet rtpPacket
	if len(bytes) < 12 {
		return packet, fmt.Errorf("rtp packet too short: %d bytes", len(bytes))
	}
	cc := int(bytes[0] & 0x0f)
	packet.extension = bytes[0]&0x10 != 0
	packet.padding = bytes[0]&0x20 != 0
	offset := 12 + cc*4
	if offset > len(bytes) {
		return packet, fmt.Errorf("rtp CSRC list exceeds packet length")
	}
	packet.csrc = make([]uint32, 0, cc)
	for i := 0; i < cc; i++ {
		at := 12 + i*4
		packet.csrc = append(packet.csrc, uint32(bytes[at])<<24|uint32(bytes[at+1])<<16|uint32(bytes[at+2])<<8|uint32(bytes[at+3]))
	}
	if packet.extension {
		if offset+4 > len(bytes) {
			return packet, fmt.Errorf("rtp extension header exceeds packet length")
		}
		// The extension length counts 32-bit words after the 4-byte header.
		words := int(bytes[offset+2])<<8 | int(bytes[offset+3])
		offset += 4 + words*4
		if offset > len(bytes) {
			return packet, fmt.Errorf("rtp extension data exceeds packet length")
		}
	}
	end := len(bytes)
	if packet.padding {
		pad := int(bytes[len(bytes)-1])
		// The padding count includes its own last byte and cannot eat the header.
		if pad == 0 || pad > end-offset {
			return packet, fmt.Errorf("rtp padding is invalid")
		}
		end -= pad
	}
	packet.version = bytes[0] >> 6
	packet.marker = bytes[1]&0x80 != 0
	packet.payloadType = bytes[1] & 0x7f
	packet.sequence = uint16(bytes[2])<<8 | uint16(bytes[3])
	packet.timestamp = uint32(bytes[4])<<24 | uint32(bytes[5])<<16 | uint32(bytes[6])<<8 | uint32(bytes[7])
	packet.ssrc = uint32(bytes[8])<<24 | uint32(bytes[9])<<16 | uint32(bytes[10])<<8 | uint32(bytes[11])
	packet.payload = append([]byte{}, bytes[offset:end]...)
	return packet, nil
}

func executeRTP(cfg Config, req *Request) (*Response, error) {
	started := time.Now()
	host, port, err := parseProtocolTarget(req.Target, "rtp", "5004", "rtp", "udp")
	if err != nil {
		return nil, err
	}
	payloadType := protocolOptUint(req, "payload_type", 96)
	if payloadType > 127 {
		return nil, fmt.Errorf("payload_type must be 0-127")
	}
	sequence := uint16(protocolOptUint(req, "sequence", 0))
	timestamp := uint32(protocolOptUint(req, "timestamp", 0))
	ssrc := uint32(protocolOptUint(req, "ssrc", 0))
	payload, err := req.Body.Decode()
	if err != nil {
		return nil, err
	}
	if len(payload)+12 > mediaMaxDatagramBytes {
		return nil, fmt.Errorf("rtp payload exceeds %d bytes", mediaMaxDatagramBytes)
	}

	conn, err := dialPinned(cfg, "udp", host, port, req.Timeout(cfg))
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	sent, packets, err := mediaUDPExchange(cfg, req, conn, func(i int) ([]byte, error) {
		return encodeRTPPacket(uint8(payloadType), sequence+uint16(i),
			timestamp+uint32(i)*160, ssrc, payload), nil
	})
	if err != nil {
		return nil, err
	}
	decoded := make([]any, 0, len(packets))
	for _, raw := range packets {
		packet, err := decodeRTPPacket(raw)
		if err != nil {
			continue // non-RTP datagrams are ignored, like the Rust executor
		}
		decoded = append(decoded, map[string]any{
			"version":      packet.version,
			"padding":      packet.padding,
			"extension":    packet.extension,
			"marker":       packet.marker,
			"payload_type": packet.payloadType,
			"sequence":     packet.sequence,
			"timestamp":    packet.timestamp,
			"ssrc":         packet.ssrc,
			"csrc":         packet.csrc,
			"payload":      mediaPayload(packet.payload),
		})
	}
	return mediaJSONResponse(started, sent, len(packets), map[string]any{"packets": decoded})
}
