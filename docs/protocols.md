# 协议调试接入约定

apidoc-go 核心不包含任何协议栈。非 HTTP 接口通过 `@protocol` + `@method` +
`@url` 声明，由使用者注册的 `Debugger`（请求/响应）或 `StreamDebugger`
（长连接/流式）执行。本文约定协议 URI、注解写法与适配器骨架。

## 1. 声明模型

- `Method` 是动作：`GET`、`PUBLISH`、`CALL`、`READ`、`SEND`、`SUBSCRIBE` 等。
- `URL` 是协议地址，scheme 标识协议；资源标识放在 path/query，使同一
  broker/host 下的不同主题、RPC、寄存器拥有独立文档条目。
- `Protocol` 可选；为空时由 `model.ProtocolOf(url)` 从 scheme 推断，显式
  `@protocol` 覆盖推断结果，统一小写。
- 存储键仍是 `(app, version, method, url)`，`ActionID` 不变。
- `in=payload` 表示原始消息体，`in=option` 表示协议控制项；允许自定义分组。
  HTTP 继续使用 `query/header/path/body`。
- 原始字节协议用 `encoding` 指定 `text`、`hex` 或 `base64`。
- `Protocol != "http"` 的路由只写入文档 store，不会转发给 HTTP 框架适配器，
  `Handler` 可以为 `nil`。

> 没有 `://` 的地址（`/api/users`、`localhost:50051`）一律推断为 `http`。

## 2. Scheme 与协议键

| Scheme | 协议键 | 示例 |
|--------|--------|------|
| `http`, `https` | `http` | `https://api.example.com/users` |
| `ws`, `wss` | `ws` | `wss://host/socket` |
| `sse` | `sse` | `sse://host/events` |
| `socketio` | `socketio` | `socketio://host/namespace` |
| `webrtc` | `webrtc` | `webrtc://host/room` |
| `mcp` | `mcp` | `mcp://host/rpc` |
| `a2a` | `a2a` | `a2a://host/agent` |
| `grpc`, `grpcs` | `grpc` | `grpc://host/pkg.Service/Method` |
| `dubbo` | `dubbo` | `dubbo://host/com.example.Service/method` |
| `thrift` | `thrift` | `thrift://host/Service/method` |
| `hessian` | `hessian` | `hessian://host/Service/method` |
| `jsonrpc` | `jsonrpc` | `jsonrpc://host/rpc` |
| `xmlrpc` | `xmlrpc` | `xmlrpc://host/rpc` |
| `mqtt`, `mqtts` | `mqtt` | `mqtt://broker:1883/sensors/temp` |
| `kafka` | `kafka` | `kafka://broker:9092/topic` |
| `amqp`, `amqps` | `amqp` | `amqp://broker/queue` |
| `stomp`, `stomps` | `stomp` | `stomp://broker/queue` |
| `tcp` | `tcp` | `tcp://host:9000?name=login` |
| `udp` | `udp` | `udp://host:9001?name=metrics` |
| `unix` | `unix` | `unix:///tmp/app.sock?name=login` |
| `modbus` | `modbus` | `modbus://host:502?unit=1&register=40001` |
| `iso8583` | `iso8583` | `iso8583://host:7000?mti=0200` |
| `coap`, `coaps` | `coap` | `coap://host/resource` |
| `lwm2m` | `lwm2m` | `lwm2m://host:5683/3303/0/5700` |
| `opcua` | `opcua` | `opcua://host:4840/ns=2;s=Temp` |
| `dnp3` | `dnp3` | `dnp3://host:20000?point=1` |
| `iec104` | `iec104` | `iec104://host:2404?common=1` |
| `bacnet` | `bacnet` | `bacnet://host:47808?object=analog-input,1` |
| `xmpp` | `xmpp` | `xmpp://host:5222/resource` |
| `sip` | `sip` | `sip://user@host:5060` |
| `rtsp` | `rtsp` | `rtsp://host/stream` |
| `rtmp` | `rtmp` | `rtmp://host/live/stream` |
| `hls` | `hls` | `hls://host/live.m3u8` |
| `rtp` | `rtp` | `rtp://host:5004?payload=96` |
| `rtcp` | `rtcp` | `rtcp://host:5005?payload=96` |

约定：

- 表中未列出的 scheme 原样作为协议键（小写），注册同名 `Debugger` 即可。
- 安全变体若未被推断表归并（如 `sses://` → `sses`、`webrtcs://` → `webrtcs`），
  需显式写 `@protocol sse` / `@protocol webrtc` 指定适配器键。
- SOAP、GraphQL、OData、Webhooks、HTTP/2、HTTP/3 都走 `http` 键和浏览器模式。
- 需要覆盖推断或使用自定义 scheme 时写 `@protocol`，例如
  `@protocol myproto` + `@url myproto://host/resource`。

## 3. 注解示例

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

```go
// @apidoc
// @method CALL
// @url grpc://localhost:50051/pkg.Weather/Get
// @title 查询天气
// @param city string true "城市" in=payload
// @param timeout_ms int false "超时" in=option default=3000
func GetWeather() {}
```

```go
// @apidoc
// @method SEND
// @url tcp://127.0.0.1:9000?name=login
// @title 登录帧
// @param payload hex true "帧内容" in=payload
func LoginFrame() {}
```

流式协议使用同样的注解；面板形态由 `Spec.Streaming` / `ResponseView`
决定，不需要额外注解。

## 4. `Debugger` 骨架（请求/响应）

```go
package mqttdebug

import (
    "context"

    "github.com/erikwang2013/apidoc-go"
)

type Debugger struct{ client *mqtt.Client }

func New(c *mqtt.Client) apidoc.Debugger { return &Debugger{client: c} }

func (d *Debugger) Protocol() string { return "mqtt" }

func (d *Debugger) Spec() apidoc.DebugSpec {
    return apidoc.DebugSpec{
        Mode:      "server",
        Actions:   []string{"PUBLISH"},
        Encodings: []string{"json", "text"},
        BodyIn:    "payload",
        Snippets: map[string]string{
            "Go":  "client.Publish({{url}}, {{body}})",
            "CLI": "mosquitto_pub -t <topic> -m '{{body}}'",
        },
    }
}

func (d *Debugger) Exec(ctx context.Context, req apidoc.DebugRequest) (*apidoc.DebugResponse, error) {
    // 适配器仍负责自己的目标校验；Config.DebugPolicy 是库级兜底。
    resp, err := d.client.Publish(ctx, req.URL, req.Body)
    if err != nil {
        return nil, err
    }
    return &apidoc.DebugResponse{
        Status:   "PUBACK",
        Body:     resp.Payload,
        Encoding: req.Encoding,
    }, nil
}
```

`Spec` 字段：

| 字段 | 说明 |
|------|------|
| `Mode` | `browser`（浏览器直接执行，仅内置 HTTP）或 `server`（`/api/debug/exec`）；空值按 `server` |
| `Actions` | 允许的动作列表；请求的 method 必须在其中 |
| `Encodings` | 允许的消息编码：`json`、`xml`、`text`、`hex`、`base64` |
| `ConnectFields` | 面板顶部的连接字段（host、token、TLS 等） |
| `BodyIn` | 哪个 `in` 分组渲染为消息体编辑器，默认 `payload` |
| `ResponseView` | `auto`、`http`、`json`、`xml`、`text`、`hex`、`transcript` |
| `AllowTargetOverride` | 是否允许面板修改已注册的目标地址 |
| `Streaming` | 是否使用 `StreamDebugger` 会话 |
| `Snippets` | 代码片段模板，占位符 `{{method}}`、`{{url}}`、`{{body}}` |

## 5. `StreamDebugger` 骨架（长连接/流式）

```go
package wsdebug

import (
    "context"
    "errors"

    "github.com/erikwang2013/apidoc-go"
    "github.com/gorilla/websocket"
)

type StreamDebugger struct{ dialer *websocket.Dialer }

func New(d *websocket.Dialer) apidoc.StreamDebugger { return &StreamDebugger{dialer: d} }

func (d *StreamDebugger) Protocol() string { return "ws" }

func (d *StreamDebugger) Spec() apidoc.DebugSpec {
    return apidoc.DebugSpec{
        Mode:         "server",
        Streaming:    true,
        Actions:      []string{"SEND"},
        Encodings:    []string{"text", "json", "base64"},
        ResponseView: "transcript",
    }
}

// Exec 用于一次性请求；流式会话走 Open。
func (d *StreamDebugger) Exec(ctx context.Context, req apidoc.DebugRequest) (*apidoc.DebugResponse, error) {
    return nil, errors.New("ws: use Open for streaming")
}

func (d *StreamDebugger) Open(ctx context.Context, req apidoc.DebugRequest) (apidoc.Session, error) {
    conn, err := d.dialer.DialContext(ctx, req.URL, nil)
    if err != nil {
        return nil, err
    }
    ctx, cancel := context.WithCancel(ctx)
    s := &session{conn: conn, events: make(chan apidoc.DebugEvent, 256), cancel: cancel}
    go s.readLoop(ctx)
    return s, nil
}

type session struct {
    conn   *websocket.Conn
    events chan apidoc.DebugEvent
    cancel context.CancelFunc
}

func (s *session) Send(ctx context.Context, msg []byte, meta map[string]string) error {
    if err := ctx.Err(); err != nil {
        return err
    }
    return s.conn.WriteMessage(websocket.TextMessage, msg)
}

func (s *session) Events() <-chan apidoc.DebugEvent { return s.events }

func (s *session) Close() error {
    s.cancel()
    return s.conn.Close()
}

func (s *session) readLoop(ctx context.Context) {
    defer close(s.events)
    for {
        _, msg, err := s.conn.ReadMessage()
        if err != nil {
            return
        }
        select {
        case s.events <- apidoc.DebugEvent{Direction: "in", Data: string(msg), Encoding: "text"}:
        case <-ctx.Done():
            return
        }
    }
}
```

`Open` 的 ctx 只覆盖建立连接（拨号/握手）；返回的 `Session` 必须在此后继续可用，
生命周期独立于该 ctx，直到调用 `Close`。服务端会为会话单独维护超时；适配器不要
把 `Open` 的 ctx 当成会话有效期，也不要因它取消而丢弃仍然有效的连接。`Send`
使用调用方传入的 ctx。

会话端点与限额：

- `POST /api/debug/open` → `{session_id}`（仅 `StreamDebugger`）
- `POST /api/debug/send` → `{session_id, body(base64), encoding, meta}`
- `GET /api/debug/events?session_id=` → SSE，`data: <event-json>`，15s 心跳；
  受保护应用可追加 `&app_token=...`（EventSource 无法设置请求头）
- `POST /api/debug/close` → `{session_id}`

默认最多 16 个会话、空闲 5 分钟、总时长 30 分钟、每会话 256 条事件、单消息
1 MiB；客户端断开时关闭会话。`Send` 必须响应自己的 ctx；会话生命周期见上。

`POST /api/debug/open|send|close` 使用 JSON + `X-Apidoc-Debug: 1`，复用全局/应用
token、action id 绑定，以及 method/target/param 白名单。`GET /api/debug/events`
是 SSE：EventSource 不能设置自定义请求头，因此用会话 cookie 或 `app_token`
查询参数认证，不要求 `X-Apidoc-Debug` 或 JSON Content-Type。`server` 模式在
`Auth.Enable` 下要求会话，关闭鉴权时必须显式 `DebugAllowUnauth: true`。

## 6. TargetPolicy 与审计

```go
cfg := apidoc.Config{
    DebugPolicy: &debug.Policy{
        AllowHosts:   []string{"broker.internal:1883", "10.0.0.0/8"},
        AllowPrivate: true,
        AllowPorts:   []int{1883, 8883},
    },
    DebugAudit: func(e debug.AuditEvent) {
        log.Printf("debug app=%s action=%s protocol=%s status=%s target=%s dur=%s bytes=%d err=%v",
            e.App, e.ActionID, e.Protocol, e.Status, e.Target, e.Duration, e.Bytes, e.Error)
    },
}
```

- `Policy.Check(ctx, rawURL)` 先解析 DNS，再逐 IP 校验 host[:port]/CIDR、
  `AllowPrivate`、`AllowLoopback`、`AllowPublic`、`AllowPorts`；exec/open 前执行。
- `AllowPorts` 使用 URL 显式端口；省略端口时按常见 scheme 的默认端口补齐
  （http/https/ws/wss/mqtt/amqp/kafka/stomp/grpc/coap/opcua/modbus/sip/rtsp/
  xmpp 等）。没有默认端口的 scheme（如 tcp/udp/rtp/iso8583）在设置
  `AllowPorts` 时必须显式写端口，否则返回 `requires an explicit port`。
- 库级策略是兜底，适配器必须继续校验自己的目标。
- `AuditEvent` 只含 `Time`、`ActionID`、`App`、`Protocol`、`Method`、`Target`、
  `Status`、`Duration`、`Bytes`、`Error`；绝不记录 params、body、options。

## 7. 密钥、文件与证书

- `Field.Secret` 只影响渲染：显示密码框，不写入 localStorage，审计中脱敏。
- `Field.Type == "file"` 渲染文件选择器；文件内容以 base64 写入
  `options[field.Name]`。单文件 256 KiB，整个请求仍受 1 MiB 上限约束。
- 证书上传复用 file 字段，由适配器把 base64 解码后交给 TLS 配置。

## 8. 示例模块约定

协议适配器放在独立嵌套模块 `example/debugger-<family>/`，核心模块不新增依赖：

```
example/debugger-<family>/
├── go.mod
└── <protocol>/
    ├── <protocol>.go
    └── <protocol>_test.go
```

```go
// go.mod
module github.com/erikwang2013/apidoc-go/example/debugger-<family>

go 1.24.2

require github.com/erikwang2013/apidoc-go v0.0.0

replace github.com/erikwang2013/apidoc-go => ../..
```

- 每个协议一个子包，导出 `New() apidoc.Debugger` 或 `New() apidoc.StreamDebugger`。
- 必须有本地 fake/echo server 测试，不依赖公网。
- 依赖只放在该模块，不修改根 `go.mod` / `go.sum`。
- 库缺失或平台不支持时报告 blocker，不写假测试。

按协议族划分如下；尚未落地的模块按同一约定添加：

| 模块 | 协议 |
|------|------|
| `debugger-web` | WebSocket、SSE、Socket.IO、WebRTC DataChannel、MCP、A2A |
| `debugger-messaging` | MQTT、Kafka、AMQP、STOMP |
| `debugger-rpc` | gRPC、Dubbo、Thrift、Hessian、JSON-RPC、XML-RPC |
| `debugger-industrial` | TCP、UDP、Unix Domain Socket、Modbus TCP、ISO 8583、CoAP、LwM2M、OPC UA、DNP3、IEC 60870-5-104、BACnet/IP |
| `debugger-telecom` | XMPP、SIP、RTSP、RTMP、HLS、RTP、RTCP |
| `debugger-http` | SOAP、GraphQL、OData、Webhooks、HTTP/2、HTTP/3 |

流式协议用 `StreamDebugger`；请求/响应协议用 `Debugger`；原始字节协议用
`hex`/`base64` 编码。

示例适配器限制：

- SSE：只接收（receive-only），面板不向 SSE 连接发送消息。
- Socket.IO：二进制附件以 base64 展示，但不会以二进制发送。
- WebRTC：需要宿主应用提供 HTTP SDP 交换端点；仅支持 STUN，不支持 TURN 与
  trickle ICE。
- MCP / A2A：均为一次性 `Debugger` 实现；A2A 的流式响应会收集到
  `Response.Events` 后一次性返回。

## 9. 适配器测试工具包

`debug/debugtest` 提供 `FakeAdapter`、`RunExec`、`RequireSpec` 等辅助，用于
在不启动文档服务器的情况下验证：

- `Spec()` 的动作、编码、连接字段与确定性（detail 响应参与 ETag）。
- `Exec`/`Open` 对 ctx 取消、超时与错误路径的处理。
- 请求参数、body、options 是否按约定映射到协议客户端。

配合本地 fake/echo server 即可覆盖端到端路径，示例见
`example/debugger/tcp_test.go`。
