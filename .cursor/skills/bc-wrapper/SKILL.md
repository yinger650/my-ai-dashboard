# board-client wrap

本机命令/作业用 `board-client wrap`，不要再对该任务 `report.py start`。

完整说明：仓库根目录 `skills/bc-wrapper/SKILL.md`。

```bash
board-client wrap --summary "一句话作业" --ttl 6h --log /path/to/job.log -- your-cmd args
```

`--log` 只读作业自己的文件。编码 Agent 会话仍用 `skills/agentboard-report`。
