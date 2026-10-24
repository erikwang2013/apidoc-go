// Package jsonrpc is an example JSON-RPC 2.0 over HTTP debug adapter.
//
// The body is the params value (a JSON array or object); the adapter wraps it
// in a JSON-RPC request. The RPC method comes from the rpc_method option, the
// ?method= query, or the action method when it is not CALL.
package jsonrpc

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

	"github.com/erikwang2013/apidoc-go"
)

const maxBody = 4 << 20

// New returns the JSON-RPC adapter.
func New() apidoc.Debugger {
	return apidoc.DebuggerFunc("jsonrpc", apidoc.DebugSpec{
		Mode:         "server",
		Actions:      []string{"CALL"},
		Encodings:    []string{"json"},
		BodyIn:       "payload",
		ResponseView: "json",
		ConnectFields: []apidoc.DebugField{
			{Name: "rpc_method", Label: "RPC method", Type: "text", Required: true, Placeholder: "Math.add"},
			{Name: "timeout_ms", Label: "Timeout (ms)", Type: "number", Default: "10000"},
		},
	}, exec)
}

func endpoint(raw string) (string, *url.URL, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", nil, fmt.Errorf("jsonrpc: bad url %q: %w", raw, err)
	}
	switch u.Scheme {
	case "jsonrpc":
		u.Scheme = "http"
	case "jsonrpcs":
		u.Scheme = "https"
	case "http", "https":
	default:
		return "", nil, fmt.Errorf("jsonrpc: url scheme must be jsonrpc, jsonrpcs, http or https, got %q", u.Scheme)
	}
	if u.Host == "" {
		return "", nil, fmt.Errorf("jsonrpc: url %q has no host", raw)
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
		return nil, fmt.Errorf("jsonrpc: set the rpc_method option or ?method= in the url")
	}

	params := bytes.TrimSpace(req.Body)
	if len(params) > 0 && params[0] != '[' && params[0] != '{' {
		return nil, fmt.Errorf("jsonrpc: body must be a JSON array or object of params")
	}
	payload := map[string]any{"jsonrpc": "2.0", "id": 1, "method": method}
	if len(params) > 0 {
		if !json.Valid(params) {
			return nil, fmt.Errorf("jsonrpc: body is not valid JSON")
		}
		payload["params"] = json.RawMessage(params)
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, target, bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")
	for k, v := range req.Headers {
		httpReq.Header.Set(k, v)
	}
	timeout := 10 * time.Second
	if ms, err := strconv.Atoi(req.Options["timeout_ms"]); err == nil && ms > 0 {
		timeout = time.Duration(ms) * time.Millisecond
	}
	resp, err := (&http.Client{Timeout: timeout}).Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("jsonrpc: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	if err != nil {
		return nil, fmt.Errorf("jsonrpc: read response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, fmt.Errorf("jsonrpc: HTTP %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}

	var out struct {
		Result json.RawMessage `json:"result"`
		Error  *struct {
			Code    int             `json:"code"`
			Message string          `json:"message"`
			Data    json.RawMessage `json:"data"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("jsonrpc: response is not JSON: %w", err)
	}
	duration := time.Since(start)
	if out.Error != nil {
		pretty, _ := json.MarshalIndent(map[string]any{
			"code": out.Error.Code, "message": out.Error.Message, "data": out.Error.Data,
		}, "", "  ")
		return &apidoc.DebugResponse{
			Status:   "ERROR",
			Headers:  map[string]string{"remote": u.Host, "method": method},
			Body:     pretty,
			Encoding: "json",
			Duration: duration,
		}, nil
	}
	result := out.Result
	if len(result) == 0 {
		result = []byte("null")
	}
	var pretty bytes.Buffer
	if err := json.Indent(&pretty, result, "", "  "); err != nil {
		pretty.Write(result)
	}
	return &apidoc.DebugResponse{
		Status:   "OK",
		Headers:  map[string]string{"remote": u.Host, "method": method},
		Body:     pretty.Bytes(),
		Encoding: "json",
		Duration: duration,
	}, nil
}
