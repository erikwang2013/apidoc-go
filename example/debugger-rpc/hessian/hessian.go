// Package hessiandbg is an example Hessian 2.0 RPC debug adapter.
//
// A call is POSTed as "H" 0x02 0x00, the method name, then the positional
// arguments, all Hessian2-encoded. The reply is "H" 0x02 0x01 with one value;
// a fault is "H" 0x02 0x02. The debug body is a JSON array of arguments.
package hessiandbg

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	hessian "github.com/apache/dubbo-go-hessian2"
	"github.com/erikwang2013/apidoc-go"
	"github.com/erikwang2013/apidoc-go/example/debugger-rpc/internal/xjson"
)

const (
	envelopeVersion = 0x02
	kindCall        = 0x00
	kindReply       = 0x01
	kindFault       = 0x02
	maxBody         = 4 << 20
)

// New returns the Hessian RPC adapter.
func New() apidoc.Debugger {
	return apidoc.DebuggerFunc("hessian", apidoc.DebugSpec{
		Mode:         "server",
		Actions:      []string{"CALL"},
		Encodings:    []string{"json"},
		BodyIn:       "payload",
		ResponseView: "json",
		ConnectFields: []apidoc.DebugField{
			{Name: "rpc_method", Label: "RPC method", Type: "text", Required: true, Placeholder: "com.example.Service.echo"},
			{Name: "timeout_ms", Label: "Timeout (ms)", Type: "number", Default: "10000"},
		},
	}, exec)
}

func endpoint(raw string) (string, *url.URL, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", nil, fmt.Errorf("hessian: bad url %q: %w", raw, err)
	}
	switch u.Scheme {
	case "hessian":
		u.Scheme = "http"
	case "hessians":
		u.Scheme = "https"
	case "http", "https":
	default:
		return "", nil, fmt.Errorf("hessian: url scheme must be hessian, hessians, http or https, got %q", u.Scheme)
	}
	if u.Host == "" {
		return "", nil, fmt.Errorf("hessian: url %q has no host", raw)
	}
	return u.String(), u, nil
}

func rpcMethod(req apidoc.DebugRequest, u *url.URL) string {
	if m := req.Options["rpc_method"]; m != "" {
		return m
	}
	if m := u.Query().Get("method"); m != "" {
		return m
	}
	if req.Method != "" && !strings.EqualFold(req.Method, "CALL") {
		return req.Method
	}
	return ""
}

func exec(ctx context.Context, req apidoc.DebugRequest) (*apidoc.DebugResponse, error) {
	start := time.Now()
	target, u, err := endpoint(req.URL)
	if err != nil {
		return nil, err
	}
	method := rpcMethod(req, u)
	if method == "" {
		return nil, fmt.Errorf("hessian: set the rpc_method option or ?method= in the url")
	}
	args, err := xjson.Args(req.Body)
	if err != nil {
		return nil, fmt.Errorf("hessian: %w", err)
	}

	e := hessian.NewEncoder()
	if err := e.Encode(method); err != nil {
		return nil, fmt.Errorf("hessian: encode method: %w", err)
	}
	for i, arg := range args {
		if err := e.Encode(arg); err != nil {
			return nil, fmt.Errorf("hessian: encode argument %d: %w", i, err)
		}
	}
	call := append([]byte{'H', envelopeVersion, kindCall}, e.Buffer()...)

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, target, bytes.NewReader(call))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/x-hessian")
	httpReq.Header.Set("Accept", "application/x-hessian")
	for k, v := range req.Headers {
		httpReq.Header.Set(k, v)
	}
	timeout := 10 * time.Second
	if ms, err := strconv.Atoi(req.Options["timeout_ms"]); err == nil && ms > 0 {
		timeout = time.Duration(ms) * time.Millisecond
	}
	client := &http.Client{Timeout: timeout}
	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("hessian: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	if err != nil {
		return nil, fmt.Errorf("hessian: read response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, fmt.Errorf("hessian: HTTP %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}
	if len(body) < 3 || body[0] != 'H' || body[1] != envelopeVersion {
		return nil, fmt.Errorf("hessian: response is not a Hessian 2.0 envelope")
	}
	status := "OK"
	switch body[2] {
	case kindReply:
	case kindFault:
		status = "ERROR"
	default:
		return nil, fmt.Errorf("hessian: unknown message kind 0x%02x", body[2])
	}
	value, err := hessian.NewDecoder(body[3:]).Decode()
	if err != nil && err != io.EOF {
		if status == "ERROR" {
			return &apidoc.DebugResponse{
				Status:   status,
				Headers:  map[string]string{"remote": u.Host, "method": method},
				Body:     body[3:],
				Encoding: "text",
				Duration: time.Since(start),
			}, nil
		}
		return nil, fmt.Errorf("hessian: decode response: %w", err)
	}
	out, err := json.MarshalIndent(xjson.Value(value), "", "  ")
	if err != nil {
		return nil, fmt.Errorf("hessian: encode response: %w", err)
	}
	return &apidoc.DebugResponse{
		Status:   status,
		Headers:  map[string]string{"remote": u.Host, "method": method},
		Body:     out,
		Encoding: "json",
		Duration: time.Since(start),
	}, nil
}
