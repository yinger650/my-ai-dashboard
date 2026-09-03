# 小白教程：配置 board-client 上报日志

这篇只教一件事：**让一台 Linux 机器把状态和日志报到看板**，打开 https://board.yinger650.com 就能看见。

不需要会 Go，也不需要手改 YAML。会 SSH、会复制粘贴即可。

---

## 0. 先分清两条路上报

看板上会出现两类东西，**不要混用**：

| 你想看什么 | 用什么 | 看板上挂在哪 |
|---|---|---|
| 这台服务器还活着、CPU、磁盘、网站通不通、本机作业日志 | **board-client**（本教程主体） | 物理机卡片 |
| Cursor / Codex 正在写代码、任务有没有做完 | `report.py`（文末附录） | 项目 `proj-*` 或虚拟机 |

同一件任务只选一种。本机训练脚本用 `wrap`；编码 Agent 会话用 `report.py`。

---

## 1. 在看板创建这台机器

1. 打开看板，登录。
2. 进入 **设置**。
3. 填一个机器代号 `machine.key`（只能小写字母、数字、`.` `_` `-`，例如 `home-server`），再填显示名（例如「家里那台」）。
4. 点 **创建机器 + Token**。
5. 立刻复制 Token。它以 `abp_m_` 开头，**只完整显示一次**，当作密码。

记住两样东西：

- 机器代号，后面配置里的 `machine.key` 必须一模一样
- Token

上报地址：

- 这台机器和看板在同一台服务器上：一般是 `http://127.0.0.1:8090`
- 另一台远程机器（阿里云等）：`https://board.yinger650.com`

---

## 2. 确认这台机器上有 client

SSH 到要监控的 Linux 机器。

```bash
board-client version
```

生产上二进制通常在 `/opt/agentboard/bin/board-client`，配置在 `/etc/agentboard/client.yaml`。没有在 PATH 里就写全路径：

```bash
/opt/agentboard/bin/board-client version
ls /etc/agentboard/client.yaml
```

如果还没有配置文件，后面用配置界面会自动新建一份。

Token **不要**写进 git。生产推荐放环境文件：

```bash
sudo mkdir -p /etc/agentboard
sudo cp deploy/board-client.env.example /etc/agentboard/board-client.env
sudo nano /etc/agentboard/board-client.env
```

写成一行（把后面换成你复制的 Token）：

```bash
ABP_MACHINE_TOKEN=abp_m_你的token
```

权限收紧：

```bash
sudo chmod 600 /etc/agentboard/board-client.env
```

---

## 3. 打开配置界面（二选一）

配置只改**这台机器自己的文件**，不是看板网站上的设置页。保存后若 daemon 在跑，会自动 reload。

配置文件路径下面都用生产默认：`/etc/agentboard/client.yaml`。

### 方法 A：终端菜单（TUI，推荐 SSH 时用）

```bash
sudo board-client config tui --config /etc/agentboard/client.yaml
```

你会看到：

- **身份**：上报网址、机器代号、显示名、Token（已有则显示掩码）
- **默认功能**：一排 `[ ]` / `[x]`，输入编号勾选或取消；升级后新出现的会标 **新增**
- **自定义**：你自己加过的 GPU 指标、网站探测、脚本，会列在这里，不会被清空

常用操作：

- 输入编号：改身份，或勾选/取消一项功能
- `s`：保存
- `q`：退出

第一次没有文件时：网址默认 `https://board.yinger650.com`，机器代号默认 `home-server`。请改成第 1 步里的真实值。

### 方法 B：本机网页（WEB）

只监听本机，外网打不开，这是故意的。

```bash
sudo board-client config web --config /etc/agentboard/client.yaml --listen 127.0.0.1:7439
```

终端里会出现 `config web http://127.0.0.1:7439`。然后：

- 你就在这台机器的桌面：浏览器打开该地址
- 你是 SSH 上来的：另开一个终端做端口转发，再在自己电脑浏览器打开

```bash
ssh -L 7439:127.0.0.1:7439 你的用户@这台机器
```

浏览器访问 http://127.0.0.1:7439 ，勾选后点 **保存并 reload**。Token 输入框留空表示「沿用原来的，不改」。

配完用 `Ctrl+C` 停掉这个临时网页即可。日常采集不靠它，靠下面的 `run` / systemd。

---

## 4. 身份填什么（必须对上）

| 项 | 填什么 |
|---|---|
| `server.url` | 第 1 步的上报地址 |
| `machine.key` | 和第 1 步创建时**完全相同** |
| `display_name` | 看板上卡片名字，可中文 |
| `machine_token` | 第 1 步的 `abp_m_…`。若已写在 `board-client.env` 里，这里可留空 |

填错的典型后果：client 在跑，看板却没有这台机器，或一直离线。

---

## 5. 勾选「我要上报什么」

没勾选的功能**不会**上报。升级 client 之后，新功能（例如 AI 巡检）会标 **新增**，默认不勾，需要你自己打开。

建议按需求勾，不必全开。

### 只想看机器活着、资源曲线

勾选：

- CPU、内存、文件系统、磁盘 IO、网络、监听端口
- 本机 ingest（给后面 Agent 日志留口，默认建议开）
- 自动升级（生产建议开；本机 `go run` 调试请关）

有 Docker / Nginx / cron / systemd 再勾对应项。systemd 勾上之后，还要在配置文件里写你关心的 unit 名（自定义区不会无故覆盖你已写的列表）。

### 想看网站通不通

勾选 **HTTP 网站探测**，并在自定义表里加目标，例如：

- `service_key`：`site-board`
- `name`：`AgentBoard`
- `url`：`https://board.yinger650.com/health/live`

探测失败时，看板上这条服务会变红，并写一条日志。

### 想看「一次本机任务」的日志（训练、脚本）

client 必须正在跑。然后包一层 `wrap`（不要再对该任务跑 `report.py start`）：

```bash
board-client wrap --summary "跑一次备份" --ttl 6h \
  --config /etc/agentboard/client.yaml \
  --log /var/log/my-backup.log \
  -- bash /usr/local/bin/backup.sh
```

- `--summary`：看板上这行任务的标题
- `--log`：作业**自己已经在写**的日志文件，client 只读、不替你建文件
- 没有 `--log`：终端输出会旁路一份给看板
- `--ttl`：到点只把任务标成超时，**不会杀进程**

看位置：这台物理机卡片 → **board-client** 这条服务下的 Runs 和滚动日志。

### 想看 Cursor / 编码 Agent 的会话日志

1. 配置里勾选 **本机 ingest**（以及如需则 **Agent 日志总结**）。
2. 仓库 `.env` 里另有一条 **项目** Token（`AGENTBOARD_TOKEN`），这和 client 的 `ABP_MACHINE_TOKEN` 不是同一个。
3. Agent 用 `report.py start` / `succeed` 上报。本机 client 会再投影一份到 `proj-目录名`。

细节见文末附录。小白若只关心「服务器还活着」，可先跳过。

### 想看 AI 巡检报告

勾选：

- **AI 总开关**（勾巡检时保存会自动打开）
- **AI 主机巡检**，并勾上默认白名单（`systemctl status`、`journalctl`、读白名单文件）
- 可选：**Agent 日志总结**

还需要这台机器上能跑 `cursor-agent`，并且环境变量里有 `CURSOR_API_KEY`（不要写进 YAML）。没有 Key 时巡检会降级或跳过，其它采集不受影响。

---

## 6. 让 client 在后台一直跑

配置界面只负责改文件。真正上报要靠 `run`。

已经用 systemd 的生产机：

```bash
sudo systemctl enable --now board-client
sudo systemctl status board-client --no-pager
```

改完配置后若没有自动 reload：

```bash
sudo systemctl reload board-client 2>/dev/null || sudo systemctl restart board-client
```

临时前台跑（调试）：

```bash
export ABP_MACHINE_TOKEN='abp_m_你的token'
board-client run --config /etc/agentboard/client.yaml
```

看到 `board-client starting`、`connected to board-server` 就对了。`401` / `403` 多半是 Token 或机器代号不对。

---

## 7. 到看板确认「有日志」

几秒到一分钟内应看到：

1. 对应机器卡片出现，CPU / 内存会动。
2. 服务列表里有 `board-client`。点进去能看到启动日志，例如「开始采集系统快照」。
3. 若刚升级过还没打开配置，这里可能有一条 **「有新功能可配置」**，按上面第 3 步打开 TUI/WEB 勾选即可。
4. 若开了 HTTP 探测 / wrap / AI 巡检，会多出对应服务（网站、`ai-inspect` 等），点进去看置顶摘要和滚动日志。

没有卡片：检查 `server.url`、`machine.key`、Token、防火墙是否放行到看板。

有卡片没日志：先看 `board-client` 自己的启动日志在不在；再确认你勾选的功能、wrap/`--log` 路径、ingest 是否打开。

---

## 8. client 自动升级之后

升级只换程序，**不会**改你的配置，也不会偷偷打开新功能。

1. 看板上 `board-client` 可能提示有新功能。
2. 再跑一次第 3 步的 `config tui` 或 `config web`。
3. 网址、机器代号、Token、你以前加的自定义项都会还在。
4. 给新功能打勾（或不勾），保存一次，提示就会消失。

---

## 9. 常见问题

**Token 写在 YAML 里还是环境变量里？**  
都能用。环境变量 `ABP_MACHINE_TOKEN` 非空时优先。推荐环境变量，YAML 里留空。

**会不会把 Token 配丢？**  
配置界面里 Token 输入框留空 = 不改原来的值。

**自定义 GPU / 网站列表会不会被默认配置覆盖？**  
不会。保存只改你动过的开关；自定义列表仍在。

**配置网页打不开？**  
必须是 `127.0.0.1`，不能写成 `0.0.0.0`。SSH 请做第 3 步的端口转发。

**wrap 了但看板没有任务？**  
`board-client run` 没在这台机器上跑。wrap 仍会执行命令，只是不上报。

**既 wrap 又 report.py？**  
不要。本机作业用 wrap；编码会话用 `report.py`。

---

## 附录：编码 Agent 自己上报（不是 board-client 配置）

给 Cursor / Codex 用，Token 来自看板上**这个项目的虚拟机**，写在仓库 `.env` 的 `AGENTBOARD_TOKEN`，不要和物理机 `ABP_MACHINE_TOKEN` 混用。

```bash
export AGENTBOARD_PROVIDER=cursor
python3 skills/agentboard-report/scripts/report.py start "正在做什么"
python3 skills/agentboard-report/scripts/report.py succeed "做完了：结果"
```

未设置 Token 时脚本会静默跳过，不会打断你干活。本机若同时开着 board-client 且勾了本机 ingest，看板上物理机下还会多一张 `proj-仓库名` 卡片，那是 client 投影的，不是 skill 改了身份。
