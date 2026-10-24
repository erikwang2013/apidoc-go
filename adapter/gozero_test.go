package adapter_test

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/erikwang2013/apidoc-go/adapter"
	"github.com/zeromicro/go-zero/rest"
)

// TestGoZeroRegisterAndMount drives a real go-zero server: rest.Server has
// no exported in-process handler, so routes are exercised over HTTP on a
// port picked from the OS.
func TestGoZeroRegisterAndMount(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()

	srv := rest.MustNewServer(rest.RestConf{Host: "127.0.0.1", Port: port})
	// go-zero's Stop only closes its logger (HTTP shutdown is signal-driven);
	// the listener dies with the test process.
	defer srv.Stop()
	base := fmt.Sprintf("http://127.0.0.1:%d", port)

	a := adapter.NewGoZero(srv)
	var got atomic.Bool
	if err := a.Register("GET", "/ping", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got.Store(true)
		w.Write([]byte("pong"))
	})); err != nil {
		t.Fatal(err)
	}
	a.Mount("/apidoc", http.HandlerFunc(uiHandler))

	go srv.Start()
	deadline := time.Now().Add(3 * time.Second)
	for {
		resp, err := http.Get(base + "/ping")
		if err == nil {
			resp.Body.Close()
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("go-zero server did not serve %s/ping within 3s: %v", base, err)
		}
		time.Sleep(20 * time.Millisecond)
	}

	do := func(method, path string) (int, string) {
		req, err := http.NewRequest(method, base+path, nil)
		if err != nil {
			t.Fatal(err)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("%s %s: %v", method, path, err)
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, string(body)
	}
	get := func(path string) (int, string) { return do(http.MethodGet, path) }

	if code, body := get("/ping"); code != http.StatusOK || body != "pong" || !got.Load() {
		t.Fatalf("route: want 200 pong, got %d %q (handler ran: %v)", code, body, got.Load())
	}
	assertMounted(t, get)

	// Mount's surface is registered per method, not just GET: POST for the
	// login endpoints, OPTIONS for CORS preflights.
	for _, tc := range []struct{ method, path, want string }{
		{http.MethodPost, "/apidoc/api/login", "UI:/api/login"},
		{http.MethodOptions, "/apidoc/api/menus", "UI:/api/menus"},
	} {
		if code, body := do(tc.method, tc.path); code != http.StatusOK || body != tc.want {
			t.Fatalf("mount %s %s: want 200 %q, got %d %q", tc.method, tc.path, tc.want, code, body)
		}
	}

	// wrong handler type names the expectation
	err = a.Register("GET", "/bad", struct{}{})
	if err == nil || !strings.Contains(err.Error(), "http.HandlerFunc") {
		t.Fatalf("type mismatch: want named error, got %v", err)
	}
}
