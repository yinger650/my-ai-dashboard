# AgentBoard Personal (`abp`)

面向个人用户的服务器、软件服务与 AI Agent 统一状态看板。本仓库实现自
[《AgentBoard Personal 设计规格 v1.0》](docs/)，包含：

- **`board-server`**：单个 Go 进程，提供采集 API、管理后台 API、前端静态资源和后台清理。
- **`board-client`**：运行在被监控 Linux 机器上的 Go 采集器，读取 `/proc` 指标并上报。
- **`web`**：React + TypeScript + Vite + Tailwind 的响应式看板前端。

> 本仓库当前实现的是设计规格的**第一版可端到端运行的核心**（里程碑 M1–M7 的主链路）。
> 已完成与尚未完成的范围见文末「实现范围」。

## 架构

```
系统/服务/日志 → board-client → (HTTPS + Machine Token) → board-server → SQLite(WAL)
                                                        ↑
                        Web / curl / board.txt ────────┘
```

事件（Event）是客户端发送的不可变记录，服务端在单个事务内完成去重、写入、指标拆分与状态投影。

## 技术栈

- 后端 / 客户端：Go 1.24+、`chi`、`modernc.org/sqlite`（纯 Go，无 CGO）、`goose` 迁移、`google/uuid`（UUIDv7）、`argon2id`、`slog`。
- 前端：React 19、TypeScript、Vite、Tailwind CSS 4、TanStack Query、React Router、Recharts、react-markdown + rehype-sanitize。
- 测试：Go `testing`、Vitest + Testing Library。

## 快速开始（从空服务器到可用看板）

### 1. 前置条件

- Go 1.24+，Node 20+，`pnpm`。

### 2. 构建

```bash
make build          # 构建前端并嵌入，再产出 bin/board-server 与 bin/board-client
```

### 3. 初始化管理员（无默认账号 / 默认密码）

```bash
export ABP_DATA_DIR=./data
export ABP_SECURE_COOKIES=false     # 仅本地 HTTP 开发；生产必须为 true 且走 HTTPS
echo 'your-strong-password' | ./bin/board-server admin set-password --password-stdin
```

### 4. 启动服务端

```bash
ABP_DATA_DIR=./data ABP_SECURE_COOKIES=false ABP_LISTEN_ADDR=127.0.0.1:8080 \
  ABP_PUBLIC_URL=http://127.0.0.1:8080 ./bin/board-server run
```

访问 <http://127.0.0.1:8080/> 登录。

### 5. 创建机器并获取 Token

在 **设置** 页填写 `machine_key`、名称、类型，点击「创建机器 + Token」。完整 Token 只显示一次，请立即复制。

### 6. 运行采集客户端

```bash
./bin/board-client print-example-config > client.yaml
# 编辑 client.yaml：server.url、machine.key 与上一步一致
export ABP_MACHINE_TOKEN='abp_m_...'      # 上一步复制的完整 Token
./bin/board-client run --config client.yaml
```

看板上对应机器卡片将在数秒内出现实时 CPU / 内存 / 磁盘 / 网络指标。

## 本地开发（后端 + 前端 + 客户端同步运行）

```bash
# 终端 A：后端 + Vite（前端 :5173 代理到后端 :8080）
make dev

# 终端 B：初始化管理员（首次）
echo 'your-strong-password' | ABP_DATA_DIR=.dev-data \
  go run ./cmd/board-server admin set-password --password-stdin

# 终端 C：客户端上报本机指标
ABP_MACHINE_TOKEN=abp_m_... go run ./cmd/board-client run --config client.yaml
```

前端开发地址：<http://127.0.0.1:5173/>。

## 纯文本 / curl 接口

```bash
# 管理员会话或 Viewer Token
curl -H "Authorization: Bearer abp_v_..." http://127.0.0.1:8080/api/v1/board.txt
curl "http://127.0.0.1:8080/api/v1/board.txt?compact=1"
```

## 测试

```bash
make test          # Go 单元/集成测试 + 前端单元测试
make test-go
make test-web
```

## 关键配置（环境变量）

| 变量 | 默认 | 说明 |
|---|---|---|
| `ABP_LISTEN_ADDR` | `127.0.0.1:8080` | 监听地址 |
| `ABP_DATA_DIR` | `/var/lib/agentboard` | 数据目录（DB、artifacts） |
| `ABP_SECURE_COOKIES` | `true` | 生产必须 true（HTTPS）；本地 HTTP 置 false |
| `ABP_PUBLIC_URL` | 必填（生产） | 对外 URL |

完整清单见 `internal/config/config.go`。

## 健康检查

- `GET /health/live`：进程存活。
- `GET /health/ready`：数据库可读写。建议配置外部 HTTP uptime 探针。

## 实现范围

**已实现（可端到端运行）：** SQLite + goose 迁移；管理员 Argon2id 密码 + 会话 + CSRF；TOTP（RFC 6238）与一次性恢复码；Machine/Service/Token CRUD 与一次性 Token；Event 采集（heartbeat/metric/service.state/status.upsert/log.append/log.pin/run.transition/`machine.service_snapshot`）、`event_id` 幂等、Machine Token 自动创建 Service、Run 状态机与非法转换拒绝；Artifact 上传/下载/图片预览与全局配额；Board / board.txt / Machine 详情 / Service 详情查询；访问日志与限流；Go 客户端真实 `/proc` 采集（CPU/内存/文件系统/磁盘 IO/网络/端口）+ systemd unit 快照 + Cursor Agent transcript 扫描与启发式日志总结 + SQLite spool + 批量发送/退避重试；React 响应式看板（登录、Dashboard、机器详情、服务详情含附件与「生成日志总结」、访问记录、设置含 TOTP）；安全 Markdown 渲染；Go + 前端单元/集成测试。

**尚未实现（规格后续里程碑 M8）：** Playwright E2E；Docker/Caddy 部署与备份恢复；每日字节配额落库与全部安全测试用例。这些不影响当前 M1–M7 核心链路的运行。
