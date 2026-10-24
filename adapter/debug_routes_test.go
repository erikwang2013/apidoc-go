package adapter_test

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/beego/beego/v2/server/web"
	"github.com/erikwang2013/apidoc-go/adapter"
	"github.com/erikwang2013/apidoc-go/server"
	"github.com/erikwang2013/apidoc-go/store"
	"github.com/gin-gonic/gin"
	"github.com/go-chi/chi/v5"
	khttp "github.com/go-kratos/kratos/v2/transport/http"
	"github.com/gofiber/fiber/v2"
	"github.com/labstack/echo/v4"
	"github.com/zeromicro/go-zero/rest"
)

const debugPrefix = "/apidoc"

// debugDoer drives one request through a mounted framework and returns
// status, Content-Type and body.
type debugDoer func(method, path, contentType, body string) (int, string, string)

func debugServer() http.Handler {
	return server.Handler(server.Opts{
		Prefix:           debugPrefix,
		Store:            store.New("p"),
		DebugAllowUnauth: true,
	})
}

func serveHTTP(serve func(http.ResponseWriter, *http.Request)) debugDoer {
	return func(method, path, contentType, body string) (int, string, string) {
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		if contentType != "" {
			r.Header.Set("Content-Type", contentType)
		}
		rec := httptest.NewRecorder()
		serve(rec, r)
		return rec.Code, rec.Header().Get("Content-Type"), rec.Body.String()
	}
}

// TestDebugRoutesAcrossAdapters mounts the real doc server through every
// framework and checks the debug route surface arrives intact. A 403 (not
// 404) from /api/debug/exec proves the CSRF gate ran, i.e. the route reached
// the doc server; /api/debug/open and /api/debug/events likewise must not
// 404. The store needs no actions: every checked gate answers before any
// action lookup.
func TestDebugRoutesAcrossAdapters(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cases := []struct {
		name  string
		mount func(t *testing.T) debugDoer
	}{
		{"net/http", func(*testing.T) debugDoer {
			mux := http.NewServeMux()
			adapter.NewNetHTTP(mux).Mount(debugPrefix, debugServer())
			return serveHTTP(mux.ServeHTTP)
		}},
		{"gin", func(*testing.T) debugDoer {
			e := gin.New()
			adapter.NewGin(e).Mount(debugPrefix, debugServer())
			return serveHTTP(e.ServeHTTP)
		}},
		{"echo", func(*testing.T) debugDoer {
			e := echo.New()
			adapter.NewEcho(e).Mount(debugPrefix, debugServer())
			return serveHTTP(e.ServeHTTP)
		}},
		{"chi", func(*testing.T) debugDoer {
			mux := chi.NewMux()
			adapter.NewChi(mux).Mount(debugPrefix, debugServer())
			return serveHTTP(mux.ServeHTTP)
		}},
		{"fiber", func(t *testing.T) debugDoer {
			app := fiber.New()
			adapter.NewFiber(app).Mount(debugPrefix, debugServer())
			return func(method, path, contentType, body string) (int, string, string) {
				r := httptest.NewRequest(method, path, strings.NewReader(body))
				if contentType != "" {
					r.Header.Set("Content-Type", contentType)
				}
				resp, err := app.Test(r)
				if err != nil {
					t.Fatalf("%s %s: %v", method, path, err)
				}
				defer resp.Body.Close()
				b, _ := io.ReadAll(resp.Body)
				return resp.StatusCode, resp.Header.Get("Content-Type"), string(b)
			}
		}},
		{"beego", func(*testing.T) debugDoer {
			app := web.NewHttpSever()
			adapter.NewBeego(app).Mount(debugPrefix, debugServer())
			app.Handlers.Init()
			return serveHTTP(app.Handlers.ServeHTTP)
		}},
		{"kratos", func(*testing.T) debugDoer {
			srv := khttp.NewServer()
			adapter.NewKratos(srv).Mount(debugPrefix, debugServer())
			return serveHTTP(srv.ServeHTTP)
		}},
		{"go-zero", func(t *testing.T) debugDoer {
			// go-zero has no in-process handler; drive it over loopback.
			ln, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			port := ln.Addr().(*net.TCPAddr).Port
			ln.Close()
			srv := rest.MustNewServer(rest.RestConf{Host: "127.0.0.1", Port: port})
			t.Cleanup(srv.Stop)
			adapter.NewGoZero(srv).Mount(debugPrefix, debugServer())
			base := fmt.Sprintf("http://127.0.0.1:%d", port)
			go srv.Start()
			deadline := time.Now().Add(3 * time.Second)
			for {
				resp, err := http.Get(base + debugPrefix + "/debug.js")
				if err == nil {
					resp.Body.Close()
					break
				}
				if time.Now().After(deadline) {
					t.Fatalf("go-zero server did not serve within 3s: %v", err)
				}
				time.Sleep(20 * time.Millisecond)
			}
			return func(method, path, contentType, body string) (int, string, string) {
				req, err := http.NewRequest(method, base+path, strings.NewReader(body))
				if err != nil {
					t.Fatal(err)
				}
				if contentType != "" {
					req.Header.Set("Content-Type", contentType)
				}
				resp, err := http.DefaultClient.Do(req)
				if err != nil {
					t.Fatalf("%s %s: %v", method, path, err)
				}
				defer resp.Body.Close()
				b, _ := io.ReadAll(resp.Body)
				return resp.StatusCode, resp.Header.Get("Content-Type"), string(b)
			}
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			do := tc.mount(t)

			if code, ct, _ := do(http.MethodGet, debugPrefix+"/debug.js", "", ""); code != http.StatusOK || !strings.HasPrefix(ct, "text/javascript") {
				t.Errorf("GET /debug.js: want 200 text/javascript, got %d %q", code, ct)
			}

			// No X-Apidoc-Debug header: the doc server's CSRF gate answers
			// 403. A 404 here means the route never reached it.
			if code, _, body := do(http.MethodPost, debugPrefix+"/api/debug/exec", "application/json", "{}"); code != http.StatusForbidden {
				t.Errorf("POST /api/debug/exec without X-Apidoc-Debug: want 403, got %d %q", code, body)
			}

			// P2 routes: reachability only, any non-404 answer counts.
			if code, _, body := do(http.MethodPost, debugPrefix+"/api/debug/open", "application/json", "{}"); code == http.StatusNotFound {
				t.Errorf("POST /api/debug/open: route missing (404) %q", body)
			}
			if code, _, body := do(http.MethodGet, debugPrefix+"/api/debug/events", "", ""); code == http.StatusNotFound {
				t.Errorf("GET /api/debug/events: route missing (404) %q", body)
			}
		})
	}
}
