package graphql

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/erikwang2013/apidoc-go"
)

func TestGraphQLBodies(t *testing.T) {
	tests := []struct {
		name      string
		body      string
		options   map[string]string
		wantQuery string
		wantVars  bool
		wantOp    string
	}{
		{
			name:      "json payload",
			body:      `{"query":"{ ping }","variables":{"id":1}}`,
			wantQuery: "{ ping }",
			wantVars:  true,
		},
		{
			name:      "raw query",
			body:      `{ ping }`,
			wantQuery: "{ ping }",
		},
		{
			name:      "options merged",
			body:      `query Q { ping }`,
			options:   map[string]string{"variables": `{"id":2}`, "operation_name": "Q"},
			wantQuery: "query Q { ping }",
			wantVars:  true,
			wantOp:    "Q",
		},
		{
			name:    "introspection",
			options: map[string]string{"introspection": "true"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var got map[string]json.RawMessage
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost {
					t.Errorf("method = %s", r.Method)
				}
				if ct := r.Header.Get("Content-Type"); ct != "application/json" {
					t.Errorf("Content-Type = %q", ct)
				}
				raw, _ := io.ReadAll(r.Body)
				if err := json.Unmarshal(raw, &got); err != nil {
					t.Errorf("body is not JSON: %v", err)
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, `{"data":{"ok":true}}`)
			}))
			defer srv.Close()

			// graphql:// must be rewritten to http:// by the adapter.
			resp, err := New().Exec(context.Background(), apidoc.DebugRequest{
				URL:      strings.Replace(srv.URL, "http://", "graphql://", 1),
				Method:   "POST",
				Body:     []byte(tc.body),
				Encoding: "json",
				Options:  tc.options,
			})
			if err != nil {
				t.Fatal(err)
			}
			if resp.Status != "200 OK" || !strings.Contains(string(resp.Body), `"ok":true`) {
				t.Fatalf("response = %+v", resp)
			}
			if resp.Encoding != "json" {
				t.Errorf("encoding = %q", resp.Encoding)
			}

			var query string
			_ = json.Unmarshal(got["query"], &query)
			if tc.wantQuery != "" && query != tc.wantQuery {
				t.Errorf("query = %q, want %q", query, tc.wantQuery)
			}
			if tc.name == "introspection" && !strings.Contains(query, "__schema") {
				t.Errorf("introspection query missing __schema: %q", query)
			}
			if _, ok := got["variables"]; ok != tc.wantVars {
				t.Errorf("variables present = %v, want %v", ok, tc.wantVars)
			}
			var op string
			_ = json.Unmarshal(got["operationName"], &op)
			if op != tc.wantOp {
				t.Errorf("operationName = %q, want %q", op, tc.wantOp)
			}
		})
	}
}

func TestGraphQLErrors(t *testing.T) {
	if _, err := New().Exec(context.Background(), apidoc.DebugRequest{
		URL: "http://example.invalid",
	}); err == nil {
		t.Error("want error for empty query")
	}
	if _, err := New().Exec(context.Background(), apidoc.DebugRequest{
		URL: "http://example.invalid", Body: []byte(`{"variables":{}}`),
	}); err == nil {
		t.Error("want error for JSON payload without query")
	}
	if _, err := New().Exec(context.Background(), apidoc.DebugRequest{
		URL: "http://example.invalid", Body: []byte(`{ ping }`),
		Options: map[string]string{"variables": "not json"},
	}); err == nil {
		t.Error("want error for invalid variables option")
	}
	if _, err := New().Exec(context.Background(), apidoc.DebugRequest{
		URL: "http://example.invalid", Method: "GET", Body: []byte(`{ ping }`),
	}); err == nil {
		t.Error("want error for non-POST method")
	}
}
