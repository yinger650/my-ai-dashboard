# AgentBoard 上报（Codex / 通用 AGENTS.md 片段）

把下面整段并入项目 `AGENTS.md`，或放到 `~/.codex/AGENTS.md`。

```markdown
## AgentBoard

长程任务（多步实现、部署、排查）必须向 AgentBoard 上报进度与结果，便于在 https://board.yinger650.com 查看是否完成。

- Token：环境变量 `AGENTBOARD_TOKEN`（Machine Token）。未设置则跳过，不要中断任务。
- URL：`AGENTBOARD_URL`，默认 `https://board.yinger650.com`。
- Provider：Codex 设 `AGENTBOARD_PROVIDER=codex`（Cursor 用 `cursor`，OpenClaw 用 `openclaw`）。
- 脚本：`python3 skills/agentboard-report/scripts/report.py <start|progress|error|succeed|fail|heartbeat> "说明"`
- 开始 `start`，里程碑 `progress`，出错 `error`，结束 `succeed`/`fail`。
- 不要打印 token，不要把上报失败当成任务失败。
```

Codex 用户级环境示例（`~/.bashrc` 或 secret store，不要进 git）：

```bash
export AGENTBOARD_URL=https://board.yinger650.com
export AGENTBOARD_TOKEN=abp_m_...          # 只保存在本机
export AGENTBOARD_PROVIDER=codex
export AGENTBOARD_SERVICE_KEY=codex
```
