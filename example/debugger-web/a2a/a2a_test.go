package a2a

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/erikwang2013/apidoc-go"
)

type fakeServer struct {
	*httptest.Server
	mu      sync.Mutex
	version string
}

func (fs *fakeServer) lastVersion() string {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	return fs.version
}

func newFakeServer(t *testing.T) *fakeServer {
	t.Helper()
	fs := &fakeServer{}
	mux := http.NewServeMux()
	mux.HandleFunc("/agent", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Method string          `json:"method"`
			Params json.RawMessage `json:"params"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		fs.mu.Lock()
		fs.version = r.Header.Get("A2A-Version")
		fs.mu.Unlock()
		switch req.Method {
		case "message/send":
			fmt.Fprint(w, `{"jsonrpc":"2.0","id":1,"result":{"kind":"task","id":"t1","status":{"state":"completed"}}}`)
		case "message/stream":
			w.Header().Set("Content-Type", "text/event-stream")
			f := w.(http.Flusher)
			fmt.Fprint(w, "data: {\"jsonrpc\":\"2.0\",\"id\":1,\"result\":{\"kind\":\"status-update\",\"status\":{\"state\":\"working\"}}}\n\n")
			f.Flush()
			fmt.Fprint(w, "data: {\"jsonrpc\":\"2.0\",\"id\":1,\"result\":{\"kind\":\"artifact-update\",\"artifact\":{\"parts\":[{\"kind\":\"text\",\"text\":\"hi\"}]}}}\n\n")
			f.Flush()
		case "tasks/get":
			fmt.Fprint(w, `{"jsonrpc":"2.0","id":1,"error":{"code":-32001,"message":"task not found"}}`)
		default:
			fmt.Fprint(w, `{"jsonrpc":"2.0","id":1,"result":{}}`)
		}
	})
	mux.HandleFunc("/empty", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
	})
	fs.Server = httptest.NewServer(mux)
	t.Cleanup(fs.Close)
	return fs
}

func a2aURL(fs *fakeServer, path string) string {
	return "a2a://" + strings.TrimPrefix(fs.URL, "http://") + path
}

func TestMessageSend(t *testing.T) {
	fs := newFakeServer(t)
	resp, err := New().Exec(context.Background(), apidoc.DebugRequest{
		Method: "message/send",
		URL:    a2aURL(fs, "/agent"),
		Body:   []byte(`{"message":{"role":"user","parts":[{"kind":"text","text":"hi"}]}}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Status != "OK" || !strings.Contains(string(resp.Body), `"completed"`) || resp.Encoding != "json" {
		t.Fatalf("response = %+v", resp)
	}
}

func TestMessageStream(t *testing.T) {
	fs := newFakeServer(t)
	resp, err := New().Exec(context.Background(), apidoc.DebugRequest{
		Method: "message/stream",
		URL:    a2aURL(fs, "/agent"),
		Body:   []byte(`{"message":{"role":"user","parts":[{"kind":"text","text":"hi"}]}}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Status != "OK" || len(resp.Events) != 2 {
		t.Fatalf("response = %+v", resp)
	}
	if resp.Events[0].Direction != "in" || !strings.Contains(resp.Events[0].Data, "working") {
		t.Fatalf("first event = %+v", resp.Events[0])
	}
	if !strings.Contains(string(resp.Body), `"artifact-update"`) {
		t.Fatalf("body = %s", resp.Body)
	}
}

func TestRPCError(t *testing.T) {
	fs := newFakeServer(t)
	resp, err := New().Exec(context.Background(), apidoc.DebugRequest{
		Method: "tasks/get", URL: a2aURL(fs, "/agent"), Body: []byte(`{"id":"missing"}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Status != "ERROR" || !strings.Contains(string(resp.Body), "-32001") {
		t.Fatalf("response = %+v", resp)
	}
}

func TestA2AVersionHeader(t *testing.T) {
	fs := newFakeServer(t)
	if _, err := New().Exec(context.Background(), apidoc.DebugRequest{
		Method: "message/send", URL: a2aURL(fs, "/agent"),
		Body:    []byte(`{"message":{}}`),
		Options: map[string]string{"a2a_version": "0.3.0"},
	}); err != nil {
		t.Fatal(err)
	}
	if got := fs.lastVersion(); got != "0.3.0" {
		t.Fatalf("A2A-Version = %q", got)
	}
}

func TestEmptyStream(t *testing.T) {
	fs := newFakeServer(t)
	resp, err := New().Exec(context.Background(), apidoc.DebugRequest{
		Method: "message/stream", URL: a2aURL(fs, "/empty"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Status != "EMPTY" || len(resp.Events) != 0 {
		t.Fatalf("response = %+v", resp)
	}
}

func TestInvalidPayload(t *testing.T) {
	if _, err := New().Exec(context.Background(), apidoc.DebugRequest{
		Method: "message/send", URL: "a2a://127.0.0.1:1/agent", Body: []byte("not-json"),
	}); err == nil {
		t.Fatal("want error for invalid JSON payload")
	}
}

func TestBadURL(t *testing.T) {
	if _, err := New().Exec(context.Background(), apidoc.DebugRequest{URL: "ftp://x"}); err == nil {
		t.Fatal("want error for unsupported scheme")
	}
}
