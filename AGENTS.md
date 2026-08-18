# AGENTS.md

面向 Codex、Cursor Cloud Agent 与其它会读取仓库指令的 agent。

## AgentBoard 上报

长程任务（多步实现、部署、排查，大约超过 2 分钟）必须向
https://board.yinger650.com 上报，用来确认任务是否做完，以及 OpenClaw 是否存活、有没有内部错误。

1. 若存在 `skills/agentboard-report/SKILL.md`，先读再执行。
2. 有 `AGENTBOARD_TOKEN` 时，用
   `python3 skills/agentboard-report/scripts/report.py <start|heartbeat|progress|error|succeed|fail> "说明"`。
3. 未设置 token：静默跳过，不要询问、不要失败。
4. 不要打印 token。上报失败不能中断用户任务。
5. Cursor：`AGENTBOARD_PROVIDER=cursor`。Codex：`codex`。OpenClaw：`openclaw`，并保持心跳（TTL 180s）。
