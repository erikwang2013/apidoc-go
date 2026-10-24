// Package http2 implements an HTTP/2 over TLS debug adapter.
package http2

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/erikwang2013/apidoc-go"
	"github.com/erikwang2013/apidoc-go/example/debugger-http/internal/httpx"
)

var schemes = map[string]string{"h2s": "https"}

// New returns a debugger that sends HTTP requests over TLS and
// negotiates HTTP/2 with ALPN. h2c (clear-text HTTP/2) is not
// supported, so the URL must be https (or h2s).
func New() apidoc.Debugger {
	return apidoc.DebuggerFunc("http2", apidoc.DebugSpec{
		Mode:         "server",
		Actions:      []string{"GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS"},
		Encodings:    []string{"json", "xml", "text", "hex", "base64"},
		BodyIn:       "payload",
		ResponseView: "http",
		ConnectFields: []apidoc.DebugField{
			{Name: "timeout_ms", Label: "Timeout (ms)", Type: "number", Default: "30000"},
			{Name: "insecure_skip_verify", Label: "Skip TLS verify", Type: "bool"},
			{Name: "ca_file", Label: "CA file (PEM)"},
		},
	}, exec)
}

func exec(ctx context.Context, req apidoc.DebugRequest) (*apidoc.DebugResponse, error) {
	tlsCfg, err := httpx.TLS(req.Options)
	if err != nil {
		return nil, err
	}
	method := req.Method
	if method == "" {
		method = "GET"
	}
	hr, err := httpx.NewRequest(ctx, req, strings.ToUpper(method), schemes)
	if err != nil {
		return nil, err
	}
	if hr.URL.Scheme != "https" {
		return nil, errors.New("http2: url must use https (h2c is not supported)")
	}

	tr := &http.Transport{ForceAttemptHTTP2: true, TLSClientConfig: tlsCfg}
	defer tr.CloseIdleConnections()
	client := &http.Client{Transport: tr, Timeout: httpx.Timeout(req.Options)}
	return httpx.Do(client, hr, req.Encoding)
}
