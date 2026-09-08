---
name: board-client-extension
description: Add a handwritten board-client custom probe (script or HTTP target) by editing YAML. Do not write status_probes.intent; that is only for config tui/web natural-language build.
---

# board-client 自定义扩展

给**正在跑的 board-client** 加一条本机采集扩展（脚本或 HTTP）。只在用户要你**直接改 YAML / 放脚本**时用本 skill。

自然语言扩展走 `board-client config tui` 或 `config web`：填 key / 类型 / 名称 / 采集间隔 / 过期时间 → 在「请输入你的想法」里描述需求 → **Build**（按钮旁 building）→ 旁边 **预览**，返回值在按钮下方。Build 会先生成完整探测规格（只读，见下方格式），再据此写脚本。**不要**在 YAML 里新写 `machine.status_probes[].intent`。

手写扩展时，必须按同一套**探测规格**来实现：先在心里（或注释里）写清「要做什么 / 输出 JSON / 约束」，再一次性写出 `probe.sh` 或 HTTP 目标。不要边写边猜字段。

## 探测规格格式

任意模型实现一条扩展，都应能靠下面这份规格一次性完成。自然语言 Build 生成的文本、以及手写脚本，都遵守它。

```markdown
# AgentBoard 探测规格

## 元数据
- key: <探针 key，[a-z0-9._-]{1,64}>
- kind: metric | service | http
- name: <看板上的中文名称>
- path: <可选绝对路径；没有则写 无>

## 要做什么
用条目写清本机只读采集步骤：读哪些文件/命令、如何计算、失败时怎么办。
不要改系统、不要发网络（http 类型除外，且 http 不写脚本）。

## 输出
### kind = metric 或 service
脚本 stdout 必须是且仅是一个 JSON 对象：

{
  "state": "running",
  "summary": "一句话中文摘要",
  "severity": "normal",
  "statuses": [
    {"key": "snake_case", "label": "中文标签", "value": "字符串", "unit": ""}
  ]
}

- state：通常 running；采不到可写 failed
- severity：normal / warning / error / critical
- statuses[].key：稳定英文蛇形；label：中文；value：字符串
- metric：value 尽量是十进制数字（进入机器卡片）；不要 logs / pinned_markdown
- service：value 可以是文本；可另加 logs 和 pinned_markdown

{
  "logs": [{"markdown": "一行说明", "severity": "info"}],
  "pinned_markdown": "| 列 | 值 |"
}

### kind = http
不要写 shell。只输出：

{"url":"http://127.0.0.1:8080/health","method":"GET","expect_status":[200],"expect_contains":""}

url 必须是无用户名密码的 http/https 绝对地址；method 只能 GET 或 HEAD；状态码 100–599。

## 约束
- POSIX sh，首行 #!/bin/sh；只输出脚本或上述 JSON，不要解释
- 禁止 curl / wget、读 token 环境变量、调用 /ingest/、改配置或服务
- 禁止把不可信字符串拼进 shell
- 失败时仍输出合法 JSON，severity=error，summary 说明原因
```

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

POSIX sh，stdout **一个** JSON 对象（`probe.Result`），字段与上面「输出」一节相同。

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
