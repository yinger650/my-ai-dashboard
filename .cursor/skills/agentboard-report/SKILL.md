---
name: agentboard-report
description: Report long-running Cursor/Codex/OpenClaw tasks, errors, and liveness to board.yinger650.com via HTTP ingest. Use at start/end, on errors, and for heartbeats.
---

# AgentBoard 上报

把长程任务进度、失败原因、以及 OpenClaw 是否还活着，发到 https://board.yinger650.com 。这是 agent 自己发 HTTP，不是 `board-client`。

完整说明：仓库根目录 `skills/agentboard-report/SKILL.md`。协议：`skills/agentboard-report/references/protocol.md`。

`AGENTBOARD_TOKEN` 未设置时**静默跳过**，不要中断用户任务，也不要打印 token。

```bash
export AGENTBOARD_URL="${AGENTBOARD_URL:-https://board.yinger650.com}"
export AGENTBOARD_PROVIDER="${AGENTBOARD_PROVIDER:-cursor}"

python3 skills/agentboard-report/scripts/report.py start "一句话任务目标"
python3 skills/agentboard-report/scripts/report.py progress "里程碑"
python3 skills/agentboard-report/scripts/report.py error "内部错误"
python3 skills/agentboard-report/scripts/report.py succeed "结果"
python3 skills/agentboard-report/scripts/report.py fail "失败原因"
python3 skills/agentboard-report/scripts/report.py heartbeat "alive"
```

OpenClaw：session 开始就 `heartbeat`，之后每 ~60s 再发；TTL 180s 无心跳则看板显示 stale。
