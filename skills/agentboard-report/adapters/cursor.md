# 安装到 Cursor

## 本仓库

已包含：

- Rule：`.cursor/rules/agentboard-report.mdc`（`alwaysApply: true`）
- Skill：`skills/agentboard-report/SKILL.md`

Cloud Agent / 本机 Cursor 只要能读到这些文件，就会在长程任务里尝试上报。

## 其它仓库

复制 rule：

```bash
mkdir -p other-repo/.cursor/rules
cp .cursor/rules/agentboard-report.mdc other-repo/.cursor/rules/
# 按需同时复制 skills/agentboard-report
```

或把 skill 放到 Cursor 用户 skills 目录（若你的 Cursor 版本支持项目外 skills）。

## 环境变量（不要进 git）

本机 `~/.bashrc`、Cursor Cloud Agent secrets / environment variables：

```bash
export AGENTBOARD_URL=https://board.yinger650.com
export AGENTBOARD_TOKEN=abp_m_...
export AGENTBOARD_PROVIDER=cursor
export AGENTBOARD_SERVICE_KEY=cursor
```

Cloud Agent 默认没有 token 时必须跳过上报，不能挡任务。
