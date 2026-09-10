# AGENTS.md

面向 Codex、Cursor Cloud Agent、Claude Code、Hermes、Pi 与其它会读取仓库指令的 agent。

## AgentBoard 上报

本项目**每一次会话/任务**必须向看板上报（地址来自项目 `.env` 的 `AGENTBOARD_URL`，不要写死域名）：
开始时 `start`（描述正在做什么），结束时 `succeed` 或 `fail`。被用户停止、无法继续、会话被掐掉前：必须 `interrupt` 或 `fail "任务被打断：原因"`。禁止只 `start` 不收口。长等待要 `progress`。

Token 与看板地址写在仓库根目录 `.env`（`AGENTBOARD_TOKEN` / `AGENTBOARD_URL`，不要入库）。脚本会自动读取。未设置 token 或 URL 时：本机 board-client 若开了 ingest，仍会投影到 `proj-*`；两边都没有则静默跳过。不要询问、不要失败。不要打印 token。

1. 若存在 `skills/agentboard-report/SKILL.md`，先读再执行。
2. 用 `python3 skills/agentboard-report/scripts/report.py <start|heartbeat|progress|error|succeed|fail|interrupt> "说明"`。
3. 不要用 `abp_v_` 上报。上报失败不能中断用户任务。
4. Provider：Cursor `cursor`；Codex `codex`；Claude Code `claude`；OpenClaw `openclaw`（并保持心跳，TTL 180s）；Hermes `hermes`；Pi `pi`。
5. 每次 `start` 一条新 Run；不要设置 `AGENTBOARD_RUN_KEY`，除非续同一条 Run。

## 双通道

- 项目 `.env` 的 `AGENTBOARD_TOKEN` → 该项目 **virtual machine**（可单独用）
- 本机 `board-client` 的 `ABP_MACHINE_TOKEN` + 本机 ingest → 物理机 `proj-*`
- **两把 machine key 都在时，同一条会话日志会同时出现在两边。**
- 本机 shell / 训练作业用 `board-client wrap`（`skills/bc-wrapper/SKILL.md`），不要再 `report.py start`。Cloud Agent 无本机 client 时仍只走 report 直连看板。

安装：`docs/agent-report-tutorial.md`。常驻原文：`skills/agentboard-report/always-on.md`。
