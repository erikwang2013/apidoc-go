package main_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/erikwang2013/apidoc-go"
	"github.com/erikwang2013/apidoc-go/adapter"
	"github.com/erikwang2013/apidoc-go/example/handlers"
	"github.com/erikwang2013/apidoc-go/parse"
)

// TestExampleNetHTTPServer wires the example's parsed handlers through
// apidoc + the net/http adapter exactly as nethttpDemo does, then hits
// the mux in-process: UI, doc API and one registered route.
func TestExampleNetHTTPServer(t *testing.T) {
	// cwd is example/, so ParseDir resolves via the "handlers" fallback —
	// the cwd-independent path the example relies on.
	rs, err := parse.ParseDir("handlers")
	if err != nil {
		t.Fatalf("ParseDir: %v", err)
	}
	if len(rs) != 3 {
		t.Fatalf("expected 3 parsed handlers, got %d", len(rs))
	}
	mux := http.NewServeMux()
	s := apidoc.New(apidoc.Config{Prefix: "/apidoc", Title: "net/http demo"})
	for i, r := range rs {
		fn := []http.HandlerFunc{handlers.ListUsers, handlers.GetUser, handlers.CreateUser}[i]
		if err := s.Register(apidoc.Route{Method: r.Method, URL: r.URL, Handler: fn, Doc: r.Doc}); err != nil {
			t.Fatalf("register %s %s: %v", r.Method, r.URL, err)
		}
	}
	if err := s.Mount(adapter.NewNetHTTP(mux)); err != nil {
		t.Fatal(err)
	}
	do := func(method, path string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(method, path, nil))
		return rec
	}
	if rec := do(http.MethodGet, "/apidoc"); rec.Code != http.StatusOK ||
		!strings.Contains(rec.Body.String(), "<html") {
		t.Fatalf("UI: want 200 html, got %d", rec.Code)
	}
	if rec := do(http.MethodGet, "/apidoc/api/menus"); rec.Code != http.StatusOK ||
		!strings.Contains(rec.Body.String(), `"key":"default"`) ||
		!strings.Contains(rec.Body.String(), `"name":"ListUsers"`) {
		t.Fatalf("menus: want 200 with parsed handlers, got %d %s", rec.Code, rec.Body.String())
	}
	if rec := do(http.MethodGet, "/api/users"); rec.Code != http.StatusOK ||
		!strings.Contains(rec.Body.String(), `"name":"erik"`) {
		t.Fatalf("registered route: want 200 erik, got %d %s", rec.Code, rec.Body.String())
	}
	if rec := do(http.MethodGet, "/apidoc/api/export?format=typescript"); rec.Code != http.StatusOK ||
		!strings.Contains(rec.Body.String(), "ListUsers") {
		t.Fatalf("export: want 200 ts, got %d", rec.Code)
	}
}
