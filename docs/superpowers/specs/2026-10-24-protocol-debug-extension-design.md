# 协议无关调试扩展设计

日期：2026-10-24
状态：P1 已获用户批准

## 意图

REST 之外（gRPC、WebSocket、MQTT、Modbus、TCP……）的接口也应能像 REST 一样：
用注解声明 → 文档页出现对应调试面板 → 填参数、发送、看响应。

核心库不实现任何协议栈。协议由使用者的 `Debugger` 适配器接入，适配器通常
只是包装项目里已有的客户端。库负责注解解析、模型、面板生成、鉴权、限额和
响应展示。

## 目标

- 任意协议都能用 `@protocol` + `@method` + `@url` 声明，进入同一套文档树。
- 文档页为每个接口渲染统一的数据驱动调试面板。
- 使用者注册 `Debugger` 后即可在页面上发请求、看响应。
- 现有 HTTP/REST 的注册、文档、调试行为与 ActionID 保持不变。
- 核心模块不新增第三方依赖。

## 非目标（P1）

- 不实现 gRPC/MQTT/Kafka/Modbus 等协议栈。
- 不做长连接/流式会话（P2 的 `StreamDebugger`）。
- 不做官方协议适配器包（P3 可选）。
- 不改变现有 HTTP 框架适配器的注册机制。

## 核心决策

1. **复用 `Method` + `URL`**
   - `Method` 是动作：`GET`、`PUBLISH`、`CALL`、`READ`。
   - `URL` 是协议地址：`mqtt://broker:1883/sensors/temp`、
     `grpc://host/pkg.Service/Method`、`tcp://host:port?name=login`。
   - 存储键仍是 `(app, version, method, url)`，去重逻辑与 ActionID 不变。
   - 资源标识放在 URL path/query 中，保证同一 broker/host 下的不同
     主题、RPC、寄存器拥有独立文档条目。

2. **新增可选 `Protocol`**
   - `model.Doc.Protocol` 与 `model.Action.Protocol`。
   - 为空时由 `model.ProtocolOf(url)` 从 scheme 推断：
     `http/https → http`、`ws/wss → ws`、`grpc/grpcs → grpc`、
     `mqtt/mqtts → mqtt`、`amqp/amqps → amqp`、其他 scheme 原样、
     无 `://`（如 `/api/x`、`localhost:50051`）→ `http`。
   - `@protocol` 显式覆盖，统一小写。
   - 旧序列化数据加载时回填 Protocol。

3. **`Debugger` SPI**
   - `Protocol() string`
   - `Spec() Spec`：决定面板字段、动作、编码、连接字段、响应视图。
   - `Exec(ctx, Request) (*Response, error)`：一次性请求/响应。
   - `debug.Func(protocol, spec, fn)` 方便用函数实现。
   - `Config.Debuggers` 注册；核心内置 HTTP 的 browser 模式 Spec，
     不内置任何实际协议 Exec。

4. **两种执行模式**
   - `browser`：浏览器直接 `fetch`，沿用现有 HTTP 调试，无 SSRF。
   - `server`：`POST /api/debug/exec` 交给注册的 Debugger 执行。
   - Spec 未写 Mode 时按 `server` 处理。

5. **非 HTTP 不转发**
   - `Server.Register` 对 `Protocol != "http"` 只写 store，不调用
     `adapter.Framework.Register`，`Handler` 可为 nil。
   - HTTP 路径保持原行为。

6. **安全默认**
   - `server` 模式必须鉴权：`Auth.Enable` 时要求会话；关闭鉴权时必须显式
     `Config.DebugAllowUnauth = true` 才允许。
   - `/api/debug/exec` 要求 `Content-Type: application/json` 与
     `X-Apidoc-Debug: 1` 头（CSRF 防护）。
   - 执行目标绑定到已注册的 action：默认不允许改 URL，只有
     `Spec.AllowTargetOverride` 才允许；动作必须在 `Spec.Actions` 内。
   - 请求体 1 MiB、响应体 4 MiB、30s 超时、并发 8 的库级兜底。
   - 目标白名单由适配器负责；P3 提供 `TargetPolicy` 辅助。
   - 库不自动加载适配器，不做远程代码加载。

## 数据模型

```go
type Doc struct {
    // 既有字段...
    Protocol string // 空 = 从 URL 推断
}

type Action struct {
    // 既有字段...
    Protocol string `json:"protocol,omitempty"`
}
```

`Param.In` 从固定四类放开为任意分组（`option`、`payload`、
`metadata`、自定义）。HTTP 继续使用 `query/header/path/body`；
非 HTTP 用 `payload` 表示原始消息体，用 `option` 表示协议控制项。

## 调试接口

```go
type Debugger interface {
    Protocol() string
    Spec() Spec
    Exec(ctx context.Context, req Request) (*Response, error)
}

type Request struct {
    ActionID string
    Method   string
    URL      string
    Params   []Value
    Body     []byte
    Encoding string
    Headers  map[string]string
    Options  map[string]string
}

type Response struct {
    Status   string
    Headers  map[string]string
    Body     []byte
    Encoding string
    Events   []Event
    Duration time.Duration
}

type Spec struct {
    Mode          string   // "browser" | "server"
    Actions       []string
    Encodings     []string
    ConnectFields []Field
    BodyIn        string   // 默认 "payload"
    ResponseView  string   // auto/json/xml/text/hex/transcript
    AllowTargetOverride bool
}
```

## HTTP API

- `GET /api/detail` 增加 `protocol` 与 `debug_spec`（未注册适配器时为 null；
  http 始终有 browser Spec）。
- `POST /api/debug/exec`
  - 请求：`{id, method, url, params, body(base64), encoding, headers, options}`
  - 响应：`{code, msg, data:{status, headers, body(base64), encoding,
    events, duration_ms, truncated}}`
  - 错误：400/401/403/404/413/415/429/502/504。
- 静态资源 `GET /debug.js`；HTML 中的 `{{PREFIX}}` 由服务端替换为挂载前缀。

## UI

一个面板，按 `debug_spec` 渲染：

1. 地址（`@url`，按 `AllowTargetOverride` 决定是否可编辑）
2. 动作（`Spec.Actions` 或 `@method`）
3. 连接字段（`Spec.ConnectFields`）
4. 参数（按 `in` 分组；type 决定 text/number/bool/select/textarea）
5. 消息体编辑器（JSON/XML/text/hex/base64）
6. 发送
7. 响应：状态、耗时、headers、body、events、截断提示

未注册适配器：显示“该协议未配置调试适配器”，文档照常展示。
所有适配器返回内容用 `textContent` 渲染，禁止 `innerHTML`。

## 协议覆盖方式

不逐个实现；按 scheme + 适配器接入：

| 协议族 | 适配器职责 | P1 |
|---|---|---|
| REST/SOAP/GraphQL/JSON-RPC/XML-RPC/OData/Webhooks/HTTP2/3 | HTTP 内置 browser 模式 | 是 |
| gRPC/Dubbo/Thrift/Hessian | 包装已有客户端，消息转换 | 一元请求 |
| WS/SSE/Socket.IO/WebRTC DataChannel/MCP/A2A | 连接、发送、收事件 | 一次性收发 |
| MQTT/AMQP/STOMP/Kafka | publish/produce | 发布 |
| TCP/UDP/UDS/CoAP/LwM2M/OPC UA/Modbus/DNP3/IEC104/BACnet/ISO8583 | 字节/帧收发 | 请求响应 |
| XMPP/SIP/RTSP/RTMP/HLS/RTP/RTCP | 信令/拉流/记录 | 文档 + 简单请求 |

P2 增加长连接/流式、订阅/消费、字段组布局；P3 增加 TargetPolicy、
适配器测试工具包与可选官方适配器包。

## 兼容性

- `ActionID` 不变；HTTP 文档链接与 ETag 行为不变。
- `/api/export` 的 JSON 新增 `protocol` 字段；TypeScript 导出逻辑不变。
- 新增字段均为追加，现有 keyed struct literal 不受影响。
- 旧 store JSON 无 `protocol` 时按 URL 回填。

## 风险

- `server` 模式重新引入 SSRF 面：必须鉴权 + 目标策略 + 限额，不能默认暴露公网。
- 适配器返回的错误可能包含敏感连接信息；不回显到日志。
- `Spec()` 应保持确定性，因为 detail 响应参与 ETag 计算。
