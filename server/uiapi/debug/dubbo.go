//go:build apidoc_debug_rpc || apidoc_debug_all

package debug

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"time"
)

const (
	dubboMagic         = 0xdabb
	dubboFlagRequest   = 0x80 | 0x40 | 0x02 // request | two-way | hessian2
	dubboSerialization = 2
	dubboStatusOK      = 20
)

func init() {
	RegisterExecutor("dubbo", executeDubbo)
	RegisterExecutor("hessian", executeDubbo)
}

func executeDubbo(cfg Config, req *Request) (*Response, error) {
	started := time.Now()
	service := req.Opts["service"]
	if service == "" {
		return nil, fmt.Errorf("dubbo requires opts.service")
	}
	method := req.Opts["method"]
	if method == "" {
		return nil, fmt.Errorf("dubbo requires opts.method")
	}
	version := req.Opts["version"]
	typesDesc := dubboParamTypes(req.Opts["param_types"])
	args, err := dubboArguments(req)
	if err != nil {
		return nil, err
	}
	if typesDesc == "" && len(args) > 0 {
		return nil, fmt.Errorf("dubbo requires opts.param_types (%d argument(s) in body)", len(args))
	}
	declared, err := countJavaDescriptors(typesDesc)
	if err != nil {
		return nil, err
	}
	if declared != len(args) {
		return nil, fmt.Errorf("dubbo opts.param_types declares %d parameter(s) but body has %d", declared, len(args))
	}

	body, err := encodeDubboRequest(service, version, method, typesDesc, args)
	if err != nil {
		return nil, err
	}
	if len(body) > cfg.Limit() {
		return nil, fmt.Errorf("dubbo request exceeds max_bytes (%d)", cfg.Limit())
	}
	if strings.TrimSpace(req.Target) == "" {
		return nil, fmt.Errorf("dubbo requires a target (nacos registry resolution is not implemented in this build)")
	}
	host, port, err := rpcAddr(req.Target, "tcp")
	if err != nil {
		return nil, err
	}
	addrs, err := CheckHost(cfg, host, port)
	if err != nil {
		return nil, err
	}
	peer := net.JoinHostPort(addrs[0].String(), port)
	responseBody, status, err := dubboRoundtrip(cfg, req, peer, body)
	if err != nil {
		return nil, err
	}
	value, err := parseDubboResponse(responseBody)
	if err != nil {
		return nil, err
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("dubbo response encode failed: %v", err)
	}
	return &Response{
		Status: 0,
		MS:     time.Since(started).Milliseconds(),
		Headers: map[string]string{
			"x-peer":          peer,
			"x-dubbo-service": service,
			"x-dubbo-method":  method,
			"x-dubbo-status":  strconv.Itoa(int(status)),
		},
		Body: Payload{Encoding: "utf8", Data: string(encoded)},
	}, nil
}

func dubboParamTypes(raw string) string {
	parts := []string{}
	for _, part := range strings.Split(raw, ",") {
		if part = strings.TrimSpace(part); part != "" {
			parts = append(parts, part)
		}
	}
	return strings.Join(parts, "")
}

func dubboArguments(req *Request) ([]any, error) {
	body, err := decodeJSONBody(req)
	if err != nil {
		return nil, fmt.Errorf("invalid dubbo request JSON: %v", err)
	}
	switch value := body.(type) {
	case []any:
		return value, nil
	case map[string]any:
		if len(value) == 0 {
			return []any{}, nil
		}
		if req.Endpoint == nil {
			return nil, fmt.Errorf("dubbo object body requires an endpoint schema (ep.params); send a JSON array instead")
		}
		known := map[string]bool{}
		for _, param := range req.Endpoint.Params {
			known[param.Name] = true
		}
		for key := range value {
			if !known[key] {
				return nil, fmt.Errorf("dubbo request body has unknown param `%s`", key)
			}
		}
		args := make([]any, 0, len(req.Endpoint.Params))
		for _, param := range req.Endpoint.Params {
			args = append(args, value[param.Name])
		}
		return args, nil
	case nil:
		return []any{}, nil
	default:
		return nil, fmt.Errorf("dubbo request body must be a JSON object or array")
	}
}

// countJavaDescriptors counts a JVM type descriptor such as
// `Ljava/lang/String;I[B` as 3 parameters.
func countJavaDescriptors(desc string) (int, error) {
	count, i := 0, 0
	for i < len(desc) {
		for i < len(desc) && desc[i] == '[' {
			i++
		}
		if i >= len(desc) {
			return 0, fmt.Errorf("invalid Java type descriptor `%s`", desc)
		}
		switch desc[i] {
		case 'L':
			end := strings.IndexByte(desc[i:], ';')
			if end < 0 {
				return 0, fmt.Errorf("invalid Java type descriptor `%s`", desc)
			}
			i += end + 1
		case 'Z', 'B', 'C', 'S', 'I', 'J', 'F', 'D':
			i++
		default:
			return 0, fmt.Errorf("invalid Java type descriptor `%s` at byte %d", desc, i)
		}
		count++
	}
	return count, nil
}

func encodeDubboRequest(service, version, method, typesDesc string, args []any) ([]byte, error) {
	out := []byte{}
	for _, value := range []any{"2.0.2", service, version, method, typesDesc} {
		encoded, err := hessianEncode(value)
		if err != nil {
			return nil, err
		}
		out = append(out, encoded...)
	}
	for _, arg := range args {
		encoded, err := hessianEncode(arg)
		if err != nil {
			return nil, err
		}
		out = append(out, encoded...)
	}
	encoded, err := hessianEncode(map[string]any{}) // attachments
	if err != nil {
		return nil, err
	}
	return append(out, encoded...), nil
}

func dubboRoundtrip(cfg Config, req *Request, peer string, body []byte) ([]byte, byte, error) {
	conn, err := dialRPC(cfg, req, "dubbo", "tcp", peer)
	if err != nil {
		return nil, 0, err
	}
	defer conn.Close()
	deadline := time.Now().Add(req.Timeout(cfg))
	_ = conn.SetDeadline(deadline)

	frame := make([]byte, 16, 16+len(body))
	binary.BigEndian.PutUint16(frame[0:], dubboMagic)
	frame[2] = dubboFlagRequest
	frame[3] = 0
	binary.BigEndian.PutUint32(frame[12:], uint32(len(body)))
	frame = append(frame, body...)
	if _, err := conn.Write(frame); err != nil {
		return nil, 0, dubboNetError(err, "dubbo write failed")
	}

	head := make([]byte, 16)
	if _, err := io.ReadFull(conn, head); err != nil {
		return nil, 0, dubboNetError(err, "dubbo read header failed")
	}
	if binary.BigEndian.Uint16(head[0:]) != dubboMagic {
		return nil, 0, fmt.Errorf("dubbo: invalid response magic")
	}
	if head[2]&0x80 != 0 {
		return nil, 0, fmt.Errorf("dubbo: expected a response frame")
	}
	if head[2]&0x1f != dubboSerialization {
		return nil, 0, fmt.Errorf("dubbo: unsupported response serialization id %d", head[2]&0x1f)
	}
	length := binary.BigEndian.Uint32(head[12:])
	if int(length) > cfg.Limit() {
		return nil, 0, fmt.Errorf("dubbo response exceeds max_bytes (%d)", cfg.Limit())
	}
	response := make([]byte, length)
	if _, err := io.ReadFull(conn, response); err != nil {
		return nil, 0, dubboNetError(err, "dubbo read body failed")
	}
	if head[3] != dubboStatusOK {
		return nil, 0, fmt.Errorf("dubbo status %d", head[3])
	}
	return response, head[3], nil
}

func dubboNetError(err error, label string) error {
	if ne, ok := err.(net.Error); ok && ne.Timeout() {
		return fmt.Errorf("dubbo request timed out")
	}
	return fmt.Errorf("%s: %v", label, err)
}

func parseDubboResponse(body []byte) (any, error) {
	if len(body) == 0 {
		return nil, fmt.Errorf("dubbo: empty response body")
	}
	flag, rest := body[0], body[1:]
	switch flag {
	case 1, 4:
		value, err := hessianDecode(&rest)
		if err != nil {
			return nil, err
		}
		if flag == 4 {
			if _, err := hessianDecode(&rest); err != nil {
				return nil, err
			}
		}
		return value, nil
	case 2, 5:
		return nil, nil
	case 0, 3:
		value, err := hessianDecode(&rest)
		if err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("dubbo exception: %s", dubboExceptionMessage(value))
	default:
		return nil, fmt.Errorf("dubbo: unknown response flag %d", flag)
	}
}

func dubboExceptionMessage(value any) string {
	if obj, ok := value.(map[string]any); ok {
		for _, key := range []string{"detailMessage", "message"} {
			if text, ok := obj[key].(string); ok {
				return text
			}
		}
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return fmt.Sprint(value)
	}
	return string(encoded)
}
