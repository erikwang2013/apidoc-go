//go:build apidoc_debug_rpc || apidoc_debug_all

package debug

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	thriftStop   = 0
	thriftBool   = 2
	thriftByte   = 3
	thriftDouble = 4
	thriftI16    = 6
	thriftI32    = 8
	thriftI64    = 10
	thriftString = 11
	thriftStruct = 12
	thriftMap    = 13
	thriftSet    = 14
	thriftList   = 15
)

const (
	thriftKindBool = iota
	thriftKindByte
	thriftKindI16
	thriftKindI32
	thriftKindI64
	thriftKindDouble
	thriftKindString
	thriftKindBinary
	thriftKindList
	thriftKindMap
	thriftKindStruct
)

type thriftSpec struct {
	kind   int
	elem   *thriftSpec
	key    *thriftSpec
	value  *thriftSpec
	fields []thriftField
}

type thriftField struct {
	name string
	id   int16
	spec *thriftSpec
}

func init() { RegisterExecutor("thrift", executeThrift) }

func (s *thriftSpec) wire() byte {
	switch s.kind {
	case thriftKindBool:
		return thriftBool
	case thriftKindByte:
		return thriftByte
	case thriftKindI16:
		return thriftI16
	case thriftKindI32:
		return thriftI32
	case thriftKindI64:
		return thriftI64
	case thriftKindDouble:
		return thriftDouble
	case thriftKindString, thriftKindBinary:
		return thriftString
	case thriftKindList:
		return thriftList
	case thriftKindMap:
		return thriftMap
	default:
		return thriftStruct
	}
}

func parseThriftSpec(ty string, children []Param) (*thriftSpec, error) {
	ty = strings.TrimSpace(ty)
	if inner, ok := strings.CutPrefix(ty, "list<"); ok && strings.HasSuffix(inner, ">") {
		elem, err := parseThriftSpec(inner[:len(inner)-1], nil)
		if err != nil {
			return nil, err
		}
		return &thriftSpec{kind: thriftKindList, elem: elem}, nil
	}
	if inner, ok := strings.CutPrefix(ty, "map<"); ok && strings.HasSuffix(inner, ">") {
		pair := inner[:len(inner)-1]
		comma := strings.IndexByte(pair, ',')
		if comma < 0 {
			return nil, fmt.Errorf("invalid thrift map type `%s`", ty)
		}
		key, err := parseThriftSpec(pair[:comma], nil)
		if err != nil {
			return nil, err
		}
		value, err := parseThriftSpec(pair[comma+1:], nil)
		if err != nil {
			return nil, err
		}
		return &thriftSpec{kind: thriftKindMap, key: key, value: value}, nil
	}
	switch ty {
	case "bool":
		return &thriftSpec{kind: thriftKindBool}, nil
	case "byte", "i8":
		return &thriftSpec{kind: thriftKindByte}, nil
	case "i16":
		return &thriftSpec{kind: thriftKindI16}, nil
	case "i32", "int":
		return &thriftSpec{kind: thriftKindI32}, nil
	case "i64", "long":
		return &thriftSpec{kind: thriftKindI64}, nil
	case "double", "float":
		return &thriftSpec{kind: thriftKindDouble}, nil
	case "string":
		return &thriftSpec{kind: thriftKindString}, nil
	case "binary", "bytes":
		return &thriftSpec{kind: thriftKindBinary}, nil
	case "enum":
		return &thriftSpec{kind: thriftKindI32}, nil
	case "struct", "object":
		spec := &thriftSpec{kind: thriftKindStruct}
		for _, child := range children {
			id, err := strconv.ParseInt(strings.TrimSpace(child.Field), 10, 16)
			if err != nil {
				return nil, fmt.Errorf("thrift struct field `%s` requires field", child.Name)
			}
			nested, err := parseThriftSpec(child.Type, child.Children)
			if err != nil {
				return nil, err
			}
			spec.fields = append(spec.fields, thriftField{name: child.Name, id: int16(id), spec: nested})
		}
		return spec, nil
	default:
		return nil, fmt.Errorf("unsupported thrift type `%s`", ty)
	}
}

func thriftWriteMessageBegin(out *[]byte, name string, msgType byte, seqID int32) {
	*out = binary.BigEndian.AppendUint32(*out, uint32(0x80010000)|uint32(msgType))
	*out = binary.BigEndian.AppendUint32(*out, uint32(len(name)))
	*out = append(*out, name...)
	*out = binary.BigEndian.AppendUint32(*out, uint32(seqID))
}

func thriftWriteField(out *[]byte, wire byte, id int16) {
	*out = append(*out, wire)
	*out = binary.BigEndian.AppendUint16(*out, uint16(id))
}

func executeThrift(cfg Config, req *Request) (*Response, error) {
	started := time.Now()
	method := req.Opts["method"]
	if method == "" {
		return nil, fmt.Errorf("thrift requires opts.method")
	}
	frame, err := buildThriftRequest(req, method)
	if err != nil {
		return nil, err
	}
	if len(frame) > cfg.Limit() {
		return nil, fmt.Errorf("request exceeds max_bytes (%d)", cfg.Limit())
	}
	transport := req.Opts["transport"]
	var responseBytes []byte
	switch transport {
	case "http":
		responseBytes, err = thriftHTTPRoundtrip(cfg, req, frame)
	case "", "tcp":
		responseBytes, err = thriftTCPRoundtrip(cfg, req, frame)
	default:
		return nil, fmt.Errorf("unsupported thrift transport `%s`", transport)
	}
	if err != nil {
		return nil, err
	}
	value, complete, err := parseThriftResponse(responseBytes, req.Endpoint)
	if err != nil {
		return nil, fmt.Errorf("thrift response parse failed: %v", err)
	}
	if !complete {
		return nil, fmt.Errorf("incomplete thrift response")
	}
	body, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("thrift response encode failed: %v", err)
	}
	return &Response{
		Status:  0,
		MS:      time.Since(started).Milliseconds(),
		Headers: map[string]string{"x-thrift-method": method},
		Body:    Payload{Encoding: "utf8", Data: string(body)},
	}, nil
}

func buildThriftRequest(req *Request, method string) ([]byte, error) {
	body, err := decodeJSONBody(req)
	if err != nil {
		return nil, fmt.Errorf("invalid thrift request JSON: %v", err)
	}
	values, _ := body.(map[string]any)
	if req.Endpoint == nil {
		return nil, fmt.Errorf("thrift requires endpoint schema (params/returned)")
	}
	out := []byte{}
	thriftWriteMessageBegin(&out, method, 1, 0)
	for _, param := range req.Endpoint.Params {
		spec, err := parseThriftSpec(param.Type, param.Children)
		if err != nil {
			return nil, err
		}
		id, err := strconv.ParseInt(strings.TrimSpace(param.Field), 10, 16)
		if err != nil {
			return nil, fmt.Errorf("thrift param `%s` requires field", param.Name)
		}
		if value, ok := values[param.Name]; ok {
			thriftWriteField(&out, spec.wire(), int16(id))
			if err := thriftWriteValue(&out, spec, value); err != nil {
				return nil, err
			}
		} else if param.Required {
			return nil, fmt.Errorf("thrift missing required param `%s`", param.Name)
		}
	}
	out = append(out, thriftStop)
	return out, nil
}

func parseThriftResponse(data []byte, ep *Endpoint) (any, bool, error) {
	r := &thriftReader{data: data}
	_, msgType, _, err := r.messageBegin()
	if errors.Is(err, errIncomplete) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	if msgType == 3 {
		ex, err := thriftReadStruct(r, nil)
		if err != nil {
			return nil, false, err
		}
		message := "thrift exception"
		if obj, ok := ex.(map[string]any); ok {
			if text, ok := obj["field_1"].(string); ok {
				message = text
			}
		}
		return nil, false, fmt.Errorf("thrift exception: %s", message)
	}
	var spec *thriftSpec
	if ep != nil {
		returned := &thriftSpec{kind: thriftKindStruct}
		for _, param := range ep.Returned {
			id, err := strconv.ParseInt(strings.TrimSpace(param.Field), 10, 16)
			if err != nil {
				return nil, false, fmt.Errorf("thrift returned `%s` requires field", param.Name)
			}
			nested, err := parseThriftSpec(param.Type, param.Children)
			if err != nil {
				return nil, false, err
			}
			returned.fields = append(returned.fields, thriftField{name: param.Name, id: int16(id), spec: nested})
		}
		spec = returned
	}
	value, err := thriftReadStruct(r, spec)
	if errors.Is(err, errIncomplete) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return value, true, nil
}

type thriftReader struct {
	data []byte
	off  int
}

func (r *thriftReader) take(length int) ([]byte, error) {
	if length < 0 {
		return nil, fmt.Errorf("invalid thrift length %d", length)
	}
	if len(r.data)-r.off < length {
		return nil, errIncomplete
	}
	out := r.data[r.off : r.off+length]
	r.off += length
	return out, nil
}

func (r *thriftReader) u8() (byte, error) {
	data, err := r.take(1)
	if err != nil {
		return 0, err
	}
	return data[0], nil
}

func (r *thriftReader) i16() (int16, error) {
	data, err := r.take(2)
	if err != nil {
		return 0, err
	}
	return int16(binary.BigEndian.Uint16(data)), nil
}

func (r *thriftReader) i32() (int32, error) {
	data, err := r.take(4)
	if err != nil {
		return 0, err
	}
	return int32(binary.BigEndian.Uint32(data)), nil
}

func (r *thriftReader) i64() (int64, error) {
	data, err := r.take(8)
	if err != nil {
		return 0, err
	}
	return int64(binary.BigEndian.Uint64(data)), nil
}

func (r *thriftReader) f64() (float64, error) {
	value, err := r.i64()
	return math.Float64frombits(uint64(value)), err
}

func (r *thriftReader) messageBegin() (string, byte, int32, error) {
	first, err := r.i32()
	if err != nil {
		return "", 0, 0, err
	}
	var name string
	var msgType byte
	if first < 0 {
		if uint32(first)&0xffff0000 != 0x80010000 {
			return "", 0, 0, fmt.Errorf("unsupported thrift version 0x%08x", uint32(first))
		}
		length, err := r.i32()
		if err != nil {
			return "", 0, 0, err
		}
		name, err = r.string(length)
		if err != nil {
			return "", 0, 0, err
		}
		msgType = byte(first & 0xff)
	} else {
		name, err = r.string(first)
		if err != nil {
			return "", 0, 0, err
		}
		msgType, err = r.u8()
		if err != nil {
			return "", 0, 0, err
		}
	}
	seqID, err := r.i32()
	if err != nil {
		return "", 0, 0, err
	}
	return name, msgType, seqID, nil
}

func (r *thriftReader) string(length int32) (string, error) {
	data, err := r.take(int(length))
	if err != nil {
		return "", err
	}
	return strings.ToValidUTF8(string(data), "�"), nil
}

func (r *thriftReader) fieldBegin() (byte, int16, bool, error) {
	wire, err := r.u8()
	if err != nil {
		return 0, 0, false, err
	}
	if wire == thriftStop {
		return 0, 0, false, nil
	}
	id, err := r.i16()
	if err != nil {
		return 0, 0, false, err
	}
	return wire, id, true, nil
}

func thriftWriteValue(out *[]byte, spec *thriftSpec, value any) error {
	switch spec.kind {
	case thriftKindBool:
		v, ok := value.(bool)
		if !ok {
			return fmt.Errorf("expected bool")
		}
		if v {
			*out = append(*out, 1)
		} else {
			*out = append(*out, 0)
		}
	case thriftKindByte, thriftKindI16, thriftKindI32, thriftKindI64:
		v, ok := thriftInt(value)
		if !ok {
			return fmt.Errorf("expected integer")
		}
		switch spec.kind {
		case thriftKindByte:
			*out = append(*out, byte(int8(v)))
		case thriftKindI16:
			*out = binary.BigEndian.AppendUint16(*out, uint16(int16(v)))
		case thriftKindI32:
			*out = binary.BigEndian.AppendUint32(*out, uint32(int32(v)))
		default:
			*out = binary.BigEndian.AppendUint64(*out, uint64(v))
		}
	case thriftKindDouble:
		v, ok := thriftFloat(value)
		if !ok {
			return fmt.Errorf("expected double")
		}
		*out = binary.BigEndian.AppendUint64(*out, math.Float64bits(v))
	case thriftKindString:
		text, ok := value.(string)
		if !ok {
			return fmt.Errorf("expected string")
		}
		*out = binary.BigEndian.AppendUint32(*out, uint32(len(text)))
		*out = append(*out, text...)
	case thriftKindBinary:
		text, ok := value.(string)
		if !ok {
			return fmt.Errorf("expected base64 string for binary")
		}
		raw, err := base64.StdEncoding.DecodeString(text)
		if err != nil {
			return fmt.Errorf("invalid base64 binary param: %v", err)
		}
		*out = binary.BigEndian.AppendUint32(*out, uint32(len(raw)))
		*out = append(*out, raw...)
	case thriftKindList:
		items, ok := value.([]any)
		if !ok {
			return fmt.Errorf("expected array")
		}
		*out = append(*out, spec.elem.wire())
		*out = binary.BigEndian.AppendUint32(*out, uint32(len(items)))
		for _, item := range items {
			if err := thriftWriteValue(out, spec.elem, item); err != nil {
				return err
			}
		}
	case thriftKindMap:
		items, ok := value.(map[string]any)
		if !ok {
			return fmt.Errorf("expected object")
		}
		*out = append(*out, spec.key.wire(), spec.value.wire())
		*out = binary.BigEndian.AppendUint32(*out, uint32(len(items)))
		keys := make([]string, 0, len(items))
		for key := range items {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			if err := thriftWriteMapKey(out, spec.key, key); err != nil {
				return err
			}
			if err := thriftWriteValue(out, spec.value, items[key]); err != nil {
				return err
			}
		}
	case thriftKindStruct:
		fields, _ := value.(map[string]any)
		for _, field := range spec.fields {
			if v, ok := fields[field.name]; ok {
				thriftWriteField(out, field.spec.wire(), field.id)
				if err := thriftWriteValue(out, field.spec, v); err != nil {
					return err
				}
			}
		}
		*out = append(*out, thriftStop)
	}
	return nil
}

func thriftWriteMapKey(out *[]byte, spec *thriftSpec, key string) error {
	switch spec.kind {
	case thriftKindString:
		*out = binary.BigEndian.AppendUint32(*out, uint32(len(key)))
		*out = append(*out, key...)
	case thriftKindBool:
		if key == "true" {
			*out = append(*out, 1)
		} else {
			*out = append(*out, 0)
		}
	case thriftKindByte, thriftKindI16, thriftKindI32, thriftKindI64:
		number, err := strconv.ParseInt(key, 10, 64)
		if err != nil {
			return fmt.Errorf("invalid numeric map key `%s`", key)
		}
		switch spec.kind {
		case thriftKindByte:
			*out = append(*out, byte(int8(number)))
		case thriftKindI16:
			*out = binary.BigEndian.AppendUint16(*out, uint16(int16(number)))
		case thriftKindI32:
			*out = binary.BigEndian.AppendUint32(*out, uint32(int32(number)))
		default:
			*out = binary.BigEndian.AppendUint64(*out, uint64(number))
		}
	default:
		return fmt.Errorf("unsupported thrift map key type")
	}
	return nil
}

func thriftReadValue(r *thriftReader, wire byte, spec *thriftSpec) (any, error) {
	switch wire {
	case thriftBool:
		value, err := r.u8()
		return value != 0, err
	case thriftByte:
		value, err := r.u8()
		return int64(int8(value)), err
	case thriftI16:
		value, err := r.i16()
		return int64(value), err
	case thriftI32:
		value, err := r.i32()
		return int64(value), err
	case thriftI64:
		return r.i64()
	case thriftDouble:
		return r.f64()
	case thriftString:
		length, err := r.i32()
		if err != nil {
			return nil, err
		}
		data, err := r.take(int(length))
		if err != nil {
			return nil, err
		}
		if spec != nil && spec.kind == thriftKindBinary {
			return base64.StdEncoding.EncodeToString(data), nil
		}
		return strings.ToValidUTF8(string(data), "�"), nil
	case thriftStruct:
		return thriftReadStruct(r, spec)
	case thriftList, thriftSet:
		return thriftReadList(r, spec)
	case thriftMap:
		return thriftReadMap(r, spec)
	default:
		return nil, fmt.Errorf("unsupported thrift wire type %d", wire)
	}
}

func thriftReadStruct(r *thriftReader, spec *thriftSpec) (any, error) {
	var fields []thriftField
	if spec != nil && spec.kind == thriftKindStruct {
		fields = spec.fields
	}
	out := map[string]any{}
	for {
		wire, id, present, err := r.fieldBegin()
		if err != nil {
			return nil, err
		}
		if !present {
			return out, nil
		}
		var known *thriftField
		for i := range fields {
			if fields[i].id == id {
				known = &fields[i]
				break
			}
		}
		var fieldSpec *thriftSpec
		if known != nil {
			fieldSpec = known.spec
		}
		value, err := thriftReadValue(r, wire, fieldSpec)
		if err != nil {
			return nil, err
		}
		name := fmt.Sprintf("field_%d", id)
		if known != nil {
			name = known.name
		}
		out[name] = value
	}
}

func thriftReadList(r *thriftReader, spec *thriftSpec) (any, error) {
	elemWire, err := r.u8()
	if err != nil {
		return nil, err
	}
	length, err := r.i32()
	if err != nil {
		return nil, err
	}
	var elemSpec *thriftSpec
	if spec != nil && spec.kind == thriftKindList {
		elemSpec = spec.elem
	}
	items := []any{}
	for i := int32(0); i < length; i++ {
		item, err := thriftReadValue(r, elemWire, elemSpec)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, nil
}

func thriftReadMap(r *thriftReader, spec *thriftSpec) (any, error) {
	keyWire, err := r.u8()
	if err != nil {
		return nil, err
	}
	valueWire, err := r.u8()
	if err != nil {
		return nil, err
	}
	length, err := r.i32()
	if err != nil {
		return nil, err
	}
	var keySpec, valueSpec *thriftSpec
	if spec != nil && spec.kind == thriftKindMap {
		keySpec, valueSpec = spec.key, spec.value
	}
	out := map[string]any{}
	for i := int32(0); i < length; i++ {
		key, err := thriftReadValue(r, keyWire, keySpec)
		if err != nil {
			return nil, err
		}
		text, ok := key.(string)
		if !ok {
			encoded, err := json.Marshal(key)
			if err != nil {
				return nil, err
			}
			text = string(encoded)
		}
		value, err := thriftReadValue(r, valueWire, valueSpec)
		if err != nil {
			return nil, err
		}
		out[text] = value
	}
	return out, nil
}

func thriftTCPRoundtrip(cfg Config, req *Request, frame []byte) ([]byte, error) {
	host, port, err := rpcAddr(req.Target, "tcp")
	if err != nil {
		return nil, err
	}
	addrs, err := CheckHost(cfg, host, port)
	if err != nil {
		return nil, err
	}
	conn, err := dialRPC(cfg, req, "thrift tcp", "tcp", net.JoinHostPort(addrs[0].String(), port))
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	_ = conn.SetWriteDeadline(time.Now().Add(req.Timeout(cfg)))
	if _, err := conn.Write(frame); err != nil {
		return nil, fmt.Errorf("thrift tcp write failed: %v", err)
	}
	deadline := time.Now().Add(rpcReadWindow(cfg, req))
	buf := []byte{}
	chunk := make([]byte, 4096)
	for {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return buf, nil
		}
		if err := conn.SetReadDeadline(time.Now().Add(remaining)); err != nil {
			return nil, fmt.Errorf("thrift tcp read failed: %v", err)
		}
		n, err := conn.Read(chunk)
		if n > 0 {
			buf = append(buf, chunk[:n]...)
			if len(buf) > cfg.Limit() {
				return nil, fmt.Errorf("response exceeds max_bytes (%d)", cfg.Limit())
			}
			_, complete, parseErr := parseThriftResponse(buf, req.Endpoint)
			if parseErr != nil {
				return nil, parseErr
			}
			if complete {
				return buf, nil
			}
		}
		if err != nil {
			if ne, ok := err.(net.Error); ok && ne.Timeout() {
				return buf, nil
			}
			if errors.Is(err, io.EOF) {
				return buf, nil
			}
			return nil, fmt.Errorf("thrift tcp read failed: %v", err)
		}
	}
}

func thriftHTTPRoundtrip(cfg Config, req *Request, frame []byte) ([]byte, error) {
	target, err := CheckTarget(cfg, req.Target)
	if err != nil {
		return nil, err
	}
	client := &http.Client{
		Timeout: req.Timeout(cfg),
		Transport: &http.Transport{
			Proxy:             nil,
			DialContext:       target.DialContext,
			DisableKeepAlives: true,
		},
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	httpReq, err := http.NewRequest(http.MethodPost, target.URL.String(), bytes.NewReader(frame))
	if err != nil {
		return nil, fmt.Errorf("thrift http request failed: %v", err)
	}
	httpReq.Header.Set("Content-Type", "application/x-thrift")
	response, err := client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("thrift http request failed: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("thrift http status %d", response.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, int64(cfg.Limit())+1))
	if err != nil {
		return nil, fmt.Errorf("thrift http read failed: %v", err)
	}
	if len(body) > cfg.Limit() {
		return nil, fmt.Errorf("response exceeds max_bytes (%d)", cfg.Limit())
	}
	return body, nil
}

func thriftInt(value any) (int64, bool) {
	switch v := value.(type) {
	case json.Number:
		n, err := v.Int64()
		return n, err == nil
	case int:
		return int64(v), true
	case int64:
		return v, true
	default:
		return 0, false
	}
}

func thriftFloat(value any) (float64, bool) {
	switch v := value.(type) {
	case json.Number:
		f, err := v.Float64()
		return f, err == nil
	case float64:
		return v, true
	default:
		return 0, false
	}
}
