package adapter_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/erikwang2013/apidoc-go/adapter"
	khttp "github.com/go-kratos/kratos/v2/transport/http"
)

func TestKratosRegisterAndMount(t *testing.T) {
	srv := khttp.NewServer()
	a := adapter.NewKratos(srv)
	var got string
	if err := a.Register("GET", "/ping", khttp.HandlerFunc(func(ctx khttp.Context) error {
		got = "pong"
		return ctx.String(http.StatusOK, "pong")
	})); err != nil {
		t.Fatal(err)
	}
	a.Mount("/apidoc", http.HandlerFunc(uiHandler))
	get := func(path string) (int, string) {
		rec := httptest.NewRecorder()
		srv.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		return rec.Code, rec.Body.String()
	}
	if code, body := get("/ping"); code != http.StatusOK || got != "pong" {
		t.Fatalf("route: want 200 pong, got %d %q", code, body)
	}
	assertMounted(t, get)
	// wrong handler type names the expectation (probe with an unrelated
	// type: khttp.HandlerFunc and net/http's HandlerFunc both print as
	// "http.HandlerFunc", so a net/http handler would make the check vacuous)
	err := a.Register("GET", "/bad", struct{}{})
	if err == nil || !strings.Contains(err.Error(), "http.HandlerFunc") {
		t.Fatalf("type mismatch: want named error, got %v", err)
	}
}
