//go:build apidoc_debug_rpc || apidoc_debug_all

package debug

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
	"unicode/utf16"
	"unicode/utf8"
)

// hessianEncode writes the non-compact Hessian 2.0 forms (N/T/F/I/L/D/S/W/H).
// Compact forms are only accepted on decode, matching common Java peers.
func hessianEncode(value any) ([]byte, error) {
	out := []byte{}
	if err := hessianWrite(&out, value); err != nil {
		return nil, err
	}
	return out, nil
}

func hessianWrite(out *[]byte, value any) error {
	switch v := value.(type) {
	case nil:
		*out = append(*out, 'N')
	case bool:
		if v {
			*out = append(*out, 'T')
		} else {
			*out = append(*out, 'F')
		}
	case json.Number:
		text := string(v)
		if strings.ContainsAny(text, ".eE") {
			f, err := v.Float64()
			if err != nil {
				return fmt.Errorf("hessian: unsupported number %s", text)
			}
			hessianWriteDouble(out, f)
			return nil
		}
		if i, err := v.Int64(); err == nil {
			hessianWriteInt(out, i)
			return nil
		}
		f, err := v.Float64()
		if err != nil {
			return fmt.Errorf("hessian: unsupported number %s", text)
		}
		hessianWriteDouble(out, f)
	case int:
		hessianWriteInt(out, int64(v))
	case int64:
		hessianWriteInt(out, v)
	case float64:
		hessianWriteDouble(out, v)
	case string:
		return hessianWriteString(out, v)
	case []any:
		*out = append(*out, 0x57) // untyped variable-length list
		for _, item := range v {
			if err := hessianWrite(out, item); err != nil {
				return err
			}
		}
		*out = append(*out, 'Z')
	case map[string]any:
		*out = append(*out, 'H')
		keys := make([]string, 0, len(v))
		for key := range v {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			if err := hessianWriteString(out, key); err != nil {
				return err
			}
			if err := hessianWrite(out, v[key]); err != nil {
				return err
			}
		}
		*out = append(*out, 'Z')
	default:
		return fmt.Errorf("hessian: unsupported value type %T", value)
	}
	return nil
}

func hessianWriteInt(out *[]byte, value int64) {
	if value >= math.MinInt32 && value <= math.MaxInt32 {
		*out = append(*out, 'I')
		*out = binary.BigEndian.AppendUint32(*out, uint32(int32(value)))
		return
	}
	*out = append(*out, 'L')
	*out = binary.BigEndian.AppendUint64(*out, uint64(value))
}

func hessianWriteDouble(out *[]byte, value float64) {
	*out = append(*out, 'D')
	*out = binary.BigEndian.AppendUint64(*out, math.Float64bits(value))
}

func hessianWriteString(out *[]byte, value string) error {
	units := len(utf16.Encode([]rune(value)))
	if units > math.MaxUint16 {
		return fmt.Errorf("hessian: string exceeds 65535 UTF-16 units")
	}
	*out = append(*out, 'S')
	*out = binary.BigEndian.AppendUint16(*out, uint16(units))
	*out = append(*out, value...)
	return nil
}

func hessianDecode(input *[]byte) (any, error) {
	tag, err := hessianReadU8(input)
	if err != nil {
		return nil, err
	}
	return hessianDecodeTag(input, tag)
}

func hessianDecodeTag(input *[]byte, tag byte) (any, error) {
	switch {
	case tag == 'N':
		return nil, nil
	case tag == 'T':
		return true, nil
	case tag == 'F':
		return false, nil
	case tag == 'I':
		value, err := hessianReadI32(input)
		return int64(value), err
	case tag == 'L':
		return hessianReadI64(input)
	case tag == 'D':
		return hessianReadF64(input)
	case tag == 'S':
		units, err := hessianReadU16(input)
		if err != nil {
			return nil, err
		}
		return hessianReadString(input, int(units))
	case tag <= 0x1f:
		return hessianReadString(input, int(tag))
	case tag >= 0x30 && tag <= 0x33:
		next, err := hessianReadU8(input)
		if err != nil {
			return nil, err
		}
		units := (int(tag-0x30) << 8) | int(next)
		return hessianReadString(input, units)
	// compact int
	case tag >= 0x80 && tag <= 0xbf:
		return int64(int8(tag)) + 112, nil
	case tag >= 0xc0 && tag <= 0xcf:
		next, err := hessianReadU8(input)
		if err != nil {
			return nil, err
		}
		return int64((int16(tag)-0xc8)<<8 + int16(next)), nil
	case tag >= 0xd0 && tag <= 0xd7:
		mid, err := hessianReadU8(input)
		if err != nil {
			return nil, err
		}
		low, err := hessianReadU8(input)
		if err != nil {
			return nil, err
		}
		return int64((int32(tag)-0xd4)<<16 + int32(mid)<<8 + int32(low)), nil
	// compact long
	case tag >= 0xd8 && tag <= 0xef:
		return int64(int8(tag)) + 32, nil
	case tag >= 0xf0 && tag <= 0xff:
		next, err := hessianReadU8(input)
		if err != nil {
			return nil, err
		}
		return int64((int16(tag)-0xf8)<<8 + int16(next)), nil
	case tag >= 0x38 && tag <= 0x3f:
		mid, err := hessianReadU8(input)
		if err != nil {
			return nil, err
		}
		low, err := hessianReadU8(input)
		if err != nil {
			return nil, err
		}
		return int64((int32(tag)-0x3c)<<16 + int32(mid)<<8 + int32(low)), nil
	case tag == 0x59:
		value, err := hessianReadI32(input)
		return int64(value), err
	case tag == 0x5b:
		return 0.0, nil
	case tag == 0x5c:
		return 1.0, nil
	case tag == 0x5d:
		value, err := hessianReadU8(input)
		if err != nil {
			return nil, err
		}
		return float64(int8(value)), nil
	case tag == 0x5e:
		value, err := hessianReadU16(input)
		if err != nil {
			return nil, err
		}
		return float64(int16(value)), nil
	case tag == 0x5f:
		value, err := hessianReadU32(input)
		if err != nil {
			return nil, err
		}
		return float64(math.Float32frombits(value)), nil
	// list
	case tag == 0x57:
		return hessianReadListUntilEnd(input)
	case tag == 0x58:
		length, err := hessianReadI32(input)
		if err != nil {
			return nil, err
		}
		return hessianReadListItems(input, length)
	case tag == 0x55:
		if _, err := hessianDecode(input); err != nil {
			return nil, err
		}
		return hessianReadListUntilEnd(input)
	case tag == 0x56:
		if _, err := hessianDecode(input); err != nil {
			return nil, err
		}
		length, err := hessianReadI32(input)
		if err != nil {
			return nil, err
		}
		return hessianReadListItems(input, length)
	case tag >= 0x70 && tag <= 0x77:
		if _, err := hessianDecode(input); err != nil {
			return nil, err
		}
		return hessianReadListItems(input, int32(tag-0x70))
	case tag >= 0x78 && tag <= 0x7f:
		return hessianReadListItems(input, int32(tag-0x78))
	// map
	case tag == 'H':
		return hessianReadMap(input)
	case tag == 'M':
		if _, err := hessianDecode(input); err != nil {
			return nil, err
		}
		return hessianReadMap(input)
	case tag == 'C' || tag == 'O':
		return nil, fmt.Errorf("hessian: class definitions/object instances are not supported; send maps/lists/scalars")
	default:
		return nil, fmt.Errorf("hessian: unsupported tag 0x%02x", tag)
	}
}

func hessianReadListUntilEnd(input *[]byte) (any, error) {
	items := []any{}
	for {
		tag, err := hessianReadU8(input)
		if err != nil {
			return nil, err
		}
		if tag == 'Z' {
			return items, nil
		}
		item, err := hessianDecodeTag(input, tag)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
}

func hessianReadListItems(input *[]byte, length int32) (any, error) {
	items := []any{}
	for i := int32(0); i < length; i++ {
		item, err := hessianDecode(input)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, nil
}

func hessianReadMap(input *[]byte) (any, error) {
	out := map[string]any{}
	for {
		tag, err := hessianReadU8(input)
		if err != nil {
			return nil, err
		}
		if tag == 'Z' {
			return out, nil
		}
		key, err := hessianDecodeTag(input, tag)
		if err != nil {
			return nil, err
		}
		value, err := hessianDecode(input)
		if err != nil {
			return nil, err
		}
		out[hessianKey(key)] = value
	}
}

func hessianKey(value any) string {
	if text, ok := value.(string); ok {
		return text
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return fmt.Sprint(value)
	}
	return string(encoded)
}

// hessianReadString consumes UTF-16 code units, not bytes: one astral rune
// counts as two units.
func hessianReadString(input *[]byte, units int) (string, error) {
	data := *input
	consumed, seen := 0, 0
	for seen < units {
		if consumed >= len(data) {
			return "", errIncomplete
		}
		r, size := utf8.DecodeRune(data[consumed:])
		if r == utf8.RuneError && size == 1 {
			return "", fmt.Errorf("hessian: invalid UTF-8 in string")
		}
		seen += utf16.RuneLen(r)
		consumed += size
	}
	if seen != units {
		return "", fmt.Errorf("hessian: string length splits a surrogate pair")
	}
	*input = data[consumed:]
	return string(data[:consumed]), nil
}

func hessianReadU8(input *[]byte) (byte, error) {
	if len(*input) == 0 {
		return 0, errIncomplete
	}
	value := (*input)[0]
	*input = (*input)[1:]
	return value, nil
}

func hessianReadU16(input *[]byte) (uint16, error) {
	if len(*input) < 2 {
		return 0, errIncomplete
	}
	value := binary.BigEndian.Uint16(*input)
	*input = (*input)[2:]
	return value, nil
}

func hessianReadU32(input *[]byte) (uint32, error) {
	if len(*input) < 4 {
		return 0, errIncomplete
	}
	value := binary.BigEndian.Uint32(*input)
	*input = (*input)[4:]
	return value, nil
}

func hessianReadI32(input *[]byte) (int32, error) {
	value, err := hessianReadU32(input)
	return int32(value), err
}

func hessianReadI64(input *[]byte) (int64, error) {
	if len(*input) < 8 {
		return 0, errIncomplete
	}
	value := binary.BigEndian.Uint64(*input)
	*input = (*input)[8:]
	return int64(value), nil
}

func hessianReadF64(input *[]byte) (float64, error) {
	value, err := hessianReadI64(input)
	if err != nil {
		return 0, err
	}
	return math.Float64frombits(uint64(value)), nil
}
