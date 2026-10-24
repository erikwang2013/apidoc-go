package mcp

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
	mu          sync.Mutex
	initializes int
	notified    bool
}

func (fs *fakeServer) state() (int, bool) {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	return fs.initializes, fs.notified
}

func newFakeServer(t *testing.T) *fakeServer {
	t.Helper()
	fs := &fakeServer{}
	mux := http.NewServeMux()
	mux.HandleFunc("/mcp", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "want POST", http.StatusMethodNotAllowed)
			return
		}
		var req struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
			Params json.RawMessage `json:"params"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		switch req.Method {
		case "initialize":
			if r.Header.Get("MCP-Protocol-Version") == "" {
				t.Error("missing MCP-Protocol-Version header")
			}
			fs.mu.Lock()
			fs.initializes++
			fs.mu.Unlock()
			w.Header().Set("Mcp-Session-Id", "sess-1")
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"jsonrpc":"2.0","id":1,"result":{"protocolVersion":"2025-06-18","capabilities":{},"serverInfo":{"name":"fake","version":"1"}}}`)
		case "notifications/initialized":
			fs.mu.Lock()
			fs.notified = true
			fs.mu.Unlock()
			w.WriteHeader(http.StatusAccepted)
		case "tools/list":
			if r.Header.Get("Mcp-Session-Id") != "sess-1" {
				http.Error(w, "missing session", http.StatusBadRequest)
				return
			}
			fmt.Fprint(w, `{"jsonrpc":"2.0","id":2,"result":{"tools":[{"name":"echo"}]}}`)
		case "tools/call":
			if r.Header.Get("Mcp-Session-Id") != "sess-1" {
				http.Error(w, "missing session", http.StatusBadRequest)
				return
			}
			var p struct {
				Name string `json:"name"`
			}
			_ = json.Unmarshal(req.Params, &p)
			if p.Name == "bad" {
				fmt.Fprint(w, `{"jsonrpc":"2.0","id":2,"error":{"code":-32601,"message":"unknown tool"}}`)
				return
			}
			w.Header().Set("Content-Type", "text/event-stream")
			fmt.Fprint(w, "event: message\ndata: {\"jsonrpc\":\"2.0\",\"method\":\"notifications/progress\",\"params\":{}}\n\n")
			fmt.Fprint(w, "event: message\ndata: {\"jsonrpc\":\"2.0\",\"id\":2,\"result\":{\"content\":[{\"type\":\"text\",\"text\":\"ok\"}]}}\n\n")
		default:
			fmt.Fprint(w, `{"jsonrpc":"2.0","id":2,"result":{}}`)
		}
	})
	mux.HandleFunc("/bad", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "token expired", http.StatusUnauthorized)
	})
	fs.Server = httptest.NewServer(mux)
	t.Cleanup(fs.Close)
	return fs
}

func mcpURL(fs *fakeServer, path string) string {
	return "mcp://" + strings.TrimPrefix(fs.URL, "http://") + path
}

func TestToolsCallOverSSE(t *testing.T) {
	fs := newFakeServer(t)
	resp, err := New().Exec(context.Background(), apidoc.DebugRequest{
		Method: "tools/call",
		URL:    mcpURL(fs, "/mcp"),
		Body:   []byte(`{"name":"echo","arguments":{"x":1}}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Status != "OK" || !strings.Contains(string(resp.Body), `"content"`) {
		t.Fatalf("response = %+v", resp)
	}
	if resp.Headers["mcp-session-id"] != "sess-1" || resp.Encoding != "json" {
		t.Fatalf("headers = %+v encoding = %q", resp.Headers, resp.Encoding)
	}
	init, notified := fs.state()
	if init != 1 || !notified {
		t.Fatalf("initialize=%d notified=%v", init, notified)
	}
}

func TestToolsListJSON(t *testing.T) {
	fs := newFakeServer(t)
	resp, err := New().Exec(context.Background(), apidoc.DebugRequest{
		Method: "tools/list",
		URL:    mcpURL(fs, "/mcp"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Status != "OK" || !strings.Contains(string(resp.Body), `"tools"`) {
		t.Fatalf("response = %+v", resp)
	}
}

func TestSessionReuseSkipsInitialize(t *testing.T) {
	fs := newFakeServer(t)
	d := New()
	if _, err := d.Exec(context.Background(), apidoc.DebugRequest{
		Method: "tools/list", URL: mcpURL(fs, "/mcp"),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Exec(context.Background(), apidoc.DebugRequest{
		Method: "tools/list", URL: mcpURL(fs, "/mcp"),
		Options: map[string]string{"session_id": "sess-1"},
	}); err != nil {
		t.Fatal(err)
	}
	if init, _ := fs.state(); init != 1 {
		t.Fatalf("initialize count = %d, want 1", init)
	}
}

func TestRPCError(t *testing.T) {
	fs := newFakeServer(t)
	resp, err := New().Exec(context.Background(), apidoc.DebugRequest{
		Method: "tools/call",
		URL:    mcpURL(fs, "/mcp"),
		Body:   []byte(`{"name":"bad"}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Status != "ERROR" || !strings.Contains(string(resp.Body), "-32601") {
		t.Fatalf("response = %+v", resp)
	}
}

func TestInitializeAction(t *testing.T) {
	fs := newFakeServer(t)
	resp, err := New().Exec(context.Background(), apidoc.DebugRequest{
		Method: "initialize", URL: mcpURL(fs, "/mcp"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Status != "OK" || !strings.Contains(string(resp.Body), "protocolVersion") {
		t.Fatalf("response = %+v", resp)
	}
	if init, notified := fs.state(); init != 1 || notified {
		t.Fatalf("initialize=%d notified=%v", init, notified)
	}
}

func TestHTTPError(t *testing.T) {
	fs := newFakeServer(t)
	_, err := New().Exec(context.Background(), apidoc.DebugRequest{
		Method: "tools/list", URL: mcpURL(fs, "/bad"),
	})
	if err == nil || !strings.Contains(err.Error(), "401") {
		t.Fatalf("error = %v", err)
	}
}

func TestInvalidPayload(t *testing.T) {
	if _, err := New().Exec(context.Background(), apidoc.DebugRequest{
		Method: "tools/call", URL: "mcp://127.0.0.1:1/mcp", Body: []byte("not-json"),
	}); err == nil {
		t.Fatal("want error for invalid JSON payload")
	}
}

func TestBadURL(t *testing.T) {
	if _, err := New().Exec(context.Background(), apidoc.DebugRequest{URL: "ftp://x"}); err == nil {
		t.Fatal("want error for unsupported scheme")
	}
}
