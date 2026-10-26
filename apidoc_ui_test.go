package apidoc_test

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/erikwang2013/apidoc-go"
	"github.com/erikwang2013/apidoc-go/adapter"
)

const mountIndexBody = "<!doctype html><title>uic1-mount</title>"

func mountUIFixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	files := map[string]string{
		"index.html":    mountIndexBody,
		"config.js":     "window.apidocFeConfig={}",
		"assets/app.js": "console.log(1)",
	}
	for name, body := range files {
		p := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestServerMountServesUIPackage(t *testing.T) {
	mux := http.NewServeMux()
	s := apidoc.New(apidoc.Config{Prefix: "/docs/v2", UIPath: mountUIFixture(t)})
	if err := s.Mount(adapter.NewNetHTTP(mux)); err != nil {
		t.Fatal(err)
	}
	do := func(method, path string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(method, path, nil))
		return rec
	}

	// Bare prefix is stripped to "": redirect to {prefix}/ preserving query.
	rec := do("GET", "/docs/v2?x=1")
	if rec.Code != http.StatusTemporaryRedirect || rec.Header().Get("Location") != "/docs/v2/?x=1" {
		t.Fatalf("bare prefix: want 307 /docs/v2/?x=1, got %d %q", rec.Code, rec.Header().Get("Location"))
	}
	for path, body := range map[string]string{
		"/docs/v2/":              mountIndexBody,
		"/docs/v2/index.html":    mountIndexBody,
		"/docs/v2/config.js":     "window.apidocFeConfig={}",
		"/docs/v2/assets/app.js": "console.log(1)",
		"/docs/v2/routes/deep":   mountIndexBody, // SPA fallback
	} {
		rec := do("GET", path)
		if rec.Code != http.StatusOK || rec.Body.String() != body {
			t.Fatalf("%s: want 200 %q, got %d %q", path, body, rec.Code, rec.Body.String())
		}
	}
	if rec := do("GET", "/docs/v2/missing.js"); rec.Code != http.StatusNotFound {
		t.Fatalf("missing asset: want 404, got %d", rec.Code)
	}
	// API routes keep precedence; unknown API paths never reach the SPA.
	if rec := do("GET", "/docs/v2/api/menus"); rec.Code != http.StatusOK ||
		!strings.Contains(rec.Body.String(), `"code":0`) {
		t.Fatalf("menus through mount: %d %q", rec.Code, rec.Body.String())
	}
	if rec := do("GET", "/docs/v2/ui/v1/capabilities"); rec.Code != http.StatusOK {
		t.Fatalf("capabilities through mount: %d", rec.Code)
	}
	if rec := do("GET", "/docs/v2/api/unknown"); rec.Code != http.StatusNotFound {
		t.Fatalf("unknown API through mount: want 404, got %d", rec.Code)
	}
	// ServeMux canonicalizes ../ away before the prefix matches.
	rec = do("GET", "/docs/v2/../secret")
	if rec.Code == http.StatusOK && strings.Contains(rec.Body.String(), "uic1") {
		t.Fatalf("traversal served the UI: %d %q", rec.Code, rec.Body.String())
	}
}

func TestServerMountUIUnset(t *testing.T) {
	mux := http.NewServeMux()
	s := apidoc.New(apidoc.Config{Prefix: "/apidoc"})
	if err := s.Mount(adapter.NewNetHTTP(mux)); err != nil {
		t.Fatal(err)
	}
	do := func(path string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		return rec
	}
	if rec := do("/apidoc/api/menus"); rec.Code != http.StatusOK {
		t.Fatalf("API regressed without UIPath: %d", rec.Code)
	}
	for _, p := range []string{"/apidoc/", "/apidoc/config.js"} {
		if rec := do(p); rec.Code != http.StatusNotFound {
			t.Fatalf("%s without UIPath: want 404, got %d", p, rec.Code)
		}
	}
}
