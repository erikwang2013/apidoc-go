# apidoc-go — Universal API Documentation Plugin for Go

[中文](../../../README.md) · [English](../en/README.md) · [한국어](../ko/README.md) · [Русский](../ru/README.md) · [Deutsch](../de/README.md) · [Français](../fr/README.md) · [Español](../es/README.md) · [Português](../pt/README.md) · [हिन्दी](../hi/README.md) · [العربية](../ar/README.md) · [বাংলা](../bn/README.md) · [Bahasa Indonesia](../id/README.md) · [日本語](../ja/README.md)

## Introduction

**apidoc-go** is a universal Go API documentation plugin library: interface documentation is declared as **typed structs** alongside route registration, so documentation is born together with the route. The embedded Web UI provides online browsing and online debugging, with built-in password authentication, multi-app / multi-version management, Mock data, and JSON / TypeScript export, annotation auto-parsing, HTTP caching, and automatic param completion. Integrate once, and it works with every framework — no need to modify your existing project.

## Features

| # | Feature | Description |
|---|---------|-------------|
| 1 | Automatic documentation generation | Typed Doc declaration alongside route registration; declare once, and documentation is born together with the route |
| 2 | Online debugging | Request real endpoints directly from the browser, no server-side forwarding, naturally SSRF-free |
| 3 | Mock data | Field-level Mock examples, get ahead in interface integration |
| 4 | Multi-app / multi-version | App / Version tree management, one plugin covers documentation for the whole project |
| 5 | Password authentication | Global password + app-level password, HMAC Token · constant-time comparison |
| 6 | Markdown documentation | Safe rendering with goldmark, native HTML automatically stripped |
| 7 | Multi-framework adaptation | net/http · Gin · Echo · Chi · Fiber, integrate once and it works with every framework |
| 8 | JSON / TypeScript export | One-click export of interface types, smoother frontend-backend integration |
| 9 | Security protection | No SSRF · CORS whitelist restriction · XSS protection · path traversal protection |
| 10 | Annotation auto-parsing | go/ast generates docs from comments; the `@apidoc` marker is all it takes |
| 11 | HTTP caching | ETag + 304, doc responses open instantly |
| 12 | Automatic param completion | reflect infers request params from the handler signature |

## Architecture Overview

![Project Architecture](../../svg/architecture.svg)

![Project Features](../../svg/features.svg)

![Project Lifecycle](../../svg/lifecycle.svg)

## Project Structure

```
apidoc-go/
├── apidoc.go            # Core entry points: New / Register / Mount / Handler
├── doc.go               # Route / Doc / Param / Response types
├── model/               # Data models
│   └── model.go         #   Project / App / Version / Controller / Action
├── store/               # Storage and deduplication
│   └── store.go         #   (app, version, method, url) deduplication, later ones overwrite
├── auth/                # Authentication
│   └── auth.go          #   Password verification · HMAC Token issuance and validation
├── server/              # JSON API + embedded Web UI
│   ├── server.go
│   └── static/
│       └── index.html
├── adapter/             # Framework adapter layer (typed interfaces)
│   ├── adapter.go
│   ├── nethttp.go       #   net/http  ServeMux
│   ├── gin.go           #   gin.HandlerFunc
│   ├── echo.go          #   echo.HandlerFunc
│   ├── chi.go           #   http.HandlerFunc
│   └── fiber.go         #   fiber.Handler
├── parse/               # Annotation auto-parser
│   └── parse.go         #   go/ast · @apidoc comment markers
├── export/              # Exports
│   └── export.go        #   TypeScript interface definitions
├── mock/                # Mock data
│   └── mock.go          #   Field-level example generation
├── example/             # Sample project (5 frameworks :8081–:8085)
│   ├── main.go
│   └── handlers/        #   @apidoc comment examples
└── docs/                # Documentation and assets
    ├── alipay.png
    ├── weixinpay.png
    └── svg/             # Architecture diagram / features diagram / lifecycle diagram
```

## Usage

### Installation

```bash
go get github.com/erikwang2013/apidoc-go
```

### Quick Start (net/http)

```go
package main

import (
	"net/http"

	"github.com/erikwang2013/apidoc-go"
	"github.com/erikwang2013/apidoc-go/adapter"
)

func main() {
	mux := http.NewServeMux()
	s := apidoc.New(apidoc.Config{Title: "My API Docs"})

	// Declare the documentation together with the route
	if err := s.Register(apidoc.Route{
		Method: "GET",
		URL:    "/hello",
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte("hello"))
		}),
		Doc: apidoc.Doc{
			App:     "demo",
			Version: "v1",
			Action:  "hello",
			Title:   "Greeting",
			Params: []apidoc.Param{
				{Name: "name", Type: "string", Required: true, Desc: "Your name"},
			},
		},
	}); err != nil {
		panic(err)
	}

	s.Mount(adapter.NewNetHTTP(mux))
	http.ListenAndServe(":8080", mux)
	// Visit http://localhost:8080/apidoc in your browser
}
```

### Quick Start (Gin)

```go
r := gin.Default()
s := apidoc.New(apidoc.Config{Title: "My API Docs"})

s.Register(apidoc.Route{
	Method:  "GET",
	URL:     "/hello",
	Handler: func(c *gin.Context) { c.String(200, "hello") },
	Doc:     apidoc.Doc{App: "demo", Version: "v1", Action: "hello", Title: "Greeting"},
})

s.Mount(adapter.NewGin(r))
r.Run(":8080")
```

For Echo / Chi / Fiber, just replace the adapter constructor: `adapter.NewEcho(e)`, `adapter.NewChi(mux)`, `adapter.NewFiber(app)` — the rest of the code stays exactly the same.

### Configuration

| Config | Default | Description |
|--------|---------|-------------|
| `Prefix` | `/apidoc` | Documentation mount path |
| `Title` | `API Docs` | Documentation title |
| `Desc` | — | Documentation description |
| `GlobalParams` | — | Global parameters, merged into every Action |
| `Auth` | — | Auth config: `Enable` / `Password` / `Secret` / `Expire` / `Secure` |
| `Apps` | — | App config: `Key` / `Title` / `Password` (independent app-level password) |
| `DebugOrigins` | — | Allowed cross-origin debugging sources (none by default) |

### Framework Adapters

| Framework | Constructor |
|-----------|-------------|
| net/http | `adapter.NewNetHTTP(mux)` |
| Gin | `adapter.NewGin(engine)` |
| Echo | `adapter.NewEcho(e)` |
| Chi | `adapter.NewChi(mux)` |
| Fiber | `adapter.NewFiber(app)` |

### Annotation Auto-parsing (go/ast)

Write `@apidoc` comments above a handler, then register the parse results:

```go
// @apidoc
// @method POST
// @url /api/v1/users
// @title Create user
// @param name string true "Username"
// @success ok User "Success"
func CreateUser(c *gin.Context, req *CreateUserReq) { ... }
```

```go
results, err := parse.ParseDir("./handlers")
if err != nil { log.Fatal(err) }
for _, r := range results {
    s.Register(apidoc.Route{Method: r.Method, URL: r.URL, Handler: CreateUser, Doc: r.Doc})
}
```

### Automatic Param Completion (reflect)

When `Doc.Params` is empty, Register infers them with reflect from the handler signature: struct args expand into body fields (following json tags), framework contexts (gin.Context / echo.Context / fiber.Ctx) are skipped automatically.

### HTTP Caching (ETag)

All doc endpoints carry `ETag` + `Cache-Control: private, max-age=300` automatically; repeat visits hit 304. No configuration needed.

### Export

| Format | Endpoint | Description |
|--------|----------|-------------|
| JSON | `GET /apidoc/api/export` | Full project tree |
| TypeScript | `GET /apidoc/api/export?format=typescript` | Interface type definitions |

### Mock Data

The detail page shows a Mock example automatically: customize with `Doc.Params[].Mock`, or let it be generated from the field type (string→"sample", int→0, bool→true, ...).

### Example Project

`example/` ships 5 framework servers (net/http :8081, Gin :8082, Echo :8083, Chi :8084, Fiber :8085). Start them all with `go run ./example`.

## Multilingual Documentation

| Language | Link |
|----------|------|
| English | [English](../en/README.md) |
| 한국어 | [한국어](../ko/README.md) |
| Русский | [Русский](../ru/README.md) |
| Deutsch | [Deutsch](../de/README.md) |
| Français | [Français](../fr/README.md) |
| Español | [Español](../es/README.md) |
| Português | [Português](../pt/README.md) |
| हिन्दी | [हिन्दी](../hi/README.md) |
| العربية | [العربية](../ar/README.md) |
| বাংলা | [বাংলা](../bn/README.md) |
| Bahasa Indonesia | [Bahasa Indonesia](../id/README.md) |
| 日本語 | [日本語](../ja/README.md) |

## Support Us

If this project has been helpful to you, feel free to scan the QR codes to tip us — it keeps us motivated to maintain and update!

<div align="center">
  <img src="../../weixinpay.png" width="130" height="130" alt="WeChat Pay donation" />
  <img src="../../alipay.png" width="130" height="130" alt="Alipay donation" />
  <p>WeChat Pay (left) · Alipay (right)</p>
</div>

### Global Bank Transfer Donations

**【Beneficiary Information】**
Beneficiary Name: WANG KEXUN
Account Number: 881015918251

**【Beneficiary Bank】**
ZA Bank
SWIFT Code: AABLHKHHXXX
Bank Name: ZA Bank Limited
Bank Code: 387
Bank Address: Core F, Cyberport 3, 100 Cyberport Road, Hong Kong

**【Cross-border Remittance Correspondent Bank (if required)】**

Please note that this is the cross-border remittance correspondent (intermediary) bank information, not the beneficiary bank information. Please check with your remitting bank whether the correspondent bank information is required.

The correspondent bank for HKD, CNY and USD remittances is Citibank —

Bank Name: Citibank N.A. Hong Kong
SWIFT Code: CITIHKHXXXX
Bank Code: 006
Branch Name: Hong Kong Branch
Branch Code: 391
Bank Address: Citibank Tower, Citibank Plaza, 3 Garden Road, Central, Hong Kong

The correspondent bank for remittances in other currencies is BNY Mellon —

Bank Name: THE BANK OF NEW YORK MELLON
SWIFT Code: IRVTUS3NXXX
Bank Address: THE BANK OF NEW YORK MELLON, 240 GREENWICH STREET, NEW YORK, United States

---

<div align="center">Made with ❤️ by <a href="https://erik.xyz">erik.xyz</a></div>
