# 协议调试 P2/P3 设计

日期：2026-10-24
状态：用户要求 P2、P3、Minor 修复、工程收尾全部完成

## 范围

1. **P2 流式**：长连接会话、SSE 事件流、实时 transcript UI、帧/字段布局基础。
2. **P3 硬化**：TargetPolicy、审计日志、Secret/files、代码片段、适配器测试工具包。
3. **Minor**：P1 最终审查记录的 5 项。
4. **适配器与收尾**：用户清单中的协议示例适配器、8 框架路由 E2E、浏览器交互测试、i18n/README/release note、push/PR。

协议栈不进入核心模块。所有协议适配器放在 `example/debugger-<family>/` 独立嵌套模块，核心保持零新增依赖。

## P2 接口

```go
// debug/stream.go
type StreamDebugger interface {
    Debugger
    Open(ctx context.Context, req Request) (Session, error)
}

type Session interface {
    Send(ctx context.Context, msg []byte, meta map[string]string) error
    Events() <-chan Event
    Close() error
}
```

`Spec` 新增：

```go
Streaming bool              `json:"streaming,omitempty"`
Snippets  map[string]string `json:"snippets,omitempty"` // 模板：{{method}} {{url}} {{body}}
```

根包别名：`apidoc.StreamDebugger`、`apidoc.Session`。

## P2 HTTP API

- `POST /api/debug/open`：与 exec 相同请求体，返回 `{session_id}`；仅 `StreamDebugger` 可用。
- `POST /api/debug/send`：`{session_id, body(base64), encoding, meta}`。
- `GET /api/debug/events?session_id=`：SSE，`data: <event-json>`，15s 心跳注释。
- `POST /api/debug/close`：`{session_id}`。
- 所有端点复用 `X-Apidoc-Debug: 1`、`Content-Type: application/json`、全局/app token、action id 绑定、method/target/param 校验。
- 会话管理器：crypto/rand ID；最大会话数（默认 16）、空闲超时（5m）、总时长（30m）、每会话队列 256 事件、单消息 1 MiB；客户端断开时关闭会话。
- SSE 必须 flush；适配器的 `Open`/`Send`/`Events`/`Close` 必须响应 ctx。

## P3

- `debug.Policy`：`AllowHosts`（host[:port] 或 CIDR）、`AllowPrivate`、`AllowLoopback`、`AllowPublic`、`AllowPorts`；`Check(ctx, rawURL) error` 解析 DNS 后逐 IP 校验。`Config.DebugPolicy *debug.Policy`；exec/open 前检查目标。适配器仍负责自己的目标策略。
- 审计：`debug.AuditEvent{Time, ActionID, App, Protocol, Method, Target, Status, Duration, Bytes, Error}`；`Config.DebugAudit func(AuditEvent)`。绝不记录 params/body/options。
- Secret：`Field.Secret` 只渲染 password；不持久化；审计脱敏。
- 代码片段：`Spec.Snippets` 在 UI 中渲染 + 复制按钮，模板占位符 `{{method}}`、`{{url}}`、`{{body}}`。
- 文件/证书：`Field.Type=="file"` 渲染 file input，读为 base64 放入 options；单文件 256 KiB，总请求仍受 1 MiB 限制。
- `debug/debugtest`：`FakeAdapter`、`RunExec`、`RequireSpec` 等测试辅助。
- 文档：`docs/protocols.md` 记录协议 URI 约定与适配器骨架。

## 5 个 Minor

1. 响应上限覆盖 `Status`、每个 `Event.Data`、响应 headers。
2. bool 默认值与空可选参数：checkbox 应用 mock/default；空的可选参数不发送。
3. `store.Register` 对显式 `Doc.Protocol` 做 lowercase/trim。
4. Content-Type 用 `mime.ParseMediaType`，拒绝 `application/jsonfoo`。
5. `sendServerDebug` 捕获网络异常并显示失败状态。

## 示例适配器模块

目录：`example/debugger-<family>/`，每个模块有自己的 `go.mod`：

```
module github.com/erikwang2013/apidoc-go/example/debugger-<family>
require github.com/erikwang2013/apidoc-go v0.0.0
replace github.com/erikwang2013/apidoc-go => ../..
```

约定：
- 每个协议一个子包，导出 `New() apidoc.Debugger` 或 `New() apidoc.StreamDebugger`。
- 必须有本地 fake/echo server 测试；不依赖公网。
- 依赖只放在该模块；不得修改根 `go.mod`/`go.sum`。
- 库缺失或平台不支持时，报告 blocker，不做假测试。

协议分配：
- `debugger-web`：WebSocket、SSE、Socket.IO、WebRTC DataChannel、MCP、A2A。
- `debugger-messaging`：MQTT、Kafka、AMQP、STOMP。
- `debugger-rpc`：gRPC、Dubbo、Thrift、Hessian、JSON-RPC、XML-RPC。
- `debugger-industrial`：TCP、UDP、Unix Domain Socket、Modbus TCP、ISO 8583、CoAP、LwM2M、OPC UA、DNP3、IEC 60870-5-104、BACnet/IP。
- `debugger-telecom`：XMPP、SIP、RTSP、RTMP、HLS、RTP、RTCP。
- `debugger-http`：SOAP、GraphQL、OData、Webhooks、HTTP/2、HTTP/3。

流式协议用 `StreamDebugger`；请求/响应协议用 `Debugger`。原始字节协议用 hex/base64 编码。

## 工程收尾

- `adapter/debug_routes_test.go`：8 个框架都能访问 `/debug.js`、`/api/debug/exec`、`/api/debug/open`、`/api/debug/events`（至少路由可达）。
- 浏览器 E2E：`e2e/` 独立模块，驱动真实浏览器完成“打开文档 → 切 Debug → 填参 → 发送 → 看到响应”；无浏览器则报告 blocker。
- README：修正“无 SSRF”表述；补 P2/P3 功能；`docs/protocols.md`；`CHANGELOG.md` 加 Unreleased 兼容性说明（非 HTTP 路由不再转发 HTTP handler）。
- i18n：`docs/i18n/*/README.md` 同步功能表与新章节。
- 最后 push 分支并创建 PR 到 `main`。

## 团队规则

- 分支 `feat/protocol-debug-p2-p3`；**不要 git add/commit/branch/switch**，lead 统一提交。
- 只改自己 ownership 内的文件；需要核心改动时 SendMessage 给 `core-p2` 或 `team-lead`。
- 不要 spawn 子代理。
- 每个任务先写失败测试，再实现；报告测试命令、结果、blocker。
- 完成后 SendMessage 给 `team-lead`，附：文件、测试结果、风险/blocker。

## 验收

- 根模块 `go test ./...`、`go vet ./...`、`go build ./...` 全绿。
- 每个示例模块 `go test ./...` 全绿，或有明确 blocker 报告。
- 8 框架路由测试通过。
- 浏览器 E2E 通过，或明确记录环境 blocker。
- 最终 reviewer 无 Critical/Important 未解决项。
