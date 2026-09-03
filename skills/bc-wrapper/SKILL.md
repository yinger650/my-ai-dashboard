---
name: bc-wrapper
description: Wrap a local command/job so board-client reports it as a Run on this machine. Use for training jobs, scrun, long shell work — not for coding-agent sessions.
---

# board-client wrap

把**本机命令/作业**登记到正在跑的 `board-client`，在物理机卡片的 `board-client` 服务下出现一条 Run。不要用这个包编码 Agent 会话。

同一任务只选一种入口：

| 入口 | 谁用 | Run 挂在哪 |
|---|---|---|
| **wrap**（本 skill） | 本机命令/作业（sensecore、训练脚本、`scrun`） | `board-client` |
| `agentboard-report` / `report.py start` | Cursor / Codex / OpenClaw 编码会话 | `proj-{目录名}`（本机 tee）或项目 virtual machine |

**不要**对同一任务既 `wrap` 又 `report.py start`。

## 怎么跑

daemon 必须在本机跑（`board-client run --config …`）。未在跑时 wrap 仍会执行命令，只是看板看不到。

```bash
board-client wrap --summary "sensecore 训 llama" --ttl 6h \
  --log /data/jobs/123/train.log -- scrun python train.py
```

| 旗标 | 含义 |
|---|---|
| `--summary` | 看板上的 Run 摘要 |
| `--ttl` | 到期只把 Run 标成 `timed_out`，**不杀进程** |
| `--log` | **只读**作业自己已经在写的文件；wrap **不会**把 stdout 转存成这个文件 |
| `--config` | 用来找 `control.sock`（也可设 `BOARD_CLIENT_CONFIG`） |
| `-- CMD...` | argv **原样 exec**，不改 env/cwd；stdout/stderr 回终端 |

日志规则：

- 有 `--log`：daemon 只 tail 该文件（可晚出现、截断重跟）。文件始终不存在 → 只看进程。
- 无 `--log`：旁路复制 stdout 给 daemon，同时原样打终端。
- stdout 也空：**不编造** `log.append`，只跟进程。

TTL 到了之后进程才退出：Run 已终态，exit code 不再上报（避免 `invalid_transition`）。

## 不要做

- 不要再对该任务 `python3 skills/agentboard-report/scripts/report.py start`
- 不要把 wrap 当日志收集器去创建/改写 `--log` 文件
- 不要用 wrap 包 Cursor/Codex 会话（用 agentboard-report）
