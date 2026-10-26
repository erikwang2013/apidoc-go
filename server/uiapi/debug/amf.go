//go:build apidoc_debug_media || apidoc_debug_all

package debug

import (
	"encoding/binary"
	"fmt"
	"math"
	"sort"
	"unicode/utf8"
)

// AMF0 subset: number/boolean/string/object/null/ECMA array/date. Unknown
// markers fail instead of being treated as opaque success.
const (
	amf0Number    = 0x00
	amf0Boolean   = 0x01
	amf0String    = 0x02
	amf0Object    = 0x03
	amf0Null      = 0x05
	amf0Undefined = 0x06
	amf0EcmaArray = 0x08
	amf0Date      = 0x0b
	amf0LongStr   = 0x0c
	amf0ObjectEnd = 0x09
)

const amf0MaxDepth = 64

type amf0Value struct {
	kind   byte
	number float64
	flag   bool
	text   string
	fields map[string]amf0Value
}

func amf0NumberValue(value float64) amf0Value {
	return amf0Value{kind: amf0Number, number: value}
}

func amf0StringValue(value string) amf0Value {
	return amf0Value{kind: amf0String, text: value}
}

func amf0ObjectValue(fields map[string]amf0Value) amf0Value {
	return amf0Value{kind: amf0Object, fields: fields}
}

var amf0NullValue = amf0Value{kind: amf0Null}

func writeAMF0(out *[]byte, value amf0Value) error {
	switch value.kind {
	case amf0Number:
		*out = append(*out, amf0Number)
		*out = binary.BigEndian.AppendUint64(*out, math.Float64bits(value.number))
	case amf0Boolean:
		flag := byte(0)
		if value.flag {
			flag = 1
		}
		*out = append(*out, amf0Boolean, flag)
	case amf0String:
		if len(value.text) > 0xffff {
			return fmt.Errorf("AMF0 string exceeds 65535 bytes")
		}
		*out = append(*out, amf0String)
		*out = binary.BigEndian.AppendUint16(*out, uint16(len(value.text)))
		*out = append(*out, value.text...)
	case amf0Null, amf0Undefined:
		*out = append(*out, amf0Null)
	case amf0Object:
		*out = append(*out, amf0Object)
		names := make([]string, 0, len(value.fields))
		for name := range value.fields {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			if len(name) > 0xffff {
				return fmt.Errorf("AMF0 property name exceeds 65535 bytes")
			}
			*out = binary.BigEndian.AppendUint16(*out, uint16(len(name)))
			*out = append(*out, name...)
			if err := writeAMF0(out, value.fields[name]); err != nil {
				return err
			}
		}
		*out = append(*out, 0x00, 0x00, amf0ObjectEnd)
	default:
		return fmt.Errorf("unsupported AMF0 type 0x%02x", value.kind)
	}
	return nil
}

func decodeAMF0All(bytes []byte) ([]amf0Value, error) {
	position := 0
	values := []amf0Value{}
	for position < len(bytes) {
		value, err := decodeAMF0(bytes, &position, 0)
		if err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	return values, nil
}

func decodeAMF0(bytes []byte, position *int, depth int) (amf0Value, error) {
	if depth >= amf0MaxDepth {
		return amf0Value{}, fmt.Errorf("AMF0 nesting exceeds %d levels", amf0MaxDepth)
	}
	marker, err := amf0Take(bytes, position, 1)
	if err != nil {
		return amf0Value{}, err
	}
	switch marker[0] {
	case amf0Number:
		raw, err := amf0Take(bytes, position, 8)
		if err != nil {
			return amf0Value{}, err
		}
		return amf0NumberValue(math.Float64frombits(binary.BigEndian.Uint64(raw))), nil
	case amf0Boolean:
		raw, err := amf0Take(bytes, position, 1)
		if err != nil {
			return amf0Value{}, err
		}
		return amf0Value{kind: amf0Boolean, flag: raw[0] != 0}, nil
	case amf0String:
		raw, err := amf0Take(bytes, position, 2)
		if err != nil {
			return amf0Value{}, err
		}
		text, err := amf0Text(bytes, position, int(binary.BigEndian.Uint16(raw)))
		if err != nil {
			return amf0Value{}, err
		}
		return amf0StringValue(text), nil
	case amf0Object:
		fields, err := amf0Properties(bytes, position, depth+1)
		if err != nil {
			return amf0Value{}, err
		}
		return amf0ObjectValue(fields), nil
	case amf0Null, amf0Undefined:
		return amf0NullValue, nil
	case amf0EcmaArray:
		if _, err := amf0Take(bytes, position, 4); err != nil { // count is untrusted
			return amf0Value{}, err
		}
		fields, err := amf0Properties(bytes, position, depth+1)
		if err != nil {
			return amf0Value{}, err
		}
		return amf0Value{kind: amf0EcmaArray, fields: fields}, nil
	case amf0Date:
		raw, err := amf0Take(bytes, position, 8)
		if err != nil {
			return amf0Value{}, err
		}
		if _, err := amf0Take(bytes, position, 2); err != nil {
			return amf0Value{}, err
		}
		return amf0NumberValue(math.Float64frombits(binary.BigEndian.Uint64(raw))), nil
	case amf0LongStr:
		raw, err := amf0Take(bytes, position, 4)
		if err != nil {
			return amf0Value{}, err
		}
		text, err := amf0Text(bytes, position, int(binary.BigEndian.Uint32(raw)))
		if err != nil {
			return amf0Value{}, err
		}
		return amf0StringValue(text), nil
	default:
		return amf0Value{}, fmt.Errorf("unsupported AMF0 type 0x%02x", marker[0])
	}
}

func amf0Properties(bytes []byte, position *int, depth int) (map[string]amf0Value, error) {
	fields := map[string]amf0Value{}
	for {
		raw, err := amf0Take(bytes, position, 2)
		if err != nil {
			return nil, err
		}
		length := int(binary.BigEndian.Uint16(raw))
		if length == 0 {
			end, err := amf0Take(bytes, position, 1)
			if err != nil {
				return nil, err
			}
			if end[0] != amf0ObjectEnd {
				return nil, fmt.Errorf("invalid AMF0 object end 0x%02x", end[0])
			}
			return fields, nil
		}
		name, err := amf0Text(bytes, position, length)
		if err != nil {
			return nil, err
		}
		value, err := decodeAMF0(bytes, position, depth)
		if err != nil {
			return nil, err
		}
		fields[name] = value
	}
}

func amf0Take(bytes []byte, position *int, length int) ([]byte, error) {
	if length < 0 || *position > len(bytes)-length {
		return nil, fmt.Errorf("truncated AMF0 value")
	}
	slice := bytes[*position : *position+length]
	*position += length
	return slice, nil
}

func amf0Text(bytes []byte, position *int, length int) (string, error) {
	slice, err := amf0Take(bytes, position, length)
	if err != nil {
		return "", err
	}
	if !utf8.Valid(slice) {
		return "", fmt.Errorf("AMF0 string is not UTF-8")
	}
	return string(slice), nil
}
