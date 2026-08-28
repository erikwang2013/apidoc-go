package apidoc_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/erikwang2013/apidoc-go"
	"github.com/erikwang2013/apidoc-go/adapter"
)

func TestServerRegisterMountFlow(t *testing.T) {
	mux := http.NewServeMux()
	s := apidoc.New(apidoc.Config{Title: "t",
		Auth: apidoc.AuthConfig{Enable: true, Password: "pw", Secret: "sec"}})

	// registered before Mount -> queued, replayed on Mount
	var pre string
	if err := s.Register(apidoc.Route{Method: "GET", URL: "/pre", Handler: http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) { pre = "ok" })}); err != nil {
		t.Fatal(err)
	}
	if err := s.Mount(adapter.NewNetHTTP(mux)); err != nil {
		t.Fatal(err)
	}
	// registered after Mount -> forwarded to the framework immediately
	var post string
	if err := s.Register(apidoc.Route{Method: "GET", URL: "/post", Handler: http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) { post = "ok" })}); err != nil {
		t.Fatal(err)
	}
	// re-registering the same endpoint after Mount must not re-forward
	// (re-adding the route would panic on a conflict); the framework keeps
	// the first handler, the store gets the updated doc.
	if err := s.Register(apidoc.Route{Method: "GET", URL: "/post", Handler: http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) { post = "again" })}); err != nil {
		t.Fatalf("re-register must not re-forward: %v", err)
	}

	do := func(method, path, body string, c ...*http.Cookie) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		for _, ck := range c {
			r.AddCookie(ck)
		}
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, r)
		return rec
	}

	if rec := do("GET", "/pre", ""); rec.Code != http.StatusOK || pre != "ok" {
		t.Fatalf("pre-mount route failed: %d", rec.Code)
	}
	if rec := do("GET", "/post", ""); rec.Code != http.StatusOK || post != "ok" {
		t.Fatalf("post-mount route failed: %d %q", rec.Code, rec.Body.String())
	}

	// UI at the exact prefix
	if rec := do("GET", "/apidoc", ""); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "<!doctype html") {
		t.Fatalf("UI at prefix failed: %d", rec.Code)
	}
	// API gated by auth
	if rec := do("GET", "/apidoc/api/menus", ""); rec.Code != http.StatusUnauthorized {
		t.Fatalf("menus without login: want 401 got %d", rec.Code)
	}
	rec := do("POST", "/apidoc/api/login", `{"password":"pw"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("login failed: %d %s", rec.Code, rec.Body.String())
	}
	cookies := rec.Result().Cookies()
	if len(cookies) != 1 || cookies[0].Path != "/apidoc" {
		t.Fatalf("login cookie wrong: %+v", cookies)
	}
	rec = do("GET", "/apidoc/api/menus", "", cookies[0])
	if rec.Code != http.StatusOK {
		t.Fatalf("menus after login: want 200 got %d", rec.Code)
	}
	var body struct {
		Data []map[string]any `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || len(body.Data) == 0 {
		t.Fatalf("menus payload wrong: %s", rec.Body.String())
	}
}

// TestConcurrentRegisterMount hammers Register while Mount replays the
// pending queue: no data race, no double-forward (which would panic on
// chi/gin), and every pre-Mount route must be reachable afterwards.
func TestConcurrentRegisterMount(t *testing.T) {
	mux := http.NewServeMux()
	s := apidoc.New(apidoc.Config{})
	ok := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})
	for i := 0; i < 20; i++ {
		if err := s.Register(apidoc.Route{Method: "GET", URL: fmt.Sprintf("/pre%d", i), Handler: ok}); err != nil {
			t.Fatal(err)
		}
	}
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			_ = s.Register(apidoc.Route{Method: "GET", URL: fmt.Sprintf("/live%d", i), Handler: ok})
			_ = s.Register(apidoc.Route{Method: "GET", URL: fmt.Sprintf("/pre%d", i%20), Handler: ok})
		}
	}()
	go func() {
		defer wg.Done()
		if err := s.Mount(adapter.NewNetHTTP(mux)); err != nil {
			t.Errorf("mount: %v", err)
		}
	}()
	wg.Wait()
	for i := 0; i < 20; i++ {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest("GET", fmt.Sprintf("/pre%d", i), nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("pre-mount route /pre%d lost: %d", i, rec.Code)
		}
	}
}
