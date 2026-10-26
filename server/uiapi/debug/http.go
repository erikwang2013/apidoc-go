//go:build apidoc_debug_http || apidoc_debug_all

package debug

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

func init() {
	for _, kind := range []string{
		"http", "http2", "soap", "graphql", "jsonrpc", "xmlrpc", "odata", "mcp", "a2a", "webhook",
	} {
		RegisterExecutor(kind, executeHTTP)
	}
}

// httpContentTypes are applied only when the client did not set Content-Type;
// they describe the protocol's conventional body format.
var httpContentTypes = map[string]string{
	"soap":    "text/xml; charset=utf-8",
	"xmlrpc":  "text/xml; charset=utf-8",
	"graphql": "application/json",
	"jsonrpc": "application/json",
	"odata":   "application/json",
	"mcp":     "application/json",
	"a2a":     "application/json",
	"webhook": "application/json",
}

func executeHTTP(cfg Config, req *Request) (*Response, error) {
	target, err := CheckTarget(cfg, req.Target)
	if err != nil {
		return nil, err
	}
	method := strings.TrimSpace(req.Method)
	if method == "" {
		method = http.MethodGet
	}
	if strings.EqualFold(method, http.MethodConnect) {
		return nil, fmt.Errorf("HTTP CONNECT is not allowed in server-side debug")
	}
	body, err := req.Body.Decode()
	if err != nil {
		return nil, err
	}
	if len(body) > cfg.Limit() {
		return nil, fmt.Errorf("request body exceeds max_bytes (%d)", cfg.Limit())
	}

	protocols := &http.Protocols{}
	if req.Kind == "http2" || req.Opts["version"] == "2" {
		// HTTP/2 prior knowledge: h2c for http:// and ALPN h2-only for https://.
		if target.URL.Scheme == "http" {
			protocols.SetUnencryptedHTTP2(true)
		} else {
			protocols.SetHTTP2(true)
		}
	} else {
		protocols.SetHTTP1(true)
		protocols.SetHTTP2(true)
	}
	transport := &http.Transport{
		Proxy:                 nil,
		DialContext:           target.DialContext,
		ForceAttemptHTTP2:     true,
		DisableKeepAlives:     true,
		TLSHandshakeTimeout:   cfg.Timeout(),
		ResponseHeaderTimeout: cfg.Timeout(),
		Protocols:             protocols,
	}
	// No redirects: a 3xx target is another unchecked address.
	client := &http.Client{
		Transport: transport,
		Timeout:   req.Timeout(cfg),
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	ctx, cancel := context.WithTimeout(context.Background(), req.Timeout(cfg))
	defer cancel()

	var reader io.Reader = http.NoBody
	if len(body) > 0 {
		reader = bytes.NewReader(body)
	}
	httpReq, err := http.NewRequestWithContext(ctx, method, target.URL.String(), reader)
	if err != nil {
		return nil, fmt.Errorf("invalid HTTP request: %v", err)
	}
	for name, value := range req.Headers {
		httpReq.Header.Set(name, value)
	}
	if httpReq.Header.Get("Content-Type") == "" && len(body) > 0 {
		if contentType := httpContentTypes[req.Kind]; contentType != "" {
			httpReq.Header.Set("Content-Type", contentType)
		}
	}

	started := time.Now()
	res, err := client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("request failed: %v", err)
	}
	defer res.Body.Close()

	headers := map[string]string{}
	for name, values := range res.Header {
		headers[strings.ToLower(name)] = strings.Join(values, ", ")
	}
	limit := cfg.Limit()
	data, err := io.ReadAll(io.LimitReader(res.Body, int64(limit)+1))
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %v", err)
	}
	if len(data) > limit {
		return nil, fmt.Errorf("response exceeds max_bytes (%d)", limit)
	}
	return &Response{
		Status:  res.StatusCode,
		MS:      time.Since(started).Milliseconds(),
		Headers: headers,
		Body:    FromBytes(data),
	}, nil
}
