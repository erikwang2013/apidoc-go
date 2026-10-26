package server

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const uiIndexBody = "<!doctype html><title>uic1</title>"

// uiFixture writes a minimal packaged UI tree and returns its directory.
func uiFixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	files := map[string]string{
		"index.html":    uiIndexBody,
		"config.js":     "window.apidocFeConfig={}",
		"style.css":     "body{}",
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

func TestUIDefaultOff(t *testing.T) {
	h := Handler(Opts{Prefix: "/apidoc", Store: newTestStore()})
	for _, p := range []string{"/", "/index.html", "/config.js", "/assets/app.js"} {
		if rec := req(t, h, http.MethodGet, p, ""); rec.Code != http.StatusNotFound {
			t.Fatalf("%s without UIPath: want 404, got %d", p, rec.Code)
		}
	}
	if rec := req(t, h, http.MethodGet, "/api/menus", ""); rec.Code != http.StatusOK {
		t.Fatalf("API regressed without UIPath: %d", rec.Code)
	}
}

func TestUIServesPackagedFiles(t *testing.T) {
	h := Handler(Opts{Prefix: "/apidoc", Store: newTestStore(), UIPath: uiFixture(t)})
	cases := []struct{ path, body, ctype string }{
		{"/", uiIndexBody, "text/html"},
		{"/index.html", uiIndexBody, "text/html"},
		{"/config.js", "window.apidocFeConfig={}", "javascript"},
		{"/assets/app.js", "console.log(1)", "javascript"},
		{"/style.css", "body{}", "text/css"},
	}
	for _, c := range cases {
		rec := req(t, h, http.MethodGet, c.path, "")
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: want 200, got %d: %s", c.path, rec.Code, rec.Body.String())
		}
		if rec.Body.String() != c.body {
			t.Fatalf("%s body: want %q, got %q", c.path, c.body, rec.Body.String())
		}
		if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, c.ctype) {
			t.Fatalf("%s content-type: want %q, got %q", c.path, c.ctype, ct)
		}
	}
	rec := req(t, h, http.MethodHead, "/", "")
	if rec.Code != http.StatusOK || rec.Body.Len() != 0 {
		t.Fatalf("HEAD /: want 200 empty, got %d %q", rec.Code, rec.Body.String())
	}
}

func TestUISPAFallback(t *testing.T) {
	h := Handler(Opts{Prefix: "/apidoc", Store: newTestStore(), UIPath: uiFixture(t)})
	for _, p := range []string{"/routes/users/42", "/routes/users/42/"} {
		rec := req(t, h, http.MethodGet, p, "")
		if rec.Code != http.StatusOK || rec.Body.String() != uiIndexBody {
			t.Fatalf("%s: want SPA index, got %d %q", p, rec.Code, rec.Body.String())
		}
	}
	// Missing assets and directories stay 404: no SPA for dotted paths, no
	// directory listing.
	for _, p := range []string{"/missing.js", "/missing.css", "/assets", "/assets/"} {
		rec := req(t, h, http.MethodGet, p, "")
		if rec.Code != http.StatusNotFound {
			t.Fatalf("%s: want 404, got %d %q", p, rec.Code, rec.Body.String())
		}
		if strings.Contains(rec.Body.String(), "uic1") {
			t.Fatalf("%s: served the SPA index for a non-SPA path", p)
		}
	}
}

func TestUIApiPrecedence(t *testing.T) {
	h := Handler(Opts{Prefix: "/apidoc", Store: newTestStore(), UIPath: uiFixture(t)})
	rec := req(t, h, http.MethodGet, "/api/menus", "")
	if rec.Code != http.StatusOK || strings.Contains(rec.Body.String(), "uic1") {
		t.Fatalf("GET /api/menus: want JSON, got %d %q", rec.Code, rec.Body.String())
	}
	rec = req(t, h, http.MethodGet, "/ui/v1/capabilities", "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"backend":"go"`) {
		t.Fatalf("GET /ui/v1/capabilities: want JSON, got %d %q", rec.Code, rec.Body.String())
	}
	// Unknown API paths never fall back to the SPA; a wrong method on a known
	// API path keeps the API's 405.
	for _, c := range []struct {
		method, path string
		want         int
	}{
		{http.MethodGet, "/api/unknown", http.StatusNotFound},
		{http.MethodPost, "/api/menus", http.StatusMethodNotAllowed},
		{http.MethodGet, "/ui/v1/unknown", http.StatusNotFound},
		{http.MethodGet, "/ui", http.StatusNotFound},
	} {
		rec := req(t, h, c.method, c.path, "")
		if rec.Code != c.want {
			t.Fatalf("%s %s: want %d, got %d", c.method, c.path, c.want, rec.Code)
		}
		if strings.Contains(rec.Body.String(), "uic1") {
			t.Fatalf("%s %s: API path fell back to the SPA", c.method, c.path)
		}
	}
	if rec := req(t, h, http.MethodPost, "/", ""); rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST /: want 405, got %d", rec.Code)
	}
}

func TestUIRejectsTraversalAndSymlinkEscape(t *testing.T) {
	base := t.TempDir()
	dir := filepath.Join(base, "ui")
	if err := os.MkdirAll(filepath.Join(dir, "assets"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte(uiIndexBody), 0o644); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(base, "outside")
	if err := os.MkdirAll(outside, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("TOPSECRET"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(outside, "secret.txt"), filepath.Join(dir, "leak.txt")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("../outside", filepath.Join(dir, "leakdir")); err != nil {
		t.Fatal(err)
	}

	h := newUI(dir)
	for _, p := range []string{
		"/../outside/secret.txt",
		"/assets/../../outside/secret.txt",
		"/leak.txt",
		"/leakdir/secret.txt",
	} {
		r := httptest.NewRequest(http.MethodGet, "http://example.test/", nil)
		r.URL.Path = p // bypass ServeMux cleaning: exercise the UI handler itself
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, r)
		if rec.Code == http.StatusOK || strings.Contains(rec.Body.String(), "TOPSECRET") {
			t.Fatalf("%s escaped UIPath: %d %q", p, rec.Code, rec.Body.String())
		}
	}
}

func TestUIMissingPathFailsClosed(t *testing.T) {
	h := Handler(Opts{Prefix: "/apidoc", Store: newTestStore(),
		UIPath: filepath.Join(t.TempDir(), "gone")})
	rec := req(t, h, http.MethodGet, "/", "")
	if rec.Code != http.StatusInternalServerError || !strings.Contains(rec.Body.String(), "UI path") {
		t.Fatalf("missing UIPath: want clear 500, got %d %q", rec.Code, rec.Body.String())
	}
	if rec := req(t, h, http.MethodGet, "/api/menus", ""); rec.Code != http.StatusOK {
		t.Fatalf("API must survive a broken UIPath: %d", rec.Code)
	}

	h = Handler(Opts{Prefix: "/apidoc", Store: newTestStore(), UIPath: t.TempDir()})
	rec = req(t, h, http.MethodGet, "/config.js", "")
	if rec.Code != http.StatusInternalServerError || !strings.Contains(rec.Body.String(), "index.html") {
		t.Fatalf("UIPath without index.html: want clear 500, got %d %q", rec.Code, rec.Body.String())
	}

	// index.html vanishing after startup still fails closed, including the
	// SPA fallback path.
	dir := uiFixture(t)
	h = Handler(Opts{Prefix: "/apidoc", Store: newTestStore(), UIPath: dir})
	if err := os.Remove(filepath.Join(dir, "index.html")); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{"/", "/routes/user"} {
		rec = req(t, h, http.MethodGet, p, "")
		if rec.Code != http.StatusInternalServerError || !strings.Contains(rec.Body.String(), "index.html") {
			t.Fatalf("%s after index removal: want clear 500, got %d %q", p, rec.Code, rec.Body.String())
		}
	}
}
