# 安装到 Cursor

## 本仓库

已包含：

- Rule：`.cursor/rules/agentboard-report.mdc`（`alwaysApply: true`）
- Skill：`skills/agentboard-report/SKILL.md`

Cloud Agent / 本机 Cursor 只要能读到这些文件，就会在每次会话开始与结束时尝试上报。

- 用仓库 `.env` 的 `AGENTBOARD_TOKEN`（该项目的 **virtual machine**）。服务是 `cursor`，每次 `start` 一条 Run。
- 本机若同时跑着 `board-client`，那是另一条链路：client 用自己的 token 报物理机，并把本机仓库投影为 `proj-*`。脚本**不会**改 skill 身份去挂物理机。

请在看板为**该项目**建一台 virtual machine，把 Token 写入 `.env`（一个项目一个 `machine_key`）。

Cursor Cloud Agent：直连看板，该环境是 `cloud-{hostname}` 服务，仍然用同一个 skill token。

本仓库把 Machine Token 放在根目录 `.env`（已 gitignore）。`report.py` 会自动读取，不必再 `export`。

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
```

不要设置 `AGENTBOARD_RUN_KEY`（除非续同一对话）。

Cloud Agent 默认没有 token 时必须跳过上报，不能挡任务；本机有 board-client 也不能代替 token。
