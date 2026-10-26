//go:build apidoc_debug_iot || apidoc_debug_all

package debug

import (
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

func init() {
	RegisterExecutor("lwm2m", executeLwM2M)
}

func executeLwM2M(cfg Config, req *Request) (*Response, error) {
	started := time.Now()
	host, port, err := parseProtocolTarget(req.Target, "coap", coapDefaultPort, "coap")
	if err != nil {
		return nil, err
	}
	duration := protocolWindow(cfg, req)
	operation := req.Opts["operation"]
	if operation == "" {
		operation = "register"
	}
	token := []byte{0xA5, 0x5A}

	var request []byte
	var expect string
	switch operation {
	case "register":
		endpoint, err := lwm2mRequired(req, "endpoint")
		if err != nil {
			return nil, err
		}
		lifetime := req.Opts["lifetime"]
		if lifetime == "" {
			lifetime = "300"
		}
		parsed, err := strconv.ParseUint(lifetime, 10, 32)
		if err != nil {
			return nil, fmt.Errorf("invalid lifetime `%s`", lifetime)
		}
		payload, err := req.Body.Decode()
		if err != nil {
			return nil, err
		}
		if len(payload) > cfg.Limit() {
			return nil, fmt.Errorf("register payload exceeds max_bytes (%d)", cfg.Limit())
		}
		request, err = coapBuildRequest("POST", "/rd", [][2]string{
			{"ep", endpoint},
			{"lt", strconv.FormatUint(parsed, 10)},
			{"lwm2m", "1.1"},
			{"b", "U"},
		}, payload, coapContentFormatTLV, false, 1, token)
		if err != nil {
			return nil, err
		}
		expect = "2.01"
	case "update":
		location, err := lwm2mRequired(req, "location")
		if err != nil {
			return nil, err
		}
		payload, err := req.Body.Decode()
		if err != nil {
			return nil, err
		}
		request, err = coapBuildRequest("POST", location, nil, payload, coapContentFormatTLV, false, 2, token)
		if err != nil {
			return nil, err
		}
		expect = "2.04"
	case "deregister":
		location, err := lwm2mRequired(req, "location")
		if err != nil {
			return nil, err
		}
		request, err = coapBuildRequest("DELETE", location, nil, nil, 0, false, 3, token)
		if err != nil {
			return nil, err
		}
		expect = "2.02"
	case "read":
		path, err := lwm2mResourcePath(req)
		if err != nil {
			return nil, err
		}
		request, err = coapBuildRequest("GET", path, nil, nil, 0, false, 4, token)
		if err != nil {
			return nil, err
		}
		expect = "2.05"
	case "write":
		path, err := lwm2mResourcePath(req)
		if err != nil {
			return nil, err
		}
		value, err := lwm2mRequired(req, "value")
		if err != nil {
			return nil, err
		}
		resource, err := lwm2mResourceID(req)
		if err != nil {
			return nil, err
		}
		encoded := lwm2mEncodeResource(resource, value)
		request, err = coapBuildRequest("PUT", path, nil, encoded, coapContentFormatTLV, false, 5, token)
		if err != nil {
			return nil, err
		}
		expect = "2.04"
	default:
		return nil, fmt.Errorf("unsupported lwm2m operation `%s`; use register, update, deregister, read or write", operation)
	}
	if len(request) > cfg.Limit() {
		return nil, fmt.Errorf("coap request exceeds max_bytes (%d)", cfg.Limit())
	}

	conn, err := dialPinned(cfg, "udp", host, port, req.Timeout(cfg))
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	responses, err := coapRoundtrip(cfg, conn, request, duration, 1)
	if err != nil {
		return nil, err
	}
	response := responses[0]
	code := coapCodeString(response.code)
	if code != expect {
		return nil, fmt.Errorf("lwm2m %s failed: expected %s, got %s%s",
			operation, expect, code, lwm2mPayloadSuffix(response))
	}

	var body any
	switch operation {
	case "register":
		location := ""
		for _, segment := range coapOptionStrings(response, 8) {
			location += "/" + segment
		}
		body = map[string]any{"code": code, "location": location}
	case "read":
		format := uint32(coapOptionUint(response, 12))
		body = map[string]any{"code": code, "value": lwm2mDecodeValue(response.payload, format)}
	default:
		body = map[string]any{"code": code}
	}
	encoded, err := json.Marshal(body)
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

func lwm2mRequired(req *Request, key string) (string, error) {
	value, found := req.Opts[key]
	if !found {
		return "", fmt.Errorf("lwm2m %s is required", key)
	}
	return value, nil
}

func lwm2mResourceID(req *Request) (uint16, error) {
	value, err := strconv.ParseUint(req.Opts["resource_id"], 10, 16)
	if err != nil {
		return 0, fmt.Errorf("invalid resource_id")
	}
	return uint16(value), nil
}

func lwm2mResourcePath(req *Request) (string, error) {
	object, err := lwm2mRequired(req, "object_id")
	if err != nil {
		return "", err
	}
	instance, err := lwm2mRequired(req, "instance_id")
	if err != nil {
		return "", err
	}
	resource, err := lwm2mRequired(req, "resource_id")
	if err != nil {
		return "", err
	}
	if _, err := lwm2mResourceID(req); err != nil {
		return "", err
	}
	return "/" + object + "/" + instance + "/" + resource, nil
}

func lwm2mPayloadSuffix(packet coapPacket) string {
	if len(packet.payload) == 0 {
		return ""
	}
	return ": " + string(packet.payload)
}

// lwm2mDecodeValue guesses a single-resource TLV value: zero length is 0,
// 1/2/4/8 bytes are signed integers, anything else is text or base64.
// shortcut: a 1-byte boolean is indistinguishable from 0/1 integers.
func lwm2mDecodeValue(payload []byte, contentFormat uint32) any {
	if contentFormat != coapContentFormatTLV {
		if utf8.Valid(payload) {
			return string(payload)
		}
		return map[string]any{"encoding": "base64", "data": base64.StdEncoding.EncodeToString(payload)}
	}
	_, value, found, err := lwm2mDecodeFirst(payload)
	if err != nil {
		return map[string]any{
			"error":    err.Error(),
			"encoding": "base64",
			"data":     base64.StdEncoding.EncodeToString(payload),
		}
	}
	if !found {
		return nil
	}
	switch len(value) {
	case 0:
		return int64(0)
	case 1, 2, 4, 8:
		return lwm2mSignedBE(value)
	default:
		if utf8.Valid(value) {
			return string(value)
		}
		return map[string]any{"encoding": "base64", "data": base64.StdEncoding.EncodeToString(value)}
	}
}

func lwm2mSignedBE(value []byte) int64 {
	var unsigned uint64
	for _, b := range value {
		unsigned = unsigned<<8 | uint64(b)
	}
	shift := uint(64 - 8*len(value))
	return int64(unsigned<<shift) >> shift
}

func lwm2mValueBytes(value string) []byte {
	if integer, err := strconv.ParseInt(value, 10, 64); err == nil {
		switch {
		case integer == 0:
			return nil
		case integer >= -128 && integer <= 127:
			return []byte{byte(integer)}
		case integer >= -32768 && integer <= 32767:
			out := make([]byte, 2)
			binary.BigEndian.PutUint16(out, uint16(int16(integer)))
			return out
		case integer >= -2147483648 && integer <= 2147483647:
			out := make([]byte, 4)
			binary.BigEndian.PutUint32(out, uint32(int32(integer)))
			return out
		default:
			out := make([]byte, 8)
			binary.BigEndian.PutUint64(out, uint64(integer))
			return out
		}
	}
	if strings.EqualFold(value, "true") {
		return []byte{1}
	}
	if strings.EqualFold(value, "false") {
		return []byte{0}
	}
	return []byte(value)
}

func lwm2mEncodeResource(id uint16, value string) []byte {
	bytes := lwm2mValueBytes(value)
	wide := id > 0xFF
	lengthType := byte(0)
	inline := byte(len(bytes))
	switch {
	case len(bytes) <= 7:
	case len(bytes) <= 0xFF:
		lengthType, inline = 1, 0
	case len(bytes) <= 0xFFFF:
		lengthType, inline = 2, 0
	default:
		lengthType, inline = 3, 0
	}
	out := []byte{0xC0 | boolByte(wide)<<5 | lengthType<<3 | inline}
	if wide {
		out = binary.BigEndian.AppendUint16(out, id)
	} else {
		out = append(out, byte(id))
	}
	switch lengthType {
	case 1:
		out = append(out, byte(len(bytes)))
	case 2:
		out = binary.BigEndian.AppendUint16(out, uint16(len(bytes)))
	case 3:
		out = append(out, byte(len(bytes)>>16), byte(len(bytes)>>8), byte(len(bytes)))
	}
	return append(out, bytes...)
}

func boolByte(value bool) byte {
	if value {
		return 1
	}
	return 0
}

// lwm2mDecodeFirst returns the first TLV resource entry.
func lwm2mDecodeFirst(data []byte) (uint16, []byte, bool, error) {
	if len(data) == 0 {
		return 0, nil, false, nil
	}
	header := data[0]
	if header>>6 != 0b11 {
		return 0, nil, false, fmt.Errorf("unsupported tlv entry type %#b", header>>6)
	}
	wide := header&0x20 != 0
	lengthType := (header >> 3) & 0x03
	inline := int(header & 0x07)
	pos := 1
	var id uint16
	if wide {
		if pos+2 > len(data) {
			return 0, nil, false, fmt.Errorf("truncated tlv id")
		}
		id = binary.BigEndian.Uint16(data[pos:])
		pos += 2
	} else {
		if pos >= len(data) {
			return 0, nil, false, fmt.Errorf("truncated tlv id")
		}
		id = uint16(data[pos])
		pos++
	}
	length := 0
	switch lengthType {
	case 0:
		length = inline
	case 1:
		if pos >= len(data) {
			return 0, nil, false, fmt.Errorf("truncated tlv length")
		}
		length = int(data[pos])
		pos++
	case 2:
		if pos+2 > len(data) {
			return 0, nil, false, fmt.Errorf("truncated tlv length")
		}
		length = int(binary.BigEndian.Uint16(data[pos:]))
		pos += 2
	default:
		if pos+3 > len(data) {
			return 0, nil, false, fmt.Errorf("truncated tlv length")
		}
		length = int(data[pos])<<16 | int(data[pos+1])<<8 | int(data[pos+2])
		pos += 3
	}
	if pos+length > len(data) {
		return 0, nil, false, fmt.Errorf("truncated tlv value")
	}
	return id, data[pos : pos+length], true, nil
}
