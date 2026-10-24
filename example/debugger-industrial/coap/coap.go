// Package coap debugs CoAP request/response endpoints over UDP.
package coap

import (
	"context"
	"time"

	"github.com/erikwang2013/apidoc-go"
	"github.com/erikwang2013/apidoc-go/example/debugger-industrial/internal/coapx"
	"github.com/erikwang2013/apidoc-go/example/debugger-industrial/internal/opts"
)

// New returns the CoAP debugger.
func New() apidoc.Debugger {
	return apidoc.DebuggerFunc("coap", apidoc.DebugSpec{
		Mode:         "server",
		Actions:      []string{"GET", "POST", "PUT", "DELETE"},
		Encodings:    []string{"text", "json", "hex", "base64"},
		ResponseView: "auto",
		ConnectFields: []apidoc.DebugField{
			{Name: "content_format", Label: "Content-Format (number)", Type: "number", Default: "0"},
			{Name: "accept", Label: "Accept (number, empty = unset)", Type: "number"},
			{Name: "timeout_ms", Label: "Timeout (ms)", Type: "number", Default: "5000"},
		},
	}, exec)
}

func exec(ctx context.Context, req apidoc.DebugRequest) (*apidoc.DebugResponse, error) {
	cf, err := opts.Uint(req.Options, "content_format", 0, 65535)
	if err != nil {
		return nil, err
	}
	timeout, err := opts.Timeout(req.Options, "timeout_ms", 5*time.Second, 30*time.Second)
	if err != nil {
		return nil, err
	}
	accept, err := opts.Uint(req.Options, "accept", 0, 65535)
	if err != nil {
		return nil, err
	}
	return coapx.Do(ctx, req, coapx.Options{
		Scheme:        "coap",
		ContentFormat: uint16(cf),
		Accept:        uint16(accept),
		SetAccept:     req.Options["accept"] != "",
		Timeout:       timeout,
	})
}
