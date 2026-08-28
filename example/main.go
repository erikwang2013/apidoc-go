// Command example runs five demo servers, one per supported framework:
// net/http on :8081, gin on :8082, echo on :8083, chi on :8084 and
// fiber on :8085. Each serves its own routes plus the apidoc UI mounted
// at /apidoc. Docs come from hand-written registration and from
// parse.ParseDir over the annotated handlers in example/handlers.
//
// Run from the module root:
//
//	go run ./example
//
// then open e.g. http://localhost:8081/apidoc. The doc JSON API, the
// export endpoint and the UI behave identically on all five servers.
package main

import (
	"log"
	"net/http"
	"os"

	"github.com/erikwang2013/apidoc-go"
	"github.com/erikwang2013/apidoc-go/adapter"
	"github.com/erikwang2013/apidoc-go/example/handlers"
	"github.com/erikwang2013/apidoc-go/parse"
	"github.com/gin-gonic/gin"
	"github.com/go-chi/chi/v5"
	"github.com/gofiber/fiber/v2"
	"github.com/labstack/echo/v4"
)

func main() {
	rs := parsedDocs()
	go nethttpDemo(rs)
	go ginDemo(rs)
	go echoDemo(rs)
	go chiDemo(rs)
	go fiberDemo()
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

func must(err error) {
	if err != nil {
		log.Fatal(err)
	}
}
