# AgentBoard Personal 设计规格 v1.0

> **还原说明（请先读）**  
> 原始文档标题为《AgentBoard Personal 个人服务器与 AI Agent 看板设计规格 v1.0》。  
> 它从未进入本仓库的 git 历史：`README.md` 从第一版实现起就链接到 `docs/`，但 `docs/` 目录从未被提交。  
> 实现它的 Cloud Agent 是 [bc-d933ce65](https://cursor.com/agents/bc-d933ce65-f7b6-4f53-b7e7-c2bb1fe15950)（PR #1，2026-08-17），该 run 的 transcript 现已不可访问。  
> 本文根据 **PR #1 说明、仓库代码里的 `spec x.y` 注释、以及当前实现** 还原章节编号与约束。  
> 标了「原稿未存」的段落是推断结构，不是逐字原文。

## 1. 产品目标（原稿未存，按 PR #1 / README 还原）

面向个人用户，把 **服务器、软件服务、AI Agent** 的状态收进一块看板。

三个组件：

| 组件 | 职责 |
|---|---|
| `board-server` | 单个 Go 进程：采集 API、管理后台 API、嵌入式前端、后台清理 |
| `board-client` | 被监控 Linux 机器上的采集器：读 `/proc` 等并上报 |
| `web` | React 看板（生产环境嵌入 `board-server`） |

事件（Event）是客户端发送的不可变记录。服务端在单个事务内完成去重、写入、指标拆分与状态投影。

## 2. 固定技术栈

实现必须使用（PR #1「严格遵循规格固定技术栈」）：

- 后端 / 客户端：Go 1.24+、`chi`、`modernc.org/sqlite`（纯 Go，无 CGO）、`goose`、UUIDv7、`argon2id`、`slog`
- 前端：React 19、TypeScript、Vite、Tailwind CSS 4、TanStack Query、React Router、Recharts、react-markdown + rehype-sanitize
- 测试：Go `testing`、Vitest + Testing Library

## 3. 架构

```
系统/服务/日志 → board-client → (HTTPS + Machine Token) → board-server → SQLite(WAL)
                                                        ↑
                        Web / curl / board.txt ────────┘
```

生产对外地址：`https://board.yinger650.com`（nginx 反代本机 `127.0.0.1:8090`）。

## 4. 领域对象（原稿未存，按模型还原）

- **Machine**：一台被监控主机。`machine_key` 匹配 `^[a-z0-9._-]{1,64}$`。心跳超时后健康变为 `stale` / `offline`。
- **Service**：机器下的软件或 Agent。可用 Machine Token 自动创建。支持 `ttl_seconds`，超时投影为 `stale`。
- **Token**：一次性明文，只显示一次。前缀：
  - `abp_m_` Machine Token（采集 / 自动建服务）
  - `abp_s_` Service Token
  - `abp_v_` Viewer Token（只读看板 / `board.txt`）
- **Run**：一次长程任务，用 `run_key` 标识，走状态机。
- **Log / Pinned log / Artifact / Status item**：服务级日志、置顶、附件、键值状态。

机器健康（实现）：

| health | 条件 |
|---|---|
| `online` | 最近一次 `last_seen_at` ≤ 2 × 心跳间隔，且资源未超阈值 |
| `degraded` | 在线但 CPU/内存/磁盘达到 warning/error |
| `stale` | 超过 2 倍间隔、不超过 3 倍间隔 |
| `offline` | 超过 3 倍心跳间隔 |
| `unknown` | 从未见过或时间戳非法 |
| `disabled` | 机器 `enabled=false` |

## 11. 事件协议

实现注释对应规格 **§11**。ingest：`POST /ingest/v1/events`，Bearer Token。一批最多 200 条，总大小 ≤ 512KiB。`event_id` 必须是 UUID，重复发送幂等忽略。时间用 UTC RFC3339（毫秒）。

### 11.1 公共信封

```json
{
  "schema_version": 1,
  "event_id": "<uuid>",
  "event_type": "service.state",
  "occurred_at": "2026-08-18T08:00:00.000Z",
  "boot_id": "<optional>",
  "sequence": 1,
  "service_key": "openclaw",
  "run_key": "<optional uuid>",
  "payload": {}
}
```

### 11.2 事件类型与严重级别

| event_type | 用途 |
|---|---|
| `machine.heartbeat` | 主机身份与心跳间隔 |
| `metric.sample` | CPU / 内存 / 磁盘 / 网络等指标 |
| `machine.port_snapshot` | 端口快照 |
| `machine.service_snapshot` | systemd（或类似）unit 快照 |
| `service.state` | 服务状态；可带 `ttl_seconds` |
| `status.upsert` | 键值状态 |
| `log.append` | 追加 markdown 日志（≤64KiB） |
| `log.pin` | 置顶一条日志 |
| `run.transition` | 长程任务状态迁移 |
| `collector.notice` | 采集器 / 运行时内部故障 |

Run 状态对应严重级别：`failed`/`timed_out` → `error`；`blocked` → `warning`；其余 → `info`。

`service_type`：`agent` / `daemon` / `job` / `scheduled` / `virtual`。

### 11.8 Run 状态机

状态：`queued`、`running`、`waiting_input`、`blocked`、`succeeded`、`failed`、`cancelled`、`timed_out`。

终态（`succeeded` / `failed` / `cancelled` / `timed_out`）不可再改。非法转换返回 409 `invalid_transition`。

允许迁移：

- `queued` → `running` | `cancelled`
- `running` → `waiting_input` | `blocked` | `succeeded` | `failed` | `cancelled` | `timed_out`
- `waiting_input` → `running` | `cancelled` | `timed_out`
- `blocked` → `running` | `failed` | `cancelled` | `timed_out`

允许直接以 `running` 开始（Agent 上报脚本的 `start` 即如此）。同一 `run_key` 的终态重发幂等。

## 12. HTTP API

实现注释对应规格 **§12.10** 错误码。成功响应：

```json
{ "data": {}, "meta": { "request_id": "...", "next_cursor": null } }
```

错误响应：

```json
{ "error": { "code": "unauthorized", "message": "...", "request_id": "..." } }
```

| code | 含义 |
|---|---|
| `invalid_json` | JSON 无法解析 |
| `unauthorized` | 未登录 / Token 无效 |
| `forbidden` | 已认证但无权 |
| `not_found` | 资源不存在 |
| `event_conflict` | 事件冲突 |
| `invalid_transition` | Run 非法迁移 |
| `payload_too_large` | 请求体过大 |
| `unsupported_media_type` | Content-Type 不支持 |
| `validation_failed` | 字段校验失败 |
| `rate_limited` | 限流 |
| `quota_exceeded` | Artifact 配额用尽 |
| `internal_error` | 内部错误 |
| `not_ready` | 依赖未就绪 |
| `unsupported_event_type` | 未知事件类型 |
| `totp_required` | 已启用 TOTP，需要验证码 |

主要路由见 `internal/server/server.go`：

- 健康：`GET /health/live`、`GET /health/ready`
- 采集：`GET /ingest/v1/ping`、`POST /ingest/v1/events`、`POST /ingest/v1/artifacts`
- 登录：`POST /auth/login`、`POST /auth/logout`、`GET /auth/session`
- 看板：`GET /api/v1/board`、`GET /api/v1/board.txt`（管理员会话或 Viewer Token）
- 管理：机器 / 服务 / Token / 设置 / TOTP / 访问日志 / 维护

无默认管理员账号。首次用：

```bash
echo 'strong-password' | ./bin/board-server admin set-password --password-stdin
```

## 13. board-server

### 13.1 全局中间件

顺序：`request_id` → `recover` → `security headers` → `client IP` → `access log`。

### 13.2 Ingest 事务

`IngestEvent` 在**单个事务**内完成：`event_id` 去重、写入事件、拆分指标、投影服务/状态/日志/Run。Machine Token 可按 `service_key` 自动创建 Service。

### 13.3 资源阈值（默认）

| 资源 | warning | error |
|---|---|---|
| CPU | 85% | 95% |
| 内存 | 90% | 97% |
| 根磁盘 | 85% | 95% |

## 14. board-client

### 14.2 配置

YAML（`configs/client.example.yaml`）。实现是规格的实用子集：server URL、machine key、采集器开关（CPU/内存/文件系统/磁盘 IO/网络/端口、systemd、HTTP 探测、Cursor transcript）。

环境变量：`ABP_MACHINE_TOKEN`（不要入库）。

### 14.3 本地 spool

SQLite spool。发送失败时缓存；容量满时**先丢最旧事件**。批量发送 + 指数退避。

远程机器可只跑 client，不必装 `board-server`（见 `deploy/board-client-remote.service`）。

## 16. 前端与颜色

实现注释对应规格 **§16.1**：

| severity | 色值 |
|---|---|
| normal | `#34d399` |
| info | `#60a5fa` |
| warning | `#fbbf24` |
| error | `#f87171` |
| unknown | `#9ca3af` |
| offline | `#6b7280` |

页面：登录、Dashboard（网格卡片 + 日志流）、机器详情、服务详情（附件 / 日志总结）、访问记录、设置（含 TOTP）。Markdown 必须经 `rehype-sanitize`。

看板可隐藏 / 显示离线机器（本机 `localStorage` 键 `abp.show-offline`，默认显示）。

## 17. 安全

### 17.1 同源、无 CORS

生产关闭 CORS。前端与 API 同源。`X-Forwarded-For` **仅**在受信代理 CIDR 内采用（`ABP_TRUSTED_PROXY_CIDRS`）。

安全响应头：CSP、`X-Frame-Options: DENY`、`Referrer-Policy: no-referrer`、HSTS（HTTPS）。

生产 `ABP_SECURE_COOKIES=true`。管理员会话 + CSRF。写操作必须带 `X-CSRF-Token`。

### 17.3 登录限流

每 IP 每分钟最多 10 次登录。

### 17.6 密码哈希

Argon2id：memory 64 MiB，iterations 3，parallelism 2，salt 16 字节，key 32 字节。PHC 格式存储。

另已实现：TOTP（RFC 6238）+ 一次性恢复码；Token 哈希存储，明文只显示一次。

## 里程碑

| 里程碑 | 范围 | 状态（相对当前仓库） |
|---|---|---|
| M1–M5 | SQLite、管理员会话、Machine/Service/Token、Event ingest、Board / board.txt、访问日志、Linux client | 已完成（PR #1） |
| M6 | TOTP / 恢复码、Artifact 上传下载预览与配额 | 已完成 |
| M7 | systemd unit 追踪、Agent 日志总结 | 已完成 |
| Agent 上报 | Cursor / Codex / OpenClaw HTTP skill + 服务 TTL | 已完成 |
| 远程探测 | 阿里云 HTTP 网站探测、远程 client unit | 已完成 |
| 看板 UX | 自由网格、卡片日志流、隐藏离线机器 | 已完成 |
| M8 | Playwright E2E；Docker/Caddy 部署与备份恢复；每日字节配额落库；全部安全测试用例 | **未完成** |

## 关键环境变量

| 变量 | 默认 | 说明 |
|---|---|---|
| `ABP_LISTEN_ADDR` | `127.0.0.1:8080` | 监听地址（生产示例 `127.0.0.1:8090`） |
| `ABP_DATA_DIR` | `/var/lib/agentboard` | 数据目录 |
| `ABP_SECURE_COOKIES` | `true` | 生产必须 true |
| `ABP_PUBLIC_URL` | （生产必填） | 对外 URL |
| `ABP_TRUSTED_PROXY_CIDRS` | — | 可信任的反向代理 |
| `AGENTBOARD_TOKEN` | — | Agent 上报用 Machine Token，禁止入库 |

完整清单见 `internal/config/config.go`。

## 相关实现入口

| 主题 | 路径 |
|---|---|
| 路由与中间件 | `internal/server/server.go` |
| 事件协议 | `internal/event/event.go` |
| Ingest 事务 | `internal/store/repo_events.go` |
| Agent HTTP 上报协议 | `skills/agentboard-report/references/protocol.md` |
| 生产 systemd / nginx | `deploy/` |
