# AgentBoard 上报

本项目每次会话都必须上报：开始时 `start`，结束时 `succeed` / `fail`，被打断时 `interrupt`。发到 `AGENTBOARD_URL`（项目 `.env`，不要写死域名）。

- 项目 `.env` 的 `AGENTBOARD_TOKEN` → **virtual machine**
- 本机 board-client 的 `ABP_MACHINE_TOKEN` + ingest → 物理机 `proj-*`
- 两把 machine key 都在时，同一条会话日志同时出现在两边。未设项目 token 时，若本机 ingest 在跑仍会 tee。

完整说明：仓库根目录 `skills/agentboard-report/SKILL.md`。安装：`docs/agent-report-tutorial.md`。

```bash
export AGENTBOARD_PROVIDER="${AGENTBOARD_PROVIDER:-cursor}"

python3 skills/agentboard-report/scripts/report.py start "一句话：正在做什么"
python3 skills/agentboard-report/scripts/report.py progress "里程碑"
python3 skills/agentboard-report/scripts/report.py succeed "已完成：结果"
python3 skills/agentboard-report/scripts/report.py fail "失败原因"
python3 skills/agentboard-report/scripts/report.py interrupt "用户停止"
python3 skills/agentboard-report/scripts/report.py heartbeat "alive"
```

不要打印 token。上报失败不能中断用户任务。本机作业用 `board-client wrap`，不要再 `report.py start`。
