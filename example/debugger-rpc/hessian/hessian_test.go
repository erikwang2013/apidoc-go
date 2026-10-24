package hessiandbg

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	hessian "github.com/apache/dubbo-go-hessian2"
	"github.com/erikwang2013/apidoc-go"
)

func TestCall(t *testing.T) {
	var gotMethod string
	var gotArgs []any
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if ct := r.Header.Get("Content-Type"); ct != "application/x-hessian" {
			t.Errorf("content-type = %q", ct)
		}
		body, _ := io.ReadAll(r.Body)
		if len(body) < 3 || string(body[:3]) != string([]byte{'H', 0x02, kindCall}) {
			t.Errorf("call envelope = %x", body)
			return
		}
		d := hessian.NewDecoder(body[3:])
		v, err := d.Decode()
		if err != nil {
			t.Errorf("decode method: %v", err)
			return
		}
		gotMethod, _ = v.(string)
		for {
			v, err := d.Decode()
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				t.Errorf("decode arg: %v", err)
				return
			}
			gotArgs = append(gotArgs, v)
		}
		e := hessian.NewEncoder()
		_ = e.Encode(map[string]any{"method": gotMethod, "echo": gotArgs[0]})
		_, _ = w.Write(append([]byte{'H', 0x02, kindReply}, e.Buffer()...))
	}))
	defer ts.Close()

	resp, err := New().Exec(context.Background(), apidoc.DebugRequest{
		URL:     "hessian://" + strings.TrimPrefix(ts.URL, "http://") + "/rpc",
		Body:    []byte(`["world",7]`),
		Options: map[string]string{"rpc_method": "demo.echo"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if gotMethod != "demo.echo" || len(gotArgs) != 2 || gotArgs[0] != "world" || gotArgs[1] != int64(7) {
		t.Fatalf("method=%q args=%v", gotMethod, gotArgs)
	}
	if resp.Status != "OK" || !strings.Contains(string(resp.Body), "world") {
		t.Fatalf("status=%q body=%s", resp.Status, resp.Body)
	}
}

func TestFault(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		e := hessian.NewEncoder()
		_ = e.Encode(map[string]any{"code": 500, "message": "boom"})
		_, _ = w.Write(append([]byte{'H', 0x02, kindFault}, e.Buffer()...))
	}))
	defer ts.Close()

	resp, err := New().Exec(context.Background(), apidoc.DebugRequest{
		URL:     "hessian://" + strings.TrimPrefix(ts.URL, "http://") + "/rpc",
		Options: map[string]string{"rpc_method": "demo.echo"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Status != "ERROR" || !strings.Contains(string(resp.Body), "boom") {
		t.Fatalf("status=%q body=%s", resp.Status, resp.Body)
	}
}

func TestMethodRequired(t *testing.T) {
	if _, err := New().Exec(context.Background(), apidoc.DebugRequest{URL: "hessian://localhost/rpc"}); err == nil {
		t.Fatal("want error when rpc_method is missing")
	}
}

func TestBadURL(t *testing.T) {
	if _, err := New().Exec(context.Background(), apidoc.DebugRequest{URL: "tcp://localhost:1"}); err == nil {
		t.Fatal("want error for unsupported scheme")
	}
}

func TestBadArgs(t *testing.T) {
	if _, err := New().Exec(context.Background(), apidoc.DebugRequest{
		URL:     "hessian://localhost/rpc",
		Body:    []byte(`{"not":"an array"}`),
		Options: map[string]string{"rpc_method": "m"},
	}); err == nil {
		t.Fatal("want error for non-array body")
	}
}
