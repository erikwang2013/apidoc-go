//go:build apidoc_debug_industrial || apidoc_debug_all

package debug

import (
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"time"
)

// dnp3MaxLinkData is the maximum LEN value (control + addresses + user data).
const dnp3MaxLinkData = 250

type dnp3LinkFrame struct {
	ctrl byte
	dst  uint16
	src  uint16
	data []byte
}

// dnp3ReadLinkFrame reads one complete link frame or returns nil when the
// deadline expires or the peer closes cleanly between frames.
func dnp3ReadLinkFrame(conn net.Conn, deadline time.Time) (*dnp3LinkFrame, error) {
	header := make([]byte, 10)
	ok, err := dnp3ReadExact(conn, header, deadline)
	if err != nil || !ok {
		return nil, err
	}
	if header[0] != 0x05 || header[1] != 0x64 {
		return nil, fmt.Errorf("invalid dnp3 start bytes")
	}
	length := int(header[2])
	if length < 5 || length > dnp3MaxLinkData {
		return nil, fmt.Errorf("invalid dnp3 link length %d", length)
	}
	if dnp3CRC16(header[2:8]) != binary.LittleEndian.Uint16(header[8:]) {
		return nil, fmt.Errorf("dnp3 header CRC mismatch")
	}
	dataLen := length - 5
	blocks := (dataLen + 15) / 16
	body := make([]byte, dataLen+blocks*2)
	ok, err = dnp3ReadExact(conn, body, deadline)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("truncated dnp3 frame")
	}
	data := make([]byte, 0, dataLen)
	for index := 0; index < blocks; index++ {
		blockLen := 16
		if index == blocks-1 {
			blockLen = dataLen - index*16
		}
		offset := index * 18
		block := body[offset : offset+blockLen]
		expected := binary.LittleEndian.Uint16(body[offset+blockLen:])
		if dnp3CRC16(block) != expected {
			return nil, fmt.Errorf("dnp3 data CRC mismatch in block %d", index)
		}
		data = append(data, block...)
	}
	return &dnp3LinkFrame{
		ctrl: header[3],
		dst:  binary.LittleEndian.Uint16(header[4:]),
		src:  binary.LittleEndian.Uint16(header[6:]),
		data: data,
	}, nil
}

func dnp3ReadExact(conn net.Conn, buf []byte, deadline time.Time) (bool, error) {
	if !time.Now().Before(deadline) {
		return false, nil
	}
	if err := conn.SetReadDeadline(deadline); err != nil {
		return false, fmt.Errorf("dnp3 deadline failed: %v", err)
	}
	_, err := io.ReadFull(conn, buf)
	if err != nil {
		if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
			return false, nil
		}
		if ne, ok := err.(net.Error); ok && ne.Timeout() {
			return false, nil
		}
		return false, fmt.Errorf("dnp3 read failed: %v", err)
	}
	return true, nil
}

// dnp3EncodeLinkFrame emits the link header plus one CRC per 16-byte block.
func dnp3EncodeLinkFrame(ctrl byte, dst, src uint16, userData []byte) ([]byte, error) {
	if len(userData)+5 > dnp3MaxLinkData {
		return nil, fmt.Errorf("dnp3 user data exceeds %d bytes", dnp3MaxLinkData)
	}
	header := []byte{0x05, 0x64, byte(len(userData) + 5), ctrl}
	header = binary.LittleEndian.AppendUint16(header, dst)
	header = binary.LittleEndian.AppendUint16(header, src)
	out := append([]byte{}, header...)
	out = binary.LittleEndian.AppendUint16(out, dnp3CRC16(header[2:]))
	for start := 0; start < len(userData); start += 16 {
		end := start + 16
		if end > len(userData) {
			end = len(userData)
		}
		chunk := userData[start:end]
		out = append(out, chunk...)
		out = binary.LittleEndian.AppendUint16(out, dnp3CRC16(chunk))
	}
	return out, nil
}

// dnp3CRC16 is CRC-16/DNP: reflected poly 0xA6BC, init 0, xorout 0xFFFF.
func dnp3CRC16(data []byte) uint16 {
	crc := uint16(0)
	for _, b := range data {
		crc ^= uint16(b)
		for i := 0; i < 8; i++ {
			if crc&1 != 0 {
				crc = (crc >> 1) ^ 0xA6BC
			} else {
				crc >>= 1
			}
		}
	}
	return crc ^ 0xFFFF
}

type dnpReader struct {
	data []byte
	pos  int
}

func (r *dnpReader) remaining() int { return len(r.data) - r.pos }

func (r *dnpReader) take(n int) ([]byte, error) {
	if n < 0 || r.pos+n > len(r.data) {
		return nil, fmt.Errorf("truncated dnp3 object value")
	}
	out := r.data[r.pos : r.pos+n]
	r.pos += n
	return out, nil
}

func (r *dnpReader) u8() (uint8, error) {
	bytes, err := r.take(1)
	if err != nil {
		return 0, err
	}
	return bytes[0], nil
}

func (r *dnpReader) u16() (uint16, error) {
	bytes, err := r.take(2)
	if err != nil {
		return 0, err
	}
	return binary.LittleEndian.Uint16(bytes), nil
}

func (r *dnpReader) u32() (uint32, error) {
	bytes, err := r.take(4)
	if err != nil {
		return 0, err
	}
	return binary.LittleEndian.Uint32(bytes), nil
}

func (r *dnpReader) i16() (int16, error) {
	value, err := r.u16()
	return int16(value), err
}

func (r *dnpReader) i32() (int32, error) {
	value, err := r.u32()
	return int32(value), err
}

func (r *dnpReader) f32() (float32, error) {
	value, err := r.u32()
	return math.Float32frombits(value), err
}

func (r *dnpReader) f64() (float64, error) {
	bytes, err := r.take(8)
	if err != nil {
		return 0, err
	}
	return math.Float64frombits(binary.LittleEndian.Uint64(bytes)), nil
}

// dnp3ParseObjects decodes the object headers in one DNP3 read response.
func dnp3ParseObjects(data []byte) ([]any, error) {
	reader := &dnpReader{data: data}
	points := []any{}
	for reader.remaining() > 0 {
		if reader.remaining() < 3 {
			return nil, fmt.Errorf("truncated dnp3 object header")
		}
		group, _ := reader.u8()
		variation, _ := reader.u8()
		qualifier, _ := reader.u8()
		if qualifier>>4 == 6 {
			return nil, fmt.Errorf("dnp3 all-objects qualifier in a response is not supported")
		}
		start, count, err := dnp3ReadRange(reader, qualifier)
		if err != nil {
			return nil, err
		}
		prefix := qualifier & 0x0F
		if prefix > 3 {
			return nil, fmt.Errorf("dnp3 size-prefix qualifier %d is not supported", prefix)
		}
		if (group == 1 || group == 3) && variation == 1 {
			if prefix != 0 {
				return nil, fmt.Errorf("dnp3 index-prefixed packed bit fields are not supported")
			}
			packed, err := dnp3ReadPacked(reader, group, variation, start, count)
			if err != nil {
				return nil, err
			}
			points = append(points, packed...)
			continue
		}
		for offset := uint32(0); offset < count; offset++ {
			index := start + offset
			switch prefix {
			case 1:
				value, err := reader.u8()
				if err != nil {
					return nil, err
				}
				index = uint32(value)
			case 2:
				value, err := reader.u16()
				if err != nil {
					return nil, err
				}
				index = uint32(value)
			case 3:
				value, err := reader.u32()
				if err != nil {
					return nil, err
				}
				index = value
			}
			value, flags, err := dnp3ReadPoint(reader, group, variation, offset)
			if err != nil {
				return nil, err
			}
			points = append(points, map[string]any{
				"group":     group,
				"variation": variation,
				"index":     index,
				"value":     value,
				"flags":     flags,
			})
		}
	}
	return points, nil
}

func dnp3ReadPacked(reader *dnpReader, group, variation uint8, start, count uint32) ([]any, error) {
	bits := 2
	if group == 1 && variation == 1 {
		bits = 1
	}
	size := (int(count)*bits + 7) / 8
	packed, err := reader.take(size)
	if err != nil {
		return nil, fmt.Errorf("truncated dnp3 packed bits")
	}
	points := make([]any, 0, count)
	for i := uint32(0); i < count; i++ {
		var value uint8
		if bits == 1 {
			if packed[i/8]&(1<<(i%8)) != 0 {
				value = 1
			}
		} else {
			value = (packed[i/4] >> ((i % 4) * 2)) & 0x03
		}
		points = append(points, map[string]any{
			"group": group, "variation": variation,
			"index": start + i, "value": value, "flags": nil,
		})
	}
	return points, nil
}

func dnp3ReadRange(reader *dnpReader, qualifier uint8) (uint32, uint32, error) {
	switch qualifier >> 4 {
	case 0:
		start, err := reader.u8()
		if err != nil {
			return 0, 0, fmt.Errorf("truncated dnp3 range")
		}
		stop, err := reader.u8()
		if err != nil {
			return 0, 0, fmt.Errorf("truncated dnp3 range")
		}
		if stop < start {
			return 0, 0, fmt.Errorf("dnp3 range stop < start")
		}
		return uint32(start), uint32(stop-start) + 1, nil
	case 1:
		start, err := reader.u16()
		if err != nil {
			return 0, 0, fmt.Errorf("truncated dnp3 range")
		}
		stop, err := reader.u16()
		if err != nil {
			return 0, 0, fmt.Errorf("truncated dnp3 range")
		}
		if stop < start {
			return 0, 0, fmt.Errorf("dnp3 range stop < start")
		}
		return uint32(start), uint32(stop-start) + 1, nil
	case 2:
		start, err := reader.u32()
		if err != nil {
			return 0, 0, fmt.Errorf("truncated dnp3 range")
		}
		stop, err := reader.u32()
		if err != nil {
			return 0, 0, fmt.Errorf("truncated dnp3 range")
		}
		if stop < start {
			return 0, 0, fmt.Errorf("dnp3 range stop < start")
		}
		return start, stop - start + 1, nil
	case 7:
		count, err := reader.u8()
		if err != nil {
			return 0, 0, fmt.Errorf("truncated dnp3 range")
		}
		return 0, uint32(count), nil
	case 8:
		count, err := reader.u16()
		if err != nil {
			return 0, 0, fmt.Errorf("truncated dnp3 range")
		}
		return 0, uint32(count), nil
	case 9:
		count, err := reader.u32()
		if err != nil {
			return 0, 0, fmt.Errorf("truncated dnp3 range")
		}
		return 0, count, nil
	default:
		return 0, 0, fmt.Errorf("unsupported dnp3 range qualifier %d", qualifier>>4)
	}
}

func dnp3ReadPoint(reader *dnpReader, group, variation uint8, offset uint32) (any, any, error) {
	withFlags := func(flags byte, value any) (any, any, error) {
		return value, flags, nil
	}
	plain := func(value any) (any, any, error) {
		return value, nil, nil
	}
	switch {
	case (group == 1 || group == 10) && variation == 2:
		flags, err := reader.u8()
		if err != nil {
			return nil, nil, err
		}
		return withFlags(flags, flags&0x80 != 0)
	case group == 3 && variation == 2:
		flags, err := reader.u8()
		if err != nil {
			return nil, nil, err
		}
		return withFlags(flags, (flags>>6)&0x03)
	case (group == 20 || group == 21) && variation == 1:
		flags, err := reader.u8()
		if err != nil {
			return nil, nil, err
		}
		value, err := reader.u32()
		if err != nil {
			return nil, nil, err
		}
		return withFlags(flags, value)
	case (group == 20 || group == 21) && variation == 2:
		flags, err := reader.u8()
		if err != nil {
			return nil, nil, err
		}
		value, err := reader.u16()
		if err != nil {
			return nil, nil, err
		}
		return withFlags(flags, value)
	case (group == 20 || group == 21) && variation == 5:
		value, err := reader.u32()
		if err != nil {
			return nil, nil, err
		}
		return plain(value)
	case (group == 20 || group == 21) && variation == 6:
		value, err := reader.u16()
		if err != nil {
			return nil, nil, err
		}
		return plain(value)
	case (group == 30 || group == 40) && variation == 1:
		flags, err := reader.u8()
		if err != nil {
			return nil, nil, err
		}
		value, err := reader.i32()
		if err != nil {
			return nil, nil, err
		}
		return withFlags(flags, value)
	case (group == 30 || group == 40) && variation == 2:
		flags, err := reader.u8()
		if err != nil {
			return nil, nil, err
		}
		value, err := reader.i16()
		if err != nil {
			return nil, nil, err
		}
		return withFlags(flags, value)
	case (group == 30 || group == 40) && variation == 3:
		value, err := reader.i32()
		if err != nil {
			return nil, nil, err
		}
		return plain(value)
	case (group == 30 || group == 40) && variation == 4:
		value, err := reader.i16()
		if err != nil {
			return nil, nil, err
		}
		return plain(value)
	case group == 30 && variation == 5:
		flags, err := reader.u8()
		if err != nil {
			return nil, nil, err
		}
		value, err := reader.f32()
		if err != nil {
			return nil, nil, err
		}
		return withFlags(flags, protocolJSONFloat(float64(value)))
	case group == 30 && variation == 6:
		flags, err := reader.u8()
		if err != nil {
			return nil, nil, err
		}
		value, err := reader.f64()
		if err != nil {
			return nil, nil, err
		}
		return withFlags(flags, protocolJSONFloat(value))
	case group == 30 && variation == 7:
		value, err := reader.f32()
		if err != nil {
			return nil, nil, err
		}
		return plain(protocolJSONFloat(float64(value)))
	case group == 30 && variation == 8:
		value, err := reader.f64()
		if err != nil {
			return nil, nil, err
		}
		return plain(protocolJSONFloat(value))
	case group == 50 && variation >= 1 && variation <= 3:
		value, err := reader.take(6)
		if err != nil {
			return nil, nil, err
		}
		return plain(map[string]any{"encoding": "hex", "data": hexString(value)})
	case group == 50 && variation == 4:
		value, err := reader.take(10)
		if err != nil {
			return nil, nil, err
		}
		return plain(map[string]any{"encoding": "hex", "data": hexString(value)})
	default:
		return nil, nil, fmt.Errorf("unsupported dnp3 object group %d variation %d at point offset %d", group, variation, offset)
	}
}

func hexString(value []byte) string {
	return hex.EncodeToString(value)
}
