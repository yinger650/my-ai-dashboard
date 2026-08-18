# 安装到 OpenClaw

OpenClaw 从 workspace `skills/`、`~/.openclaw/skills` 加载 `SKILL.md`。

```bash
# 推荐：链到本仓库 skill（保持更新）
mkdir -p ~/.openclaw/workspace/skills
ln -sfn /path/to/my-ai-dashboard/skills/agentboard-report \
  ~/.openclaw/workspace/skills/agentboard-report

# 或复制
cp -a skills/agentboard-report ~/.openclaw/workspace/skills/agentboard-report
```

然后：

```bash
openclaw skills list    # 应看到 agentboard-report
```

新开 session（`/new`）或 `openclaw gateway restart` 后 skill 才会进 system prompt。

## 环境变量（不要进 git）

写入 `~/.openclaw/openclaw.json` 的 env、systemd `EnvironmentFile`，或 shell profile：

```bash
export AGENTBOARD_URL=https://board.yinger650.com
export AGENTBOARD_TOKEN=abp_m_...
export AGENTBOARD_PROVIDER=openclaw
export AGENTBOARD_SERVICE_KEY=openclaw
export AGENTBOARD_TTL_SECONDS=180
```

OpenClaw gating 需要 `AGENTBOARD_TOKEN` 与 `python3`。没有 token 时该 skill 可能被过滤；这是预期行为。

## 存活心跳

聊天 turn 里的 `heartbeat` 不够稳。再加 cron / OpenClaw 定时任务，每 1–2 分钟打一次：

```bash
*/2 * * * * AGENTBOARD_PROVIDER=openclaw AGENTBOARD_SERVICE_KEY=openclaw \
  python3 /path/to/skills/agentboard-report/scripts/report.py heartbeat "cron"
```

超过 180s 没有心跳，https://board.yinger650.com 上 `openclaw` 服务会显示 **TTL 过期**（可能已挂）。

内部错误（gateway 崩溃循环、工具超时、认证失败）用：

```bash
python3 .../report.py error "gateway 反复重启：原因"
```

这会同时写 `log.append` 与 `collector.notice`。
