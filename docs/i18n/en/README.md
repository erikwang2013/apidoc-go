![Apidoc Pet](../../svg/apidoc-pet.svg)

# apidoc-go — Universal API Documentation Plugin for Go

[中文](../../../README.md) · [English](../en/README.md) · [한국어](../ko/README.md) · [Русский](../ru/README.md) · [Deutsch](../de/README.md) · [Français](../fr/README.md) · [Español](../es/README.md) · [Português](../pt/README.md) · [हिन्दी](../hi/README.md) · [العربية](../ar/README.md) · [বাংলা](../bn/README.md) · [Bahasa Indonesia](../id/README.md) · [日本語](../ja/README.md)

## Introduction

**Current version: v1.1.1** (see [Releases](https://github.com/erikwang2013/apidoc-go/releases))

**apidoc-go** is a universal Go API documentation plugin library: interface documentation is declared as **typed structs** alongside route registration, so documentation is born together with the route. Documentation is served through both the legacy `/api/*` and the UIC-1 `/ui/v1/*` JSON APIs, so any external UI can render and make test requests — or a packaged Vue UI can be served in the same process via `Config.UIPath`. Built in: password authentication, multi-app / multi-version management, Mock data, JSON / TypeScript export, comment and nine-format protocol-definition parsing, response caching and automatic param completion; server-side protocol executors compile on demand by build tag and cover the HTTP, RPC, socket, messaging, IoT, industrial and media protocol families. Integrate once, and it works with every framework — no need to modify your existing project.

## Features

| # | Feature | Description |
|---|---------|-------------|
| 1 | Automatic documentation generation | Typed Doc declaration alongside route registration; declare once, and documentation is born together with the route |
| 2 | Documentation API | Legacy `/api/*` (menus / protocols / detail / export) and UIC-1 `/ui/v1/*` (docs, debug, shares, generators, export), for external UIs or the bundled Vue UI |
| 3 | Mock data | Field-level Mock examples, get ahead in interface integration |
| 4 | Multi-app / multi-version | App / Version tree management, one plugin covers documentation for the whole project |
| 5 | Password authentication | Global password + app-level password, HMAC Token · constant-time comparison |
| 6 | Markdown documentation | Safe rendering with goldmark, native HTML automatically stripped |
| 7 | Multi-framework adaptation | net/http · Gin · Echo · Chi · Fiber · beego · Kratos · go-zero, integrate once and it works with every framework |
| 8 | JSON / TypeScript export | One-click export of interface types, smoother frontend-backend integration |
| 9 | Security protection | Password authentication · CORS whitelist · XSS / path-traversal protection · SSRF guard and session/byte limits for server-side debug |
| 10 | Annotation and definition parsing | go/ast parses `@apidoc` / `@protocol` / `@definition`; nine built-in definition formats: OpenAPI, AsyncAPI, GraphQL, proto, WSDL, EDMX, JSON Schema, JSON-RPC and XML-RPC |
| 11 | HTTP caching | Versioned response cache + ETag/304, doc endpoints respond in milliseconds |
| 12 | Automatic param completion | reflect infers request params from the handler signature |
| 13 | Any-protocol annotations | `@protocol` marks the protocol; detail returns protocol/method/url/params/responses/mock plus parsed protocol definitions |
| 14 | Bundled Vue UI (optional) | Point `Config.UIPath` at a UIC-1 packaged build to serve the Vue UI in the same process; an empty value falls back to the JSON API |
| 15 | Server-side protocol executors | Seven protocol families compiled by build tag; off by default and fail closed when not compiled, with `apidoc_debug_all` enabling everything |

## Architecture Overview

![Project Architecture](../../svg/architecture.svg)

![Project Features](../../svg/features.svg)

![Project Lifecycle](../../svg/lifecycle.svg)

## Project Structure

```
apidoc-go/
├── apidoc.go            # Core entry points: New / Register / Mount / Handler
├── VERSION              # Project version (v1.1.1); the release workflow tags from it
├── doc.go               # Route / Doc / Param / Response types
├── model/               # Data models
│   ├── model.go         #   Project / App / Version / Controller / Action
│   ├── definition.go    #   @definition external protocol definitions
│   ├── protocol.go      #   URL scheme → protocol
│   └── protocols.go     #   protocol registry / aliases / default ports
├── store/               # Storage and deduplication
│   └── store.go         #   (app, version, method, url) deduplication, later ones overwrite
├── auth/                # Authentication
│   └── auth.go          #   Password verification · HMAC Token issuance and validation
├── server/              # HTTP API
│   ├── server.go        #   legacy /api routes · auth / CORS / caching
│   ├── ui.go            #   UIPath static Vue UI (SPA fallback · path-confined)
│   └── uiapi/           #   UIC-1 /ui/v1 API
│       ├── uiapi.go     #     capabilities / config / menus / detail
│       ├── read.go      #     document reads · definition parsing
│       ├── debug.go     #     debugSend / debugStart / debugPoll / debugStop
│       ├── manage.go    #     generators · code templates · shares · cache
│       ├── share.go
│       └── debug/       #     server-side protocol executors (build-tag per family)
├── adapter/             # Framework adapter layer (typed interfaces)
│   ├── adapter.go
│   ├── nethttp.go       #   net/http  ServeMux
│   ├── gin.go           #   gin.HandlerFunc
│   ├── echo.go          #   echo.HandlerFunc
│   ├── chi.go           #   http.HandlerFunc
│   └── fiber.go         #   fiber.Handler
├── parse/               # Annotation auto-parser
│   ├── parse.go         #   go/ast · @apidoc / @protocol / @definition
│   └── protocol/        #   nine protocol-definition parsers
├── export/              # Exports
│   └── export.go        #   TypeScript interface definitions
├── mock/                # Mock data
│   └── mock.go          #   Field-level example generation
├── example/             # Sample project (8 frameworks :8081–:8088)
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
	// External UIs read /apidoc/api/menus or UIC-1 /apidoc/ui/v1/*;
	// with UIPath set, open http://localhost:8080/apidoc/ directly
}
```

### Enabling the UIC-1 Bundled Vue UI and Server-Side Executors

```go
s := apidoc.New(apidoc.Config{
	Prefix: "/apidoc",
	// Optional: the output of apidoc-ui's npm run package:ui
	// (index.html / config.js / assets/).
	// Leave empty to serve only the JSON API and keep using an external UI.
	UIPath: "/srv/myapp/dist/uic1-ui",
	// Server-side relay is off by default; when enabled it is still subject
	// to the SSRF whitelist and byte/session limits.
	Debug: apidoc.DebugConfig{EnableServerSide: true},
})
```

Executors are selected at build time by protocol family: `go build -tags apidoc_debug_all` compiles all of them, or trim them with `apidoc_debug_http`, `apidoc_debug_sockets`, `apidoc_debug_rpc`, `apidoc_debug_grpc`, `apidoc_debug_http3`, `apidoc_debug_messaging`, `apidoc_debug_iot`, `apidoc_debug_industrial` and `apidoc_debug_media`. Calls to protocols that were not compiled in always fail closed.

### Any-protocol Annotations

Non-HTTP interfaces reuse `@method` + `@url`, and specify the protocol with `@protocol` (it can also be inferred from the URL scheme):

```go
// @apidoc
// @protocol mqtt
// @method PUBLISH
// @url mqtt://broker:1883/sensors/temp
// @title Publish temperature
// @param qos int false "QoS" in=option default=1
// @param payload json true "Message body" in=payload
func PublishTemp() {}
```

`@protocol` accepts the complete protocol registry and its aliases (HTTP/2, HTTP/3, SOAP, GraphQL, JSON-RPC, XML-RPC, OData, Webhooks, gRPC, Dubbo, Thrift, Hessian, MCP, A2A, ISO 8583, WebSocket, SSE, Socket.IO, WebRTC DataChannel, RTSP, RTMP, HLS, SIP, XMPP, AMQP, MQTT, STOMP, Kafka, CoAP, LwM2M, OPC UA, Modbus TCP, DNP3, IEC 60870-5-104, BACnet/IP, RTP, RTCP, TCP, UDP, Raw Socket, Unix Domain Socket); see `GET /api/protocols` for the full key, alias, scheme and default-port registry. Declare binary bodies with `@param ... type=binary` (or `hex`/`base64`) + `in=payload`.

`/api/detail` and UIC-1 `/ui/v1/docDetail` return `protocol`, `protocol_info`, `method`, `url`, `params`, `responses`, `mock` and the parsed `definitions`. Debugging has two channels: direct from the browser (HTTP / WebSocket / SSE, always available) and server-side relay (with `Debug` enabled plus the matching build tag). The server-side protocol families are HTTP (http / http2 / http3 / SOAP / GraphQL / JSON-RPC / XML-RPC / OData / MCP / A2A / Webhook), RPC (gRPC / Thrift / Dubbo / Hessian / ISO 8583), sockets (TCP / UDP / Raw / Unix), messaging (MQTT / STOMP / AMQP / Kafka / XMPP), IoT (CoAP / LwM2M), industrial (Modbus / DNP3 / IEC 104 / BACnet / OPC UA) and media (RTSP / SIP / RTP / RTCP / RTMP). Default builds compile no executors; when enabled, targets must first pass the SSRF guard (`AllowHosts` whitelist, private/loopback/link-local/cloud-metadata addresses denied by default, per-IP validation after DNS resolution with the connection pinned), and are bounded by timeouts, a 1 MiB per-message limit, the response `MaxBytes` and stream-session limits.

### Protocol Definition Parsing (@definition)

Use `@definition` to pull in external protocol definitions (the format is auto-detected, or set it with `format=`); the parsed result is returned with detail so the UIC can render message structures and build debug forms:

```go
// @apidoc
// @protocol http
// @method POST
// @url /api/v1/users
// @definition file=specs/openapi.yaml operation=createUser
// @definition file=specs/service.proto message=User
```

Supported formats: OpenAPI, AsyncAPI, GraphQL SDL, proto, WSDL, EDMX (OData), JSON Schema, JSON-RPC and XML-RPC. `Config.DefinitionRoot` confines readable files to a root directory (the process working directory by default); parsing is limited to 1 MiB and 50,000 lines per file and a nested-reference depth of 32; see the `parse` package comments for options such as `operation=`, `message=`, `direction=`, `sample=` and `inline=`.

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

For Echo / Chi / Fiber, just replace the adapter constructor: `adapter.NewEcho(e)`, `adapter.NewChi(mux)`, `adapter.NewFiber(app)` — the rest of the code stays exactly the same. For beego / Kratos / go-zero, see the Framework Adapters table below.

### Configuration

| Config | Default | Description |
|--------|---------|-------------|
| `Prefix` | `/apidoc` | Documentation mount path |
| `Title` | `API Docs` | Documentation title |
| `Desc` | — | Documentation description |
| `GlobalParams` | — | Global parameters, merged into every Action |
| `Auth` | — | Auth config: `Enable` / `Password` / `Secret` / `Expire` / `Secure` |
| `Apps` | — | App config: `Key` / `Title` / `Password` (independent app-level password) |
| `DebugOrigins` | — | Origins allowed to read the documentation API cross-origin (external UI, none by default) |
| `PublicHost` | — | Absolute base URL used by external UIs for debugging; empty means same-origin |
| `ShareDir` | `apidoc-go-shares` under the temp directory | UIC-1 share-record directory |
| `DefinitionRoot` | process working directory | `@definition` file root; out-of-root paths and symlinks are rejected |
| `Generator` / `CodeTemplates` | — | Declarative generators and code templates (rendering is side-effect free) |
| `ShareActions` | — | Share-action declarations interpreted by the UI |
| `Debug` | zero value = off | Server-side executors: `EnableServerSide` / `AllowPrivateNetworks` / `AllowHosts` / `AllowUnixSockets` / `TimeoutMS` / `MaxBytes` |
| `UIPath` | — (no bundled UI) | Directory of a packaged UIC-1 Vue build; when non-empty, serves the static UI in the same process under `{Prefix}/` |

### UIC-1 API and Bundled Vue UI (Optional)

The library serves two JSON documentation APIs by default: legacy `/api/*` and UIC-1 `/ui/v1/*`, consumed by an external UI. `GET {Prefix}/ui/v1/capabilities` returns the contract, backend version and feature matrix (browser/server debug modes, shares, cache, generators, export, auth); the UI uses it to decide which panels are available. `/ui/v1/config` returns the menus plus generator and code-template configuration. After handing a packaged `apidoc-ui` build to `Config.UIPath`, the same process also serves the Vue UI directly under `{Prefix}/`:

```go
s := apidoc.New(apidoc.Config{
	Prefix: "/apidoc",
	UIPath: "/srv/myapp/dist/uic1-ui", // output directory of scripts/package-ui.sh
})
```

The library itself does not embed frontend assets, so there is no build-time dependency; a bundled-UI release package should point `UIPath` at the shipped `dist/uic1-ui/` directory by default — the wiring above is the recommended setup. Deployment steps:

1. Build and package in the UI repository: `npm run package:ui` (output `dist/uic1-ui/`).
2. Deploy to the target directory (it must exist and be empty, or hold the previous uic1-ui package): `npm run package:ui -- --dest /srv/myapp/dist/uic1-ui`.
3. Set `UIPath` at startup; leave it empty and restart to roll back to documentation-API-only mode.

Behavior:

- `{Prefix}/`, `{Prefix}/index.html`, `{Prefix}/config.js` and `{Prefix}/assets/*` are served as static files, with MIME type determined by extension;
- unknown extension-less paths fall back to `index.html` (SPA fallback); missing `.js` / `.css` and similar assets return 404;
- `{Prefix}` (without a trailing slash) redirects with 307 to `{Prefix}/` so relative asset paths stay correct;
- `{Prefix}/api/*` and `{Prefix}/ui/v1/*` take precedence; unknown API paths return 404 and never fall back to the UI;
- file access is confined to `UIPath` (`..` and out-of-root symlinks are rejected), with no directory listing; if the directory or `index.html` is missing the UI route returns 500 and the documentation API is unaffected;
- the go-zero adapter only registers the fixed API routes and does not forward arbitrary paths under the prefix, so the bundled UI is unavailable with that adapter (all other adapters support it);
- deployments that still use an external UI need no changes; just keep `HTTP.API_PREFIX` in `config.js` equal to the server `Prefix`.

### Server-Side Protocol Executors (Build Tags)

Server-side relay compiles no executors by default, and calling an uncompiled protocol returns a fail-closed error. Select build tags by protocol family; `apidoc_debug_all` aggregates all of them:

| Build tag | Protocol family | Protocols |
|-----------|-----------------|-----------|
| `apidoc_debug_http` | HTTP / Web | http, http2, soap, graphql, jsonrpc, xmlrpc, odata, mcp, a2a, webhook |
| `apidoc_debug_http3` | HTTP/3 | http3 (QUIC, https only) |
| `apidoc_debug_sockets` | Sockets | tcp, udp, raw, unix (unary + streaming) |
| `apidoc_debug_rpc` | RPC / finance | thrift, dubbo, hessian, iso8583 |
| `apidoc_debug_grpc` | gRPC | grpc (unary; descriptor or server reflection) |
| `apidoc_debug_messaging` | Messaging | mqtt, stomp, amqp, kafka, xmpp (unary + streaming; also selectable individually with `apidoc_debug_mqtt` / `_stomp` / `_amqp` / `_kafka` / `_xmpp`) |
| `apidoc_debug_iot` | IoT | coap, lwm2m |
| `apidoc_debug_industrial` | Industrial | modbus, dnp3, iec104, bacnet, opcua |
| `apidoc_debug_media` | Media | rtsp, sip, rtp, rtcp, rtmp |
| `apidoc_debug_all` | All | every family above |

Security boundary: `Debug.EnableServerSide` is off by default; when enabled, targets must still pass the `AllowHosts` whitelist (suffix matching such as `*.example.com`), private/loopback/link-local/CGNAT and cloud-metadata addresses are denied by default (only `AllowPrivateNetworks` permits them), per-IP validation happens after DNS resolution with the connection pinned, and `CONNECT`, redirects and proxies are forbidden; requests and responses are bounded by `TimeoutMS`, `MaxBytes` (1 MiB by default), stream sessions (4 per scope), frame counts and idle timeouts. Definition parsing only reads regular files inside `DefinitionRoot`, bounded by file size, line count and reference depth.

### Framework Adapters

| Framework | Constructor |
|-----------|-------------|
| net/http | `adapter.NewNetHTTP(mux)` |
| Gin | `adapter.NewGin(engine)` |
| Echo | `adapter.NewEcho(e)` |
| Chi | `adapter.NewChi(mux)` |
| Fiber | `adapter.NewFiber(app)` |
| beego | `adapter.NewBeego(app)` |
| Kratos | `adapter.NewKratos(srv)` |
| go-zero | `adapter.NewGoZero(srv)` |

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

### HTTP Caching (Response Cache + ETag)

Responses for `/api/menus`, `/api/protocols`, and `/api/export` are serialized once per document version and cached in-process. The cache is warmed when `Handler` is created and invalidated automatically by `Register` / `SetAppMeta`. Responses still carry `ETag` + `Cache-Control: private, max-age=300`, so repeat visits hit 304. No configuration needed.

### Export

| Format | Endpoint | Description |
|--------|----------|-------------|
| JSON | `GET /apidoc/api/export` | Full project tree |
| TypeScript | `GET /apidoc/api/export?format=typescript` | Interface type definitions |

### Mock Data

The detail page shows a Mock example automatically: customize with `Doc.Params[].Mock`, or let it be generated from the field type (string→"sample", int→0, bool→true, ...).

### Example Project

`example/` ships 8 framework servers (net/http :8081, Gin :8082, Echo :8083, Chi :8084, Fiber :8085, beego :8086, Kratos :8087, go-zero :8088). Start them all with `go run ./example`; the legacy doc API is served at `/apidoc/api/...` on each port, the UIC-1 API at `/apidoc/ui/v1/...`, and either an external UI or the bundled Vue UI via `UIPath` can consume them.

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
