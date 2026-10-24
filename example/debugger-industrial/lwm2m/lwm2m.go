// Package lwm2m debugs LwM2M resources over raw CoAP, addressed by OMA paths
// such as /3303/0/5700. It does not implement the LwM2M object model,
// registration/bootstrap interfaces, observe, or TLV/JSON payload codecs;
// payloads are sent and returned as raw bytes with the selected Content-Format.
package lwm2m

import (
	"context"
	"fmt"
	"time"

	"github.com/erikwang2013/apidoc-go"
	"github.com/erikwang2013/apidoc-go/example/debugger-industrial/internal/coapx"
	"github.com/erikwang2013/apidoc-go/example/debugger-industrial/internal/opts"
)

// New returns the LwM2M debugger.
func New() apidoc.Debugger {
	return apidoc.DebuggerFunc("lwm2m", apidoc.DebugSpec{
		Mode:         "server",
		Actions:      []string{"READ", "WRITE", "EXECUTE", "DISCOVER"},
		Encodings:    []string{"text", "json", "hex", "base64"},
		ResponseView: "auto",
		ConnectFields: []apidoc.DebugField{
			{Name: "content_format", Label: "Content-Format (0 text, 42 opaque, 11542 TLV, 11543 JSON)", Type: "number", Default: "0"},
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

	method := ""
	o := coapx.Options{Scheme: "lwm2m", ContentFormat: uint16(cf), Timeout: timeout}
	switch req.Method {
	case "READ":
		method = "GET"
		o.Accept, o.SetAccept = uint16(accept), true
	case "WRITE":
		method = "PUT"
	case "EXECUTE":
		method = "POST"
	case "DISCOVER":
		method = "GET"
		o.Accept, o.SetAccept = 40, true // application/link-format
	default:
		return nil, fmt.Errorf("lwm2m: unknown action %q", req.Method)
	}
	req.Method = method
	return coapx.Do(ctx, req, o)
}
