//go:build apidoc_debug_http || apidoc_debug_all

package uiapi

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestRelayHTTPUnary(t *testing.T) {
	var gotMethod, gotBody, gotHeader string
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		gotMethod, gotBody, gotHeader = r.Method, string(body), r.Header.Get("X-Token")
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Mock", "one")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":1}`))
	}))
	defer mock.Close()

	h, id := debugStoreHandler(t, "http", "POST", mock.URL+"/users", DebugConfig{
		EnableServerSide: true, AllowPrivateNetworks: true, TimeoutMS: 2000,
	})
	data := debugSendData(t, h, id, `{"headers":{"X-Token":"t"},"payload":"hello","encoding":"utf8","timeout":2}`)
	if data["ok"] != true || data["status"] != float64(http.StatusCreated) {
		t.Fatalf("unexpected result: %+v", data)
	}
	if data["body"] != `{"id":1}` || data["encoding"] != "text" {
		t.Fatalf("unexpected body: %+v", data)
	}
	headers := data["headers"].(map[string]any)
	if headers["x-mock"] != "one" || headers["content-type"] != "application/json" {
		t.Fatalf("unexpected headers: %+v", headers)
	}
	if gotMethod != "POST" || gotBody != "hello" || gotHeader != "t" {
		t.Fatalf("upstream saw method=%q body=%q header=%q", gotMethod, gotBody, gotHeader)
	}
	if _, ok := data["meta"].(map[string]any)["ms"]; !ok {
		t.Fatalf("missing meta.ms: %+v", data)
	}
}

func TestRelayHTTP2PriorKnowledge(t *testing.T) {
	mock := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(r.Proto))
	}))
	protocols := new(http.Protocols)
	protocols.SetUnencryptedHTTP2(true)
	mock.Config.Protocols = protocols
	mock.Start()
	defer mock.Close()

	h, id := debugStoreHandler(t, "http2", "POST", mock.URL+"/h2", DebugConfig{
		EnableServerSide: true, AllowPrivateNetworks: true, TimeoutMS: 2000,
	})
	data := debugSendData(t, h, id, `{}`)
	if data["ok"] != true || data["body"] != "HTTP/2.0" {
		t.Fatalf("h2c prior knowledge not used: %+v", data)
	}
}

func TestRelayHTTPDoesNotFollowRedirect(t *testing.T) {
	var followed atomic.Int32
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/next" {
			followed.Add(1)
			_, _ = w.Write([]byte("next"))
			return
		}
		http.Redirect(w, r, "/next", http.StatusFound)
	}))
	defer mock.Close()

	h, id := debugStoreHandler(t, "http", "GET", mock.URL+"/start", DebugConfig{
		EnableServerSide: true, AllowPrivateNetworks: true,
	})
	data := debugSendData(t, h, id, `{}`)
	if data["ok"] != true || data["status"] != float64(http.StatusFound) {
		t.Fatalf("redirect must be returned, not followed: %+v", data)
	}
	if followed.Load() != 0 {
		t.Fatal("relay followed an unchecked redirect target")
	}
}

func TestRelaySSRFBlockedByDefault(t *testing.T) {
	var hits atomic.Int32
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
	}))
	defer mock.Close()

	h, id := debugStoreHandler(t, "http", "GET", mock.URL+"/private", DebugConfig{
		EnableServerSide: true,
	})
	data := debugSendData(t, h, id, `{}`)
	if data["ok"] != false || !strings.Contains(data["error"].(string), "private") {
		t.Fatalf("loopback target must be rejected: %+v", data)
	}
	if hits.Load() != 0 {
		t.Fatal("blocked target was contacted")
	}
}

func TestRelayHTTPProtocolContentType(t *testing.T) {
	var contentType string
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		contentType = r.Header.Get("Content-Type")
		_, _ = w.Write([]byte(`{"data":{"ok":true}}`))
	}))
	defer mock.Close()

	h, id := debugStoreHandler(t, "graphql", "POST", mock.URL+"/graphql", DebugConfig{
		EnableServerSide: true, AllowPrivateNetworks: true,
	})
	data := debugSendData(t, h, id, `{"payload":"{\"query\":\"{ x }\"}","encoding":"utf8"}`)
	if data["ok"] != true {
		t.Fatalf("graphql relay: %+v", data)
	}
	if contentType != "application/json" {
		t.Fatalf("graphql content type = %q", contentType)
	}
}

func TestRelayHTTPTimeoutAndResponseCap(t *testing.T) {
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(150 * time.Millisecond)
		_, _ = w.Write([]byte("0123456789"))
	}))
	defer mock.Close()

	h, id := debugStoreHandler(t, "http", "GET", mock.URL+"/slow", DebugConfig{
		EnableServerSide: true, AllowPrivateNetworks: true, TimeoutMS: 50,
	})
	if data := debugSendData(t, h, id, `{}`); data["ok"] != false {
		t.Fatalf("timeout must fail: %+v", data)
	}

	fast, fastID := debugStoreHandler(t, "http", "GET", mock.URL+"/slow", DebugConfig{
		EnableServerSide: true, AllowPrivateNetworks: true, TimeoutMS: 2000, MaxBytes: 5,
	})
	if data := debugSendData(t, fast, fastID, `{}`); data["ok"] != false || !strings.Contains(data["error"].(string), "max_bytes") {
		t.Fatalf("response cap must fail: %+v", data)
	}
}
