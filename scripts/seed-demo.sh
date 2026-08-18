#!/usr/bin/env bash
# Seed a few machines with statuses and logs so the dashboard can be demoed.
set -euo pipefail
cd "$(dirname "$0")/.."

BASE="${ABP_PUBLIC_URL:-http://127.0.0.1:8080}"
ADMIN_PW="${ABP_DEV_ADMIN_PASSWORD:-devpassword123}"

cookie_jar=$(mktemp)
trap 'rm -f "$cookie_jar"' EXIT

login_json=$(curl -sS -c "$cookie_jar" -b "$cookie_jar" -H 'Content-Type: application/json' \
  -d "{\"password\":\"${ADMIN_PW}\"}" "$BASE/auth/login")
csrf=$(printf '%s' "$login_json" | python3 -c 'import json,sys; print(json.load(sys.stdin).get("data",{}).get("csrf_token",""))')
if [[ -z "$csrf" ]]; then
  echo "login failed: $login_json" >&2
  exit 1
fi

api() {
  local method=$1 path=$2 data=${3:-}
  if [[ -n "$data" ]]; then
    curl -sS -c "$cookie_jar" -b "$cookie_jar" -H "Content-Type: application/json" -H "X-CSRF-Token: $csrf" \
      -X "$method" -d "$data" "$BASE$path"
  else
    curl -sS -c "$cookie_jar" -b "$cookie_jar" -X "$method" "$BASE$path"
  fi
}

create_machine() {
  local key=$1 name=$2 kind=$3
  api POST /api/v1/admin/machines "$(python3 - <<PY
import json
print(json.dumps({
  "machine_key": "$key",
  "name": "$name",
  "kind": "$kind",
  "create_machine_token": True,
}))
PY
)"
}

ingest() {
  local token=$1 payload=$2
  curl -sS -H "Authorization: Bearer $token" -H "Content-Type: application/json" \
    -d "$payload" "$BASE/ingest/v1/events" >/dev/null
}

uuid() { python3 -c 'import uuid; print(uuid.uuid4())'; }
now() { python3 -c 'from datetime import datetime,timezone; print(datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%S.000Z"))'; }

seed_box() {
  local key=$1 name=$2 kind=$3 cpu=$4 memu=$5 memt=$6 disku=$7 diskt=$8
  local created token
  created=$(create_machine "$key" "$name" "$kind")
  token=$(printf '%s' "$created" | python3 -c 'import json,sys; d=json.load(sys.stdin).get("data",{}); print(((d.get("token") or {}).get("token")) or "")')
  if [[ -z "$token" ]]; then
    echo "skip $key (already exists or create failed): $created" >&2
    return 0
  fi
  echo "seeded $name token=${token:0:16}…"

  ingest "$token" "$(python3 - <<PY
import json
print(json.dumps({
  "schema_version": 1,
  "event_id": "$(uuid)",
  "event_type": "machine.heartbeat",
  "occurred_at": "$(now)",
  "payload": {"hostname": "$key", "os": "linux", "arch": "amd64", "heartbeat_interval_seconds": 30, "collector_version": "dev"}
}))
PY
)"
  ingest "$token" "$(python3 - <<PY
import json
print(json.dumps({
  "schema_version": 1,
  "event_id": "$(uuid)",
  "event_type": "metric.sample",
  "occurred_at": "$(now)",
  "payload": {
    "cpu_percent": $cpu,
    "memory_used_bytes": $memu,
    "memory_total_bytes": $memt,
    "root_disk_used_bytes": $disku,
    "root_disk_total_bytes": $diskt,
    "network_rx_bps": 6800,
    "network_tx_bps": 10400
  }
}))
PY
)"
  ingest "$token" "$(python3 - <<PY
import json
print(json.dumps({
  "schema_version": 1,
  "event_id": "$(uuid)",
  "event_type": "service.state",
  "occurred_at": "$(now)",
  "service_key": "agent",
  "payload": {"name": "Agent", "type": "agent", "state": "running", "summary": "idle", "severity": "normal"}
}))
PY
)"
  ingest "$token" "$(python3 - <<PY
import json
print(json.dumps({
  "schema_version": 1,
  "event_id": "$(uuid)",
  "event_type": "status.upsert",
  "occurred_at": "$(now)",
  "service_key": "agent",
  "payload": {"items": [
    {"key": "queue", "label": "队列", "value": 3, "value_type": "number", "severity": "info", "sort_order": 1},
    {"key": "last", "label": "上次运行", "value": "ok", "value_type": "string", "severity": "normal", "sort_order": 2}
  ]}
}))
PY
)"
  ingest "$token" "$(python3 - <<PY
import json
print(json.dumps({
  "schema_version": 1,
  "event_id": "$(uuid)",
  "event_type": "log.pin",
  "occurred_at": "$(now)",
  "service_key": "agent",
  "payload": {"markdown": "**置顶**：$name 当前任务：同步状态并等待下一次心跳。", "severity": "info"}
}))
PY
)"
  local i
  for i in $(seq 1 12); do
    ingest "$token" "$(python3 - <<PY
import json
from datetime import datetime, timezone, timedelta
ts = (datetime.now(timezone.utc) - timedelta(seconds=12-$i)).strftime("%Y-%m-%dT%H:%M:%S.000Z")
sev = "warning" if $i % 4 == 0 else "info"
print(json.dumps({
  "schema_version": 1,
  "event_id": "$(uuid)",
  "event_type": "log.append",
  "occurred_at": ts,
  "service_key": "agent",
  "payload": {"markdown": "心跳 #$i：采集完成，CPU ${cpu}%", "severity": sev, "source": "collector"}
}))
PY
)"
  done
}

seed_box "vm-16-3-opencloudos" "腾讯云 OpenCloudOS" "vm" 1.3 6012954214 17179869184 2348810240 10737418240
seed_box "home-nas" "家庭 NAS" "physical" 12.4 12884901888 34359738368 900000000000 2000000000000
seed_box "build-agent" "构建 Agent" "virtual" 48.2 6442450944 8589934592 21474836480 107374182400

echo "seed complete"
