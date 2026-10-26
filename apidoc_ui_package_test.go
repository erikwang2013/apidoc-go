package apidoc_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/erikwang2013/apidoc-go"
	"github.com/erikwang2013/apidoc-go/adapter"
)

// firstAssetRE finds the hashed asset reference in the packaged index.html.
var firstAssetRE = regexp.MustCompile(`(?:src|href)="\./(assets/[^"]+)"`)

// TestPackagedUIServed is the Phase 5.3 gate against the real dist/uic1-ui
// package: UIC1_UI_PACKAGE points at a directory produced by apidoc-ui's
// scripts/package-ui.sh. The E2E script sets it; plain `go test ./...` skips.
func TestPackagedUIServed(t *testing.T) {
	pkg := os.Getenv("UIC1_UI_PACKAGE")
	if pkg == "" {
		t.Skip("UIC1_UI_PACKAGE not set; skipping packaged UI serve test")
	}
	manifestJSON, err := os.ReadFile(filepath.Join(pkg, "manifest.json"))
	if err != nil {
		t.Fatalf("UIC1_UI_PACKAGE=%q: %v", pkg, err)
	}
	var manifest struct {
		Name    string `json:"name"`
		Version string `json:"version"`
		SHA256  string `json:"sha256"`
	}
	if err := json.Unmarshal(manifestJSON, &manifest); err != nil {
		t.Fatalf("%s/manifest.json: %v", pkg, err)
	}
	if manifest.Name != "uic1-ui" || manifest.Version == "" || len(manifest.SHA256) != 64 {
		t.Fatalf("%s/manifest.json: unexpected %+v", pkg, manifest)
	}

	mux := http.NewServeMux()
	s := apidoc.New(apidoc.Config{Prefix: "/apidoc", UIPath: pkg})
	if err := s.Mount(adapter.NewNetHTTP(mux)); err != nil {
		t.Fatal(err)
	}
	get := func(path string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		return rec
	}

	rec := get("/apidoc/")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Header().Get("Content-Type"), "text/html") {
		t.Fatalf("/apidoc/: want 200 text/html, got %d %q", rec.Code, rec.Header().Get("Content-Type"))
	}
	m := firstAssetRE.FindStringSubmatch(rec.Body.String())
	if m == nil {
		t.Fatalf("/apidoc/ index has no ./assets/ reference")
	}

	for _, path := range []string{
		"/apidoc/config.js",
		"/apidoc/" + m[1],
		"/apidoc/manifest.json",
	} {
		got := get(path)
		if got.Code != http.StatusOK {
			t.Fatalf("%s: want 200, got %d", path, got.Code)
		}
		if got.Body.Len() == 0 {
			t.Fatalf("%s: empty body", path)
		}
		if path == "/apidoc/manifest.json" && got.Body.String() != string(manifestJSON) {
			t.Fatalf("%s: served bytes differ from %s/manifest.json", path, pkg)
		}
	}
}
