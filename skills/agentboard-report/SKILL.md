---
name: agentboard-report
description: Report long-running tasks, errors, and OpenClaw/Cursor/Codex liveness to board.yinger650.com. Use at start/end, on errors, and for heartbeats.
homepage: https://board.yinger650.com
metadata: {"openclaw": {"requires": {"bins": ["python3"], "env": ["AGENTBOARD_TOKEN"]}, "primaryEnv": "AGENTBOARD_TOKEN"}}
---

# AgentBoard 上报

把本 agent 的长程任务进度、失败原因、以及（OpenClaw）进程是否还活着，发到 https://board.yinger650.com 。用本 skill 的 `AGENTBOARD_TOKEN`（项目 **virtual machine** 的 Machine Token）直连看板。**不是** systemd 采集客户端，也**不**借用本机 `board-client` 的 token。

本机 `board-client` 用**自己的** `ABP_MACHINE_TOKEN` 上报物理机（`board-client`、systemd、probe，以及本机打开的仓库 `proj-*`）。两条链路独立：发现本机 ingest 只表示 client 在跑，不会改 agent 的身份。

**互斥：** 编码 Agent 会话用本 skill（`report.py`）。本机 shell / 训练作业用 `board-client wrap`（见 `skills/bc-wrapper/SKILL.md`），**不要**再对本机命令包一层 `report.py start`。本机 tee 打开时，会话终态会在物理机 `board-client` 上多一条「完成 task」滚动日志，Run 仍在 `proj-*`。Cloud Agent 不在这台机上时仍只走本 skill 直连看板。

`AGENTBOARD_TOKEN` 未设置时**静默跳过**，不要中断用户任务。本机有 client **不能**代替 token。

## 何时必须上报

1. **会话/任务开始**：立刻 `start`，用一句话描述正在做什么
2. **关键里程碑**（已定位根因、开始改代码、开始部署）：`progress`
3. **出错 / 内部异常**（工具失败、OpenClaw gateway 报错、重复崩溃）：`error`
4. **会话/任务结束**：成功 `succeed`，失败 `fail`
5. **OpenClaw 存活**：每个 session 开始、以及之后大约每 60s 或每个 turn 开头：`heartbeat`（TTL 180s）。超时未心跳，看板上该服务会变成 `stale`

本仓库每次会话都必须上报开始与结束。Token 写在仓库根目录 `.env`，脚本会自动读取。

Cursor/Codex：**每次 `start` 一条新 Run**（同一对话里的 `progress`/`succeed`/`fail` 续这条）。不要手动设 `AGENTBOARD_RUN_KEY`（除非明确续同一条 Run）。`start`/`succeed`/`fail` 都会写滚动日志。

## 怎么发

优先跑脚本（`{baseDir}` 是本 skill 目录）：

```bash
export AGENTBOARD_URL="${AGENTBOARD_URL:-https://board.yinger650.com}"
# AGENTBOARD_TOKEN 已由环境注入；不要打印它
export AGENTBOARD_PROVIDER="${AGENTBOARD_PROVIDER:-openclaw}"   # cursor | codex | openclaw

python3 "{baseDir}/scripts/report.py" heartbeat "alive"
python3 "{baseDir}/scripts/report.py" start "实现 M7 部署"
python3 "{baseDir}/scripts/report.py" progress "已完成 systemd 投影"
python3 "{baseDir}/scripts/report.py" error "gateway 连接被拒绝"
python3 "{baseDir}/scripts/report.py" succeed "已部署到生产"
python3 "{baseDir}/scripts/report.py" fail "测试失败：TTL overlay"
```

没有脚本时，用 curl 发同样的 ingest 协议，见 `{baseDir}/references/protocol.md`。

OpenClaw 建议再加一条 cron / 定时心跳，避免只在聊天 turn 才更新存活：

```bash
# 每 2 分钟
AGENTBOARD_PROVIDER=openclaw AGENTBOARD_SERVICE_KEY=openclaw \
  python3 "{baseDir}/scripts/report.py" heartbeat "cron"
```

## 环境变量

| 变量 | 必填 | 说明 |
|---|---|---|
| `AGENTBOARD_TOKEN` | 是（否则跳过） | 项目 virtual machine 的 Machine Token `abp_m_…` |
| `AGENTBOARD_URL` | 否 | 默认 `https://board.yinger650.com` |
| `AGENTBOARD_PROVIDER` | 否 | `cursor` / `codex` / `openclaw` |
| `AGENTBOARD_SERVICE_KEY` | 否 | 默认等于 provider（`cursor` / `codex` / `openclaw`） |
| `AGENTBOARD_SERVICE_NAME` | 否 | 看板上显示名 |
| `AGENTBOARD_TTL_SECONDS` | 否 | 默认 180 |
| `AGENTBOARD_RUN_KEY` | 否 | 不要设，除非续同一对话 |
| `AGENTBOARD_LOCAL_INGEST` | 否 | 仅覆盖本机 log tee 的 URL；**不会**改身份或鉴权 |

Token 只出现在环境或本机 secret 文件，**永远不要**写进仓库、commit、PR 或聊天记录。

## 看板上看什么

- 本 skill：Token 绑定的 **virtual machine** 下有一个 `cursor` / `codex` / `openclaw` 服务
- Cursor Cloud Agent：同一 virtual machine 下是 `cloud-{hostname}` 服务
- 本机 `board-client`：物理机卡片上有采集服务，以及本机打开的仓库（`proj-*`，由 client 投影，不是 skill 改身份）
- 本机作业：用 `board-client wrap`，Run 挂在 `board-client`；不要和 `report.py start` 叠用
- 服务卡片：进行中任务数 + 每个 Run 的摘要；`stale` = TTL 过期
- Runs：每次 `start` 一条，从 `running` 到 `succeeded`/`failed`
- 日志：`start`/`progress`/`succeed`/`fail` 的 markdown

完整字段：`{baseDir}/references/protocol.md`。安装到 Cursor / Codex / OpenClaw：`{baseDir}/adapters/`。
