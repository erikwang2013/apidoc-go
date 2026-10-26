//go:build apidoc_debug_industrial || apidoc_debug_all

package debug

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"strconv"
	"time"
)

const (
	iec104DefaultPort = "2404"
	iec104MaxFrames   = 1000
	iec104MaxAPDU     = 253
)

func init() {
	RegisterExecutor("iec104", executeIEC104)
}

// IEC 60870-5-104 read-only subset: STARTDT + general interrogation / single
// point read. Controls, clock sync, file transfer, redundancy and TLS are out
// of scope.
func executeIEC104(cfg Config, req *Request) (*Response, error) {
	started := time.Now()
	host, port, err := parseProtocolTarget(req.Target, "iec104", iec104DefaultPort,
		"iec104", "iec-60870-5-104", "tcp")
	if err != nil {
		return nil, err
	}
	commonAddress := iec104OptU16(req, "common_address", 1)
	duration := protocolWindow(cfg, req)
	operation := req.Opts["operation"]
	if operation == "" {
		operation = "interrogation"
	}

	// Build and validate the ASDU before connecting: invalid operations never
	// reach the device.
	var asdu []byte
	switch operation {
	case "interrogation":
		asdu = []byte{100, 0x01, 0x06, 0x00,
			byte(commonAddress), byte(commonAddress >> 8),
			0x00, 0x00, 0x00, 20} // IOA 0, QOI 20 = station
	case "read":
		ioaValue, found := req.Opts["ioa"]
		if !found {
			return nil, fmt.Errorf("iec104 `ioa` is required for read")
		}
		ioa, err := strconv.ParseUint(ioaValue, 10, 32)
		if err != nil {
			return nil, fmt.Errorf("invalid iec104 ioa `%s`", ioaValue)
		}
		if ioa > 0xFFFFFF {
			return nil, fmt.Errorf("iec104 ioa must fit in 24 bits")
		}
		asdu = []byte{102, 0x01, 0x05, 0x00,
			byte(commonAddress), byte(commonAddress >> 8),
			byte(ioa), byte(ioa >> 8), byte(ioa >> 16)}
	default:
		return nil, fmt.Errorf("unsupported iec104 operation `%s`; use interrogation or read", operation)
	}

	conn, err := dialPinned(cfg, "tcp", host, port, req.Timeout(cfg))
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	deadline := time.Now().Add(req.Timeout(cfg))

	// STARTDT act -> con. Data frames before confirmation are ignored.
	if err := iec104WriteAPDU(conn, []byte{0x07, 0x00, 0x00, 0x00}, nil); err != nil {
		return nil, err
	}
	startedDT := false
	for !startedDT {
		body, err := iec104ReadAPDU(conn, deadline)
		if err != nil {
			return nil, err
		}
		if body == nil {
			return nil, fmt.Errorf("iec104 STARTDT was not confirmed")
		}
		if len(body) == 4 && body[0] == 0x0B {
			startedDT = true
		}
	}

	if err := iec104WriteAPDU(conn, []byte{0x00, 0x00, 0x00, 0x00}, asdu); err != nil {
		return nil, err
	}

	collectDeadline := time.Now().Add(duration)
	rows, receivedSeq, err := iec104Collect(cfg, conn, collectDeadline)
	if err != nil {
		return nil, err
	}
	// Close the read-only session with an S-frame acknowledging received I-frames.
	sFrame := []byte{0x01, 0x00, byte((receivedSeq << 1) & 0xFF), byte((receivedSeq >> 7) & 0xFF)}
	_ = iec104WriteAPDU(conn, sFrame, nil)

	if len(rows) == 0 {
		return nil, fmt.Errorf("iec104 timed out after %dms without an ASDU", duration.Milliseconds())
	}
	encoded, err := json.Marshal(map[string]any{"asdu": rows})
	if err != nil {
		return nil, fmt.Errorf("encode response: %v", err)
	}
	return &Response{
		Status:  0,
		MS:      time.Since(started).Milliseconds(),
		Headers: map[string]string{"x-peer": conn.RemoteAddr().String()},
		Body:    FromBytes(encoded),
	}, nil
}

// iec104WriteAPDU sends `68 len ctrl(4) asdu…`; U/S frames carry no ASDU.
func iec104WriteAPDU(conn net.Conn, ctrl, asdu []byte) error {
	length := 4 + len(asdu)
	if length > iec104MaxAPDU {
		return fmt.Errorf("iec104 ASDU exceeds %d bytes", iec104MaxAPDU)
	}
	apdu := make([]byte, 0, 2+length)
	apdu = append(apdu, 0x68, byte(length))
	apdu = append(apdu, ctrl...)
	apdu = append(apdu, asdu...)
	if err := conn.SetWriteDeadline(time.Now().Add(30 * time.Second)); err != nil {
		return fmt.Errorf("iec104 deadline failed: %v", err)
	}
	if _, err := conn.Write(apdu); err != nil {
		return fmt.Errorf("iec104 write failed: %v", err)
	}
	return nil
}

// iec104ReadAPDU reads one APDU and returns control(4)+ASDU; timeout/EOF is nil.
func iec104ReadAPDU(conn net.Conn, deadline time.Time) ([]byte, error) {
	header, ok, err := iec104ReadExact(conn, 2, deadline)
	if err != nil || !ok {
		return nil, err
	}
	if header[0] != 0x68 {
		return nil, fmt.Errorf("invalid iec104 start byte")
	}
	length := int(header[1])
	if length < 4 || length > iec104MaxAPDU {
		return nil, fmt.Errorf("invalid iec104 APDU length %d", length)
	}
	body, ok, err := iec104ReadExact(conn, length, deadline)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, nil
	}
	return body, nil
}

func iec104ReadExact(conn net.Conn, size int, deadline time.Time) ([]byte, bool, error) {
	if !time.Now().Before(deadline) {
		return nil, false, nil
	}
	if err := conn.SetReadDeadline(deadline); err != nil {
		return nil, false, fmt.Errorf("iec104 deadline failed: %v", err)
	}
	buf := make([]byte, size)
	if _, err := io.ReadFull(conn, buf); err != nil {
		if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
			return nil, false, nil
		}
		if ne, ok := err.(net.Error); ok && ne.Timeout() {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("iec104 read failed: %v", err)
	}
	return buf, true, nil
}

func iec104Collect(cfg Config, conn net.Conn, deadline time.Time) ([]any, uint16, error) {
	rows := []any{}
	total := 0
	receivedSeq := uint16(0)
	frames := 0
	for frames < iec104MaxFrames {
		body, err := iec104ReadAPDU(conn, deadline)
		if err != nil {
			return nil, 0, err
		}
		if body == nil {
			break
		}
		total += len(body) + 2
		if total > cfg.Limit() {
			return nil, 0, fmt.Errorf("iec104 response exceeds max_bytes (%d)", cfg.Limit())
		}
		frames++
		if body[0]&0x01 != 0 {
			continue // S frame or U frame
		}
		receivedSeq = binary.LittleEndian.Uint16(body[2:]) >> 1
		parsed, err := iec104ParseASDUs(body[4:])
		if err != nil {
			return nil, 0, err
		}
		rows = append(rows, parsed...)
	}
	return rows, receivedSeq, nil
}

const iec104MaxRows = 10000

func iec104ParseASDUs(data []byte) ([]any, error) {
	rows := []any{}
	for len(data) > 0 {
		if len(data) < 6 {
			return nil, fmt.Errorf("truncated iec104 ASDU header")
		}
		typeID := data[0]
		vsq := data[1]
		sequence := vsq&0x80 != 0
		count := int(vsq & 0x7F)
		cause := data[2] & 0x3F
		data = data[6:]

		baseIOA := uint32(0)
		for index := 0; index < count; index++ {
			var ioa uint32
			if sequence && index > 0 {
				ioa = baseIOA + uint32(index)
			} else {
				if len(data) < 3 {
					return nil, fmt.Errorf("truncated iec104 IOA")
				}
				baseIOA = uint32(data[0]) | uint32(data[1])<<8 | uint32(data[2])<<16
				ioa = baseIOA
				data = data[3:]
			}
			value, used, err := iec104Value(typeID, data)
			if err != nil {
				return nil, err
			}
			if used > len(data) {
				return nil, fmt.Errorf("truncated iec104 value")
			}
			data = data[used:]
			if len(rows) >= iec104MaxRows {
				return nil, fmt.Errorf("iec104 response exceeds %d rows", iec104MaxRows)
			}
			rows = append(rows, map[string]any{
				"type_id": typeID,
				"cause":   cause,
				"ioa":     ioa,
				"value":   value,
			})
		}
	}
	return rows, nil
}

// iec104Value returns the decoded value and consumed bytes; CP56Time2a tags
// are skipped because only the value is exposed.
func iec104Value(typeID byte, data []byte) (any, int, error) {
	byteAt := func(n int) (byte, error) {
		if n >= len(data) {
			return 0, fmt.Errorf("truncated iec104 value")
		}
		return data[n], nil
	}
	i16At := func(n int) (int16, error) {
		if n+2 > len(data) {
			return 0, fmt.Errorf("truncated iec104 value")
		}
		return int16(binary.LittleEndian.Uint16(data[n:])), nil
	}
	i32At := func(n int) (int32, error) {
		if n+4 > len(data) {
			return 0, fmt.Errorf("truncated iec104 value")
		}
		return int32(binary.LittleEndian.Uint32(data[n:])), nil
	}
	f32At := func(n int) (any, error) {
		if n+4 > len(data) {
			return nil, fmt.Errorf("truncated iec104 value")
		}
		value := math.Float32frombits(binary.LittleEndian.Uint32(data[n:]))
		return protocolJSONFloat32(value), nil
	}
	switch typeID {
	case 1:
		b, err := byteAt(0)
		return b&0x01 != 0, 1, err
	case 3:
		b, err := byteAt(0)
		return b & 0x03, 1, err
	case 5:
		b, err := byteAt(0)
		return b, 2, err
	case 9, 11:
		value, err := i16At(0)
		return value, 3, err
	case 13:
		value, err := f32At(0)
		return value, 5, err
	case 15:
		value, err := i32At(0)
		return value, 5, err
	case 37:
		value, err := i32At(0)
		return value, 12, err
	case 30:
		b, err := byteAt(0)
		return b&0x01 != 0, 8, err
	case 31:
		b, err := byteAt(0)
		return b & 0x03, 8, err
	case 34, 35:
		value, err := i16At(0)
		return value, 10, err
	case 36:
		value, err := f32At(0)
		return value, 12, err
	case 45, 46, 47:
		b, err := byteAt(0)
		return b&0x01 != 0, 1, err
	case 48, 49:
		value, err := i16At(0)
		return value, 3, err
	case 50:
		value, err := f32At(0)
		return value, 5, err
	case 58, 59, 60:
		b, err := byteAt(0)
		return b&0x01 != 0, 8, err
	case 61:
		value, err := i16At(0)
		return value, 10, err
	case 62:
		value, err := f32At(0)
		return value, 12, err
	case 63:
		return nil, 8, nil
	case 70, 100, 101:
		b, err := byteAt(0)
		return b, 1, err
	case 102:
		return nil, 0, nil
	case 103:
		return nil, 7, nil
	default:
		return nil, 0, fmt.Errorf("unsupported iec104 ASDU type %d; "+
			"read-only monitoring types are supported", typeID)
	}
}

func iec104OptU16(req *Request, key string, def uint16) uint16 {
	if value, err := strconv.ParseUint(req.Opts[key], 10, 16); err == nil {
		return uint16(value)
	}
	return def
}
