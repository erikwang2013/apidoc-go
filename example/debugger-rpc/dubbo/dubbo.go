// Package dubbodbg is an example Apache Dubbo debug adapter.
//
// It implements the Dubbo TCP frame (magic, flags, request id, body length)
// and the request/response body layout, with Hessian2 serialization from the
// official dubbo-go-hessian2 library. The body written in the debug panel is a
// JSON array of positional arguments.
package dubbodbg

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"

	hessian "github.com/apache/dubbo-go-hessian2"
	"github.com/erikwang2013/apidoc-go"
	"github.com/erikwang2013/apidoc-go/example/debugger-rpc/internal/xjson"
)

const (
	magic0 = 0xda
	magic1 = 0xbb

	// request | twoway | hessian2 serialization id
	requestFlag  = 0xc2
	responseFlag = 0x02
	statusOK     = 0x14 // 20, Dubbo's RESPONSE_OK

	// Response body flags written by Dubbo before the Hessian value.
	responseWithException = 0x00
	responseValue         = 0x01
	responseNull          = 0x02
	responseExceptionAtt  = 0x03
	responseValueAtt      = 0x04
	responseNullAtt       = 0x05

	maxBody = 4 << 20
)

// New returns the Dubbo adapter.
func New() apidoc.Debugger {
	return apidoc.DebuggerFunc("dubbo", apidoc.DebugSpec{
		Mode:         "server",
		Actions:      []string{"CALL"},
		Encodings:    []string{"json"},
		BodyIn:       "payload",
		ResponseView: "json",
		ConnectFields: []apidoc.DebugField{
			{Name: "timeout_ms", Label: "Timeout (ms)", Type: "number", Default: "5000"},
			{Name: "param_types", Label: "JVM parameter types", Placeholder: "Ljava/lang/String;J", Type: "text"},
		},
	}, exec)
}

type endpoint struct {
	addr     string
	service  string
	method   string
	version  string
	override string
}

func parseURL(raw string) (endpoint, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return endpoint{}, fmt.Errorf("dubbo: bad url %q: %w", raw, err)
	}
	if u.Scheme != "dubbo" {
		return endpoint{}, fmt.Errorf("dubbo: url scheme must be dubbo, got %q", u.Scheme)
	}
	if u.Host == "" {
		return endpoint{}, fmt.Errorf("dubbo: url %q has no host", raw)
	}
	p := strings.Trim(u.Path, "/")
	ep := endpoint{addr: u.Host, version: u.Query().Get("version"), override: u.Query().Get("param_types")}
	i := strings.LastIndexByte(p, '/')
	if i > 0 {
		ep.service, ep.method = p[:i], p[i+1:]
	} else {
		ep.service, ep.method = p, u.Query().Get("method")
	}
	if ep.service == "" || ep.method == "" {
		return endpoint{}, errors.New("dubbo: url path must be /<service>/<method>")
	}
	return ep, nil
}

func exec(ctx context.Context, req apidoc.DebugRequest) (*apidoc.DebugResponse, error) {
	start := time.Now()
	ep, err := parseURL(req.URL)
	if err != nil {
		return nil, err
	}
	timeout := 5 * time.Second
	if ms, err := strconv.Atoi(req.Options["timeout_ms"]); err == nil && ms > 0 {
		timeout = time.Duration(ms) * time.Millisecond
	}
	args, err := xjson.Args(req.Body)
	if err != nil {
		return nil, fmt.Errorf("dubbo: %w", err)
	}
	types := req.Options["param_types"]
	if types == "" {
		types = ep.override
	}
	body, err := encodeRequest(ep, args, types)
	if err != nil {
		return nil, err
	}

	dialer := net.Dialer{Timeout: timeout}
	conn, err := dialer.DialContext(ctx, "tcp", ep.addr)
	if err != nil {
		return nil, fmt.Errorf("dubbo: dial %s: %w", ep.addr, err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(timeout))

	if err := writeFrame(conn, requestFlag, 0, 1, body); err != nil {
		return nil, err
	}
	flag, status, id, respBody, err := readFrame(conn)
	if err != nil {
		return nil, err
	}
	if flag&0x80 != 0 {
		return nil, errors.New("dubbo: expected a response frame")
	}
	if id != 1 {
		return nil, fmt.Errorf("dubbo: response id %d does not match request", id)
	}
	if status != statusOK {
		return nil, fmt.Errorf("dubbo: server status %d", status)
	}
	value, remote, err := decodeResponse(respBody)
	if err != nil {
		return nil, err
	}
	out, err := json.MarshalIndent(xjson.Value(value), "", "  ")
	if err != nil {
		return nil, fmt.Errorf("dubbo: encode response: %w", err)
	}
	statusText := "OK"
	if remote {
		statusText = "ERROR"
	}
	return &apidoc.DebugResponse{
		Status:   statusText,
		Headers:  map[string]string{"remote": ep.addr, "service": ep.service + "/" + ep.method},
		Body:     out,
		Encoding: "json",
		Duration: time.Since(start),
	}, nil
}

func encodeRequest(ep endpoint, args []any, types string) ([]byte, error) {
	if types == "" {
		types = javaTypes(args)
	}
	e := hessian.NewEncoder()
	for _, s := range []string{"2.0.2", ep.service, ep.version, ep.method, types} {
		if err := e.Encode(s); err != nil {
			return nil, fmt.Errorf("dubbo: encode request: %w", err)
		}
	}
	for i, arg := range args {
		if err := e.Encode(arg); err != nil {
			return nil, fmt.Errorf("dubbo: encode argument %d: %w", i, err)
		}
	}
	return e.Buffer(), nil
}

// javaTypes infers JVM descriptors from the JSON argument types, so callers
// that do not set param_types still send a usable invocation.
func javaTypes(args []any) string {
	var b strings.Builder
	for _, arg := range args {
		switch arg.(type) {
		case string:
			b.WriteString("Ljava/lang/String;")
		case bool:
			b.WriteString("Z")
		case int64:
			b.WriteString("J")
		case float64:
			b.WriteString("D")
		case []any:
			b.WriteString("Ljava/util/List;")
		case map[string]any:
			b.WriteString("Ljava/util/Map;")
		default:
			b.WriteString("Ljava/lang/Object;")
		}
	}
	return b.String()
}

func writeFrame(w io.Writer, flag, status byte, id int64, body []byte) error {
	var hdr [16]byte
	hdr[0], hdr[1] = magic0, magic1
	hdr[2], hdr[3] = flag, status
	binary.BigEndian.PutUint64(hdr[4:12], uint64(id))
	binary.BigEndian.PutUint32(hdr[12:16], uint32(len(body)))
	if _, err := w.Write(hdr[:]); err != nil {
		return fmt.Errorf("dubbo: write header: %w", err)
	}
	if _, err := w.Write(body); err != nil {
		return fmt.Errorf("dubbo: write body: %w", err)
	}
	return nil
}

func readFrame(r io.Reader) (flag, status byte, id int64, body []byte, err error) {
	var hdr [16]byte
	if _, err = io.ReadFull(r, hdr[:]); err != nil {
		return 0, 0, 0, nil, fmt.Errorf("dubbo: read header: %w", err)
	}
	if hdr[0] != magic0 || hdr[1] != magic1 {
		return 0, 0, 0, nil, errors.New("dubbo: bad magic")
	}
	n := binary.BigEndian.Uint32(hdr[12:16])
	if n > maxBody {
		return 0, 0, 0, nil, fmt.Errorf("dubbo: body too large (%d bytes)", n)
	}
	body = make([]byte, n)
	if _, err = io.ReadFull(r, body); err != nil {
		return 0, 0, 0, nil, fmt.Errorf("dubbo: read body: %w", err)
	}
	return hdr[2], hdr[3], int64(binary.BigEndian.Uint64(hdr[4:12])), body, nil
}

// decodeResponse returns the Hessian value and whether it is a remote
// exception. Attachment cruft after the value is ignored.
func decodeResponse(body []byte) (any, bool, error) {
	if len(body) == 0 {
		return nil, false, errors.New("dubbo: empty response body")
	}
	flag := body[0]
	remote := flag == responseWithException || flag == responseExceptionAtt
	switch flag {
	case responseNull, responseNullAtt:
		return nil, false, nil
	case responseValue, responseValueAtt, responseWithException, responseExceptionAtt:
		v, err := hessian.NewDecoder(body[1:]).Decode()
		if err != nil && err != io.EOF {
			return nil, false, fmt.Errorf("dubbo: decode response: %w", err)
		}
		return v, remote, nil
	default:
		return nil, false, fmt.Errorf("dubbo: unknown response flag 0x%02x", flag)
	}
}
