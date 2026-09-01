# AGENTS.md

面向 Codex、Cursor Cloud Agent 与其它会读取仓库指令的 agent。

## AgentBoard 上报

本项目**每一次会话/任务**必须向 https://board.yinger650.com 上报：
开始时 `start`（描述正在做什么），结束时 `succeed` 或 `fail`。

Token 写在仓库根目录 `.env`（`AGENTBOARD_TOKEN`，不要入库）。脚本会自动读取 `.env`。未设置 token 时静默跳过；本机 board-client 用自己的 token 报物理机，不能代替本 skill 的 token。

1. 若存在 `skills/agentboard-report/SKILL.md`，先读再执行。
2. 用 `python3 skills/agentboard-report/scripts/report.py <start|heartbeat|progress|error|succeed|fail> "说明"`。
3. 未设置 token：静默跳过，不要询问、不要失败。本机有 client 不能代替 token。
4. 不要打印 token。上报失败不能中断用户任务。
5. Cursor：`AGENTBOARD_PROVIDER=cursor`。Codex：`codex`。OpenClaw：`openclaw`，并保持心跳（TTL 180s）。
6. 每次 `start` 一条新 Run；不要设置 `AGENTBOARD_RUN_KEY`，除非续同一条 Run。
