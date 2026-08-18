-- +goose Up
-- +goose StatementBegin
CREATE TABLE settings (
  key TEXT PRIMARY KEY,
  value_json TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE admin_credentials (
  id INTEGER PRIMARY KEY CHECK (id = 1),
  password_hash TEXT NOT NULL,
  totp_secret_encrypted TEXT,
  recovery_codes_hash_json TEXT,
  failed_attempts INTEGER NOT NULL DEFAULT 0,
  locked_until TEXT,
  updated_at TEXT NOT NULL
);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE admin_sessions (
  id TEXT PRIMARY KEY,
  token_hash TEXT NOT NULL UNIQUE,
  csrf_token_hash TEXT NOT NULL,
  created_at TEXT NOT NULL,
  expires_at TEXT NOT NULL,
  last_seen_at TEXT NOT NULL,
  ip TEXT,
  user_agent TEXT
);
-- +goose StatementEnd
-- +goose StatementBegin
CREATE INDEX idx_admin_sessions_expires ON admin_sessions(expires_at);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE machines (
  id TEXT PRIMARY KEY,
  machine_key TEXT NOT NULL UNIQUE,
  name TEXT NOT NULL,
  kind TEXT NOT NULL CHECK(kind IN ('physical','vm','container_host','virtual')),
  description TEXT NOT NULL DEFAULT '',
  os TEXT,
  arch TEXT,
  hostname TEXT,
  collector_version TEXT,
  boot_id TEXT,
  heartbeat_interval_seconds INTEGER NOT NULL DEFAULT 30,
  last_seen_at TEXT,
  last_event_at TEXT,
  enabled INTEGER NOT NULL DEFAULT 1,
  auto_create_services INTEGER NOT NULL DEFAULT 1,
  metadata_json TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  deleted_at TEXT
);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE services (
  id TEXT PRIMARY KEY,
  machine_id TEXT NOT NULL REFERENCES machines(id),
  service_key TEXT NOT NULL,
  name TEXT NOT NULL,
  type TEXT NOT NULL CHECK(type IN ('daemon','scheduled','job','agent','virtual')),
  description TEXT NOT NULL DEFAULT '',
  current_state TEXT NOT NULL DEFAULT 'unknown',
  state_summary TEXT NOT NULL DEFAULT '',
  severity TEXT NOT NULL DEFAULT 'unknown',
  ttl_seconds INTEGER,
  last_seen_at TEXT,
  last_run_at TEXT,
  enabled INTEGER NOT NULL DEFAULT 1,
  sort_order INTEGER NOT NULL DEFAULT 0,
  metadata_json TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  deleted_at TEXT,
  UNIQUE(machine_id, service_key)
);
-- +goose StatementEnd
-- +goose StatementBegin
CREATE INDEX idx_services_machine ON services(machine_id, deleted_at);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE api_tokens (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  token_prefix TEXT NOT NULL,
  token_hash TEXT NOT NULL UNIQUE,
  scope TEXT NOT NULL CHECK(scope IN ('machine_ingest','service_ingest','viewer')),
  machine_id TEXT REFERENCES machines(id),
  service_id TEXT REFERENCES services(id),
  ip_allowlist_json TEXT NOT NULL DEFAULT '[]',
  requests_per_minute INTEGER NOT NULL DEFAULT 120,
  bytes_per_day INTEGER NOT NULL DEFAULT 104857600,
  allow_artifact_download INTEGER NOT NULL DEFAULT 0,
  last_used_at TEXT,
  last_used_ip TEXT,
  expires_at TEXT,
  enabled INTEGER NOT NULL DEFAULT 1,
  created_at TEXT NOT NULL,
  revoked_at TEXT
);
-- +goose StatementEnd
-- +goose StatementBegin
CREATE INDEX idx_api_tokens_prefix ON api_tokens(token_prefix);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE runs (
  id TEXT PRIMARY KEY,
  service_id TEXT NOT NULL REFERENCES services(id),
  run_key TEXT NOT NULL,
  status TEXT NOT NULL CHECK(status IN (
    'queued','running','waiting_input','blocked','succeeded',
    'failed','cancelled','timed_out'
  )),
  summary TEXT NOT NULL DEFAULT '',
  started_at TEXT,
  finished_at TEXT,
  provider TEXT,
  provider_agent_id TEXT,
  provider_run_id TEXT,
  duration_ms INTEGER,
  input_tokens INTEGER,
  output_tokens INTEGER,
  metadata_json TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  UNIQUE(service_id, run_key)
);
-- +goose StatementEnd
-- +goose StatementBegin
CREATE INDEX idx_runs_service_time ON runs(service_id, created_at DESC);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE events (
  id TEXT PRIMARY KEY,
  event_id TEXT NOT NULL UNIQUE,
  machine_id TEXT NOT NULL REFERENCES machines(id),
  service_id TEXT REFERENCES services(id),
  run_id TEXT REFERENCES runs(id),
  event_type TEXT NOT NULL,
  severity TEXT NOT NULL DEFAULT 'info',
  occurred_at TEXT NOT NULL,
  received_at TEXT NOT NULL,
  boot_id TEXT,
  sequence INTEGER,
  payload_json TEXT NOT NULL,
  expires_at TEXT
);
-- +goose StatementEnd
-- +goose StatementBegin
CREATE INDEX idx_events_machine_time ON events(machine_id, occurred_at DESC);
-- +goose StatementEnd
-- +goose StatementBegin
CREATE INDEX idx_events_service_time ON events(service_id, occurred_at DESC);
-- +goose StatementEnd
-- +goose StatementBegin
CREATE INDEX idx_events_type_time ON events(event_type, occurred_at DESC);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE metric_samples (
  id TEXT PRIMARY KEY,
  event_id TEXT NOT NULL UNIQUE REFERENCES events(event_id) ON DELETE CASCADE,
  machine_id TEXT NOT NULL REFERENCES machines(id),
  occurred_at TEXT NOT NULL,
  cpu_percent REAL,
  load1 REAL,
  load5 REAL,
  load15 REAL,
  memory_used_bytes INTEGER,
  memory_total_bytes INTEGER,
  swap_used_bytes INTEGER,
  swap_total_bytes INTEGER,
  disk_read_bps REAL,
  disk_write_bps REAL,
  network_rx_bps REAL,
  network_tx_bps REAL,
  root_disk_used_bytes INTEGER,
  root_disk_total_bytes INTEGER,
  extra_json TEXT NOT NULL DEFAULT '{}'
);
-- +goose StatementEnd
-- +goose StatementBegin
CREATE INDEX idx_metrics_machine_time ON metric_samples(machine_id, occurred_at DESC);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE current_status (
  service_id TEXT NOT NULL REFERENCES services(id),
  status_key TEXT NOT NULL,
  label TEXT NOT NULL,
  value_json TEXT NOT NULL,
  value_type TEXT NOT NULL CHECK(value_type IN ('string','number','boolean','progress','duration','bytes')),
  unit TEXT,
  severity TEXT NOT NULL CHECK(severity IN ('normal','info','warning','error','unknown')),
  display_format TEXT NOT NULL DEFAULT 'text',
  sort_order INTEGER NOT NULL DEFAULT 0,
  occurred_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  PRIMARY KEY(service_id, status_key)
);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE pinned_logs (
  service_id TEXT PRIMARY KEY REFERENCES services(id),
  event_id TEXT NOT NULL REFERENCES events(event_id),
  markdown TEXT NOT NULL,
  severity TEXT NOT NULL,
  occurred_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE artifacts (
  id TEXT PRIMARY KEY,
  upload_event_id TEXT NOT NULL UNIQUE,
  machine_id TEXT NOT NULL REFERENCES machines(id),
  service_id TEXT REFERENCES services(id),
  stored_name TEXT NOT NULL UNIQUE,
  original_name TEXT NOT NULL,
  mime_type TEXT NOT NULL,
  size_bytes INTEGER NOT NULL,
  sha256 TEXT NOT NULL,
  created_at TEXT NOT NULL,
  deleted_at TEXT
);
-- +goose StatementEnd
-- +goose StatementBegin
CREATE INDEX idx_artifacts_service_time ON artifacts(service_id, created_at DESC);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE access_logs (
  id TEXT PRIMARY KEY,
  occurred_at TEXT NOT NULL,
  request_id TEXT NOT NULL,
  actor_type TEXT NOT NULL CHECK(actor_type IN ('anonymous','admin','machine','service','viewer','system')),
  actor_id TEXT,
  method TEXT NOT NULL,
  path TEXT NOT NULL,
  status_code INTEGER NOT NULL,
  ip TEXT,
  user_agent TEXT,
  bytes_in INTEGER NOT NULL DEFAULT 0,
  duration_ms INTEGER NOT NULL DEFAULT 0,
  result TEXT NOT NULL,
  reason TEXT NOT NULL DEFAULT '',
  is_abnormal INTEGER NOT NULL DEFAULT 0
);
-- +goose StatementEnd
-- +goose StatementBegin
CREATE INDEX idx_access_time ON access_logs(occurred_at DESC);
-- +goose StatementEnd
-- +goose StatementBegin
CREATE INDEX idx_access_abnormal ON access_logs(is_abnormal, occurred_at DESC);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE token_daily_usage (
  token_id TEXT NOT NULL REFERENCES api_tokens(id) ON DELETE CASCADE,
  usage_date TEXT NOT NULL,
  request_count INTEGER NOT NULL DEFAULT 0,
  bytes_in INTEGER NOT NULL DEFAULT 0,
  updated_at TEXT NOT NULL,
  PRIMARY KEY(token_id, usage_date)
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS token_daily_usage;
DROP TABLE IF EXISTS access_logs;
DROP TABLE IF EXISTS artifacts;
DROP TABLE IF EXISTS pinned_logs;
DROP TABLE IF EXISTS current_status;
DROP TABLE IF EXISTS metric_samples;
DROP TABLE IF EXISTS events;
DROP TABLE IF EXISTS runs;
DROP TABLE IF EXISTS api_tokens;
DROP TABLE IF EXISTS services;
DROP TABLE IF EXISTS machines;
DROP TABLE IF EXISTS admin_sessions;
DROP TABLE IF EXISTS admin_credentials;
DROP TABLE IF EXISTS settings;
-- +goose StatementEnd
