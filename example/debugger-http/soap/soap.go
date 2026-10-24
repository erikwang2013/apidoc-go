// Package soap implements a SOAP 1.1/1.2 over HTTP debug adapter.
package soap

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/erikwang2013/apidoc-go"
	"github.com/erikwang2013/apidoc-go/example/debugger-http/internal/httpx"
)

var schemes = map[string]string{"soap": "http", "soaps": "https"}

// New returns a debugger that POSTs a raw SOAP envelope. Option
// soap_action sets the SOAPAction value (1.1) or action parameter
// (1.2); soap_version selects "1.1" (default) or "1.2".
func New() apidoc.Debugger {
	return apidoc.DebuggerFunc("soap", apidoc.DebugSpec{
		Mode:         "server",
		Actions:      []string{"POST"},
		Encodings:    []string{"xml", "text"},
		BodyIn:       "payload",
		ResponseView: "xml",
		ConnectFields: []apidoc.DebugField{
			{Name: "soap_action", Label: "SOAPAction", Placeholder: "urn:example:Operation"},
			{Name: "soap_version", Label: "SOAP version", Type: "select", Options: []string{"1.1", "1.2"}, Default: "1.1"},
		},
	}, exec)
}

func exec(ctx context.Context, req apidoc.DebugRequest) (*apidoc.DebugResponse, error) {
	if req.Method != "" && strings.ToUpper(req.Method) != "POST" {
		return nil, fmt.Errorf("soap: unsupported method %q", req.Method)
	}
	if len(req.Body) == 0 {
		return nil, errors.New("soap: empty envelope")
	}
	version := req.Options["soap_version"]
	if version == "" {
		version = "1.1"
	}
	action := req.Options["soap_action"]

	hr, err := httpx.NewRequest(ctx, req, "POST", schemes)
	if err != nil {
		return nil, err
	}
	switch version {
	case "1.1":
		if hr.Header.Get("Content-Type") == "" {
			hr.Header.Set("Content-Type", "text/xml; charset=utf-8")
		}
		if action != "" && hr.Header.Get("SOAPAction") == "" {
			hr.Header.Set("SOAPAction", strconv.Quote(action))
		}
	case "1.2":
		if hr.Header.Get("Content-Type") == "" {
			ct := "application/soap+xml; charset=utf-8"
			if action != "" {
				ct += "; action=" + strconv.Quote(action)
			}
			hr.Header.Set("Content-Type", ct)
		}
	default:
		return nil, fmt.Errorf("soap: unknown version %q", version)
	}

	client := &http.Client{Timeout: httpx.Timeout(req.Options)}
	return httpx.Do(client, hr, "xml")
}
