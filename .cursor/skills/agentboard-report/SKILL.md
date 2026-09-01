# AgentBoard 上报

本项目每次会话都必须上报：开始时 `start`（正在做什么），结束时 `succeed` / `fail`。发到 https://board.yinger650.com 。用 `.env` 里的 `AGENTBOARD_TOKEN`（virtual machine）直连看板。本机 board-client 用自己的 token 报物理机，并把打开的仓库投影为 `proj-*`。

完整说明：仓库根目录 `skills/agentboard-report/SKILL.md`。协议：`skills/agentboard-report/references/protocol.md`。

Token 在仓库根目录 `.env`，脚本会自动读取。未设置时**静默跳过**，不要中断用户任务，也不要打印 token。本机有 client 不能代替 token。

每次 `start` 一条新 Run；不要手动设 `AGENTBOARD_RUN_KEY`。

```bash
export AGENTBOARD_PROVIDER="${AGENTBOARD_PROVIDER:-cursor}"

python3 skills/agentboard-report/scripts/report.py start "一句话：正在做什么"
python3 skills/agentboard-report/scripts/report.py progress "里程碑"
python3 skills/agentboard-report/scripts/report.py error "内部错误"
python3 skills/agentboard-report/scripts/report.py succeed "已完成：结果"
python3 skills/agentboard-report/scripts/report.py fail "失败原因"
python3 skills/agentboard-report/scripts/report.py heartbeat "alive"
```

OpenClaw：session 开始就 `heartbeat`，之后每 ~60s 再发；TTL 180s 无心跳则看板显示 stale。
