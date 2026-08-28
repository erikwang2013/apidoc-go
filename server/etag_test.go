package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// GET responses carry ETag + Cache-Control; a matching If-None-Match
// yields 304 with no body; POST gets no cache headers.
func TestETagCaching(t *testing.T) {
	h := Handler(Opts{Prefix: "/apidoc", Store: newTestStore()})

	rec := req(t, h, http.MethodGet, "/api/menus", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET menus: want 200 got %d", rec.Code)
	}
	etag := rec.Header().Get("ETag")
	if etag == "" || rec.Header().Get("Cache-Control") != "private, max-age=300" || rec.Body.Len() == 0 {
		t.Fatalf("GET: want ETag+Cache-Control+body, got etag=%q cc=%q body=%d", etag, rec.Header().Get("Cache-Control"), rec.Body.Len())
	}

	r := httptest.NewRequest(http.MethodGet, "/api/menus", nil)
	r.Header.Set("If-None-Match", etag)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	if rec.Code != http.StatusNotModified || rec.Body.Len() != 0 || rec.Header().Get("ETag") != etag {
		t.Fatalf("matching If-None-Match: want 304+no body+etag, got %d body=%d etag=%q", rec.Code, rec.Body.Len(), rec.Header().Get("ETag"))
	}

	rec = req(t, h, http.MethodPost, "/api/app-login", "")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("POST app-login: want 400 got %d", rec.Code)
	}
	if etag := rec.Header().Get("ETag"); etag != "" {
		t.Fatalf("POST got ETag %q", etag)
	}
}
