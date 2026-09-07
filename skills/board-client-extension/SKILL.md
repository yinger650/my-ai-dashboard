---
name: board-client-extension
description: Add a handwritten board-client custom probe (script or HTTP target) by editing YAML. Do not write status_probes.intent; that is only for config tui/web natural-language build.
---

# board-client 自定义扩展

给**正在跑的 board-client** 加一条本机采集扩展（脚本或 HTTP）。只在用户要你**直接改 YAML / 放脚本**时用本 skill。

自然语言扩展走 `board-client config tui` 或 `config web`：展开条目 → 写描述 → Build → 预览 → 启用。**不要**在 YAML 里新写 `machine.status_probes[].intent`。

## 目录

默认根目录是 `dirname(storage.spool_path)/extensions`（可用 `storage.extensions_path` 覆盖）：

```text
extensions/
  nl/<key>/        # TUI/Web Build 产物（不要手改）
  custom/<key>/    # 本 skill 写这里
    probe.sh
```

`nl/` 与 `custom/` 是兄弟目录。手写脚本放 `custom/<key>/probe.sh`，`command[0]` 必须是绝对路径。

## 脚本 stdout

POSIX sh，stdout 一个 JSON 对象（`probe.Result`）：

```json
{"state":"running","summary":"...","severity":"normal","statuses":[{"key":"...","label":"...","value":"...","unit":""}]}
```

- `metric`：`value` 尽量是数字，会进机器 heartbeat metadata
- `service`：可含 `logs`、`pinned_markdown`，投影成独立 virtual Service
- 权限：绝对路径、可执行、不得 group/other writable
- 禁止：`curl` / `wget`、读 token 环境变量、调用 `/ingest/`、改服务

HTTP 探测不要写脚本，用下面的 `collectors.http.targets`。

## YAML

`status_probes` 只带 `key` / `kind` / `command`（不要 `intent`）：

```yaml
machine:
  status_probes:
    - key: disk-data
      kind: metric
      name: "/data 占用"
      command: ["/var/lib/agentboard-client/extensions/custom/disk-data/probe.sh"]
      interval: 60s
```

或沿用手写列表：

```yaml
collectors:
  probes:
    enabled: true
    scripts:
      - service_key: disk-data
        name: "/data 占用"
        command: ["/var/lib/agentboard-client/extensions/custom/disk-data/probe.sh"]
        format: json
        interval: 60s
        ttl_seconds: 180
  http:
    enabled: true
    targets:
      - service_key: site-board
        name: AgentBoard
        url: "https://board.yinger650.com/health/live"
        method: GET
        expect_status: [200]
```

改完后让 daemon reload（`board-client` 若在跑，配置 UI 保存会 reload；否则 `systemctl reload board-client` 或重启）。

## 不要做

- 不要写 `machine.status_probes[].intent` 或让模型「编译」脚本进 `extensions/nl/`
- 不要把 Cursor/看板 token 写进脚本或 YAML
- 不要用 `command` 配 `kind: http`（http 用手写 `collectors.http.targets` 或 TUI/Web 的 http 自然语言条目）
