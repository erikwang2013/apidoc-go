// Package mcp adapts Model Context Protocol servers over the streamable HTTP
// transport to apidoc's one-shot debug interface. URLs use mcp:// or mcps://.
// Exec performs the initialize handshake (unless a session_id option is
// supplied) and then one JSON-RPC call whose params come from the payload.
package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/erikwang2013/apidoc-go"
	"github.com/erikwang2013/apidoc-go/example/debugger-web/internal/httpc"
	sseio "github.com/erikwang2013/apidoc-go/example/debugger-web/internal/sse"
)

// Protocol is the apidoc protocol key.
const (
	Protocol            = "mcp"
	defaultProtoVersion = "2025-06-18"
	maxBody             = 4 << 20
)

// New returns a one-shot debugger for MCP streamable HTTP endpoints.
func New() apidoc.Debugger { return debugger{} }

type debugger struct{}

func (debugger) Protocol() string { return Protocol }

func (debugger) Spec() apidoc.DebugSpec {
	return apidoc.DebugSpec{
		Mode: "server",
		Actions: []string{
			"initialize", "ping", "tools/list", "tools/call",
			"resources/list", "resources/read", "prompts/list", "prompts/get",
		},
		Encodings:    []string{"json"},
		BodyIn:       "payload",
		ResponseView: "json",
		ConnectFields: []apidoc.DebugField{
			{Name: "protocol_version", Label: "MCP protocol version", Default: defaultProtoVersion},
			{Name: "session_id", Label: "MCP session id", Placeholder: "reuse an existing session"},
		},
	}
}

func (debugger) Exec(ctx context.Context, req apidoc.DebugRequest) (*apidoc.DebugResponse, error) {
	start := time.Now()
	endpoint, err := endpointURL(req.URL)
	if err != nil {
		return nil, err
	}
	if len(req.Body) > 0 && !json.Valid(req.Body) {
		return nil, errors.New("mcp: payload must be valid JSON")
	}
	proto := req.Options["protocol_version"]
	if proto == "" {
		proto = defaultProtoVersion
	}
	sid := req.Options["session_id"]

	if req.Method == "initialize" {
		params := req.Body
		if len(params) == 0 {
			params = defaultInit(proto)
		}
		raw, hdr, status, err := rpc(ctx, endpoint, proto, "", 1, req.Method, params, req.Headers)
		if err != nil {
			return nil, err
		}
		return response(raw, hdr, status, start), nil
	}

	if sid == "" {
		raw, hdr, _, err := rpc(ctx, endpoint, proto, "", 1, "initialize", defaultInit(proto), req.Headers)
		if err != nil {
			return nil, err
		}
		if msg := rpcError(raw); msg != "" {
			return nil, fmt.Errorf("mcp: initialize: %s", msg)
		}
		sid = hdr.Get("Mcp-Session-Id")
		if err := notify(ctx, endpoint, proto, sid, "notifications/initialized", req.Headers); err != nil {
			return nil, err
		}
	}

	raw, hdr, status, err := rpc(ctx, endpoint, proto, sid, 2, req.Method, req.Body, req.Headers)
	if err != nil {
		return nil, err
	}
	r := response(raw, hdr, status, start)
	if sid != "" {
		r.Headers["mcp-session-id"] = sid
	}
	return r, nil
}

type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int             `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

func rpc(ctx context.Context, endpoint, proto, sid string, id int, method string, params []byte, headers map[string]string) ([]byte, http.Header, int, error) {
	resp, err := send(ctx, endpoint, proto, sid, id, method, params, headers)
	if err != nil {
		return nil, nil, 0, err
	}
	defer resp.Body.Close()
	raw, err := readResponse(resp)
	if err != nil {
		return nil, resp.Header, resp.StatusCode, err
	}
	if len(raw) == 0 {
		return nil, resp.Header, resp.StatusCode, errors.New("mcp: empty response")
	}
	return raw, resp.Header, resp.StatusCode, nil
}

func notify(ctx context.Context, endpoint, proto, sid, method string, headers map[string]string) error {
	resp, err := send(ctx, endpoint, proto, sid, 0, method, nil, headers)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
	return nil
}

func send(ctx context.Context, endpoint, proto, sid string, id int, method string, params []byte, headers map[string]string) (*http.Response, error) {
	body, err := json.Marshal(rpcRequest{JSONRPC: "2.0", ID: id, Method: method, Params: params})
	if err != nil {
		return nil, fmt.Errorf("mcp: %w", err)
	}
	hreq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("mcp: %w", err)
	}
	hreq.Header.Set("Content-Type", "application/json")
	hreq.Header.Set("Accept", "application/json, text/event-stream")
	hreq.Header.Set("MCP-Protocol-Version", proto)
	if sid != "" {
		hreq.Header.Set("Mcp-Session-Id", sid)
	}
	for k, v := range headers {
		hreq.Header.Set(k, v)
	}
	resp, err := httpc.Client.Do(hreq)
	if err != nil {
		return nil, fmt.Errorf("mcp: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		_ = resp.Body.Close()
		return nil, fmt.Errorf("mcp: %s: %s", resp.Status, strings.TrimSpace(string(msg)))
	}
	return resp, nil
}

var errStop = errors.New("stop")

// readResponse takes the JSON-RPC envelope from either a plain JSON body or
// the event stream a streamable HTTP server may return.
func readResponse(resp *http.Response) ([]byte, error) {
	if mt, _, err := mime.ParseMediaType(resp.Header.Get("Content-Type")); err == nil && mt == "text/event-stream" {
		var out []byte
		err := sseio.Read(resp.Body, func(ev sseio.Event) error {
			if !json.Valid([]byte(ev.Data)) {
				return nil
			}
			var env struct {
				Method string          `json:"method"`
				ID     json.RawMessage `json:"id"`
			}
			if json.Unmarshal([]byte(ev.Data), &env) != nil || env.Method != "" || len(env.ID) == 0 {
				return nil
			}
			out = []byte(ev.Data)
			return errStop
		})
		if err != nil && !errors.Is(err, errStop) {
			return nil, fmt.Errorf("mcp: %w", err)
		}
		return out, nil
	}
	return io.ReadAll(io.LimitReader(resp.Body, maxBody))
}

func response(raw []byte, hdr http.Header, status int, start time.Time) *apidoc.DebugResponse {
	r := &apidoc.DebugResponse{
		Status:   "OK",
		Headers:  map[string]string{"http_status": fmt.Sprint(status)},
		Body:     raw,
		Encoding: "json",
		Duration: time.Since(start),
	}
	if v := hdr.Get("Mcp-Session-Id"); v != "" {
		r.Headers["mcp-session-id"] = v
	}
	if v := hdr.Get("Content-Type"); v != "" {
		r.Headers["content-type"] = v
	}
	if msg := rpcError(raw); msg != "" {
		r.Status = "ERROR"
	}
	return r
}

func rpcError(raw []byte) string {
	var env struct {
		Error *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if json.Unmarshal(raw, &env) == nil && env.Error != nil {
		return fmt.Sprintf("%d %s", env.Error.Code, env.Error.Message)
	}
	return ""
}

func defaultInit(proto string) []byte {
	b, _ := json.Marshal(map[string]any{
		"protocolVersion": proto,
		"capabilities":    map[string]any{},
		"clientInfo":      map[string]string{"name": "apidoc-go-debugger", "version": "example"},
	})
	return b
}

func endpointURL(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return "", fmt.Errorf("mcp: bad url %q", raw)
	}
	switch u.Scheme {
	case "http", "https":
	case "mcp":
		u.Scheme = "http"
	case "mcps":
		u.Scheme = "https"
	default:
		return "", fmt.Errorf("mcp: bad scheme %q", u.Scheme)
	}
	return u.String(), nil
}
