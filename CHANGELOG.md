# Changelog

本项目记录所有值得注意的变更。格式参考 [Keep a Changelog](https://keepachangelog.com/zh-CN/1.1.0/)，
版本号遵循语义化版本。

## [Unreleased]

## [v1.1.1] - 2026-10-27

### Fixed

- `/ui/v1/capabilities.version` now reports the actual apidoc-go module version
  from Go build info instead of always returning `dev`; development builds still
  fall back to `dev`. `Options.Version` can override it.
- Documentation and translated READMEs now describe UIC-1, server executors,
  nine definition formats and the optional Vue UI; 1.1.1 version references.

## [v1.1.0] - 2026-10-26

### Added

- **UIC-1 HTTP API**：`/ui/v1/capabilities`、`/config`、`/menus`、`/detail`、
  `/docMenus`、`/docDetail`、`/verifyAuth`、`/debugSend`、`/debugStart`、`/debugPoll`、
  `/debugStop`、分享（`addApiShare` / `getApiShareList` / `getApiShareDetail` /
  `deleteApiShare` / `handleApiShareAction`）、缓存管理、声明式生成器、代码模板与
  `exportAllData`；`capabilities` 按实际编译结果上报调试模式、分享、缓存、生成器、
  导出与鉴权能力。
- **服务端协议执行器（构建标签）**：HTTP/Web（http、http2、soap、graphql、jsonrpc、
  xmlrpc、odata、mcp、a2a、webhook）、HTTP/3、套接字（tcp、udp、raw、unix）、
  RPC/金融（grpc、thrift、dubbo、hessian、iso8583）、消息（mqtt、stomp、amqp、
  kafka、xmpp）、IoT（coap、lwm2m）、工业（modbus、dnp3、iec104、bacnet、opcua）
  与媒体（rtsp、sip、rtp、rtcp、rtmp）；支持一元与流式会话。默认构建不编译执行器，
  `apidoc_debug_all` 聚合全部，也可用 `apidoc_debug_http` / `_http3` / `_sockets` /
  `_rpc` / `_grpc` / `_messaging` / `_iot` / `_industrial` / `_media` 等标签按协议族
  裁剪，未编译的协议 fail closed。
- **九种协议定义解析**：`@definition` 注解支持 OpenAPI、AsyncAPI、GraphQL SDL、
  proto、WSDL、EDMX、JSON Schema、JSON-RPC 与 XML-RPC；自动探测格式、解析
  import / `$ref`、校验样例，结果随 UIC detail 返回；`Config.DefinitionRoot`
  限定可读文件根目录。
- **UIPath Vue UI**：`Config.UIPath` 指向 `apidoc-ui` 的 `dist/uic1-ui/` 打包产物，
  同一进程提供 UIC-1 Vue UI（SPA 兜底、静态资源 MIME、无尾斜杠 307、路径与符号
  链接限定）；留空保持纯 JSON API。
- 声明式生成器 `Generator`、代码模板 `CodeTemplates`、分享动作 `ShareActions`
  配置，以及 `PublicHost` / `AppConfig.PublicHost` 调试基地址。

### Changed

- `@protocol` 注解与 `/api/detail` 的 `protocol_info` 继续作为协议文档元数据；
  UIC-1 detail 额外返回解析后的 `definitions`、`validation`、调试模式与能力信息。
- 响应缓存覆盖旧版 `/api` 与 UIC-1；`/ui/v1/cache/clear`、`/ui/v1/cache/warm`
  可管理进程内缓存。
- README、架构/功能/生命周期/社交预览图与项目结构更新到 1.1.0；发布工作流改为
  读取根目录 `VERSION`，仅在 `v$VERSION` 不存在时创建 tag 与 GitHub Release。

### Security

- 服务端执行器默认关闭，目标先过 SSRF 守卫：`AllowHosts` 白名单、默认拒绝私网/
  回环/链路本地/CGNAT 与云元数据地址、DNS 解析后逐 IP 校验并钉住连接，禁止
  CONNECT、重定向与代理。
- 调试请求与响应限额：单消息 1 MiB、`MaxBytes`、超时、每作用域 4 个流会话、
  1000 帧、100 条/轮轮询与 60s 空闲回收；DNP3 单响应 10,000 点、IEC 104
  10,000 行上限。
- `@definition` 解析限制：单文件 1 MiB、5 万行、引用深度 32；仅读取
  `DefinitionRoot` 内普通文件，拒绝绝对路径、`..` 与越界符号链接。
- 鉴权能力如实上报：全局密码为空时不广告全局鉴权，应用鉴权按已配置密码上报。

## [v1.0.15] - 2026-10-25

### Changed

- 多语言 README 的项目结构补齐 `model/protocol.go` 与 `model/protocols.go`。
- README 恢复 `pet-expressions:start` 标记，宠物区块可再次由脚本生成。

## [v1.0.14] - 2026-10-25

### Changed

- 文档 API 响应缓存：`/api/menus`、`/api/protocols`、`/api/export`（JSON / TypeScript）
  的响应字节与 ETag 按 store 版本缓存，`Handler` 创建时预热，`Register` / `SetAppMeta`
  后自动失效。5000 个 action 的本机基准中 `/api/menus` 由约 84ms 降至 0.8ms，
  `/api/export` 由约 109ms 降至约 8ms（剩余成本为响应体写出）。
- `store.Store` 新增 `Version()`，写入时自增；缓存响应复用预计算 ETag。
- README、多语言 README 与架构/功能/生命周期/社交预览图同步响应缓存说明。

## [v1.0.13] - 2026-10-25

### Added

- 协议注册表 `GET /api/protocols`：key、名称、分类、scheme、别名、默认端口。
- `@protocol` 支持完整协议清单、别名与多词名称（HTTP/2、SOAP、gRPC、ISO 8583、
  OPC UA、WebRTC DataChannel、Modbus TCP、IEC 60870-5-104、BACnet/IP 等）。
- `/api/detail` 新增 `protocol_info`；二进制 body 用
  `@param ... type=binary|hex|base64 in=payload` 声明。

### Removed

- 内嵌 Web UI、`/api/debug/*` 执行与流式会话、`Debugger`/`StreamDebugger` SPI、
  TargetPolicy/审计、协议适配器示例模块、e2e 与示例测试脚本。
- 库现在只提供注解解析与文档 API（`/api/menus`、`/api/protocols`、`/api/detail`、
  `/api/export`）；文档渲染与测试请求由外部 UI 负责，库不实现任何协议或服务。

### Changed

- README、多语言 README、功能/生命周期/社交预览图对齐 API-only 范围；
  删除未引用的本地化 SVG 副本。

## [v1.0.9] - 2026-10-24

### Added

- **协议无关调试（P1）**
  - `model.Doc.Protocol` / `model.Action.Protocol` 与 `model.ProtocolOf`：从 URL
    scheme 推断协议；无 `://` 的地址（`/api/x`、`localhost:50051`）按 HTTP 处理。
  - `debug.Debugger` SPI（`Protocol` / `Spec` / `Exec`）与根包别名
    `apidoc.Debugger`、`apidoc.DebuggerFunc`、`apidoc.DebugSpec` 等。
  - `@protocol` 注解与任意 `in=` 参数分组（`payload`、`option`、自定义）。
  - `/api/detail` 新增 `protocol`、`debug_spec`；`/api/export` JSON 追加 `protocol`。
  - `POST /api/debug/exec`：绑定 action id，校验 method/target/param，限制请求体
    1 MiB、响应 4 MiB、默认 30s 超时、默认并发 8。
  - `GET /debug.js` 与数据驱动调试面板；适配器返回内容一律用 `textContent` 渲染。
- **流式调试（P2）**
  - `debug.StreamDebugger` / `debug.Session`，根包别名 `apidoc.StreamDebugger`、
    `apidoc.Session`；`Spec.Streaming` 标记流式面板。
  - `POST /api/debug/open`、`POST /api/debug/send`、`GET /api/debug/events`
    （SSE，15s 心跳）、`POST /api/debug/close`。
  - 会话管理：`crypto/rand` ID、默认最多 16 个会话、空闲 5 分钟、总时长
    30 分钟、每会话 256 条事件、单消息 1 MiB；客户端断开时关闭会话。
- **调试硬化（P3）**
  - `debug.Policy` 与 `Config.DebugPolicy`：`AllowHosts`（host[:port] 或 CIDR）、
    `AllowPrivate`、`AllowLoopback`、`AllowPublic`、`AllowPorts`；DNS 解析后逐
    IP 校验，exec/open 前执行。
  - `debug.AuditEvent` 与 `Config.DebugAudit`：记录动作、目标、状态、耗时、
    字节数与错误，不记录 params/body/options。
  - `Field.Secret`：仅渲染密码框，不持久化，审计脱敏。
  - `Spec.Snippets`：代码片段模板（`{{method}}`、`{{url}}`、`{{body}}`）与复制按钮。
  - `Field.Type == "file"`：文件/证书选择器，内容以 base64 写入 options；单文件
    256 KiB，整个请求仍受 1 MiB 限制。
  - `debug/debugtest`：`FakeAdapter`、`RunExec`、`RequireSpec` 适配器测试辅助。
  - `docs/protocols.md`：协议 URI 约定、注解与适配器骨架。
- 8 个框架适配器的路由测试：`/debug.js`、`/api/debug/exec`、`/api/debug/open`、
  `/api/debug/events` 均可达。

### Changed

- **不兼容行为**：`Protocol != "http"` 的路由只写入文档 store，不再转发给 HTTP
  框架适配器，`Handler` 可以为 `nil`。HTTP 路由的注册、文档、调试与 `ActionID`
  行为不变。
- `store.Register` 对显式 `Doc.Protocol` 做小写化与去空格；旧 store JSON 缺少
  `protocol` 时按 URL 回填。
- `Spec()` 必须保持确定性，因为 detail 响应参与 ETag 计算。
- 新增字段均为追加，现有 keyed struct literal 不受影响。

### Fixed

- 响应上限覆盖 `Status`、每个 `Event.Data` 与响应 headers。
- bool 默认值与空可选参数：checkbox 应用 mock/default；空的可选参数不发送。
- Content-Type 解析改用 `mime.ParseMediaType`，拒绝 `application/jsonfoo`。
- `sendServerDebug` 捕获网络异常并显示失败状态。
- 流式会话切换到 browser-mode action 后 Send 按钮恢复显示。
- 声明为 `file` 的字段中无效 base64 返回 400；超过 256 KiB 仍返回 413。
- SSE 断线后关闭 EventSource 并提示重新连接，不再对已移除 session 无限重试。
- `AllowPorts` 为常见协议补齐默认端口；无默认端口的 scheme 返回显式端口提示。
- `docs/protocols.md` 区分 JSON/CSRF 端点与 SSE events 端点。

### Security

- `server` 模式调试要求鉴权；关闭鉴权时必须显式设置 `DebugAllowUnauth: true`；
  `/api/debug/*` 要求 `X-Apidoc-Debug: 1` 与 JSON Content-Type（CSRF 防护）。
- 执行目标默认绑定已注册 action，只有 `Spec.AllowTargetOverride` 才允许改 URL。
- 库级 `DebugPolicy` 是兜底，适配器仍负责自己的目标策略。
- 文件上传、请求体、响应体、事件数量与并发均有上限。
- `debugOpen` 握手复用并发信号量；响应 header key 也有上限。
- 审计 Target 与 Policy 错误剥离 URL userinfo，避免凭据进入日志。
