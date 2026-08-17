export interface MetricSample {
  occurred_at: string;
  cpu_percent: number | null;
  load1: number | null;
  memory_used_bytes: number | null;
  memory_total_bytes: number | null;
  swap_used_bytes: number | null;
  swap_total_bytes: number | null;
  disk_read_bps: number | null;
  disk_write_bps: number | null;
  network_rx_bps: number | null;
  network_tx_bps: number | null;
  root_disk_used_bytes: number | null;
  root_disk_total_bytes: number | null;
}

export interface LogEntry {
  event_id: string;
  markdown: string;
  severity: string;
  source?: string;
  occurred_at: string;
}

export interface SparkPoint {
  t: string;
  cpu: number | null;
  mem: number | null;
  net: number | null;
}

export interface ServiceCounts {
  normal: number;
  info: number;
  warning: number;
  error: number;
  unknown: number;
}

export interface BoardMachine {
  id: string;
  machine_key: string;
  name: string;
  kind: string;
  health: string;
  resource_severity: string;
  last_seen_at: string | null;
  os: string | null;
  arch: string | null;
  latest_metric: MetricSample | null;
  service_counts: ServiceCounts;
  recent_logs: LogEntry[] | null;
  sparkline: SparkPoint[] | null;
}

export interface Board {
  title: string;
  machines: BoardMachine[];
  recent_abnormal: number;
  server_time: string;
}

export interface Machine {
  id: string;
  machine_key: string;
  name: string;
  kind: string;
  description: string;
  os: string | null;
  arch: string | null;
  hostname: string | null;
  collector_version: string | null;
  boot_id: string | null;
  heartbeat_interval_seconds: number;
  last_seen_at: string | null;
  enabled: boolean;
}

export interface Service {
  id: string;
  machine_id: string;
  service_key: string;
  name: string;
  type: string;
  description: string;
  current_state: string;
  state_summary: string;
  severity: string;
  last_seen_at: string | null;
  last_run_at: string | null;
  enabled: boolean;
}

export interface StatusItem {
  status_key: string;
  label: string;
  value_json: string;
  value_type: string;
  unit: string | null;
  severity: string;
  display_format: string;
  sort_order: number;
}

export interface Run {
  id: string;
  run_key: string;
  status: string;
  summary: string;
  started_at: string | null;
  finished_at: string | null;
  provider: string | null;
  duration_ms: number | null;
  created_at: string;
}

export interface PinnedLog {
  markdown: string;
  severity: string;
  occurred_at: string;
}

export interface AccessLog {
  id: string;
  occurred_at: string;
  request_id: string;
  actor_type: string;
  method: string;
  path: string;
  status_code: number;
  ip: string | null;
  result: string;
  reason: string;
  is_abnormal: boolean;
}

export interface TokenInfo {
  id: string;
  name: string;
  token_prefix: string;
  scope: string;
  machine_id: string | null;
  service_id: string | null;
  last_used_at: string | null;
  last_used_ip: string | null;
  enabled: boolean;
  revoked_at: string | null;
}
