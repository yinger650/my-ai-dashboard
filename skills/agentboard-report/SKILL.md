---
name: agentboard-report
description: Report long-running tasks, errors, and OpenClaw/Cursor/Codex liveness to board.yinger650.com. Use at start/end, on errors, and for heartbeats.
homepage: https://board.yinger650.com
metadata: {"openclaw": {"requires": {"bins": ["python3"], "env": ["AGENTBOARD_TOKEN"]}, "primaryEnv": "AGENTBOARD_TOKEN"}}
---

# AgentBoard 上报

把本 agent 的长程任务进度、失败原因、以及（OpenClaw）进程是否还活着，发到 https://board.yinger650.com 。**不是** systemd 采集客户端；这是 agent 自己发 HTTP。

Token 未设置时**静默跳过**，不要中断用户任务。

## 何时必须上报

1. **长程任务开始**（预计超过 ~2 分钟，或多步实现/部署/调查）：`start`
2. **关键里程碑**（已定位根因、开始改代码、开始部署）：`progress`
3. **出错 / 内部异常**（工具失败、OpenClaw gateway 报错、重复崩溃）：`error`
4. **任务结束**：成功 `succeed`，失败 `fail`
5. **OpenClaw 存活**：每个 session 开始、以及之后大约每 60s 或每个 turn 开头：`heartbeat`（TTL 180s）。超时未心跳，看板上该服务会变成 `stale`

短问答（一句就能答完）不必上报。

## 怎么发

优先跑脚本（`{baseDir}` 是本 skill 目录）：

```bash
export AGENTBOARD_URL="${AGENTBOARD_URL:-https://board.yinger650.com}"
# AGENTBOARD_TOKEN 已由环境注入；不要打印它
export AGENTBOARD_PROVIDER="${AGENTBOARD_PROVIDER:-openclaw}"   # cursor | codex | openclaw
export AGENTBOARD_SERVICE_KEY="${AGENTBOARD_SERVICE_KEY:-$AGENTBOARD_PROVIDER}"

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
| `AGENTBOARD_TOKEN` | 是 | Machine Token `abp_m_…`（推荐，可自动建 service）或 Service Token `abp_s_…` |
| `AGENTBOARD_URL` | 否 | 默认 `https://board.yinger650.com` |
| `AGENTBOARD_PROVIDER` | 否 | `cursor` / `codex` / `openclaw` |
| `AGENTBOARD_SERVICE_KEY` | 否 | 默认等于 provider |
| `AGENTBOARD_SERVICE_NAME` | 否 | 看板上显示名 |
| `AGENTBOARD_TTL_SECONDS` | 否 | 默认 180 |
| `AGENTBOARD_RUN_KEY` | 否 | 固定一次任务的 run id |

Token 只出现在环境或本机 secret 文件，**永远不要**写进仓库、commit、PR 或聊天记录。

## 看板上看什么

- 机器 **Agents**（或当前 Machine Token 绑定的机器）下会出现 `cursor` / `codex` / `openclaw` 服务
- 服务卡片：`running` = 活着；`stale` + 「TTL 过期」= 心跳断了
- Runs：一条长程任务从 `running` 到 `succeeded`/`failed`
- 日志：progress / error markdown
- OpenClaw 内部问题走 `error`（`log.append` + `collector.notice`）

完整字段：`{baseDir}/references/protocol.md`。安装到 Cursor / Codex / OpenClaw：`{baseDir}/adapters/`。
