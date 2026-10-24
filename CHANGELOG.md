# Changelog

本项目记录所有值得注意的变更。格式参考 [Keep a Changelog](https://keepachangelog.com/zh-CN/1.1.0/)，
版本号遵循语义化版本。

## [Unreleased]

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

### Security

- `server` 模式调试要求鉴权；关闭鉴权时必须显式设置 `DebugAllowUnauth: true`；
  `/api/debug/*` 要求 `X-Apidoc-Debug: 1` 与 JSON Content-Type（CSRF 防护）。
- 执行目标默认绑定已注册 action，只有 `Spec.AllowTargetOverride` 才允许改 URL。
- 库级 `DebugPolicy` 是兜底，适配器仍负责自己的目标策略。
- 文件上传、请求体、响应体、事件数量与并发均有上限。
