// Package odata implements an OData over HTTP debug adapter.
package odata

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/erikwang2013/apidoc-go"
	"github.com/erikwang2013/apidoc-go/example/debugger-http/internal/httpx"
)

var schemes = map[string]string{"odata": "http", "odatas": "https"}

// New returns a debugger that issues OData GET requests. Query params
// (including $filter, $select, $top, ...) come from documented params
// with in=query and from options whose key starts with "$".
func New() apidoc.Debugger {
	return apidoc.DebuggerFunc("odata", apidoc.DebugSpec{
		Mode:         "server",
		Actions:      []string{"GET"},
		Encodings:    []string{"json", "xml", "text"},
		ResponseView: "auto",
	}, exec)
}

func exec(ctx context.Context, req apidoc.DebugRequest) (*apidoc.DebugResponse, error) {
	if req.Method != "" && strings.ToUpper(req.Method) != "GET" {
		return nil, fmt.Errorf("odata: unsupported method %q", req.Method)
	}
	if len(req.Body) > 0 {
		return nil, errors.New("odata: GET requests do not take a body")
	}

	hr, err := httpx.NewRequest(ctx, req, "GET", schemes)
	if err != nil {
		return nil, err
	}
	q := hr.URL.Query()
	for k, v := range req.Options {
		if strings.HasPrefix(k, "$") {
			q.Set(k, v)
		}
	}
	hr.URL.RawQuery = q.Encode()
	if hr.Header.Get("Accept") == "" {
		hr.Header.Set("Accept", "application/json")
	}

	client := &http.Client{Timeout: httpx.Timeout(req.Options)}
	return httpx.Do(client, hr, "json")
}
