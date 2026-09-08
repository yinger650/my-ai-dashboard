---
name: board-client-extension
description: Add a handwritten board-client custom probe by editing YAML. Do not write status_probes.intent.
---

# board-client 自定义扩展

直接改 YAML 或放脚本时用仓库根目录 `skills/board-client-extension/SKILL.md`。不要写 `machine.status_probes[].intent`；自然语言扩展让人去 `board-client config tui|web` 里填想法、Build、预览。

手写脚本放 `dirname(spool)/extensions/custom/<key>/probe.sh`（与 TUI/Web 产物 `extensions/nl/` 同级）。实现前先按该 skill 里的 **探测规格格式**（元数据 / 要做什么 / 输出 JSON / 约束）想清楚，再一次性写出。stdout 必须是 `probe.Result` JSON。HTTP 用 `collectors.http.targets`。
