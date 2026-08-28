package adapter_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/erikwang2013/apidoc-go/adapter"
	"github.com/gin-gonic/gin"
	"github.com/go-chi/chi/v5"
	"github.com/gofiber/fiber/v2"
	"github.com/labstack/echo/v4"
)

// uiHandler echoes the path it sees, so tests prove the mount strips the
// prefix before the doc handler is invoked.
func uiHandler(w http.ResponseWriter, r *http.Request) {
	_, _ = w.Write([]byte("UI:" + r.URL.Path))
}

func assertMounted(t *testing.T, get func(path string) (int, string)) {
	t.Helper()
	// {prefix} strips to "" (the doc server rewrites "" to "/"), {prefix}/
	// to "/", and {prefix}/path to the bare path.
	want := map[string]string{
		"/apidoc":           "UI:",
		"/apidoc/":          "UI:/",
		"/apidoc/api/menus": "UI:/api/menus",
	}
	for path, wantBody := range want {
		code, body := get(path)
		if code != http.StatusOK || body != wantBody {
			t.Fatalf("mount %s: want 200 %q, got %d %q", path, wantBody, code, body)
		}
	}
}

func TestNetHTTPRegisterAndMount(t *testing.T) {
	mux := http.NewServeMux()
	a := adapter.NewNetHTTP(mux)
	var got string
	if err := a.Register("GET", "/ping", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = "pong"
		w.Write([]byte("pong"))
	})); err != nil {
		t.Fatal(err)
	}
	a.Mount("/apidoc", http.HandlerFunc(uiHandler))
	get := func(path string) (int, string) {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		return rec.Code, rec.Body.String()
	}
	if code, body := get("/ping"); code != http.StatusOK || got != "pong" {
		t.Fatalf("route: want 200 pong, got %d %q", code, body)
	}
	assertMounted(t, get)
	// wrong handler type names the expectation
	err := a.Register("GET", "/bad", gin.HandlerFunc(func(*gin.Context) {}))
	if err == nil || !strings.Contains(err.Error(), "http.HandlerFunc") {
		t.Fatalf("type mismatch: want named error, got %v", err)
	}
}

func TestGinRegisterAndMount(t *testing.T) {
	gin.SetMode(gin.TestMode)
	e := gin.New()
	a := adapter.NewGin(e)
	var got string
	if err := a.Register("GET", "/ping", gin.HandlerFunc(func(c *gin.Context) {
		got = "pong"
		c.String(http.StatusOK, "pong")
	})); err != nil {
		t.Fatal(err)
	}
	a.Mount("/apidoc", http.HandlerFunc(uiHandler))
	get := func(path string) (int, string) {
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		return rec.Code, rec.Body.String()
	}
	if code, body := get("/ping"); code != http.StatusOK || got != "pong" {
		t.Fatalf("route: want 200 pong, got %d %q", code, body)
	}
	assertMounted(t, get)
}

func TestEchoRegisterAndMount(t *testing.T) {
	e := echo.New()
	a := adapter.NewEcho(e)
	var got string
	if err := a.Register("GET", "/ping", echo.HandlerFunc(func(c echo.Context) error {
		got = "pong"
		return c.String(http.StatusOK, "pong")
	})); err != nil {
		t.Fatal(err)
	}
	a.Mount("/apidoc", http.HandlerFunc(uiHandler))
	get := func(path string) (int, string) {
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		return rec.Code, rec.Body.String()
	}
	if code, body := get("/ping"); code != http.StatusOK || got != "pong" {
		t.Fatalf("route: want 200 pong, got %d %q", code, body)
	}
	assertMounted(t, get)
}

func TestChiRegisterAndMount(t *testing.T) {
	mux := chi.NewMux()
	a := adapter.NewChi(mux)
	var got string
	if err := a.Register("GET", "/ping", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = "pong"
		w.Write([]byte("pong"))
	})); err != nil {
		t.Fatal(err)
	}
	a.Mount("/apidoc", http.HandlerFunc(uiHandler))
	get := func(path string) (int, string) {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		return rec.Code, rec.Body.String()
	}
	if code, body := get("/ping"); code != http.StatusOK || got != "pong" {
		t.Fatalf("route: want 200 pong, got %d %q", code, body)
	}
	assertMounted(t, get)
}

func TestFiberRegisterAndMount(t *testing.T) {
	app := fiber.New()
	a := adapter.NewFiber(app)
	var got string
	if err := a.Register("GET", "/ping", func(c *fiber.Ctx) error {
		got = "pong"
		return c.SendString("pong")
	}); err != nil {
		t.Fatal(err)
	}
	a.Mount("/apidoc", http.HandlerFunc(uiHandler))
	get := func(path string) (int, string) {
		resp, err := app.Test(httptest.NewRequest(http.MethodGet, path, nil))
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, string(body)
	}
	if code, body := get("/ping"); code != http.StatusOK || got != "pong" {
		t.Fatalf("route: want 200 pong, got %d %q", code, body)
	}
	assertMounted(t, get)
}
