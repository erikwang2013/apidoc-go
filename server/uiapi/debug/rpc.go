//go:build apidoc_debug_rpc || apidoc_debug_all

package debug

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// errIncomplete is the internal sentinel for "more bytes needed"; binary
// codecs use it to keep reading while a response frame is still arriving.
var errIncomplete = errors.New("incomplete")

// decodeJSONBody parses the request body with number literals preserved so
// binary codecs can tell integers (I/L) from doubles.
func decodeJSONBody(req *Request) (any, error) {
	if req.Body == nil {
		return map[string]any{}, nil
	}
	raw, err := req.Body.Decode()
	if err != nil {
		return nil, err
	}
	if len(raw) == 0 {
		return map[string]any{}, nil
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return nil, errors.New("trailing JSON data")
	}
	return value, nil
}

// rpcAddr parses "host:port" or "scheme://host:port" targets.
func rpcAddr(target, scheme string) (string, string, error) {
	raw := target
	if !strings.Contains(raw, "://") {
		raw = scheme + "://" + raw
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return "", "", fmt.Errorf("invalid target `%s`: %v", target, err)
	}
	host, port := parsed.Hostname(), parsed.Port()
	if host == "" {
		return "", "", fmt.Errorf("target requires a host")
	}
	if port == "" {
		return "", "", fmt.Errorf("target requires a port")
	}
	return host, port, nil
}

func dialRPC(cfg Config, req *Request, label, network, address string) (net.Conn, error) {
	ctx, cancel := context.WithTimeout(context.Background(), req.Timeout(cfg))
	defer cancel()
	conn, err := (&net.Dialer{}).DialContext(ctx, network, address)
	if err != nil {
		if ctx.Err() != nil {
			return nil, fmt.Errorf("%s connect timed out", label)
		}
		return nil, fmt.Errorf("%s connect failed: %v", label, err)
	}
	return conn, nil
}

// rpcReadWindow mirrors the socket read budget: opts.read_ms clamped to the
// request timeout, defaulting to 500ms.
func rpcReadWindow(cfg Config, req *Request) time.Duration {
	ms := 500
	if value, err := strconv.Atoi(req.Opts["read_ms"]); err == nil {
		ms = value
	}
	if ms < 1 {
		ms = 1
	}
	limit := int(req.Timeout(cfg) / time.Millisecond)
	if limit < 1 {
		limit = 1
	}
	if ms > limit {
		ms = limit
	}
	return time.Duration(ms) * time.Millisecond
}
