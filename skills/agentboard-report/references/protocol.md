# AgentBoard HTTP 上报协议

看板 ingest：`POST {AGENTBOARD_URL}/ingest/v1/events`

鉴权：`Authorization: Bearer {AGENTBOARD_TOKEN}`（`abp_m_…` Machine Token 或 `abp_s_…` Service Token）。

## 请求体

```json
{
  "events": [
    {
      "schema_version": 1,
      "event_id": "<uuid>",
      "event_type": "service.state",
      "occurred_at": "2026-08-18T08:00:00.000Z",
      "service_key": "openclaw",
      "run_key": "<uuid, optional>",
      "payload": {}
    }
  ]
}
```

- `event_id` 必须是 UUID；重复发送同一 `event_id` 会被幂等忽略。
- `occurred_at` 使用 UTC RFC3339（毫秒）。
- `service_key` 匹配 `^[a-z0-9._-]{1,64}$`。推荐：`cursor`、`codex`、`openclaw`。
- 一批最多 200 条，总大小 ≤ 512KiB。

## 事件类型（本 skill 使用）

| event_type | 用途 |
|---|---|
| `service.state` | 存活/状态。带 `ttl_seconds`；超时未再上报则看板显示 **TTL 过期**。 |
| `status.upsert` | 键值状态（存活、provider、最近心跳）。 |
| `run.transition` | 一次长程任务的生命周期。`run_key` 必填。 |
| `log.append` | 进度/错误日志（markdown，≤64KiB）。 |
| `log.pin` | 置顶一条总结（可选）。 |
| `collector.notice` | OpenClaw/运行时内部故障，会记入访问异常之外的服务事件。 |

## run.transition 状态机

`queued → running → (waiting_input\|blocked) → succeeded\|failed\|cancelled\|timed_out`

允许直接以 `running` 开始（本脚本的 `start` 这样做）。终态不能再改。同一 `run_key` 的终态重发是幂等的。

payload 要点：

```json
{
  "service_name": "OpenClaw",
  "service_type": "agent",
  "status": "running",
  "summary": "短描述",
  "provider": "openclaw"
}
```

`service_type` 必须是 `agent`（或 `daemon`/`job`/`scheduled`/`virtual`）。

## service.state + TTL

```json
{
  "name": "OpenClaw",
  "type": "agent",
  "state": "running",
  "summary": "alive",
  "severity": "normal",
  "ttl_seconds": 180
}
```

推荐 TTL=180s。OpenClaw / 长驻进程至少每 60s 发一次 `heartbeat`。超过 TTL 未上报，看板将该服务标为 `stale` / warning。这就是「还活着没有」的依据。

## Ping

`GET {AGENTBOARD_URL}/ingest/v1/ping` + Bearer token。用来确认 token 有效。

## 不要做的事

- 不要把 token 写进 git、skill 正文、PR、截图。
- 不要用 Viewer Token（`abp_v_`）上报。
- 不要因为上报失败而中断用户任务。
