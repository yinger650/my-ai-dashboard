# Claude Code

本仓库的项目约定见 `AGENTS.md`。Claude Code 每次会话都必须遵守其中的 **AgentBoard 上报**。

Skill 在 `skills/agentboard-report/`（也可链到 `.claude/skills/agentboard-report`）。开始 `start`，结束 `succeed`/`fail`，被打断（含用户按 Escape / Stop）时 `interrupt`。

```bash
export AGENTBOARD_PROVIDER="${AGENTBOARD_PROVIDER:-claude}"
python3 skills/agentboard-report/scripts/report.py start "一句话：正在做什么"
python3 skills/agentboard-report/scripts/report.py succeed "已完成：结果摘要"
python3 skills/agentboard-report/scripts/report.py interrupt "用户停止"
```

可选：把 `skills/agentboard-report/hooks/claude-stop.sh` 配进 `.claude/settings.json` 的 Stop / SessionEnd hook，用户点停止后仍会尝试把进行中的 Run 标成被打断。详见 `skills/agentboard-report/adapters/claude.md`。
