// Package graphql implements a GraphQL over HTTP debug adapter.
package graphql

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/erikwang2013/apidoc-go"
	"github.com/erikwang2013/apidoc-go/example/debugger-http/internal/httpx"
)

var schemes = map[string]string{"graphql": "http", "graphqls": "https"}

const introspectionQuery = `query IntrospectionQuery {
  __schema {
    queryType { name }
    mutationType { name }
    subscriptionType { name }
    types { kind name description }
  }
}`

// New returns a debugger that POSTs a GraphQL query. The body may be a
// raw query or a JSON object with query/variables/operationName.
// Options operation_name and variables (JSON) are merged into the
// payload; introspection sends the schema query instead.
func New() apidoc.Debugger {
	return apidoc.DebuggerFunc("graphql", apidoc.DebugSpec{
		Mode:         "server",
		Actions:      []string{"POST"},
		Encodings:    []string{"json"},
		BodyIn:       "payload",
		ResponseView: "json",
		ConnectFields: []apidoc.DebugField{
			{Name: "operation_name", Label: "Operation name"},
			{Name: "variables", Label: "Variables (JSON)", Type: "textarea", Placeholder: `{"id":1}`},
			{Name: "introspection", Label: "Introspection query", Type: "bool"},
		},
	}, exec)
}

func exec(ctx context.Context, req apidoc.DebugRequest) (*apidoc.DebugResponse, error) {
	if req.Method != "" && strings.ToUpper(req.Method) != "POST" {
		return nil, fmt.Errorf("graphql: unsupported method %q", req.Method)
	}
	body, err := buildBody(req)
	if err != nil {
		return nil, err
	}
	req.Body = body

	hr, err := httpx.NewRequest(ctx, req, "POST", schemes)
	if err != nil {
		return nil, err
	}
	if hr.Header.Get("Content-Type") == "" {
		hr.Header.Set("Content-Type", "application/json")
	}
	if hr.Header.Get("Accept") == "" {
		hr.Header.Set("Accept", "application/json")
	}

	client := &http.Client{Timeout: httpx.Timeout(req.Options)}
	return httpx.Do(client, hr, "json")
}

func buildBody(req apidoc.DebugRequest) ([]byte, error) {
	if req.Options["introspection"] == "true" {
		return json.Marshal(map[string]string{"query": introspectionQuery})
	}

	raw := bytes.TrimSpace(req.Body)
	if len(raw) == 0 {
		return nil, errors.New("graphql: empty query")
	}

	payload := map[string]json.RawMessage{}
	if raw[0] == '{' && json.Unmarshal(raw, &payload) == nil {
		var query string
		if err := json.Unmarshal(payload["query"], &query); err != nil || query == "" {
			return nil, errors.New("graphql: missing query")
		}
	} else {
		// Plain text body: treat it as the query document.
		payload = map[string]json.RawMessage{}
		query, _ := json.Marshal(string(raw))
		payload["query"] = query
	}

	if err := applyOptions(payload, req.Options); err != nil {
		return nil, err
	}
	return json.Marshal(payload)
}

func applyOptions(payload map[string]json.RawMessage, opts map[string]string) error {
	if v := opts["variables"]; v != "" {
		if !json.Valid([]byte(v)) {
			return errors.New("graphql: variables is not valid JSON")
		}
		payload["variables"] = json.RawMessage(v)
	}
	if op := opts["operation_name"]; op != "" {
		name, _ := json.Marshal(op)
		payload["operationName"] = name
	}
	return nil
}
