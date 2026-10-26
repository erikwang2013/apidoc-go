![Apidoc 宠物](docs/svg/apidoc-pet.svg)

# apidoc-go — Go 通用 API 文档插件

[![Release](https://img.shields.io/github/v/release/erikwang2013/apidoc-go)](https://github.com/erikwang2013/apidoc-go/releases)

[English](docs/i18n/en/README.md) · [한국어](docs/i18n/ko/README.md) · [Русский](docs/i18n/ru/README.md) · [Deutsch](docs/i18n/de/README.md) · [Français](docs/i18n/fr/README.md) · [Español](docs/i18n/es/README.md) · [Português](docs/i18n/pt/README.md) · [हिन्दी](docs/i18n/hi/README.md) · [العربية](docs/i18n/ar/README.md) · [বাংলা](docs/i18n/bn/README.md) · [Bahasa Indonesia](docs/i18n/id/README.md) · [日本語](docs/i18n/ja/README.md)

## 项目介绍

**当前版本：v1.1.1**（以 [Releases](https://github.com/erikwang2013/apidoc-go/releases) 为准）

**apidoc-go** 是一个通用 Go API 文档插件库：接口文档以**类型化结构体**随路由注册时一同声明，文档与路由同生；同时提供旧版 `/api/*` 与 UIC-1 `/ui/v1/*` 两套 JSON API，可用任意外部 UI 渲染调试，也可通过 `Config.UIPath` 在同一进程提供打包后的 Vue UI。库内置密码鉴权、多应用/多版本管理、Mock 数据、JSON / TypeScript 导出、注释与九种协议定义解析、接口缓存与参数自动补全；服务端协议执行器按构建标签按需编译，覆盖 HTTP、RPC、套接字、消息、IoT、工业与媒体协议族。一次接入，全框架通用，无需改造现有项目。

## 项目功能

| # | 功能 | 说明 |
|---|------|------|
| 1 | 文档自动生成 | 类型化 Doc 声明，随路由注册，一处声明，文档与路由同生 |
| 2 | 文档 API | 旧版 `/api/*`（menus / protocols / detail / export）与 UIC-1 `/ui/v1/*`（文档、调试、分享、生成器、导出），供外部 UI 或内置 Vue UI 使用 |
| 3 | Mock 数据 | 字段级 Mock 示例，接口联调快人一步 |
| 4 | 多应用 / 多版本 | App / Version 树形管理，一套插件覆盖全项目文档 |
| 5 | 密码鉴权 | 全局密码 + 应用级密码，HMAC Token · 恒时比较 |
| 6 | Markdown 文档 | goldmark 安全渲染，原生 HTML 自动剥离 |
| 7 | 多框架适配 | net/http · Gin · Echo · Chi · Fiber · beego · Kratos · go-zero，一次接入全框架通用 |
| 8 | JSON / TypeScript 导出 | 接口类型一键导出，前后端联调更顺畅 |
| 9 | 安全防护 | 密码鉴权 · CORS 白名单 · 防 XSS / 路径穿越 · 服务端调试 SSRF 守卫 · 会话与字节限额 |
| 10 | 注解与定义解析 | go/ast 解析 `@apidoc` / `@protocol` / `@definition`；内置 OpenAPI、AsyncAPI、GraphQL、proto、WSDL、EDMX、JSON Schema、JSON-RPC、XML-RPC 九种定义格式 |
| 11 | 接口缓存 | 版本化响应缓存 + ETag/304，文档接口毫秒级返回 |
| 12 | 参数自动补全 | reflect 从 handler 签名自动推断请求参数 |
| 13 | 任意协议注解 | `@protocol` 标注协议；detail 返回 protocol/method/url/params/responses/mock 与解析后的协议定义 |
| 14 | 内置 Vue UI（可选） | `Config.UIPath` 指向 UIC-1 打包产物即可同进程提供 Vue UI；留空时回退纯 JSON API |
| 15 | 服务端协议执行器 | 7 大协议族按构建标签编译；默认关闭、未编译即 fail closed，`apidoc_debug_all` 一键全开 |

## 架构总览

![项目架构](docs/svg/architecture.svg)

![项目功能](docs/svg/features.svg)

![项目生命周期](docs/svg/lifecycle.svg)

## 项目结构

```
apidoc-go/
├── apidoc.go            # 核心入口：New / Register / Mount / Handler
├── VERSION              # 项目版本（v1.1.1），发布工作流据此建 tag
├── doc.go               # Route / Doc / Param / Response 类型
├── model/               # 数据模型
│   ├── model.go         #   Project / App / Version / Controller / Action
│   ├── definition.go    #   @definition 外部协议定义
│   ├── protocol.go      #   URL scheme → protocol
│   └── protocols.go     #   协议注册表 / 别名 / 默认端口
├── store/               # 存储与去重
│   └── store.go         #   (app, version, method, url) 去重，后者覆盖
├── auth/                # 鉴权
│   └── auth.go          #   密码校验 · HMAC Token 签发与验证
├── server/              # HTTP API
│   ├── server.go        #   旧版 /api 路由 · 鉴权 / CORS / 缓存
│   ├── ui.go            #   UIPath 静态 Vue UI（SPA 兜底 · 路径限定）
│   └── uiapi/           #   UIC-1 /ui/v1 API
│       ├── uiapi.go     #     capabilities / config / menus / detail
│       ├── read.go      #     文档读取 · 定义解析
│       ├── debug.go     #     debugSend / debugStart / debugPoll / debugStop
│       ├── manage.go    #     生成器 · 代码模板 · 分享 · 缓存
│       ├── share.go
│       └── debug/       #     服务端协议执行器（构建标签按协议族编译）
├── adapter/             # 框架适配层（类型化接口）
│   ├── adapter.go
│   ├── nethttp.go       #   net/http  ServeMux
│   ├── gin.go           #   gin.HandlerFunc
│   ├── echo.go          #   echo.HandlerFunc
│   ├── chi.go           #   http.HandlerFunc
│   └── fiber.go         #   fiber.Handler
├── parse/               # 注释自动解析
│   ├── parse.go         #   go/ast · @apidoc / @protocol / @definition
│   └── protocol/        #   九种协议定义解析
├── export/              # 导出
│   └── export.go        #   TypeScript 接口定义
├── mock/                # Mock 数据
│   └── mock.go          #   字段级示例值生成
├── example/             # 示例项目（8 框架 :8081–:8088）
│   ├── main.go
│   └── handlers/        #   @apidoc 注释示例
└── docs/                # 文档与素材
    ├── alipay.png
    ├── weixinpay.png
    └── svg/             # 架构图 / 功能图 / 生命周期图
```

## 使用说明

### 安装

```bash
go get github.com/erikwang2013/apidoc-go
```

### 快速开始（net/http）

```go
package main

import (
	"net/http"

	"github.com/erikwang2013/apidoc-go"
	"github.com/erikwang2013/apidoc-go/adapter"
)

func main() {
	mux := http.NewServeMux()
	s := apidoc.New(apidoc.Config{Title: "我的 API 文档"})

	// 文档与路由一同声明
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
			Title:   "打招呼",
			Params: []apidoc.Param{
				{Name: "name", Type: "string", Required: true, Desc: "你的名字"},
			},
		},
	}); err != nil {
		panic(err)
	}

	s.Mount(adapter.NewNetHTTP(mux))
	http.ListenAndServe(":8080", mux)
	// 外部 UI 读取 /apidoc/api/menus 或 UIC-1 /apidoc/ui/v1/*；
	// 配置 UIPath 后也可直接打开 http://localhost:8080/apidoc/
}
```

### 启用 UIC-1 内置 Vue UI 与服务端执行器

```go
s := apidoc.New(apidoc.Config{
	Prefix: "/apidoc",
	// 可选：apidoc-ui 的 npm run package:ui 产物（index.html / config.js / assets/）。
	// 留空则只提供 JSON API，继续使用外部 UI。
	UIPath: "/srv/myapp/dist/uic1-ui",
	// 服务端代发默认关闭；开启后仍受 SSRF 白名单与限额约束。
	Debug: apidoc.DebugConfig{EnableServerSide: true},
})
```

执行器在构建期按协议族选择：`go build -tags apidoc_debug_all` 编译全部，
或用 `apidoc_debug_http`、`apidoc_debug_sockets`、`apidoc_debug_rpc`、
`apidoc_debug_grpc`、`apidoc_debug_http3`、`apidoc_debug_messaging`、
`apidoc_debug_iot`、`apidoc_debug_industrial`、`apidoc_debug_media` 按需裁剪；
未编译的协议调用一律 fail closed。

### 任意协议注解

非 HTTP 接口复用 `@method` + `@url`，用 `@protocol` 指定协议（也可从 URL
scheme 推断）：

```go
// @apidoc
// @protocol mqtt
// @method PUBLISH
// @url mqtt://broker:1883/sensors/temp
// @title 发布温度
// @param qos int false "QoS" in=option default=1
// @param payload json true "消息体" in=payload
func PublishTemp() {}
```

`@protocol` 接受完整协议清单及别名（HTTP/2、HTTP/3、SOAP、GraphQL、JSON-RPC、
XML-RPC、OData、Webhooks、gRPC、Dubbo、Thrift、Hessian、MCP、A2A、ISO 8583、
WebSocket、SSE、Socket.IO、WebRTC DataChannel、RTSP、RTMP、HLS、SIP、XMPP、
AMQP、MQTT、STOMP、Kafka、CoAP、LwM2M、OPC UA、Modbus TCP、DNP3、
IEC 60870-5-104、BACnet/IP、RTP、RTCP、TCP、UDP、Raw Socket、Unix Domain
Socket）；完整 key、别名、scheme 与默认端口见 `GET /api/protocols`。二进制 body
用 `@param ... type=binary`（或 `hex`/`base64`）+ `in=payload` 声明。

`/api/detail` 与 UIC-1 `/ui/v1/docDetail` 返回 `protocol`、`protocol_info`、`method`、
`url`、`params`、`responses`、`mock` 及解析后的 `definitions`。调试有两种通道：
浏览器直连（HTTP / WebSocket / SSE，始终可用）与服务端代发（`Debug` 开启 + 对应
构建标签）。服务端协议族包括 HTTP（http / http2 / http3 / SOAP / GraphQL / JSON-RPC /
XML-RPC / OData / MCP / A2A / Webhook）、RPC（gRPC / Thrift / Dubbo / Hessian /
ISO 8583）、套接字（TCP / UDP / Raw / Unix）、消息（MQTT / STOMP / AMQP / Kafka /
XMPP）、IoT（CoAP / LwM2M）、工业（Modbus / DNP3 / IEC 104 / BACnet / OPC UA）与
媒体（RTSP / SIP / RTP / RTCP / RTMP）。默认构建不编译执行器；开启后目标先过 SSRF
守卫（`AllowHosts` 白名单、默认拒绝私网/回环/链路本地/云元数据地址、DNS 解析后逐
IP 校验并钉住连接），并受超时、单消息 1 MiB、响应 `MaxBytes` 与流会话上限约束。

### 协议定义解析（@definition）

用 `@definition` 引入外部协议定义（格式自动探测，也可用 `format=` 指定），解析结果
随 detail 返回，供 UIC 渲染报文结构并生成调试表单：

```go
// @apidoc
// @protocol http
// @method POST
// @url /api/v1/users
// @definition file=specs/openapi.yaml operation=createUser
// @definition file=specs/service.proto message=User
```

支持 OpenAPI、AsyncAPI、GraphQL SDL、proto、WSDL、EDMX (OData)、JSON Schema、
JSON-RPC 与 XML-RPC 九种格式。`Config.DefinitionRoot` 限定可读文件根目录（默认进程
工作目录），解析受单文件 1 MiB、5 万行与嵌套引用深度 32 的限制；`operation=`、
`message=`、`direction=`、`sample=`、`inline=` 等选项见 `parse` 包注释。

### 快速开始（Gin）

```go
r := gin.Default()
s := apidoc.New(apidoc.Config{Title: "我的 API 文档"})

s.Register(apidoc.Route{
	Method:  "GET",
	URL:     "/hello",
	Handler: func(c *gin.Context) { c.String(200, "hello") },
	Doc:     apidoc.Doc{App: "demo", Version: "v1", Action: "hello", Title: "打招呼"},
})

s.Mount(adapter.NewGin(r))
r.Run(":8080")
```

Echo / Chi / Fiber 仅需替换适配器构造器：`adapter.NewEcho(e)`、`adapter.NewChi(mux)`、`adapter.NewFiber(app)`，其余代码完全一致；beego / Kratos / go-zero 见下方「框架适配器」表。

### 配置项

| 配置项 | 默认值 | 说明 |
|--------|--------|------|
| `Prefix` | `/apidoc` | 文档挂载路径 |
| `Title` | `API Docs` | 文档标题 |
| `Desc` | — | 文档描述 |
| `GlobalParams` | — | 全局参数，合并进每个 Action |
| `Auth` | — | 鉴权配置：`Enable` / `Password` / `Secret` / `Expire` / `Secure` |
| `Apps` | — | 应用配置：`Key` / `Title` / `Password`（应用级独立密码） |
| `DebugOrigins` | — | 允许跨域读取文档 API 的来源（外部 UI，默认无） |
| `PublicHost` | — | 外部 UI 调试使用的绝对基地址；空则同源 |
| `ShareDir` | 临时目录下 `apidoc-go-shares` | UIC-1 分享记录目录 |
| `DefinitionRoot` | 进程工作目录 | `@definition` 文件根目录；越界路径与符号链接拒绝 |
| `Generator` / `CodeTemplates` | — | 声明式生成器与代码模板（渲染无副作用） |
| `ShareActions` | — | 分享动作声明，由 UI 解释执行 |
| `Debug` | 零值关闭 | 服务端执行器：`EnableServerSide` / `AllowPrivateNetworks` / `AllowHosts` / `AllowUnixSockets` / `TimeoutMS` / `MaxBytes` |
| `UIPath` | —（不内置 UI） | UIC-1 Vue 打包产物目录；非空时在 `{Prefix}/` 下同进程提供静态 UI |

### UIC-1 API 与内置 Vue UI（可选）

库默认提供两套 JSON 文档 API：旧版 `/api/*` 与 UIC-1 `/ui/v1/*`，由外部 UI 消费。
`GET {Prefix}/ui/v1/capabilities` 返回 contract、后端版本与功能矩阵（浏览器/服务端
调试模式、分享、缓存、生成器、导出、鉴权），UI 据此决定可用面板；`/ui/v1/config`
返回菜单与生成器、代码模板配置。把 `apidoc-ui` 的打包产物交给 `Config.UIPath` 后，
同一进程还会在 `{Prefix}/` 下直接提供 Vue UI：

```go
s := apidoc.New(apidoc.Config{
	Prefix: "/apidoc",
	UIPath: "/srv/myapp/dist/uic1-ui", // scripts/package-ui.sh 的产物目录
})
```

库本身不内嵌前端资源，所以没有构建期依赖；内置 UI 的发布包应在启动配置中默认
把 `UIPath` 指向随包分发的 `dist/uic1-ui/` 目录，上面即为推荐接线。部署步骤：

1. 在 UI 仓库构建并打包：`npm run package:ui`（产物 `dist/uic1-ui/`）。
2. 部署到目标目录（必须是已存在且为空、或上一次的 uic1-ui 包）：
   `npm run package:ui -- --dest /srv/myapp/dist/uic1-ui`。
3. 启动时设置 `UIPath`；留空并重启即回滚为纯文档 API 模式。

行为：

- `{Prefix}/`、`{Prefix}/index.html`、`{Prefix}/config.js`、`{Prefix}/assets/*` 直接返回静态文件，MIME 由扩展名决定；
- 未知的无扩展名路径回退 `index.html`（SPA 兜底）；缺失的 `.js` / `.css` 等资源返回 404；
- `{Prefix}`（无尾斜杠）307 到 `{Prefix}/`，保证相对资源路径正确；
- `{Prefix}/api/*`、`{Prefix}/ui/v1/*` 优先；未知 API 路径返回 404，不会回退 UI；
- 文件访问被限制在 `UIPath` 内（拒绝 `..` 与越界符号链接），不做目录列表；目录或 `index.html` 缺失时 UI 路由返回 500，文档 API 不受影响；
- go-zero 适配器只注册固定的 API 路由、不转发前缀下的任意路径，因此该适配器下内置 UI 不可用（其余适配器均支持）；
- 仍使用外部 UI 的部署无需改动；只要 `config.js` 的 `HTTP.API_PREFIX` 与服务端 `Prefix` 一致即可。

### 服务端协议执行器（构建标签）

服务端代发默认不编译任何执行器，调用未编译的协议会返回 fail closed 错误。按协议族
选择构建标签，`apidoc_debug_all` 聚合全部：

| 构建标签 | 协议族 | 协议 |
|----------|--------|------|
| `apidoc_debug_http` | HTTP / Web | http、http2、soap、graphql、jsonrpc、xmlrpc、odata、mcp、a2a、webhook |
| `apidoc_debug_http3` | HTTP/3 | http3（QUIC，仅 https） |
| `apidoc_debug_sockets` | 套接字 | tcp、udp、raw、unix（一元 + 流） |
| `apidoc_debug_rpc` | RPC / 金融 | thrift、dubbo、hessian、iso8583 |
| `apidoc_debug_grpc` | gRPC | grpc（一元；descriptor 或 server reflection） |
| `apidoc_debug_messaging` | 消息 | mqtt、stomp、amqp、kafka、xmpp（一元 + 流；也可用 `apidoc_debug_mqtt` / `_stomp` / `_amqp` / `_kafka` / `_xmpp` 单独裁剪） |
| `apidoc_debug_iot` | IoT | coap、lwm2m |
| `apidoc_debug_industrial` | 工业 | modbus、dnp3、iec104、bacnet、opcua |
| `apidoc_debug_media` | 媒体 | rtsp、sip、rtp、rtcp、rtmp |
| `apidoc_debug_all` | 全部 | 以上所有协议族 |

安全边界：`Debug.EnableServerSide` 默认关闭；开启后仍要求目标通过 `AllowHosts`
白名单（`*.example.com` 后缀匹配）、默认拒绝私网/回环/链路本地/CGNAT 与云元数据
地址（`AllowPrivateNetworks` 才放行）、DNS 解析后逐 IP 校验并钉住连接，禁止
`CONNECT`、重定向与代理；请求/响应受 `TimeoutMS`、`MaxBytes`（默认 1 MiB）、流
会话数（每作用域 4 个）、帧数与空闲超时约束。定义解析只读 `DefinitionRoot` 内的
普通文件，并限制文件大小、行数与引用深度。

### 框架适配器

| 框架 | 构造器 |
|------|--------|
| net/http | `adapter.NewNetHTTP(mux)` |
| Gin | `adapter.NewGin(engine)` |
| Echo | `adapter.NewEcho(e)` |
| Chi | `adapter.NewChi(mux)` |
| Fiber | `adapter.NewFiber(app)` |
| beego | `adapter.NewBeego(app)` |
| Kratos | `adapter.NewKratos(srv)` |
| go-zero | `adapter.NewGoZero(srv)` |

### 注释自动解析（go/ast）

在 handler 上方写 `@apidoc` 注释，用 `parse.ParseDir` 解析后注册：

```go
// @apidoc
// @method POST
// @url /api/v1/users
// @title 创建用户
// @param name string true "用户名"
// @success ok User "成功"
func CreateUser(c *gin.Context, req *CreateUserReq) { ... }
```

```go
results, err := parse.ParseDir("./handlers")
if err != nil { log.Fatal(err) }
for _, r := range results {
    s.Register(apidoc.Route{Method: r.Method, URL: r.URL, Handler: CreateUser, Doc: r.Doc})
}
```

### 参数自动补全（reflect）

`Register` 时若 `Doc.Params` 为空，插件用 reflect 从 handler 签名自动推断：结构体参数展开为 body 字段（遵循 json tag），框架 Context（gin.Context / echo.Context / fiber.Ctx）自动跳过。

### 接口缓存（响应缓存 + ETag）

`/api/menus`、`/api/protocols`、`/api/export` 的响应按文档版本在进程内预序列化缓存，`Handler` 创建时预热，`Register` / `SetAppMeta` 后自动失效。响应仍携带 `ETag` + `Cache-Control: private, max-age=300`，浏览器再次访问命中 304，无需额外配置。

### 导出

| 格式 | 地址 | 说明 |
|------|------|------|
| JSON | `GET /apidoc/api/export` | 完整项目树 |
| TypeScript | `GET /apidoc/api/export?format=typescript` | 接口类型定义 |

### Mock 数据

接口详情页自动展示 Mock 示例：`Doc.Params[].Mock` 可自定义，未指定时按字段类型自动生成（string→"sample"、int→0、bool→true 等）。

### 示例项目

`example/` 内置 8 个框架服务器（net/http :8081、Gin :8082、Echo :8083、Chi :8084、Fiber :8085、beego :8086、Kratos :8087、go-zero :8088），`go run ./example` 一键启动；旧版文档 API 位于各端口的 `/apidoc/api/...`，UIC-1 API 位于 `/apidoc/ui/v1/...`，可配合外部 UI 或 `UIPath` 内置 Vue UI 使用。

## 多语言文档

| 语言 | 链接 |
|------|------|
| English | [English](docs/i18n/en/README.md) |
| 한국어 | [한국어](docs/i18n/ko/README.md) |
| Русский | [Русский](docs/i18n/ru/README.md) |
| Deutsch | [Deutsch](docs/i18n/de/README.md) |
| Français | [Français](docs/i18n/fr/README.md) |
| Español | [Español](docs/i18n/es/README.md) |
| Português | [Português](docs/i18n/pt/README.md) |
| हिन्दी | [हिन्दी](docs/i18n/hi/README.md) |
| العربية | [العربية](docs/i18n/ar/README.md) |
| বাংলা | [বাংলা](docs/i18n/bn/README.md) |
| Bahasa Indonesia | [Bahasa Indonesia](docs/i18n/id/README.md) |
| 日本語 | [日本語](docs/i18n/ja/README.md) |

## 支持我们

如果这个项目对你有帮助，欢迎扫码打赏支持，让我们有动力持续维护和更新！

<div align="center">
  <img src="docs/weixinpay.png" width="130" height="130" alt="微信支付打赏" />
  <img src="docs/alipay.png" width="130" height="130" alt="支付宝打赏" />
  <p>微信支付（左） · 支付宝（右）</p>
</div>

<!-- pet-expressions:start -->
## 项目宠物表情形态 · Apidoc

<table align="center">
<tr><td align="center"><img src="docs/pet-expressions/smile.svg" width="88" alt="笑"><br><b>笑</b><br><code>:emoji-smile:</code></td><td align="center"><img src="docs/pet-expressions/joy.svg" width="88" alt="欢乐"><br><b>欢乐</b><br><code>:emoji-joy:</code></td><td align="center"><img src="docs/pet-expressions/cry.svg" width="88" alt="哭"><br><b>哭</b><br><code>:emoji-cry:</code></td><td align="center"><img src="docs/pet-expressions/depressed.svg" width="88" alt="沮丧"><br><b>沮丧</b><br><code>:emoji-depressed:</code></td><td align="center"><img src="docs/pet-expressions/terrified.svg" width="88" alt="惊恐"><br><b>惊恐</b><br><code>:emoji-terrified:</code></td><td align="center"><img src="docs/pet-expressions/angry.svg" width="88" alt="愤怒"><br><b>愤怒</b><br><code>:emoji-angry:</code></td></tr>
<tr><td align="center"><img src="docs/pet-expressions/coquettish.svg" width="88" alt="撒娇"><br><b>撒娇</b><br><code>:emoji-coquettish:</code></td><td align="center"><img src="docs/pet-expressions/cute.svg" width="88" alt="卖萌"><br><b>卖萌</b><br><code>:emoji-cute:</code></td><td align="center"><img src="docs/pet-expressions/pleasing.svg" width="88" alt="讨好"><br><b>讨好</b><br><code>:emoji-pleasing:</code></td><td align="center"><img src="docs/pet-expressions/knocking.svg" width="88" alt="敲打"><br><b>敲打</b><br><code>:emoji-knocking:</code></td><td align="center"><img src="docs/pet-expressions/facepalm.svg" width="88" alt="捂脸"><br><b>捂脸</b><br><code>:emoji-facepalm:</code></td><td align="center"><img src="docs/pet-expressions/tongue.svg" width="88" alt="吐舌头"><br><b>吐舌头</b><br><code>:emoji-tongue:</code></td></tr>
</table>

> 12 种彩色动态形态：笑 / 欢乐 / 哭 / 沮丧 / 惊恐 / 愤怒 / 撒娇 / 卖萌 / 讨好 / 敲打 / 捂脸 / 吐舌头。每个 SVG 自带表情、情绪特效与整体动效，并遵循 `prefers-reduced-motion`。
<!-- pet-expressions:end -->

<!-- assets-license:start -->
## 视觉资产授权 / Visual Assets License

本项目的项目宠物、图片、图标、表情（包括 `docs/pet-expressions/` 等目录下的
SVG / PNG）**不是开源资产**，版权归作者所有。未经书面许可不得复制、修改、再分发或
用于商业用途；商业使用需取得商业授权，联系 **erik@erik.xyz**。详见
[ASSETS-LICENSE.md](ASSETS-LICENSE.md)。

> 代码部分仍遵循仓库原有 `LICENSE`；第三方素材（如明确标注的 Go Gopher 等）仍遵循其原许可证。
<!-- assets-license:end -->
