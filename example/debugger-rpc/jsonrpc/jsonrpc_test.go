package jsonrpc

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

func TestCall(t *testing.T) {
	var got struct {
		JSONRPC string          `json:"jsonrpc"`
		ID      int             `json:"id"`
		Method  string          `json:"method"`
		Params  json.RawMessage `json:"params"`
	}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("request = %s %s", r.Method, r.Header.Get("Content-Type"))
		}
		body, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(body, &got); err != nil {
			t.Errorf("request body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{"sum":3}}`))
	}))
	defer ts.Close()

	resp, err := New().Exec(context.Background(), apidoc.DebugRequest{
		URL:     "jsonrpc://" + strings.TrimPrefix(ts.URL, "http://") + "/rpc",
		Body:    []byte(`[1,2]`),
		Options: map[string]string{"rpc_method": "Math.add"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.JSONRPC != "2.0" || got.ID != 1 || got.Method != "Math.add" || string(got.Params) != "[1,2]" {
		t.Fatalf("request = %+v", got)
	}
	if resp.Status != "OK" || !strings.Contains(string(resp.Body), `"sum": 3`) {
		t.Fatalf("status=%q body=%s", resp.Status, resp.Body)
	}
}

func TestErrorResponse(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"error":{"code":-32601,"message":"method not found"}}`))
	}))
	defer ts.Close()

	resp, err := New().Exec(context.Background(), apidoc.DebugRequest{
		URL:     "jsonrpc://" + strings.TrimPrefix(ts.URL, "http://") + "?method=Math.nope",
		Options: map[string]string{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Status != "ERROR" || !strings.Contains(string(resp.Body), "method not found") {
		t.Fatalf("status=%q body=%s", resp.Status, resp.Body)
	}
}

func TestMethodFromAction(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(body), `"method":"Demo.ping"`) {
			t.Errorf("body = %s", body)
		}
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":null}`))
	}))
	defer ts.Close()

	resp, err := New().Exec(context.Background(), apidoc.DebugRequest{
		Method: "Demo.ping",
		URL:    "jsonrpc://" + strings.TrimPrefix(ts.URL, "http://") + "/rpc",
	})
	if err != nil {
		t.Fatal(err)
	}
	if string(resp.Body) != "null" {
		t.Fatalf("body = %s", resp.Body)
	}
}

func TestMethodRequired(t *testing.T) {
	if _, err := New().Exec(context.Background(), apidoc.DebugRequest{URL: "jsonrpc://localhost/rpc"}); err == nil {
		t.Fatal("want error when method is missing")
	}
}

func TestBadParams(t *testing.T) {
	if _, err := New().Exec(context.Background(), apidoc.DebugRequest{
		URL:    "jsonrpc://localhost/rpc",
		Method: "x",
		Body:   []byte(`"not params"`),
	}); err == nil {
		t.Fatal("want error for non-structured params")
	}
}
