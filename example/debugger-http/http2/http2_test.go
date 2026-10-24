package http2

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/erikwang2013/apidoc-go"
)

func TestHTTP2Exec(t *testing.T) {
	var gotProto, gotHeader, gotQuery, gotBody string
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotProto = r.Proto
		gotHeader = r.Header.Get("X-Test")
		gotQuery = r.URL.Query().Get("q")
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"proto":"`+r.Proto+`"}`)
	}))
	srv.EnableHTTP2 = true
	srv.StartTLS()
	defer srv.Close()

	// h2s:// must be rewritten to https:// by the adapter.
	resp, err := New().Exec(context.Background(), apidoc.DebugRequest{
		URL:      strings.Replace(srv.URL, "https://", "h2s://", 1) + "/echo",
		Method:   "POST",
		Body:     []byte(`{"hello":"world"}`),
		Encoding: "json",
		Params: []apidoc.DebugValue{
			{Name: "q", In: "query", Value: "x y"},
			{Name: "X-Test", In: "header", Value: "v"},
		},
		Options: map[string]string{"insecure_skip_verify": "true", "timeout_ms": "5000"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if gotProto != "HTTP/2.0" {
		t.Errorf("server saw %s, want HTTP/2.0", gotProto)
	}
	if gotBody != `{"hello":"world"}` || gotHeader != "v" || gotQuery != "x y" {
		t.Errorf("server saw body %q header %q query %q", gotBody, gotHeader, gotQuery)
	}
	if resp.Status != "200 OK" || !strings.Contains(string(resp.Body), "HTTP/2.0") {
		t.Fatalf("response = %+v", resp)
	}
	if resp.Headers["Content-Type"] != "application/json" {
		t.Errorf("response headers = %v", resp.Headers)
	}
}

func TestHTTP2RejectsPlainHTTP(t *testing.T) {
	if _, err := New().Exec(context.Background(), apidoc.DebugRequest{
		URL: "http://example.invalid",
	}); err == nil {
		t.Error("want error for non-TLS URL")
	}
	if _, err := New().Exec(context.Background(), apidoc.DebugRequest{
		URL:     "https://example.invalid",
		Options: map[string]string{"insecure_skip_verify": "not-a-bool"},
	}); err == nil {
		t.Error("want error for invalid insecure_skip_verify")
	}
}
