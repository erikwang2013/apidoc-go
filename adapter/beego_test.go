package adapter_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/beego/beego/v2/server/web"
	"github.com/erikwang2013/apidoc-go/adapter"
)

func TestBeegoRegisterAndMount(t *testing.T) {
	app := web.NewHttpSever()
	a := adapter.NewBeego(app)
	var got string
	if err := a.Register("GET", "/ping", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = "pong"
		w.Write([]byte("pong"))
	})); err != nil {
		t.Fatal(err)
	}
	a.Mount("/apidoc", http.HandlerFunc(uiHandler))
	app.Handlers.Init()
	serve := func(req *http.Request) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		app.Handlers.ServeHTTP(rec, req)
		return rec
	}
	get := func(path string) (int, string) {
		rec := serve(httptest.NewRequest(http.MethodGet, path, nil))
		return rec.Code, rec.Body.String()
	}
	if code, body := get("/ping"); code != http.StatusOK || got != "pong" {
		t.Fatalf("route: want 200 pong, got %d %q", code, body)
	}
	// beego registers all methods, so the guard answers other ones with 405
	if rec := serve(httptest.NewRequest(http.MethodPost, "/ping", nil)); rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("method guard: want 405, got %d %q", rec.Code, rec.Body.String())
	}
	assertMounted(t, get)
	// wrong handler type names the expectation
	err := a.Register("GET", "/bad", struct{}{})
	if err == nil || !strings.Contains(err.Error(), "http.HandlerFunc") {
		t.Fatalf("type mismatch: want named error, got %v", err)
	}
}
