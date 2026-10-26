//go:build apidoc_debug_industrial || apidoc_debug_all

package debug

import (
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"math"
	"strings"
)

// opcuaDecoder reads the little-endian binary encoding. Unsupported complex
// types fail explicitly instead of guessing lengths.
type opcuaDecoder struct {
	data []byte
	pos  int
}

func (d *opcuaDecoder) take(length int) ([]byte, error) {
	if length < 0 || length > len(d.data)-d.pos {
		return nil, fmt.Errorf("truncated opcua message")
	}
	out := d.data[d.pos : d.pos+length]
	d.pos += length
	return out, nil
}

func (d *opcuaDecoder) rest() []byte { return d.data[d.pos:] }

func (d *opcuaDecoder) u8() (uint8, error) {
	bytes, err := d.take(1)
	if err != nil {
		return 0, err
	}
	return bytes[0], nil
}

func (d *opcuaDecoder) u16() (uint16, error) {
	bytes, err := d.take(2)
	if err != nil {
		return 0, err
	}
	return binary.LittleEndian.Uint16(bytes), nil
}

func (d *opcuaDecoder) u32() (uint32, error) {
	bytes, err := d.take(4)
	if err != nil {
		return 0, err
	}
	return binary.LittleEndian.Uint32(bytes), nil
}

func (d *opcuaDecoder) i16() (int16, error) {
	value, err := d.u16()
	return int16(value), err
}

func (d *opcuaDecoder) i32() (int32, error) {
	value, err := d.u32()
	return int32(value), err
}

func (d *opcuaDecoder) u64() (uint64, error) {
	bytes, err := d.take(8)
	if err != nil {
		return 0, err
	}
	return binary.LittleEndian.Uint64(bytes), nil
}

func (d *opcuaDecoder) i64() (int64, error) {
	value, err := d.u64()
	return int64(value), err
}

func (d *opcuaDecoder) f32() (float32, error) {
	value, err := d.u32()
	return math.Float32frombits(value), err
}

func (d *opcuaDecoder) f64() (float64, error) {
	value, err := d.u64()
	return math.Float64frombits(value), err
}

// string returns the value and whether it was non-null.
func (d *opcuaDecoder) string() (string, bool, error) {
	bytes, found, err := d.byteString()
	if err != nil || !found {
		return "", false, err
	}
	return strings.ToValidUTF8(string(bytes), "�"), true, nil
}

func (d *opcuaDecoder) byteString() ([]byte, bool, error) {
	length, err := d.i32()
	if err != nil {
		return nil, false, err
	}
	if length < 0 {
		return nil, false, nil
	}
	bytes, err := d.take(int(length))
	if err != nil {
		return nil, false, err
	}
	return bytes, true, nil
}

func opcuaCheckResponseHeader(dec *opcuaDecoder) error {
	if _, err := dec.i64(); err != nil { // timestamp
		return err
	}
	if _, err := dec.u32(); err != nil { // requestHandle
		return err
	}
	status, err := dec.u32()
	if err != nil {
		return err
	}
	if err := opcuaSkipDiagnosticInfo(dec); err != nil {
		return err
	}
	stringsCount, err := dec.i32()
	if err != nil {
		return err
	}
	for i := int32(0); i < stringsCount; i++ {
		if _, _, err := dec.string(); err != nil {
			return err
		}
	}
	if err := opcuaSkipExtensionObject(dec); err != nil {
		return err
	}
	if status != 0 {
		return fmt.Errorf("opcua service status %s", opcuaStatusLabel(status))
	}
	return nil
}

func opcuaDecodeDataValue(dec *opcuaDecoder) (any, error) {
	mask, err := dec.u8()
	if err != nil {
		return nil, err
	}
	var value any
	hasValue := false
	if mask&0x01 != 0 {
		value, err = opcuaDecodeVariant(dec)
		if err != nil {
			return nil, err
		}
		hasValue = true
	}
	status := uint32(0)
	if mask&0x02 != 0 {
		status, err = dec.u32()
		if err != nil {
			return nil, err
		}
	}
	if mask&0x04 != 0 {
		if _, err := dec.i64(); err != nil {
			return nil, err
		}
	}
	if mask&0x08 != 0 {
		if _, err := dec.u32(); err != nil {
			return nil, err
		}
	}
	if mask&0x10 != 0 {
		if _, err := dec.i64(); err != nil {
			return nil, err
		}
	}
	if mask&0x20 != 0 {
		if _, err := dec.u32(); err != nil {
			return nil, err
		}
	}
	if hasValue && status == 0 {
		return value, nil
	}
	return nil, fmt.Errorf("opcua read status %s", opcuaStatusLabel(status))
}

func opcuaDecodeVariant(dec *opcuaDecoder) (any, error) {
	mask, err := dec.u8()
	if err != nil {
		return nil, err
	}
	isArray := mask&0x80 != 0
	hasDimensions := mask&0x40 != 0
	typeID := mask & 0x3F
	var value any
	if isArray {
		count, err := dec.i32()
		if err != nil {
			return nil, err
		}
		if count < 0 {
			value = []any{}
		} else {
			items := []any{}
			for i := int32(0); i < count && i < 10000; i++ {
				item, err := opcuaDecodeScalar(dec, typeID)
				if err != nil {
					return nil, err
				}
				items = append(items, item)
			}
			value = items
		}
	} else {
		value, err = opcuaDecodeScalar(dec, typeID)
		if err != nil {
			return nil, err
		}
	}
	if hasDimensions {
		count, err := dec.i32()
		if err != nil {
			return nil, err
		}
		for i := int32(0); i < count; i++ {
			if _, err := dec.u32(); err != nil {
				return nil, err
			}
		}
	}
	return value, nil
}

func opcuaDecodeScalar(dec *opcuaDecoder, typeID uint8) (any, error) {
	switch typeID {
	case 0:
		return nil, nil
	case 1:
		value, err := dec.u8()
		return value != 0, err
	case 2:
		value, err := dec.u8()
		return int8(value), err
	case 3:
		return dec.u8()
	case 4:
		return dec.i16()
	case 5:
		return dec.u16()
	case 6:
		return dec.i32()
	case 7:
		return dec.u32()
	case 8:
		return dec.i64()
	case 9:
		return dec.u64()
	case 10:
		value, err := dec.f32()
		return protocolJSONFloat32(value), err
	case 11:
		value, err := dec.f64()
		return protocolJSONFloat(value), err
	case 12:
		value, found, err := dec.string()
		if err != nil {
			return nil, err
		}
		if !found {
			return nil, nil
		}
		return value, nil
	case 13:
		return dec.i64()
	case 15:
		bytes, found, err := dec.byteString()
		if err != nil {
			return nil, err
		}
		if !found {
			return nil, nil
		}
		return map[string]any{
			"encoding": "base64",
			"data":     base64.StdEncoding.EncodeToString(bytes),
		}, nil
	case 17:
		return opcuaDecodeNodeID(dec)
	case 19:
		status, err := dec.u32()
		return opcuaStatusLabel(status), err
	case 20:
		namespace, err := dec.u16()
		if err != nil {
			return nil, err
		}
		name, _, err := dec.string()
		if err != nil {
			return nil, err
		}
		return map[string]any{"namespace_index": namespace, "name": name}, nil
	case 21:
		return opcuaDecodeLocalizedText(dec)
	default:
		return nil, fmt.Errorf("unsupported opcua variant type %d", typeID)
	}
}

func opcuaDecodeNodeID(dec *opcuaDecoder) (string, error) {
	encoding, err := dec.u8()
	if err != nil {
		return "", err
	}
	var id string
	switch encoding & 0x0F {
	case 0:
		value, err := dec.u8()
		if err != nil {
			return "", err
		}
		id = fmt.Sprintf("i=%d", value)
	case 1:
		namespace, err := dec.u8()
		if err != nil {
			return "", err
		}
		value, err := dec.u16()
		if err != nil {
			return "", err
		}
		if namespace == 0 {
			id = fmt.Sprintf("i=%d", value)
		} else {
			id = fmt.Sprintf("ns=%d;i=%d", namespace, value)
		}
	case 2:
		namespace, err := dec.u16()
		if err != nil {
			return "", err
		}
		value, err := dec.u32()
		if err != nil {
			return "", err
		}
		if namespace == 0 {
			id = fmt.Sprintf("i=%d", value)
		} else {
			id = fmt.Sprintf("ns=%d;i=%d", namespace, value)
		}
	case 3:
		namespace, err := dec.u16()
		if err != nil {
			return "", err
		}
		name, _, err := dec.string()
		if err != nil {
			return "", err
		}
		id = fmt.Sprintf("ns=%d;s=%s", namespace, name)
	case 4:
		guid, err := dec.take(16)
		if err != nil {
			return "", err
		}
		id = "g=" + hexString(guid)
	case 5:
		bytes, _, err := dec.byteString()
		if err != nil {
			return "", err
		}
		id = "b=" + base64.StdEncoding.EncodeToString(bytes)
	default:
		return "", fmt.Errorf("unsupported opcua node id encoding %d", encoding&0x0F)
	}
	if encoding&0x40 != 0 {
		if _, _, err := dec.string(); err != nil { // namespace URI
			return "", err
		}
	}
	if encoding&0x80 != 0 {
		if _, err := dec.u32(); err != nil { // server index
			return "", err
		}
	}
	return id, nil
}

func opcuaSkipNodeID(dec *opcuaDecoder) error {
	_, err := opcuaDecodeNodeID(dec)
	return err
}

func opcuaSkipExtensionObject(dec *opcuaDecoder) error {
	if err := opcuaSkipNodeID(dec); err != nil {
		return err
	}
	encoding, err := dec.u8()
	if err != nil {
		return err
	}
	switch encoding {
	case 0:
		return nil
	case 1, 2:
		length, err := dec.i32()
		if err != nil {
			return err
		}
		if length < 0 {
			length = 0
		}
		_, err = dec.take(int(length))
		return err
	default:
		return fmt.Errorf("unsupported opcua extension object encoding %d", encoding)
	}
}

func opcuaSkipDiagnosticInfo(dec *opcuaDecoder) error {
	mask, err := dec.u8()
	if err != nil {
		return err
	}
	if mask&0x01 != 0 {
		if _, _, err := dec.string(); err != nil {
			return err
		}
	}
	if mask&0x02 != 0 {
		if _, err := opcuaDecodeLocalizedText(dec); err != nil {
			return err
		}
	}
	if mask&0x04 != 0 {
		if _, _, err := dec.string(); err != nil {
			return err
		}
	}
	if mask&0x08 != 0 {
		if _, _, err := dec.string(); err != nil {
			return err
		}
	}
	if mask&0x10 != 0 {
		if _, err := dec.u32(); err != nil {
			return err
		}
	}
	if mask&0x20 != 0 {
		return opcuaSkipDiagnosticInfo(dec)
	}
	return nil
}

func opcuaDecodeLocalizedText(dec *opcuaDecoder) (string, error) {
	mask, err := dec.u8()
	if err != nil {
		return "", err
	}
	if mask&0x01 != 0 {
		if _, _, err := dec.string(); err != nil { // locale
			return "", err
		}
	}
	text := ""
	if mask&0x02 != 0 {
		value, _, err := dec.string()
		if err != nil {
			return "", err
		}
		text = value
	}
	return text, nil
}

func opcuaStatusLabel(status uint32) string {
	switch status {
	case 0:
		return "Good"
	case 0x80000000:
		return "BadUnexpectedError"
	case 0x801F0000:
		return "BadUserAccessDenied"
	case 0x80200000:
		return "BadIdentityTokenInvalid"
	case 0x80210000:
		return "BadIdentityTokenRejected"
	case 0x80350000:
		return "BadNotConnected"
	default:
		return fmt.Sprintf("Bad (0x%08X)", status)
	}
}
