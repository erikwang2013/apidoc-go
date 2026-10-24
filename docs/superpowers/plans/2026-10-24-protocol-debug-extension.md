# 协议无关调试扩展 P1 实现计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 让任意协议都能用注解声明，并在文档页获得“填参数、发送、看响应”的调试面板；协议本身由使用者的 `Debugger` 适配器实现。

**Architecture:** 复用 `Method` + `URL` 作为“动作 + 地址”，新增可选 `Protocol` 并从 URL scheme 推断；新增 `debug.Debugger` SPI 与数据驱动面板；非 HTTP 注册只写 store，不转发给 HTTP 框架；`server` 模式通过 `/api/debug/exec` 执行，带鉴权、CSRF 与资源限额。

**Tech Stack:** Go 1.24.2、标准库、现有 gin/echo/chi/fiber/beego/kratos/go-zero 适配器；P1 不新增依赖。前端为原生 HTML/JS，无框架。

**Spec:** `docs/superpowers/specs/2026-10-24-protocol-debug-extension-design.md`

## Global Constraints

- 核心模块 `go.mod` 不新增任何依赖；示例只用标准库。
- 每个 Go 文件、`index.html`、`debug.js` 保持在 500 行以内。
- 现有 HTTP 行为、`ActionID`、菜单/详情/导出 JSON 的既有字段必须保持不变；新字段只增不改。
- 非 HTTP 注册绝不调用 `adapter.Framework.Register`，包括 Mount 时的 pending 重放。
- `server` 模式必须鉴权：`Auth.Enable` 时要求会话；关闭鉴权时必须显式 `DebugAllowUnauth = true`。
- `/api/debug/exec` 必须要求 `Content-Type: application/json` 与 `X-Apidoc-Debug: 1`。
- 执行目标默认绑定已注册 action；只有 `Spec.AllowTargetOverride` 才允许改 URL。
- 所有适配器返回内容在 UI 中必须用 `textContent` 渲染，`debug.js` 禁止出现 `innerHTML`。
- 提交信息沿用项目风格（`feat:` / `docs:`），不加 `Co-Authored-By`。

## Review Focus

- **无 scheme 的 `host:port` 不能被当成 scheme**：`localhost:50051` 必须推断为 `http`，只有包含 `://` 才解析 scheme。
- **非 HTTP 注册泄漏到 HTTP 框架**：注册前 Mount 与注册后 Mount 都不得触发 `Framework.Register`。
- **调试端点被当成通用代理**：请求必须绑定 action id；method 只能在 `Spec.Actions` 内，target 只有允许时才可改，param 必须在已注册参数内。
- **未鉴权/跨站调用调试端点**：auth 关闭且未显式允许时返回 403；缺 CSRF 头返回 403；app 受保护时必须带 app token。
- **适配器超时/超大响应拖垮进程**：超时返回 504，并发超限返回 429，请求体/响应体/事件/参数数量都有上限；UI 不能把返回内容当 HTML 执行。

---

### Task 1: Protocol metadata and inference

**Files:**
- Modify: `model/model.go`
- Create: `model/protocol.go`
- Create: `model/protocol_test.go`
- Modify: `store/store.go`
- Modify: `store/store_test.go`

**Interfaces:**
- Produces: `model.Doc.Protocol string`, `model.Action.Protocol string`,
  `model.ProtocolOf(rawURL string) string`.
- `store.Register` fills `Action.Protocol`; `Store.UnmarshalJSON` backfills old data.

- [ ] **Step 1: Write the failing model test**

Create `model/protocol_test.go`:

```go
package model

import "testing"

func TestProtocolOf(t *testing.T) {
	cases := []struct{ in, want string }{
		{"https://api.example.com/x", "http"},
		{"wss://api.example.com/ws", "ws"},
		{"grpcs://host/pkg.Svc/M", "grpc"},
		{"mqtts://broker:8883/topic", "mqtt"},
		{"amqps://broker/queue", "amqp"},
		{"tcp://127.0.0.1:9000", "tcp"},
		{"ssh://host", "ssh"},
		{"/api/users", "http"},
		{"localhost:50051", "http"}, // no "://": not a scheme
		{"", "http"},
	}
	for _, c := range cases {
		if got := ProtocolOf(c.in); got != c.want {
			t.Errorf("ProtocolOf(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./model -run TestProtocolOf -v`
Expected: FAIL with `undefined: ProtocolOf`.

- [ ] **Step 3: Implement model fields and inference**

Add to `model.Action` (after `URL`):

```go
Protocol   string     `json:"protocol,omitempty"`
```

Add to `model.Doc`:

```go
Protocol string
```

Create `model/protocol.go`:

```go
package model

import (
	"net/url"
	"strings"
)

// ProtocolOf maps a documented address to a protocol key. An address
// without "://" is an HTTP path or host:port, never a scheme.
func ProtocolOf(rawURL string) string {
	if !strings.Contains(rawURL, "://") {
		return "http"
	}
	u, err := url.Parse(rawURL)
	if err != nil || u.Scheme == "" {
		return "http"
	}
	s := strings.ToLower(u.Scheme)
	switch s {
	case "http", "https":
		return "http"
	case "ws", "wss":
		return "ws"
	case "grpc", "grpcs":
		return "grpc"
	case "mqtt", "mqtts":
		return "mqtt"
	case "amqp", "amqps":
		return "amqp"
	case "stomp", "stomps":
		return "stomp"
	case "coap", "coaps":
		return "coap"
	default:
		return s
	}
}
```

- [ ] **Step 4: Run the model test to verify it passes**

Run: `go test ./model -run TestProtocolOf -v`
Expected: PASS.

- [ ] **Step 5: Write the failing store tests**

Append to `store/store_test.go`:

```go
func TestRegisterProtocolDefaultAndBackfill(t *testing.T) {
	s := New("p")
	s.Register("PUBLISH", "mqtt://broker/topic", model.Doc{App: "a", Version: "v1"})
	a, ok := s.Action(ActionID("a", "v1", "PUBLISH", "mqtt://broker/topic"))
	if !ok || a.Protocol != "mqtt" {
		t.Fatalf("protocol not inferred: ok=%v action=%+v", ok, a)
	}
	s.Register("CALL", "grpc://host/svc/m", model.Doc{App: "a", Version: "v1", Protocol: "custom"})
	b, _ := s.Action(ActionID("a", "v1", "CALL", "grpc://host/svc/m"))
	if b.Protocol != "custom" {
		t.Fatalf("explicit protocol lost: %+v", b)
	}

	// Old serialized data has no protocol field: loading must backfill.
	p := &model.Project{Name: "p", Apps: []*model.App{{
		Key: "a", Versions: []*model.Version{{Name: "v1", Controllers: []*model.Controller{{
			Name: "c", Actions: []*model.Action{{
				ID: "legacy", App: "a", Version: "v1", Controller: "c",
				Method: "GET", URL: "/legacy",
			}},
		}}}},
	}}}
	bts, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	var loaded Store
	if err := json.Unmarshal(bts, &loaded); err != nil {
		t.Fatal(err)
	}
	c, ok := loaded.Action("legacy")
	if !ok || c.Protocol != "http" {
		t.Fatalf("backfill failed: ok=%v action=%+v", ok, c)
	}
}
```

- [ ] **Step 6: Run the store test to verify it fails**

Run: `go test ./store -run TestRegisterProtocolDefaultAndBackfill -v`
Expected: FAIL because `Action.Protocol` stays empty.

- [ ] **Step 7: Implement store defaults and backfill**

In `store.Register`, before building the action:

```go
if d.Protocol == "" {
	d.Protocol = model.ProtocolOf(u)
}
```

Add `Protocol: d.Protocol` to the `&model.Action{...}` literal.

In `Store.UnmarshalJSON`, inside the action loop:

```go
if a.Protocol == "" {
	a.Protocol = model.ProtocolOf(a.URL)
}
```

- [ ] **Step 8: Run model and store tests**

Run: `go test ./model ./store`
Expected: PASS (including the existing dedupe and JSON round-trip tests).

- [ ] **Step 9: Commit**

```bash
git add model/model.go model/protocol.go model/protocol_test.go store/store.go store/store_test.go
git commit -m "feat: add protocol metadata and scheme inference"
```

---

### Task 2: Debugger SPI

**Files:**
- Create: `debug/debug.go`
- Create: `debug/debug_test.go`

**Interfaces:**
- Produces: `debug.Debugger`, `debug.Spec`, `debug.Field`, `debug.Value`,
  `debug.Request`, `debug.Response`, `debug.Event`, `debug.Func`,
  `debug.HTTPBrowserSpec`, and the `Mode*` / `View*` constants.

- [ ] **Step 1: Write the failing test**

Create `debug/debug_test.go`:

```go
package debug

import (
	"context"
	"testing"
)

func TestFuncDebugger(t *testing.T) {
	d := Func("mqtt", Spec{Mode: ModeServer, Actions: []string{"PUBLISH"}},
		func(ctx context.Context, req Request) (*Response, error) {
			if req.Method != "PUBLISH" || req.URL != "mqtt://broker/topic" {
				t.Errorf("request not passed through: %+v", req)
			}
			return &Response{Status: "PUBACK", Body: []byte("ok"), Encoding: "text"}, nil
		})
	if d.Protocol() != "mqtt" {
		t.Fatalf("protocol = %q", d.Protocol())
	}
	if d.Spec().Mode != ModeServer || d.Spec().Actions[0] != "PUBLISH" {
		t.Fatalf("spec = %+v", d.Spec())
	}
	resp, err := d.Exec(context.Background(), Request{Method: "PUBLISH", URL: "mqtt://broker/topic"})
	if err != nil || resp.Status != "PUBACK" || string(resp.Body) != "ok" {
		t.Fatalf("exec = %+v, %v", resp, err)
	}
}

func TestHTTPBrowserSpec(t *testing.T) {
	s := HTTPBrowserSpec()
	if s.Mode != ModeBrowser {
		t.Fatalf("mode = %q", s.Mode)
	}
	found := false
	for _, a := range s.Actions {
		if a == "GET" {
			found = true
		}
	}
	if !found {
		t.Fatalf("GET missing from %+v", s.Actions)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./debug -v`
Expected: FAIL because package `debug` has no types.

- [ ] **Step 3: Implement `debug/debug.go`**

```go
// Package debug defines the protocol-agnostic debug extension point.
// A protocol implementation supplies Spec (panel shape) and Exec
// (one request/response round trip); the doc server owns auth, limits
// and rendering.
package debug

import (
	"context"
	"time"
)

const (
	ModeBrowser = "browser"
	ModeServer  = "server"

	ViewAuto       = "auto"
	ViewHTTP       = "http"
	ViewJSON       = "json"
	ViewXML        = "xml"
	ViewText       = "text"
	ViewHex        = "hex"
	ViewTranscript = "transcript"
)

// Value is one documented parameter with the value typed in the UI.
type Value struct {
	Name  string `json:"name"`
	In    string `json:"in"`
	Value string `json:"value"`
}

// Request is one debug execution.
type Request struct {
	ActionID string            `json:"action_id,omitempty"`
	Method   string            `json:"method"`
	URL      string            `json:"url"`
	Params   []Value           `json:"params,omitempty"`
	Body     []byte            `json:"body,omitempty"`
	Encoding string            `json:"encoding,omitempty"`
	Headers  map[string]string `json:"headers,omitempty"`
	Options  map[string]string `json:"options,omitempty"`
}

// Event is one transcript entry for streaming or multi-response protocols.
type Event struct {
	Time      time.Time `json:"time"`
	Direction string    `json:"direction"` // "in" | "out"
	Name      string    `json:"name,omitempty"`
	Data      string    `json:"data,omitempty"`
	Encoding  string    `json:"encoding,omitempty"`
}

// Response is the adapter result.
type Response struct {
	Status   string            `json:"status"`
	Headers  map[string]string `json:"headers,omitempty"`
	Body     []byte            `json:"body,omitempty"`
	Encoding string            `json:"encoding,omitempty"`
	Events   []Event           `json:"events,omitempty"`
	Duration time.Duration     `json:"duration,omitempty"`
}

// Field is a debug-only connection input rendered above the message.
type Field struct {
	Name        string   `json:"name"`
	Label       string   `json:"label,omitempty"`
	Type        string   `json:"type,omitempty"` // text|password|number|bool|select|textarea
	Default     string   `json:"default,omitempty"`
	Placeholder string   `json:"placeholder,omitempty"`
	Required    bool     `json:"required,omitempty"`
	Secret      bool     `json:"secret,omitempty"`
	Options     []string `json:"options,omitempty"`
}

// Spec tells the UI how to render the debug panel for one protocol.
type Spec struct {
	Mode                string   `json:"mode"`
	Actions             []string `json:"actions,omitempty"`
	Encodings           []string `json:"encodings,omitempty"`
	ConnectFields       []Field  `json:"connect_fields,omitempty"`
	BodyIn              string   `json:"body_in,omitempty"`      // default "payload"
	ResponseView        string   `json:"response_view,omitempty"` // default "auto"
	AllowTargetOverride bool     `json:"allow_target_override,omitempty"`
}

// Debugger connects one protocol. Implementations usually wrap a
// client the host application already owns. Exec must return when ctx
// is done; the server relies on that for its timeout.
type Debugger interface {
	Protocol() string
	Spec() Spec
	Exec(context.Context, Request) (*Response, error)
}

type funcDebugger struct {
	protocol string
	spec     Spec
	fn       func(context.Context, Request) (*Response, error)
}

func (d funcDebugger) Protocol() string { return d.protocol }
func (d funcDebugger) Spec() Spec       { return d.spec }
func (d funcDebugger) Exec(ctx context.Context, req Request) (*Response, error) {
	return d.fn(ctx, req)
}

// Func adapts a function to a Debugger.
func Func(protocol string, spec Spec, fn func(context.Context, Request) (*Response, error)) Debugger {
	return funcDebugger{protocol: protocol, spec: spec, fn: fn}
}

// HTTPBrowserSpec is the built-in HTTP/HTTPS panel. The browser executes
// the request, so no server-side Exec is registered.
func HTTPBrowserSpec() Spec {
	return Spec{
		Mode:         ModeBrowser,
		Actions:      []string{"GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS"},
		Encodings:    []string{"json", "xml", "text"},
		ResponseView: ViewHTTP,
	}
}
```

- [ ] **Step 4: Run the debug tests**

Run: `go test ./debug -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add debug/debug.go debug/debug_test.go
git commit -m "feat: add protocol debugger interface"
```

---

### Task 3: Config, registry, and non-HTTP registration

**Files:**
- Create: `debug.go` (root package aliases)
- Modify: `apidoc.go`
- Create: `apidoc_debug_test.go`

**Interfaces:**
- Consumes: `debug.Debugger`, `model.ProtocolOf`.
- Produces: `apidoc.Debugger` (alias), `apidoc.DebugSpec`, `apidoc.DebugField`,
  `apidoc.DebugValue`, `apidoc.DebugRequest`, `apidoc.DebugResponse`,
  `apidoc.DebugEvent`, `apidoc.DebuggerFunc`.
- Produces: `Config.Debuggers []Debugger`, `Config.DebugAllowUnauth bool`,
  `Config.DebugTimeout time.Duration`, `Config.DebugMaxConcurrent int`.
- `Server.Register` stores non-HTTP docs only; HTTP behavior unchanged.

- [ ] **Step 1: Write the failing tests**

Create `apidoc_debug_test.go`:

```go
package apidoc

import (
	"net/http"
	"testing"

	"github.com/erikwang2013/apidoc-go/store"
)

type fakeFramework struct {
	registered []string
	mounted    bool
}

func (f *fakeFramework) Register(method, path string, h any) error {
	f.registered = append(f.registered, method+" "+path)
	return nil
}

func (f *fakeFramework) Mount(prefix string, h http.Handler) { f.mounted = true }

func TestNonHTTPRegisterDoesNotForward(t *testing.T) {
	ff := &fakeFramework{}
	s := New(Config{})
	if err := s.Mount(ff); err != nil {
		t.Fatal(err)
	}
	if err := s.Register(Route{
		Method: "SEND",
		URL:    "tcp://127.0.0.1:9000",
		Doc:    Doc{Protocol: "tcp", Title: "tcp echo"},
	}); err != nil {
		t.Fatal(err)
	}
	if len(ff.registered) != 0 {
		t.Fatalf("non-http route forwarded: %v", ff.registered)
	}
	a, ok := s.st.Action(store.ActionID("default", "v1", "SEND", "tcp://127.0.0.1:9000"))
	if !ok || a.Protocol != "tcp" {
		t.Fatalf("doc not stored: ok=%v action=%+v", ok, a)
	}
}

func TestNonHTTPPendingNotForwardedOnMount(t *testing.T) {
	s := New(Config{})
	if err := s.Register(Route{Method: "SEND", URL: "tcp://127.0.0.1:9000",
		Doc: Doc{Protocol: "tcp"}}); err != nil {
		t.Fatal(err)
	}
	ff := &fakeFramework{}
	if err := s.Mount(ff); err != nil {
		t.Fatal(err)
	}
	if len(ff.registered) != 0 {
		t.Fatalf("pending non-http route forwarded: %v", ff.registered)
	}
}

func TestHTTPStillForwards(t *testing.T) {
	s := New(Config{})
	ff := &fakeFramework{}
	if err := s.Mount(ff); err != nil {
		t.Fatal(err)
	}
	if err := s.Register(Route{Method: "GET", URL: "/ping",
		Handler: http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}),
		Doc:     Doc{Title: "ping"}}); err != nil {
		t.Fatal(err)
	}
	if len(ff.registered) != 1 || ff.registered[0] != "GET /ping" {
		t.Fatalf("http route not forwarded: %v", ff.registered)
	}
}

func TestDebuggerRegistryValidation(t *testing.T) {
	cases := []struct {
		name string
		cfg  Config
	}{
		{"duplicate", Config{Debuggers: []Debugger{
			DebuggerFunc("mqtt", DebugSpec{}, nil),
			DebuggerFunc("MQTT", DebugSpec{}, nil),
		}}},
		{"empty protocol", Config{Debuggers: []Debugger{
			DebuggerFunc("", DebugSpec{}, nil),
		}}},
		{"nil debugger", Config{Debuggers: []Debugger{nil}}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Fatal("want panic")
				}
			}()
			New(c.cfg)
		})
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test . -run 'TestNonHTTP|TestHTTPStill|TestDebuggerRegistry' -v`
Expected: FAIL: non-HTTP route is forwarded; `DebuggerFunc` / `Debuggers` undefined.

- [ ] **Step 3: Add root aliases and config**

Create `debug.go`:

```go
package apidoc

import (
	"context"

	"github.com/erikwang2013/apidoc-go/debug"
)

type (
	Debugger      = debug.Debugger
	DebugSpec     = debug.Spec
	DebugField    = debug.Field
	DebugValue    = debug.Value
	DebugRequest  = debug.Request
	DebugResponse = debug.Response
	DebugEvent    = debug.Event
)

// DebuggerFunc adapts a function to a Debugger.
func DebuggerFunc(protocol string, spec DebugSpec, fn func(context.Context, DebugRequest) (*DebugResponse, error)) Debugger {
	return debug.Func(protocol, spec, fn)
}
```

In `apidoc.go`, add to `Config`:

```go
Debuggers          []Debugger
DebugAllowUnauth   bool
DebugTimeout       time.Duration
DebugMaxConcurrent int
```

Add `"time"` to imports if needed.

- [ ] **Step 4: Add the registry and non-HTTP early return**

In `apidoc.go`, add to `Server`:

```go
debuggers map[string]Debugger
```

In `New`, before building `s`:

```go
dbg := make(map[string]Debugger, len(cfg.Debuggers))
for _, d := range cfg.Debuggers {
	if d == nil {
		panic("apidoc: nil debugger")
	}
	key := strings.ToLower(strings.TrimSpace(d.Protocol()))
	if key == "" {
		panic("apidoc: debugger protocol is empty")
	}
	if _, ok := dbg[key]; ok {
		panic("apidoc: duplicate debugger for protocol " + key)
	}
	dbg[key] = d
}
```

Set `debuggers: dbg` in the `&Server{...}` literal. Add `"strings"` and
`"github.com/erikwang2013/apidoc-go/model"` to imports.

Restructure the top of `Register`:

```go
func (s *Server) Register(r Route) error {
	if r.Method == "" || r.URL == "" {
		return fmt.Errorf("apidoc: method and url are required")
	}
	r.Doc.Protocol = strings.ToLower(strings.TrimSpace(r.Doc.Protocol))
	if r.Doc.Protocol == "" {
		r.Doc.Protocol = model.ProtocolOf(r.URL)
	}
	if r.Doc.Protocol != "http" {
		s.st.Register(r.Method, r.URL, r.Doc)
		return nil
	}
	// existing reflectParams + store + forwarding code follows unchanged
	...
}
```

- [ ] **Step 5: Run the root tests**

Run: `go test . -v`
Expected: PASS, including existing `TestServerRegisterMountFlow` and the new tests.

- [ ] **Step 6: Commit**

```bash
git add apidoc.go debug.go apidoc_debug_test.go
git commit -m "feat: register protocol-neutral endpoints without framework forwarding"
```

---

### Task 4: Parse `@protocol` and custom param groups

**Files:**
- Modify: `parse/parse.go`
- Modify: `parse/parse_test.go`

**Interfaces:**
- Consumes: `model.ProtocolOf`.
- Produces: `parse.Result.Doc.Protocol`; `in=` accepts any non-empty token.

- [ ] **Step 1: Write the failing tests**

Append to `parse/parse_test.go`:

```go
func TestProtocolTagAndCustomIn(t *testing.T) {
	res, err := parse.ParseSource("proto.go", []byte(src(
		"package p",
		"// @apidoc",
		"// @protocol MQTT",
		"// @method PUBLISH",
		"// @url mqtt://broker:1883/sensors/temp",
		"// @param qos int false \"QoS\" in=option default=1",
		"// @param payload json true \"消息体\" in=payload",
		"func Publish() {}",
	)))
	if err != nil {
		t.Fatal(err)
	}
	r := res[0]
	if r.Doc.Protocol != "mqtt" {
		t.Fatalf("protocol = %q", r.Doc.Protocol)
	}
	if r.Doc.Params[0].In != "option" || r.Doc.Params[1].In != "payload" {
		t.Fatalf("custom in groups lost: %+v", r.Doc.Params)
	}
}

func TestProtocolInferredFromURL(t *testing.T) {
	res, err := parse.ParseSource("infer.go", []byte(src(
		"package p",
		"// @apidoc",
		"// @method CALL",
		"// @url grpc://localhost:50051/pkg.Svc/M",
		"func Call() {}",
	)))
	if err != nil || len(res) != 1 {
		t.Fatalf("parse: %v %d", err, len(res))
	}
	if res[0].Doc.Protocol != "grpc" {
		t.Fatalf("inferred = %q", res[0].Doc.Protocol)
	}
	res, err = parse.ParseSource("http.go", []byte(src(
		"package p",
		"// @apidoc",
		"// @method GET",
		"// @url /api/x",
		"func Get() {}",
	)))
	if err != nil || res[0].Doc.Protocol != "http" {
		t.Fatalf("http infer: %v %+v", err, res)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./parse -run 'TestProtocol' -v`
Expected: FAIL because `@protocol` is ignored and `in=option` is rejected.

- [ ] **Step 3: Implement the parser changes**

In `parseTags`, add:

```go
case "protocol":
	if len(toks) < 1 {
		err = fmt.Errorf("@protocol wants a name")
	} else {
		r.Doc.Protocol = strings.ToLower(toks[0])
	}
```

In `parseParam`, replace the `in` whitelist with:

```go
case "in":
	if val == "" {
		return p, fmt.Errorf("empty in=")
	}
	p.In = strings.ToLower(val)
```

At the end of `parseFunc`, after `parseTags` and `extractStructs`:

```go
if r.Doc.Protocol == "" {
	r.Doc.Protocol = model.ProtocolOf(r.URL)
}
```

- [ ] **Step 4: Run the parse tests**

Run: `go test ./parse`
Expected: PASS, including existing `TestParams`.

- [ ] **Step 5: Commit**

```bash
git add parse/parse.go parse/parse_test.go
git commit -m "feat: parse protocol tags and custom param groups"
```

---

### Task 5: Expose protocol and debug spec in detail

**Files:**
- Modify: `server/server.go`
- Modify: `server/server_test.go`
- Modify: `apidoc.go` (pass the registry to `server.Handler`)

**Interfaces:**
- Consumes: `debug.Debugger`, `debug.HTTPBrowserSpec`, `model.ProtocolOf`.
- Produces: `server.Opts.Debuggers map[string]debug.Debugger`; `/api/detail`
  fields `protocol` and `debug_spec`.

- [ ] **Step 1: Write the failing tests**

Append to `server/server_test.go`:

```go
func TestDetailProtocolAndDebugSpec(t *testing.T) {
	s := store.New("p")
	s.Register("PUBLISH", "mqtt://broker/topic", model.Doc{
		App: "a", Version: "v1", Protocol: "mqtt", Title: "publish",
	})
	d := debug.Func("mqtt", debug.Spec{Mode: debug.ModeServer, Actions: []string{"PUBLISH"}}, nil)
	h := Handler(Opts{Prefix: "/apidoc", Store: s,
		Debuggers: map[string]debug.Debugger{"mqtt": d},
	})
	id := store.ActionID("a", "v1", "PUBLISH", "mqtt://broker/topic")
	rec := req(t, h, http.MethodGet, "/api/detail?id="+id, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("detail status %d: %s", rec.Code, rec.Body.String())
	}
	var b struct {
		Data struct {
			Protocol  string       `json:"protocol"`
			DebugSpec *debug.Spec  `json:"debug_spec"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &b); err != nil {
		t.Fatal(err)
	}
	if b.Data.Protocol != "mqtt" || b.Data.DebugSpec == nil ||
		b.Data.DebugSpec.Mode != debug.ModeServer {
		t.Fatalf("detail protocol/spec wrong: %+v", b.Data)
	}

	// HTTP has a built-in browser spec even with no registered debugger.
	h2 := Handler(Opts{Prefix: "/apidoc", Store: newTestStore()})
	rec = req(t, h2, http.MethodGet, "/api/detail?id="+
		store.ActionID("users", "v1", "GET", "/users"), "")
	var httpDetail struct {
		Data struct {
			Protocol  string      `json:"protocol"`
			DebugSpec *debug.Spec `json:"debug_spec"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &httpDetail); err != nil {
		t.Fatal(err)
	}
	if httpDetail.Data.Protocol != "http" || httpDetail.Data.DebugSpec == nil ||
		httpDetail.Data.DebugSpec.Mode != debug.ModeBrowser {
		t.Fatalf("http detail wrong: %+v", httpDetail.Data)
	}

	// A protocol with no debugger gets a null spec.
	s.Register("SEND", "tcp://127.0.0.1:9000", model.Doc{
		App: "a", Version: "v1", Protocol: "tcp", Title: "send",
	})
	h3 := Handler(Opts{Prefix: "/apidoc", Store: s})
	rec = req(t, h3, http.MethodGet, "/api/detail?id="+
		store.ActionID("a", "v1", "SEND", "tcp://127.0.0.1:9000"), "")
	var tcpDetail struct {
		Data struct {
			DebugSpec *debug.Spec `json:"debug_spec"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &tcpDetail); err != nil {
		t.Fatal(err)
	}
	if tcpDetail.Data.DebugSpec != nil {
		t.Fatalf("unexpected debug spec: %+v", tcpDetail.Data.DebugSpec)
	}
}
```

Add the `debug` import to the test file.

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./server -run TestDetailProtocolAndDebugSpec -v`
Expected: FAIL because `Opts.Debuggers` and the detail fields do not exist.

- [ ] **Step 3: Implement the detail fields**

In `server/server.go`:

```go
type Opts struct {
	// existing fields...
	Debuggers map[string]debug.Debugger
}

func (h *apiHandler) specFor(protocol string) *debug.Spec {
	if d, ok := h.o.Debuggers[protocol]; ok {
		s := d.Spec()
		return &s
	}
	if protocol == "http" {
		s := debug.HTTPBrowserSpec()
		return &s
	}
	return nil
}
```

In `detail`, compute:

```go
protocol := a.Protocol
if protocol == "" {
	protocol = model.ProtocolOf(a.URL)
}
```

Add to the response map:

```go
"protocol": protocol,
"debug_spec": h.specFor(protocol),
```

In `apidoc.go`'s `Handler`, pass:

```go
Debuggers: s.debuggers,
```

- [ ] **Step 4: Run the server tests**

Run: `go test ./server`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add server/server.go server/server_test.go apidoc.go
git commit -m "feat: expose protocol and debug spec in detail"
```

---

### Task 6: Debug execution endpoint (core)

**Files:**
- Modify: `server/server.go`
- Create: `server/debug_exec_test.go`
- Modify: `apidoc.go`

**Interfaces:**
- Consumes: `Opts.Debuggers`, `Opts.DebugAllowUnauth`, `debug.Debugger`,
  `store.Action`, `mergeParams`.
- Produces: `POST /api/debug/exec` with request
  `{id, method, url, params, body, encoding, headers, options}` and
  response data `{status, headers, body, encoding, events, duration_ms, truncated}`.

- [ ] **Step 1: Write the failing tests**

Create `server/debug_exec_test.go` (package `server`):

```go
package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/erikwang2013/apidoc-go/auth"
	"github.com/erikwang2013/apidoc-go/debug"
	"github.com/erikwang2013/apidoc-go/model"
	"github.com/erikwang2013/apidoc-go/store"
)

type captureDebugger struct {
	spec debug.Spec
	got  debug.Request
	resp *debug.Response
	err  error
}

func (d *captureDebugger) Protocol() string { return "mqtt" }
func (d *captureDebugger) Spec() debug.Spec { return d.spec }
func (d *captureDebugger) Exec(ctx context.Context, req debug.Request) (*debug.Response, error) {
	d.got = req
	if d.err != nil {
		return nil, d.err
	}
	if d.resp != nil {
		return d.resp, nil
	}
	return &debug.Response{Status: "PUBACK", Body: []byte("ok"), Encoding: "text"}, nil
}

func newDebugStore() *store.Store {
	s := store.New("p")
	s.Register("PUBLISH", "mqtt://broker/topic", model.Doc{
		App: "a", Version: "v1", Protocol: "mqtt", Title: "publish",
		Params: []model.Param{
			{Name: "qos", In: "option", Type: "int"},
			{Name: "payload", In: "payload", Type: "string"},
		},
	})
	return s
}

func debugPost(t *testing.T, h http.Handler, body string, csrf bool) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(http.MethodPost, "/api/debug/exec", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	if csrf {
		r.Header.Set("X-Apidoc-Debug", "1")
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	return rec
}

func TestDebugExecSuccess(t *testing.T) {
	d := &captureDebugger{spec: debug.Spec{
		Mode: debug.ModeServer, Actions: []string{"PUBLISH"},
		Encodings: []string{"json", "text"},
	}}
	h := Handler(Opts{Prefix: "/apidoc", Store: newDebugStore(),
		Debuggers: map[string]debug.Debugger{"mqtt": d}, DebugAllowUnauth: true})
	id := store.ActionID("a", "v1", "PUBLISH", "mqtt://broker/topic")
	body := fmt.Sprintf(`{"id":%q,"method":"PUBLISH","url":"mqtt://broker/topic",`+
		`"params":[{"name":"qos","in":"option","value":"1"},`+
		`{"name":"payload","in":"payload","value":"hello"}],`+
		`"body":"aGVsbG8=","encoding":"text",`+
		`"headers":{"X-A":"b"},"options":{"clientId":"c"}}`, id)
	rec := debugPost(t, h, body, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	var out struct {
		Data struct {
			Status     string            `json:"status"`
			Body       []byte            `json:"body"`
			Encoding   string            `json:"encoding"`
			DurationMS int64             `json:"duration_ms"`
			Headers    map[string]string `json:"headers"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out.Data.Status != "PUBACK" || string(out.Data.Body) != "ok" ||
		out.Data.Encoding != "text" || out.Data.DurationMS < 0 {
		t.Fatalf("response data wrong: %+v", out.Data)
	}
	got := d.got
	if got.ActionID != id || got.Method != "PUBLISH" || got.URL != "mqtt://broker/topic" ||
		got.Encoding != "text" || string(got.Body) != "hello" ||
		got.Headers["X-A"] != "b" || got.Options["clientId"] != "c" ||
		len(got.Params) != 2 || got.Params[1].Value != "hello" {
		t.Fatalf("adapter request wrong: %+v", got)
	}
}

func TestDebugExecErrors(t *testing.T) {
	id := store.ActionID("a", "v1", "PUBLISH", "mqtt://broker/topic")
	valid := func(method, url string) string {
		return fmt.Sprintf(`{"id":%q,"method":%q,"url":%q,`+
			`"params":[{"name":"qos","in":"option","value":"1"}],`+
			`"body":"","encoding":"json"}`, id, method, url)
	}
	spec := debug.Spec{Mode: debug.ModeServer, Actions: []string{"PUBLISH"}}
	newHandler := func(d debug.Debugger, allow bool) http.Handler {
		return Handler(Opts{Prefix: "/apidoc", Store: newDebugStore(),
			Debuggers: map[string]debug.Debugger{"mqtt": d}, DebugAllowUnauth: allow})
	}
	tests := []struct {
		name   string
		h      http.Handler
		body   string
		csrf   bool
		status int
	}{
		{"missing csrf", newHandler(&captureDebugger{spec: spec}, true), valid("PUBLISH", "mqtt://broker/topic"), false, http.StatusForbidden},
		{"unknown id", newHandler(&captureDebugger{spec: spec}, true), `{"id":"nope"}`, true, http.StatusNotFound},
		{"no debugger", Handler(Opts{Prefix: "/apidoc", Store: newDebugStore(), DebugAllowUnauth: true}), valid("PUBLISH", "mqtt://broker/topic"), true, http.StatusBadRequest},
		{"browser mode", newHandler(&captureDebugger{spec: debug.HTTPBrowserSpec()}, true), valid("PUBLISH", "mqtt://broker/topic"), true, http.StatusBadRequest},
		{"action not allowed", newHandler(&captureDebugger{spec: spec}, true), valid("DELETE", "mqtt://broker/topic"), true, http.StatusBadRequest},
		{"target override denied", newHandler(&captureDebugger{spec: spec}, true), valid("PUBLISH", "mqtt://evil/topic"), true, http.StatusBadRequest},
		{"debug disabled", newHandler(&captureDebugger{spec: spec}, false), valid("PUBLISH", "mqtt://broker/topic"), true, http.StatusForbidden},
		{"adapter error", newHandler(&captureDebugger{spec: spec, err: fmt.Errorf("boom")}, true), valid("PUBLISH", "mqtt://broker/topic"), true, http.StatusBadGateway},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec := debugPost(t, tc.h, tc.body, tc.csrf)
			if rec.Code != tc.status {
				t.Fatalf("status = %d, want %d: %s", rec.Code, tc.status, rec.Body.String())
			}
		})
	}
}

func TestDebugExecTargetOverrideAllowed(t *testing.T) {
	d := &captureDebugger{spec: debug.Spec{
		Mode: debug.ModeServer, Actions: []string{"PUBLISH"}, AllowTargetOverride: true,
	}}
	h := Handler(Opts{Prefix: "/apidoc", Store: newDebugStore(),
		Debuggers: map[string]debug.Debugger{"mqtt": d}, DebugAllowUnauth: true})
	id := store.ActionID("a", "v1", "PUBLISH", "mqtt://broker/topic")
	body := fmt.Sprintf(`{"id":%q,"method":"PUBLISH","url":"mqtt://other/topic",`+
		`"params":[],"body":"","encoding":"json"}`, id)
	if rec := debugPost(t, h, body, true); rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	if d.got.URL != "mqtt://other/topic" {
		t.Fatalf("override not passed: %+v", d.got)
	}
}

func TestDebugExecAuth(t *testing.T) {
	id := store.ActionID("a", "v1", "PUBLISH", "mqtt://broker/topic")
	body := fmt.Sprintf(`{"id":%q,"method":"PUBLISH","url":"mqtt://broker/topic",`+
		`"params":[],"body":"","encoding":"json"}`, id)
	d := &captureDebugger{spec: debug.Spec{Mode: debug.ModeServer, Actions: []string{"PUBLISH"}}}

	// Auth enabled: no session cookie -> 401 from withAuth.
	h := Handler(Opts{Prefix: "/apidoc", Store: newDebugStore(),
		Debuggers:  map[string]debug.Debugger{"mqtt": d},
		Auth:       auth.Config{Enable: true, Password: "pw", Secret: "sec"},
		DebugAllowUnauth: true})
	if rec := debugPost(t, h, body, true); rec.Code != http.StatusUnauthorized {
		t.Fatalf("auth on without cookie: %d", rec.Code)
	}

	// App-protected action with auth off requires the app token.
	h = Handler(Opts{Prefix: "/apidoc", Store: newDebugStore(),
		Debuggers: map[string]debug.Debugger{"mqtt": d},
		AppPWs:    map[string]string{"a": "apppw"},
		Auth:      auth.Config{Secret: "sec"},
		DebugAllowUnauth: true})
	if rec := debugPost(t, h, body, true); rec.Code != http.StatusUnauthorized {
		t.Fatalf("app token missing: %d", rec.Code)
	}
	tok, err := auth.Issue("sec", time.Hour, "a")
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest(http.MethodPost, "/api/debug/exec", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("X-Apidoc-Debug", "1")
	r.Header.Set("X-Apidoc-App-Token", tok)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	if rec.Code != http.StatusOK {
		t.Fatalf("app token valid: %d %s", rec.Code, rec.Body.String())
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./server -run TestDebugExec -v`
Expected: FAIL with 404 (route missing) and missing fields.

- [ ] **Step 3: Implement the endpoint**

In `server/server.go`, add `DebugAllowUnauth bool` to `Opts`, register the route:

```go
mux.HandleFunc("POST /api/debug/exec", h.debugExec)
```

Add the DTOs:

```go
type debugExecReq struct {
	ID       string            `json:"id"`
	Method   string            `json:"method"`
	URL      string            `json:"url"`
	Params   []debug.Value     `json:"params"`
	Body     []byte            `json:"body"`
	Encoding string            `json:"encoding"`
	Headers  map[string]string `json:"headers"`
	Options  map[string]string `json:"options"`
}

type debugExecResp struct {
	Status     string            `json:"status"`
	Headers    map[string]string `json:"headers,omitempty"`
	Body       []byte            `json:"body,omitempty"`
	Encoding   string            `json:"encoding,omitempty"`
	Events     []debug.Event     `json:"events,omitempty"`
	DurationMS int64             `json:"duration_ms"`
	Truncated  bool              `json:"truncated,omitempty"`
}
```

Add the handler:

```go
func (h *apiHandler) debugExec(w http.ResponseWriter, r *http.Request) {
	if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		errJSON(w, http.StatusUnsupportedMediaType, "content-type must be application/json")
		return
	}
	if r.Header.Get("X-Apidoc-Debug") != "1" {
		errJSON(w, http.StatusForbidden, "missing X-Apidoc-Debug header")
		return
	}
	var req debugExecReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errJSON(w, http.StatusBadRequest, "invalid body")
		return
	}
	a, ok := h.o.Store.Action(req.ID)
	if !ok {
		errJSON(w, http.StatusNotFound, "action not found")
		return
	}
	if !h.allowDebug(w, r, a) {
		return
	}
	d, ok := h.o.Debuggers[a.Protocol]
	if !ok {
		errJSON(w, http.StatusBadRequest, "no debugger for protocol "+a.Protocol)
		return
	}
	spec := d.Spec()
	if spec.Mode == debug.ModeBrowser {
		errJSON(w, http.StatusBadRequest, "protocol is browser-mode")
		return
	}
	method := a.Method
	if req.Method != "" && req.Method != a.Method {
		if !slices.Contains(spec.Actions, req.Method) {
			errJSON(w, http.StatusBadRequest, "action not allowed")
			return
		}
		method = req.Method
	}
	target := a.URL
	if req.URL != "" && req.URL != a.URL {
		if !spec.AllowTargetOverride {
			errJSON(w, http.StatusBadRequest, "target override not allowed")
			return
		}
		target = req.URL
	}
	allowed := map[string]bool{}
	for _, p := range mergeParams(h.o.GlobalParams, a.Params) {
		allowed[p.Name+"\x00"+p.In] = true
	}
	for _, p := range req.Params {
		if !allowed[p.Name+"\x00"+p.In] {
			errJSON(w, http.StatusBadRequest, "unknown param "+p.Name)
			return
		}
	}
	enc := req.Encoding
	if enc == "" {
		enc = "json"
	}
	if len(spec.Encodings) > 0 && !slices.Contains(spec.Encodings, enc) {
		errJSON(w, http.StatusBadRequest, "unsupported encoding "+enc)
		return
	}
	start := time.Now()
	resp, err := d.Exec(r.Context(), debug.Request{
		ActionID: a.ID, Method: method, URL: target, Params: req.Params,
		Body: req.Body, Encoding: enc, Headers: req.Headers, Options: req.Options,
	})
	if err != nil {
		errJSON(w, http.StatusBadGateway, err.Error())
		return
	}
	if resp == nil {
		errJSON(w, http.StatusBadGateway, "debugger returned nil response")
		return
	}
	dur := resp.Duration
	if dur <= 0 {
		dur = time.Since(start)
	}
	writeJSON(w, r, http.StatusOK, map[string]any{"code": 0, "msg": "ok", "data": debugExecResp{
		Status: resp.Status, Headers: resp.Headers, Body: resp.Body,
		Encoding: resp.Encoding, Events: resp.Events, DurationMS: dur.Milliseconds(),
	}})
}

func (h *apiHandler) allowDebug(w http.ResponseWriter, r *http.Request, a *model.Action) bool {
	if h.o.Auth.Enable && !validSession(h.o, r) {
		errJSON(w, http.StatusUnauthorized, "unauthorized")
		return false
	}
	if pw, protected := h.o.AppPWs[a.App]; protected && pw != "" {
		data, ok := auth.Verify(h.o.Auth.Secret, r.Header.Get("X-Apidoc-App-Token"))
		if !ok || data != a.App {
			errJSON(w, http.StatusUnauthorized, "app token required")
			return false
		}
		return true
	}
	if !h.o.Auth.Enable && !h.o.DebugAllowUnauth {
		errJSON(w, http.StatusForbidden, "debugging requires auth or DebugAllowUnauth")
		return false
	}
	return true
}
```

Add `X-Apidoc-Debug` to the `Access-Control-Allow-Headers` value in
`withCORS` (currently `"Content-Type, X-Apidoc-App-Token"`).

In `apidoc.go`'s `Handler`, pass:

```go
DebugAllowUnauth: s.cfg.DebugAllowUnauth,
```

- [ ] **Step 4: Run the server tests**

Run: `go test ./server`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add server/server.go server/debug_exec_test.go apidoc.go
git commit -m "feat: add protocol debug execution endpoint"
```

---

### Task 7: Bound debug execution resources

**Files:**
- Modify: `server/server.go`
- Modify: `server/debug_exec_test.go`
- Modify: `apidoc.go`

**Interfaces:**
- Produces: `Opts.DebugTimeout`, `Opts.DebugMaxConcurrent`; limits:
  body 1 MiB, response 4 MiB, 30s default timeout, 8 default concurrent,
  256 params, 64 headers/options, 1000 events.

- [ ] **Step 1: Write the failing tests**

Append to `server/debug_exec_test.go`:

```go
type blockingDebugger struct {
	started chan struct{}
	release chan struct{}
}

func (d *blockingDebugger) Protocol() string { return "mqtt" }
func (d *blockingDebugger) Spec() debug.Spec {
	return debug.Spec{Mode: debug.ModeServer, Actions: []string{"PUBLISH"}}
}
func (d *blockingDebugger) Exec(ctx context.Context, req debug.Request) (*debug.Response, error) {
	close(d.started)
	select {
	case <-d.release:
		return &debug.Response{Status: "OK"}, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func TestDebugExecTimeout(t *testing.T) {
	d := &blockingDebugger{started: make(chan struct{}), release: make(chan struct{})}
	h := Handler(Opts{Prefix: "/apidoc", Store: newDebugStore(),
		Debuggers: map[string]debug.Debugger{"mqtt": d},
		DebugAllowUnauth: true, DebugTimeout: 20 * time.Millisecond})
	id := store.ActionID("a", "v1", "PUBLISH", "mqtt://broker/topic")
	body := fmt.Sprintf(`{"id":%q,"method":"PUBLISH","url":"mqtt://broker/topic",`+
		`"params":[],"body":"","encoding":"json"}`, id)
	rec := debugPost(t, h, body, true)
	if rec.Code != http.StatusGatewayTimeout {
		t.Fatalf("timeout status = %d: %s", rec.Code, rec.Body.String())
	}
}

func TestDebugExecConcurrency(t *testing.T) {
	d := &blockingDebugger{started: make(chan struct{}), release: make(chan struct{})}
	h := Handler(Opts{Prefix: "/apidoc", Store: newDebugStore(),
		Debuggers: map[string]debug.Debugger{"mqtt": d},
		DebugAllowUnauth: true, DebugMaxConcurrent: 1})
	id := store.ActionID("a", "v1", "PUBLISH", "mqtt://broker/topic")
	body := fmt.Sprintf(`{"id":%q,"method":"PUBLISH","url":"mqtt://broker/topic",`+
		`"params":[],"body":"","encoding":"json"}`, id)

	first := make(chan int, 1)
	go func() { first <- debugPost(t, h, body, true).Code }()
	<-d.started
	if rec := debugPost(t, h, body, true); rec.Code != http.StatusTooManyRequests {
		t.Fatalf("second status = %d", rec.Code)
	}
	close(d.release)
	if code := <-first; code != http.StatusOK {
		t.Fatalf("first status = %d", code)
	}
}

func TestDebugExecCaps(t *testing.T) {
	spec := debug.Spec{Mode: debug.ModeServer, Actions: []string{"PUBLISH"}}
	large := &captureDebugger{spec: spec, resp: &debug.Response{
		Status: "OK", Body: make([]byte, maxDebugResponse+1),
	}}
	h := Handler(Opts{Prefix: "/apidoc", Store: newDebugStore(),
		Debuggers: map[string]debug.Debugger{"mqtt": large}, DebugAllowUnauth: true})
	id := store.ActionID("a", "v1", "PUBLISH", "mqtt://broker/topic")
	body := fmt.Sprintf(`{"id":%q,"method":"PUBLISH","url":"mqtt://broker/topic",`+
		`"params":[],"body":"","encoding":"json"}`, id)
	rec := debugPost(t, h, body, true)
	var out struct {
		Data struct {
			Body      []byte `json:"body"`
			Truncated bool   `json:"truncated"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if !out.Data.Truncated || len(out.Data.Body) != maxDebugResponse {
		t.Fatalf("response not capped: truncated=%v len=%d", out.Data.Truncated, len(out.Data.Body))
	}

	tooBig := fmt.Sprintf(`{"id":%q,"params":[{"name":"qos","in":"option","value":%q}]}`,
		id, strings.Repeat("x", maxDebugBody))
	if rec := debugPost(t, h, tooBig, true); rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("body cap status = %d", rec.Code)
	}
}

func TestDebugExecInputCaps(t *testing.T) {
	h := Handler(Opts{Prefix: "/apidoc", Store: newDebugStore(),
		Debuggers: map[string]debug.Debugger{"mqtt": &captureDebugger{
			spec: debug.Spec{Mode: debug.ModeServer, Actions: []string{"PUBLISH"}},
		}}, DebugAllowUnauth: true})
	id := store.ActionID("a", "v1", "PUBLISH", "mqtt://broker/topic")
	params := make([]debug.Value, maxDebugParams+1)
	for i := range params {
		params[i] = debug.Value{Name: "qos", In: "option", Value: "1"}
	}
	b, _ := json.Marshal(debugExecReq{ID: id, Method: "PUBLISH",
		URL: "mqtt://broker/topic", Params: params})
	if rec := debugPost(t, h, string(b), true); rec.Code != http.StatusBadRequest {
		t.Fatalf("param count cap status = %d", rec.Code)
	}
	long := strings.Repeat("x", maxDebugParamValue+1)
	b, _ = json.Marshal(debugExecReq{ID: id, Method: "PUBLISH",
		URL:    "mqtt://broker/topic",
		Params: []debug.Value{{Name: "qos", In: "option", Value: long}}})
	if rec := debugPost(t, h, string(b), true); rec.Code != http.StatusBadRequest {
		t.Fatalf("param value cap status = %d", rec.Code)
	}
}
```

Add `debug` and `store` imports as needed.

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./server -run 'TestDebugExecTimeout|TestDebugExecConcurrency|TestDebugExecCaps' -v`
Expected: FAIL (no timeout, no concurrency cap, no size cap).

- [ ] **Step 3: Implement the limits**

Add constants near the other server constants:

```go
const (
	maxDebugBody        = 1 << 20
	maxDebugResponse    = 4 << 20
	maxDebugParams      = 256
	maxDebugParamValue  = 64 << 10
	maxDebugHeaders     = 64
	maxDebugHeaderValue = 8 << 10
	maxDebugOptions     = 64
	maxDebugEvents      = 1000
)
```

Add `DebugTimeout time.Duration` and `DebugMaxConcurrent int` to `Opts`.
Add `sem chan struct{}` to `apiHandler`. In `Handler`:

```go
max := o.DebugMaxConcurrent
if max <= 0 {
	max = 8
}
h := &apiHandler{o: o, sem: make(chan struct{}, max)}
```

At the start of `debugExec`:

```go
select {
case h.sem <- struct{}{}:
	defer func() { <-h.sem }()
default:
	errJSON(w, http.StatusTooManyRequests, "too many concurrent debug requests")
	return
}
```

Wrap the body before decoding:

```go
r.Body = http.MaxBytesReader(w, r.Body, maxDebugBody)
if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
	var tooBig *http.MaxBytesError
	if errors.As(err, &tooBig) {
		errJSON(w, http.StatusRequestEntityTooLarge, "request body too large")
		return
	}
	errJSON(w, http.StatusBadRequest, "invalid body")
	return
}
```

After decode, before looking up the action:

```go
if len(req.Params) > maxDebugParams {
	errJSON(w, http.StatusBadRequest, "too many params")
	return
}
for _, p := range req.Params {
	if len(p.Value) > maxDebugParamValue {
		errJSON(w, http.StatusBadRequest, "param value too large")
		return
	}
}
if len(req.Headers) > maxDebugHeaders || len(req.Options) > maxDebugOptions {
	errJSON(w, http.StatusBadRequest, "too many headers or options")
	return
}
for k, v := range req.Headers {
	if len(k)+len(v) > maxDebugHeaderValue {
		errJSON(w, http.StatusBadRequest, "header too large")
		return
	}
}
for k, v := range req.Options {
	if len(k)+len(v) > maxDebugHeaderValue {
		errJSON(w, http.StatusBadRequest, "option too large")
		return
	}
}
```

Before `Exec`:

```go
timeout := h.o.DebugTimeout
if timeout <= 0 {
	timeout = 30 * time.Second
}
ctx, cancel := context.WithTimeout(r.Context(), timeout)
defer cancel()
resp, err := d.Exec(ctx, debug.Request{...})
if err != nil {
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		errJSON(w, http.StatusGatewayTimeout, "debug request timed out")
		return
	}
	errJSON(w, http.StatusBadGateway, err.Error())
	return
}
```

After `Exec`:

```go
truncated := false
body := resp.Body
if len(body) > maxDebugResponse {
	body = body[:maxDebugResponse]
	truncated = true
}
events := resp.Events
if len(events) > maxDebugEvents {
	events = events[:maxDebugEvents]
	truncated = true
}
```

Use `body`, `events`, `truncated` in `debugExecResp`.

Add `"errors"` and `"context"` to imports. In `apidoc.go`'s `Handler`, pass:

```go
DebugTimeout:       s.cfg.DebugTimeout,
DebugMaxConcurrent: s.cfg.DebugMaxConcurrent,
```

- [ ] **Step 4: Run the server tests**

Run: `go test ./server`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add server/server.go server/debug_exec_test.go apidoc.go
git commit -m "feat: bound debug execution resources"
```

---

### Task 8: Protocol-aware debug panel

**Files:**
- Create: `server/static/debug.js`
- Modify: `server/static/index.html`
- Modify: `server/server.go`
- Modify: `server/server_test.go`

**Interfaces:**
- Consumes: detail fields `protocol` and `debug_spec`.
- Produces: `GET /debug.js`; `{{PREFIX}}` substitution in the UI; server-mode
  panel that POSTs to `/api/debug/exec`.

- [ ] **Step 1: Write the failing tests**

Append to `server/server_test.go`:

```go
func TestDebugJSServedAndSafe(t *testing.T) {
	h := Handler(Opts{Prefix: "/apidoc", Store: newTestStore()})
	rec := req(t, h, http.MethodGet, "/debug.js", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("debug.js status %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/javascript") {
		t.Fatalf("debug.js content-type %q", ct)
	}
	if !strings.Contains(rec.Body.String(), "sendServerDebug") {
		t.Fatal("debug.js missing sendServerDebug")
	}
	if strings.Contains(rec.Body.String(), "innerHTML") {
		t.Fatal("debug.js must render adapter data with textContent, not innerHTML")
	}
}

func TestUIPrefixSubstitution(t *testing.T) {
	h := Handler(Opts{Prefix: "/x", Store: newTestStore()})
	rec := req(t, h, http.MethodGet, "/", "")
	if !strings.Contains(rec.Body.String(), `src="/x/debug.js"`) {
		t.Fatalf("UI does not reference /x/debug.js:\n%s", rec.Body.String())
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./server -run 'TestDebugJS|TestUIPrefix' -v`
Expected: FAIL with 404 for `/debug.js` and missing `{{PREFIX}}` substitution.

- [ ] **Step 3: Serve the script and substitute the prefix**

In `server/server.go`:

```go
var debugJSPayload = readUI("static/debug.js")
```

In `Handler`, before creating the handler:

```go
page := bytes.ReplaceAll(indexHTML, []byte("{{PREFIX}}"), []byte(html.EscapeString(o.Prefix)))
h := &apiHandler{o: o, page: page, sem: make(chan struct{}, max)}
```

Add `page []byte` to `apiHandler`, change `ui` to write `h.page`, and add:

```go
mux.HandleFunc("GET /debug.js", h.debugJS)
```

```go
func (h *apiHandler) debugJS(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=300")
	_, _ = w.Write(debugJSPayload)
}
```

Add `"html"` to imports.

- [ ] **Step 4: Update the HTML**

At the end of `index.html`, before the inline script, add:

```html
<script src="{{PREFIX}}/debug.js"></script>
```

Remove the existing `setupDebug` and `sendDebug` functions from the inline
script (they move to `debug.js`). Wrap the existing HTTP controls:

```html
<section id="debug" class="hidden">
  <p id="dg-unavailable" class="hidden"></p>
  <div id="dg-browser" class="hidden">
    <div class="row">
      <select id="dg-method">...</select>
      <input type="text" id="dg-url" placeholder="/api/users">
    </div>
    <div id="dg-params"></div>
  </div>
  <div id="dg-server" class="hidden">
    <div class="row">
      <select id="dg-action"></select>
      <input type="text" id="dg-target" placeholder="tcp://host:port">
    </div>
    <div id="dg-connect"></div>
    <div class="row"><select id="dg-encoding"></select></div>
    <div id="dg-s-params"></div>
    <textarea id="dg-body" rows="6" placeholder="message body"></textarea>
  </div>
  <button class="send" onclick="sendDebug()">Send</button>
  <div id="result"><div id="res-status" class="meta"></div><pre id="res-body"></pre></div>
</section>
```

Add CSS for `#dg-body` and a neutral protocol badge:

```css
.badge.proto{background:#57606a}
#dg-body{width:100%;margin:8px 0;padding:8px;border:1px solid #d0d7de;border-radius:5px;font:12px/1.5 ui-monospace,Menlo,monospace}
```

In `showDetail`, set the protocol badge:

```js
const proto = d.protocol || 'http';
const isHTTP = proto === 'http';
document.getElementById('d-method').textContent = isHTTP ? d.method : proto.toUpperCase();
document.getElementById('d-method').className = 'badge ' + (isHTTP ? d.method.toLowerCase() : 'proto');
document.getElementById('d-url').textContent = isHTTP ? d.url : d.method + ' ' + d.url;
```

- [ ] **Step 5: Implement `server/static/debug.js`**

Create the file with these functions. Every adapter-provided value must be
inserted with `textContent` or `createElement`; the file must not contain the
string `innerHTML`.

- `setupDebug(d)`: store `debugDoc = d`; read `d.debug_spec`; toggle
  `#dg-unavailable`, `#dg-browser`, `#dg-server`.
  - No spec: show `#dg-unavailable` with
    `"No debugger registered for protocol " + (d.protocol || 'http')`.
  - `spec.mode === 'browser'`: show `#dg-browser` and call
    `setupBrowserDebug(d)`.
  - Otherwise: show `#dg-server` and call `setupServerDebug(d, spec)`.
- `setupBrowserDebug(d)`: the existing HTTP debug setup (method select, URL
  input, one row per param, values from `mock`/`default`). Populate
  `#dg-method` from `spec.actions` when present, otherwise keep the existing
  options. Set `#dg-method` to `d.method` and `#dg-url` to `d.url`.
- `setupServerDebug(d, spec)`:
  - `#dg-target.value = d.url`; `disabled = !spec.allow_target_override`.
  - `#dg-action` options: `spec.actions` or `[d.method]`; select `d.method`.
  - `#dg-encoding` options: `spec.encodings` or `['json','text']`; select the
    first.
  - `#dg-connect`: one row per `spec.connect_fields`; input type from
    `field.type` (`password` for `secret`, `checkbox` for `bool`, `select`
    with `field.options`, `number`, `textarea`, default `text`); store
    `data-name`.
  - `#dg-s-params`: group `d.params` by `in`. Params whose `in` equals
    `spec.body_in || 'payload'` go to `#dg-body` (value `mock`/`default`,
    placeholder `desc`); if the param type is one of `spec.encodings`, select
    it in `#dg-encoding`. Other params become rows with `data-in` and
    `data-name`, type-aware as above.
- `sendDebug()`: return if `debugDoc` is null; if
  `debugDoc.debug_spec.mode === 'browser'` call `sendBrowserDebug()`, else
  `sendServerDebug()`.
- `sendBrowserDebug()`: existing logic, building query/header/path/body and
  using `fetch`; write the result to `#res-status`/`#res-body`.
- `sendServerDebug()`:
  - Collect params from `#dg-s-params` rows (`{name, in, value}`).
  - Read `#dg-body`; convert to bytes by `#dg-encoding`: `hex` pairs,
    `base64` via `atob`, otherwise `TextEncoder`.
  - Encode bytes as base64 (loop, no spread on large arrays).
  - `POST` to `base + '/api/debug/exec'` with headers
    `Content-Type: application/json`, `X-Apidoc-Debug: 1`, and
    `X-Apidoc-App-Token` when `appTokens[debugDoc.app]` exists.
  - Body: `{id: debugDoc.id, method, url, params, body, encoding, headers, options}`.
  - On non-200, show `data.msg`; on 200, call `renderDebugResponse(data.data)`.
- `renderDebugResponse(r)`:
  - Status line: `r.status + ' · ' + r.duration_ms + ' ms'`.
  - Headers: a table built with `textContent`.
  - Body: decode base64 to bytes; `hex`/`response_view==='hex'` → hex dump;
    otherwise `TextDecoder` and pretty-print JSON when the view/encoding is
    `json`; XML/text as-is.
  - Events: a table (time, direction, name, data) built with `textContent`.
  - Show a truncation note when `r.truncated`.
- `bytesToB64`, `b64ToBytes`, `hexDump`, `fieldInput` helpers.

- [ ] **Step 6: Run the server tests**

Run: `go test ./server`
Expected: PASS, including the pet icon test and the new asset tests.

- [ ] **Step 7: Commit**

```bash
git add server/static/debug.js server/static/index.html server/server.go server/server_test.go
git commit -m "feat: render protocol-aware debug panel"
```

---

### Task 9: TCP echo example and adapter test

**Files:**
- Create: `example/debugger/tcp.go`
- Create: `example/debugger/tcp_test.go`
- Modify: `example/main.go`

**Interfaces:**
- Produces: `debugger.TCP() apidoc.Debugger`; example endpoint
  `SEND tcp://127.0.0.1:19000` on the net/http demo (`:8081`).

- [ ] **Step 1: Write the failing adapter test**

Create `example/debugger/tcp_test.go`:

```go
package debugger

import (
	"context"
	"io"
	"net"
	"testing"

	"github.com/erikwang2013/apidoc-go"
)

func TestTCPDebuggerEcho(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		defer c.Close()
		_, _ = io.Copy(c, c)
	}()

	resp, err := TCP().Exec(context.Background(), apidoc.DebugRequest{
		URL:      "tcp://" + ln.Addr().String(),
		Body:     []byte("hello"),
		Encoding: "text",
		Options:  map[string]string{"timeout_ms": "100"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if string(resp.Body) != "hello" || resp.Status != "OK" {
		t.Fatalf("response = %+v", resp)
	}
}

func TestTCPDebuggerBadURL(t *testing.T) {
	if _, err := TCP().Exec(context.Background(), apidoc.DebugRequest{URL: "http://x"}); err == nil {
		t.Fatal("want error for non-tcp url")
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./example/debugger -v`
Expected: FAIL because `TCP` is undefined.

- [ ] **Step 3: Implement the adapter**

Create `example/debugger/tcp.go`:

```go
// Package debugger contains example debug adapters. They wrap plain
// standard-library clients; apidoc-go itself does not implement protocols.
package debugger

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"strconv"
	"time"

	"github.com/erikwang2013/apidoc-go"
)

// TCP returns a debugger that sends raw bytes to a TCP echo endpoint and
// returns everything received before the read deadline.
func TCP() apidoc.Debugger {
	return apidoc.DebuggerFunc("tcp", apidoc.DebugSpec{
		Mode:         "server",
		Actions:      []string{"SEND"},
		Encodings:    []string{"text", "hex", "base64"},
		BodyIn:       "payload",
		ResponseView: "text",
		ConnectFields: []apidoc.DebugField{
			{Name: "timeout_ms", Label: "Read timeout (ms)", Type: "number", Default: "500"},
		},
	}, func(ctx context.Context, req apidoc.DebugRequest) (*apidoc.DebugResponse, error) {
		u, err := url.Parse(req.URL)
		if err != nil || u.Scheme != "tcp" || u.Host == "" {
			return nil, fmt.Errorf("tcp: bad url %q", req.URL)
		}
		d := net.Dialer{Timeout: 2 * time.Second}
		conn, err := d.DialContext(ctx, "tcp", u.Host)
		if err != nil {
			return nil, err
		}
		defer conn.Close()

		timeout := 500 * time.Millisecond
		if v := req.Options["timeout_ms"]; v != "" {
			if ms, err := strconv.Atoi(v); err == nil && ms > 0 {
				timeout = time.Duration(ms) * time.Millisecond
			}
		}
		if timeout > 5*time.Second {
			timeout = 5 * time.Second
		}
		_ = conn.SetDeadline(time.Now().Add(timeout))
		if _, err := conn.Write(req.Body); err != nil {
			return nil, err
		}
		buf, err := io.ReadAll(conn)
		if err != nil && !errors.Is(err, os.ErrDeadlineExceeded) {
			return nil, err
		}
		return &apidoc.DebugResponse{
			Status: "OK", Body: buf, Encoding: req.Encoding,
			Headers: map[string]string{"remote": u.Host},
		}, nil
	})
}
```

- [ ] **Step 4: Run the adapter test**

Run: `go test ./example/debugger -v`
Expected: PASS.

- [ ] **Step 5: Wire the example**

In `example/main.go`:

- Add the import
  `"github.com/erikwang2013/apidoc-go/example/debugger"`.
- In `main`, add `go tcpEchoDemo()`.
- In `nethttpDemo`, change the `Config` to:

```go
s := apidoc.New(apidoc.Config{
	Prefix: "/apidoc", Title: "net/http demo",
	Debuggers:        []apidoc.Debugger{debugger.TCP()},
	DebugAllowUnauth: true, // demo only
})
```

- In `nethttpDemo`, after the health-check registration, add:

```go
must(s.Register(apidoc.Route{
	Method: "SEND",
	URL:    "tcp://127.0.0.1:19000",
	Doc: apidoc.Doc{
		Protocol: "tcp", Title: "TCP Echo", Author: "demo",
		Params: []apidoc.Param{{
			Name: "payload", In: "payload", Type: "text",
			Required: true, Desc: "发送内容",
		}},
		Responses: []apidoc.Response{{Name: "ok", Type: "string", Desc: "回显内容"}},
	},
}))
```

- Add the echo server:

```go
// tcpEchoDemo runs a raw TCP echo server for the example debugger.
func tcpEchoDemo() {
	ln, err := net.Listen("tcp", "127.0.0.1:19000")
	if err != nil {
		log.Printf("tcp echo: %v", err)
		return
	}
	for {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		go func() {
			defer c.Close()
			_, _ = io.Copy(c, c)
		}()
	}
}
```

Add `"io"` and `"net"` to `example/main.go` imports.

- [ ] **Step 6: Run the example tests and build**

Run: `go test ./example/... && go build ./...`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add example/debugger example/main.go
git commit -m "feat(example): add TCP echo debugger"
```

---

### Task 10: README and full verification

**Files:**
- Modify: `README.md`

**Interfaces:**
- Documents `@protocol`, `Debugger`, `Config.Debuggers`, and the security defaults.

- [ ] **Step 1: Update README**

Add a feature-table row:

```md
| 13 | 任意协议调试 | `@protocol` 声明 + `Debugger` 扩展，统一面板填参、发送、看响应 |
```

Add a section after the quick-start section:

````md
### 任意协议调试

非 HTTP 接口复用 `@method` + `@url`，用 `@protocol` 指定协议（也可从
URL scheme 推断）：

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

协议本身由使用者提供的 `Debugger` 执行，通常只是包装项目里已有的客户端：

```go
s := apidoc.New(apidoc.Config{
    Debuggers: []apidoc.Debugger{
        apidoc.DebuggerFunc("mqtt", apidoc.DebugSpec{
            Mode: "server", Actions: []string{"PUBLISH"},
        }, func(ctx context.Context, req apidoc.DebugRequest) (*apidoc.DebugResponse, error) {
            return mqttClient.Publish(ctx, req)
        }),
    },
})
```

`server` 模式需要鉴权；关闭全局鉴权时必须显式设置
`DebugAllowUnauth: true`。目标白名单由适配器负责。完整可运行示例见
`example/debugger/tcp.go`（net/http demo，`:8081`，TCP echo 在 `:19000`）。
````

- [ ] **Step 2: Run formatting, vet, tests, and build**

```bash
gofmt -l .
go vet ./...
go test ./...
go build ./...
```

Expected:
- `gofmt -l .` prints nothing.
- `go vet ./...` exits 0.
- `go test ./...` passes.
- `go build ./...` exits 0.

- [ ] **Step 3: Optional manual smoke test**

Run `go run ./example` in a background terminal, open
`http://localhost:8081/apidoc`, select **TCP Echo**, enter `hello`, click
**Send**, and confirm the response body is `hello`. Stop the process when done.

- [ ] **Step 4: Commit**

```bash
git add README.md
git commit -m "docs: document protocol debug extension"
```
