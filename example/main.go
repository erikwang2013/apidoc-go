// Command example runs eight demo servers, one per supported framework:
// net/http on :8081, gin on :8082, echo on :8083, chi on :8084,
// fiber on :8085, beego on :8086, kratos on :8087 and go-zero on :8088.
// Each serves its own routes plus the apidoc UI mounted at /apidoc. Docs
// come from hand-written registration and from parse.ParseDir over the
// annotated handlers in example/handlers.
//
// Run from the module root:
//
//	go run ./example
//
// then open e.g. http://localhost:8081/apidoc. The doc JSON API, the
// export endpoint and the UI behave identically on all eight servers.
package main

import (
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/beego/beego/v2/server/web"
	"github.com/erikwang2013/apidoc-go"
	"github.com/erikwang2013/apidoc-go/adapter"
	"github.com/erikwang2013/apidoc-go/example/handlers"
	"github.com/erikwang2013/apidoc-go/parse"
	"github.com/gin-gonic/gin"
	"github.com/go-chi/chi/v5"
	"github.com/go-kratos/kratos/v2"
	khttp "github.com/go-kratos/kratos/v2/transport/http"
	"github.com/gofiber/fiber/v2"
	"github.com/labstack/echo/v4"
	"github.com/zeromicro/go-zero/rest"
)

func main() {
	rs := parsedDocs()
	go nethttpDemo(rs)
	go ginDemo(rs)
	go echoDemo(rs)
	go chiDemo(rs)
	go fiberDemo()
	go beegoDemo(rs)
	go kratosDemo(rs)
	go gozeroDemo(rs)
	select {}
}

// parsedDocs parses the annotated handlers; the directory resolves
// whether the process runs from the module root or from example/.
func parsedDocs() []parse.Result {
	dir := "example/handlers"
	if _, err := os.Stat(dir); err != nil {
		dir = "handlers"
	}
	rs, err := parse.ParseDir(dir)
	if err != nil {
		log.Fatal(err)
	}
	return rs
}

// parsedFns backs the parsed docs with concrete handlers, in the same
// order parse.ParseDir reports them (one file, declaration order).
var parsedFns = []http.HandlerFunc{handlers.ListUsers, handlers.GetUser, handlers.CreateUser}

func nethttpDemo(rs []parse.Result) {
	mux := http.NewServeMux()
	s := apidoc.New(apidoc.Config{Prefix: "/apidoc", Title: "net/http demo"})
	must(s.Register(apidoc.Route{Method: "GET", URL: "/api/health",
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("ok")) }),
		Doc: apidoc.Doc{Title: "健康检查", Author: "demo",
			Params:    []apidoc.Param{{Name: "verbose", In: "query", Type: "bool", Desc: "详细输出"}},
			Responses: []apidoc.Response{{Name: "ok", Type: "string", Desc: "状态"}},
		}}))
	for i, r := range rs {
		must(s.Register(apidoc.Route{Method: r.Method, URL: r.URL, Handler: parsedFns[i], Doc: r.Doc}))
	}
	must(s.Mount(adapter.NewNetHTTP(mux)))
	log.Fatal(http.ListenAndServe(":8081", mux))
}

func ginDemo(rs []parse.Result) {
	gin.SetMode(gin.ReleaseMode)
	e := gin.New()
	s := apidoc.New(apidoc.Config{Prefix: "/apidoc", Title: "gin demo"})
	must(s.Register(apidoc.Route{Method: "GET", URL: "/api/ping",
		Handler: gin.HandlerFunc(func(c *gin.Context) { c.String(http.StatusOK, "pong") }),
		Doc:     apidoc.Doc{Title: "Ping", Author: "demo", Responses: []apidoc.Response{{Name: "ok", Type: "string"}}},
	}))
	for i, r := range rs {
		must(s.Register(apidoc.Route{Method: r.Method, URL: r.URL, Handler: gin.WrapH(parsedFns[i]), Doc: r.Doc}))
	}
	must(s.Mount(adapter.NewGin(e)))
	log.Fatal(e.Run(":8082"))
}

func echoDemo(rs []parse.Result) {
	e := echo.New()
	s := apidoc.New(apidoc.Config{Prefix: "/apidoc", Title: "echo demo"})
	must(s.Register(apidoc.Route{Method: "GET", URL: "/api/hello",
		Handler: echo.HandlerFunc(func(c echo.Context) error { return c.String(http.StatusOK, "hello") }),
		Doc:     apidoc.Doc{Title: "Hello", Author: "demo", Responses: []apidoc.Response{{Name: "ok", Type: "string"}}},
	}))
	for i, r := range rs {
		must(s.Register(apidoc.Route{Method: r.Method, URL: r.URL, Handler: echo.WrapHandler(parsedFns[i]), Doc: r.Doc}))
	}
	must(s.Mount(adapter.NewEcho(e)))
	log.Fatal(e.Start(":8083"))
}

func chiDemo(rs []parse.Result) {
	mux := chi.NewMux()
	s := apidoc.New(apidoc.Config{Prefix: "/apidoc", Title: "chi demo"})
	must(s.Register(apidoc.Route{Method: "GET", URL: "/api/time",
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("now")) }),
		Doc:     apidoc.Doc{Title: "Time", Author: "demo", Responses: []apidoc.Response{{Name: "ok", Type: "string"}}},
	}))
	for i, r := range rs {
		must(s.Register(apidoc.Route{Method: r.Method, URL: r.URL, Handler: parsedFns[i], Doc: r.Doc}))
	}
	must(s.Mount(adapter.NewChi(mux)))
	log.Fatal(http.ListenAndServe(":8084", mux))
}

func fiberDemo() {
	app := fiber.New()
	s := apidoc.New(apidoc.Config{Prefix: "/apidoc", Title: "fiber demo"})
	must(s.Register(apidoc.Route{Method: "GET", URL: "/api/version",
		Handler: func(c *fiber.Ctx) error { return c.SendString("1.0.0") },
		Doc:     apidoc.Doc{Title: "Version", Author: "demo", Responses: []apidoc.Response{{Name: "ok", Type: "string"}}},
	}))
	must(s.Register(apidoc.Route{Method: "GET", URL: "/api/health",
		Handler: func(c *fiber.Ctx) error { return c.SendString("ok") },
		Doc:     apidoc.Doc{Title: "健康检查", Author: "demo", Responses: []apidoc.Response{{Name: "ok", Type: "string"}}},
	}))
	must(s.Mount(adapter.NewFiber(app)))
	log.Fatal(app.Listen(":8085"))
}

func beegoDemo(rs []parse.Result) {
	app := web.NewHttpSever()
	s := apidoc.New(apidoc.Config{Prefix: "/apidoc", Title: "beego demo"})
	must(s.Register(apidoc.Route{Method: "GET", URL: "/api/health",
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("ok")) }),
		Doc: apidoc.Doc{Title: "健康检查", Author: "demo",
			Params:    []apidoc.Param{{Name: "verbose", In: "query", Type: "bool", Desc: "详细输出"}},
			Responses: []apidoc.Response{{Name: "ok", Type: "string", Desc: "状态"}},
		}}))
	must(s.Register(apidoc.Route{Method: "GET", URL: "/api/version",
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("1.0.0")) }),
		Doc:     apidoc.Doc{Title: "Version", Author: "demo", Responses: []apidoc.Response{{Name: "ok", Type: "string"}}},
	}))
	// beego holds one handler per path (see adapter.Beego), so a second
	// registration for the same URL would shadow the first with 405s. The
	// parsed handlers register GET and POST on /api/users, so only the
	// first entry per URL is registered — and only it is documented.
	seen := map[string]bool{}
	for i, r := range rs {
		if seen[r.URL] {
			continue
		}
		seen[r.URL] = true
		must(s.Register(apidoc.Route{Method: r.Method, URL: r.URL, Handler: parsedFns[i], Doc: r.Doc}))
	}
	must(s.Mount(adapter.NewBeego(app)))
	app.Run(":8086") // blocks; beego reports startup failures itself
}

func kratosDemo(rs []parse.Result) {
	srv := khttp.NewServer(khttp.Address(":8087"))
	s := apidoc.New(apidoc.Config{Prefix: "/apidoc", Title: "kratos demo"})
	must(s.Register(apidoc.Route{Method: "GET", URL: "/api/health",
		Handler: khttp.HandlerFunc(func(ctx khttp.Context) error { return ctx.String(http.StatusOK, "ok") }),
		Doc: apidoc.Doc{Title: "健康检查", Author: "demo",
			Params:    []apidoc.Param{{Name: "verbose", In: "query", Type: "bool", Desc: "详细输出"}},
			Responses: []apidoc.Response{{Name: "ok", Type: "string", Desc: "状态"}},
		}}))
	must(s.Register(apidoc.Route{Method: "GET", URL: "/api/version",
		Handler: khttp.HandlerFunc(func(ctx khttp.Context) error { return ctx.String(http.StatusOK, "1.0.0") }),
		Doc:     apidoc.Doc{Title: "Version", Author: "demo", Responses: []apidoc.Response{{Name: "ok", Type: "string"}}},
	}))
	for i, r := range rs {
		must(s.Register(apidoc.Route{Method: r.Method, URL: kratosPath(r.URL), Handler: kratosHTTP(parsedFns[i]), Doc: r.Doc}))
	}
	must(s.Mount(adapter.NewKratos(srv)))
	app := kratos.New(kratos.Name("example-kratos"), kratos.Server(srv))
	log.Fatal(app.Run()) // Run returns nil on shutdown, taking the example down
}

// kratosHTTP adapts a plain http.HandlerFunc to kratos's handler type:
// the parsed handlers write to kratos's own ResponseWriter.
func kratosHTTP(h http.HandlerFunc) khttp.HandlerFunc {
	return func(ctx khttp.Context) error {
		h(ctx.Response(), ctx.Request())
		return nil
	}
}

// kratosPath rewrites a ":param" path to the "{param}" syntax kratos's
// gorilla router matches on. The doc is registered under the same path,
// so the kratos page shows "{id}" where the other demos show ":id".
func kratosPath(url string) string {
	parts := strings.Split(url, "/")
	for i, s := range parts {
		if strings.HasPrefix(s, ":") {
			parts[i] = "{" + s[1:] + "}"
		}
	}
	return strings.Join(parts, "/")
}

func gozeroDemo(rs []parse.Result) {
	srv := rest.MustNewServer(rest.RestConf{Host: "0.0.0.0", Port: 8088})
	s := apidoc.New(apidoc.Config{Prefix: "/apidoc", Title: "go-zero demo"})
	must(s.Register(apidoc.Route{Method: "GET", URL: "/api/health",
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("ok")) }),
		Doc: apidoc.Doc{Title: "健康检查", Author: "demo",
			Params:    []apidoc.Param{{Name: "verbose", In: "query", Type: "bool", Desc: "详细输出"}},
			Responses: []apidoc.Response{{Name: "ok", Type: "string", Desc: "状态"}},
		}}))
	must(s.Register(apidoc.Route{Method: "GET", URL: "/api/version",
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("1.0.0")) }),
		Doc:     apidoc.Doc{Title: "Version", Author: "demo", Responses: []apidoc.Response{{Name: "ok", Type: "string"}}},
	}))
	for i, r := range rs {
		must(s.Register(apidoc.Route{Method: r.Method, URL: r.URL, Handler: parsedFns[i], Doc: r.Doc}))
	}
	must(s.Mount(adapter.NewGoZero(srv)))
	srv.Start() // blocks; go-zero logs its own startup failures
}

func must(err error) {
	if err != nil {
		log.Fatal(err)
	}
}
