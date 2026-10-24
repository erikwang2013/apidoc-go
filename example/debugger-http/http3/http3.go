// Package http3 implements an HTTP/3 (QUIC) debug adapter.
package http3

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/erikwang2013/apidoc-go"
	"github.com/erikwang2013/apidoc-go/example/debugger-http/internal/httpx"
	quichttp3 "github.com/quic-go/quic-go/http3"
)

var schemes = map[string]string{"h3": "https", "h3s": "https"}

// New returns a debugger that sends HTTP requests over QUIC using
// quic-go's HTTP/3 transport. URLs must be https (or h3/h3s).
func New() apidoc.Debugger {
	return apidoc.DebuggerFunc("http3", apidoc.DebugSpec{
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
		return nil, errors.New("http3: url must use https")
	}

	tr := &quichttp3.Transport{TLSClientConfig: tlsCfg}
	defer tr.Close()
	client := &http.Client{Transport: tr, Timeout: httpx.Timeout(req.Options)}
	return httpx.Do(client, hr, req.Encoding)
}
