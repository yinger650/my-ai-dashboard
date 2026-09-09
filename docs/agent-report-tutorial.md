# 小白教程：给编码 Agent 装 AgentBoard 上报

这篇只教一件事：让 Cursor / Codex / Claude Code / OpenClaw / Hermes / Pi **把自己正在做什么报到看板**。打开 https://board.yinger650.com 就能看见任务开始、完成、失败，以及中途被掐掉。

机器 CPU、磁盘、网站探测请看 [配置 board-client 上报日志](./client-log-tutorial.md)。那是另一条链路。

---

## 0. 两把钥匙，可以同时推

看板上会出现两类卡片，**不是二选一**：

| 你想看什么 | 用哪把 key | 看板上挂在哪 |
|---|---|---|
| 这个 Git 项目的编码任务 | 项目 `.env` 的 `AGENTBOARD_TOKEN` | 该项目的 **virtual machine**，服务名 `cursor` / `codex` / `claude` … |
| 这台电脑上打开了哪些仓库 | 本机 `board-client` 的 `ABP_MACHINE_TOKEN` + 勾选本机 ingest | 物理机下的 `proj-仓库名` |

**两把 machine key 都配好时，同一条会话日志会同时出现在两边。** 只有项目 token 时，Cloud Agent / 远程环境也能报。只有本机 client 时，脚本仍会把日志 tee 到 `proj-*`。

不要叠的是：本机训练脚本用 `board-client wrap`，编码会话用 `report.py`。同一件事不要两种都包。

---

## 1. 给这个项目建 virtual machine

1. 打开看板，登录。
2. 进入 **设置**。
3. 机器类型选 virtual（或按界面创建一台虚拟机），`machine_key` 用项目代号（小写、数字、`.` `_` `-`），例如 `my-app`。
4. 点 **创建机器 + Token**，立刻复制 `abp_m_` 开头的 Token。

写入**这个仓库**根目录 `.env`（已 gitignore，不要提交）：

```bash
AGENTBOARD_URL=https://board.yinger650.com
AGENTBOARD_TOKEN=abp_m_你的token
AGENTBOARD_PROVIDER=cursor
```

飞书 edition 的看板网页是站点根路径；`AGENTBOARD_URL` 必须带上设置页里的 8 位 slug，例如 `https://<飞书看板域名>/xxxxxxxx`。详见 [Feishu edition](./feishu-edition.md)。

一个项目一把项目 Token。换项目就换 `.env`。

（可选）本机再跑 `board-client`，Token 是另一把 `ABP_MACHINE_TOKEN`，配置见 [board-client 教程](./client-log-tutorial.md)，并勾选 **本机 ingest**。

---

## 2. 装 skill + 常驻片段

skill 目录是 `skills/agentboard-report`（里面有 `scripts/report.py`）。各产品还要一份**每次会话都会读**的短文，否则 skill 可能按需才加载，忘了 `start`。

捷径（在本仓库里）：

```bash
chmod +x skills/agentboard-report/install.sh
./skills/agentboard-report/install.sh
```

它会按本机已装的 agent，把 skill 链到用户目录，并提示还要把常驻片段贴进 `AGENTS.md`。

常驻原文：[`skills/agentboard-report/always-on.md`](../skills/agentboard-report/always-on.md)。

### Cursor

本仓库已有 `.cursor/rules/agentboard-report.mdc`（始终应用）。其它仓库：

其它仓库把 rule 拷到 `.cursor/rules/`，并把 `skills/agentboard-report` 整目录拷过去（或 `ln -sfn` 到 `~/.cursor/skills/`）。常驻片段见 `skills/agentboard-report/always-on.md`。

或链到 `~/.cursor/skills/agentboard-report`。`AGENTBOARD_PROVIDER=cursor`。

细节：[adapters/cursor.md](../skills/agentboard-report/adapters/cursor.md)

### Codex

把 `always-on.md` 并入项目 `AGENTS.md` 或 `~/.codex/AGENTS.md`。仓库里要能跑到 `skills/agentboard-report/scripts/report.py`。`AGENTBOARD_PROVIDER=codex`。

细节：[adapters/codex.md](../skills/agentboard-report/adapters/codex.md)

### Claude Code

skill 按需加载，**必须**同时有 `CLAUDE.md` 或 `AGENTS.md`：

```bash
mkdir -p ~/.claude/skills
ln -sfn /path/to/my-ai-dashboard/skills/agentboard-report ~/.claude/skills/agentboard-report
```

把 `always-on.md` 拷进项目 `CLAUDE.md`。`AGENTBOARD_PROVIDER=claude`。

可选：Stop / SessionEnd hook 调 `hooks/claude-stop.sh`，用户点停止后仍尝试把 Run 标成被打断。见 [adapters/claude.md](../skills/agentboard-report/adapters/claude.md)。

### OpenClaw

```bash
mkdir -p ~/.openclaw/workspace/skills
ln -sfn /path/to/my-ai-dashboard/skills/agentboard-report \
  ~/.openclaw/workspace/skills/agentboard-report
openclaw skills list
```

`AGENTBOARD_PROVIDER=openclaw`，并加 cron 心跳。见 [adapters/openclaw.md](../skills/agentboard-report/adapters/openclaw.md)。

### Hermes

```bash
mkdir -p ~/.hermes/skills
ln -sfn /path/to/my-ai-dashboard/skills/agentboard-report ~/.hermes/skills/agentboard-report
```

把 `always-on.md` 并入 `AGENTS.md`。`AGENTBOARD_PROVIDER=hermes`。见 [adapters/hermes.md](../skills/agentboard-report/adapters/hermes.md)。

### Pi

```bash
mkdir -p ~/.pi/agent/skills
ln -sfn /path/to/my-ai-dashboard/skills/agentboard-report \
  ~/.pi/agent/skills/agentboard-report
```

或链到项目 `.pi/skills/`。把 `always-on.md` 并入 `AGENTS.md`。`AGENTBOARD_PROVIDER=pi`。见 [adapters/pi.md](../skills/agentboard-report/adapters/pi.md)。

---

## 3. Agent 每次会话跑什么

```bash
python3 skills/agentboard-report/scripts/report.py start "一句话：正在做什么"
python3 skills/agentboard-report/scripts/report.py progress "现在做到哪"
python3 skills/agentboard-report/scripts/report.py succeed "做完了：结果"
python3 skills/agentboard-report/scripts/report.py fail "失败原因"
python3 skills/agentboard-report/scripts/report.py interrupt "用户停止"
```

- 开始立刻 `start`，结束必须 `succeed` 或 `fail`。
- 用户停止、对话被掐、你决定中止：`interrupt`（看板上是 fail，摘要以「任务被打断」开头）。
- 编译/测试等长时间不要沉默，隔一会儿 `progress`。

---

## 4. 到看板确认

几秒内应看到：

1. 对应 **virtual** 机器卡片上出现 `cursor`（或你设的 provider）服务，有一条 running 的 Run。
2. 本机若开了 ingest：物理机下还有 `proj-仓库名`，内容同一份。
3. 结束后 Run 变成 succeeded / failed。

被用户直接杀掉、Cloud Agent 被停掉、来不及跑 `interrupt` 时：大约 **30 分钟**没有新日志，看板会把这条 agent 任务标成 **failed**，并写「任务被打断（无后续上报）」。本机 `wrap` 训练作业不会走这 30 分钟规则。

---

## 5. 常见问题

**没 Token 会怎样？**  
脚本不报错、不挡你干活。有本机 ingest 就只出现在 `proj-*`；否则看板什么也没有。

**能不能只用 viewer token？**  
不能。`abp_v_` 只能看，不能上报。

**既 wrap 又 report.py？**  
不要。本机作业用 wrap；编码会话用 `report.py`。

**任务一直显示进行中？**  
多半是会话被掐掉且还不到 30 分钟。能跑命令就补一条 `interrupt`。长编译请 `progress`，免得被当成打断。

**Token 写进 git 了怎么办？**  
立刻在看板作废并换新 Token，从 git 历史里清掉。
