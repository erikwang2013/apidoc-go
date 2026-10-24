// Package a2a adapts Agent2Agent JSON-RPC endpoints to apidoc's one-shot
// debug interface. URLs use a2a:// or a2as://. The payload is the JSON-RPC
// params object. message/stream responses arrive as text/event-stream and are
// collected into DebugResponse.Events.
package a2a

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
	Protocol = "a2a"
	maxBody  = 4 << 20
)

// New returns a one-shot debugger for Agent2Agent endpoints.
func New() apidoc.Debugger { return debugger{} }

type debugger struct{}

func (debugger) Protocol() string { return Protocol }

func (debugger) Spec() apidoc.DebugSpec {
	return apidoc.DebugSpec{
		Mode:         "server",
		Actions:      []string{"message/send", "message/stream", "tasks/get", "tasks/cancel"},
		Encodings:    []string{"json"},
		BodyIn:       "payload",
		ResponseView: "auto",
		ConnectFields: []apidoc.DebugField{
			{Name: "a2a_version", Label: "A2A-Version header", Placeholder: "0.3.0"},
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
		return nil, errors.New("a2a: payload must be valid JSON")
	}
	body, err := json.Marshal(rpcRequest{JSONRPC: "2.0", ID: 1, Method: req.Method, Params: req.Body})
	if err != nil {
		return nil, fmt.Errorf("a2a: %w", err)
	}
	hreq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("a2a: %w", err)
	}
	hreq.Header.Set("Content-Type", "application/json")
	hreq.Header.Set("Accept", "application/json, text/event-stream")
	if v := req.Options["a2a_version"]; v != "" {
		hreq.Header.Set("A2A-Version", v)
	}
	for k, v := range req.Headers {
		hreq.Header.Set(k, v)
	}
	resp, err := httpc.Client.Do(hreq)
	if err != nil {
		return nil, fmt.Errorf("a2a: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("a2a: %s: %s", resp.Status, strings.TrimSpace(string(msg)))
	}

	headers := map[string]string{"http_status": resp.Status}
	if ct := resp.Header.Get("Content-Type"); ct != "" {
		headers["content-type"] = ct
	}
	if mt, _, err := mime.ParseMediaType(resp.Header.Get("Content-Type")); err == nil && mt == "text/event-stream" {
		var evs []apidoc.DebugEvent
		var last []byte
		err := sseio.Read(resp.Body, func(ev sseio.Event) error {
			data := strings.TrimSpace(ev.Data)
			if data == "" {
				return nil
			}
			name := ev.Name
			if name == "" {
				name = "message"
			}
			last = []byte(data)
			evs = append(evs, apidoc.DebugEvent{
				Time: time.Now(), Direction: "in", Name: name, Data: data, Encoding: "json",
			})
			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("a2a: %w", err)
		}
		status := "OK"
		if len(evs) == 0 {
			status = "EMPTY"
		} else if rpcError(last) != "" {
			status = "ERROR"
		}
		return &apidoc.DebugResponse{
			Status: status, Headers: headers, Body: last,
			Encoding: "json", Events: evs, Duration: time.Since(start),
		}, nil
	}

	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	if err != nil {
		return nil, fmt.Errorf("a2a: %w", err)
	}
	status := "OK"
	if rpcError(raw) != "" {
		status = "ERROR"
	}
	return &apidoc.DebugResponse{
		Status: status, Headers: headers, Body: raw,
		Encoding: "json", Duration: time.Since(start),
	}, nil
}

type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int             `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
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

func endpointURL(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return "", fmt.Errorf("a2a: bad url %q", raw)
	}
	switch u.Scheme {
	case "http", "https":
	case "a2a":
		u.Scheme = "http"
	case "a2as":
		u.Scheme = "https"
	default:
		return "", fmt.Errorf("a2a: bad scheme %q", u.Scheme)
	}
	return u.String(), nil
}
