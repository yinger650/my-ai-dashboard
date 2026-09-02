package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"agentboard/internal/event"
)

const metricCols = `occurred_at, cpu_percent, load1, load5, load15, memory_used_bytes, memory_total_bytes, swap_used_bytes, swap_total_bytes, disk_read_bps, disk_write_bps, network_rx_bps, network_tx_bps, root_disk_used_bytes, root_disk_total_bytes, extra_json`

func scanMetric(sc interface{ Scan(...any) error }) (*MetricSample, error) {
	var m MetricSample
	err := sc.Scan(&m.OccurredAt, &m.CPUPercent, &m.Load1, &m.Load5, &m.Load15, &m.MemoryUsedBytes, &m.MemoryTotalBytes,
		&m.SwapUsedBytes, &m.SwapTotalBytes, &m.DiskReadBps, &m.DiskWriteBps, &m.NetworkRxBps, &m.NetworkTxBps,
		&m.RootDiskUsedBytes, &m.RootDiskTotalBytes, &m.ExtraJSON)
	if err != nil {
		return nil, err
	}
	return &m, nil
}

// LatestPortSnapshot returns the newest machine.port_snapshot payload.
func (s *Store) LatestPortSnapshot(ctx context.Context, machineID string) (payload string, occurredAt string, err error) {
	err = s.db.QueryRowContext(ctx, `
		SELECT payload_json, occurred_at FROM events
		WHERE machine_id = ? AND event_type = 'machine.port_snapshot'
		ORDER BY occurred_at DESC LIMIT 1`, machineID).Scan(&payload, &occurredAt)
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", ErrNotFound
	}
	return payload, occurredAt, err
}

// LatestMetric returns the most recent metric sample for a machine.
func (s *Store) LatestMetric(ctx context.Context, machineID string) (*MetricSample, error) {
	row := s.db.QueryRowContext(ctx, `SELECT `+metricCols+` FROM metric_samples WHERE machine_id = ? ORDER BY occurred_at DESC LIMIT 1`, machineID)
	m, err := scanMetric(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return m, err
}

// MetricsSince returns metric samples at/after the given time, ascending, capped.
func (s *Store) MetricsSince(ctx context.Context, machineID, sinceISO string, limit int) ([]*MetricSample, error) {
	if limit <= 0 || limit > 2000 {
		limit = 1000
	}
	rows, err := s.db.QueryContext(ctx, `SELECT `+metricCols+` FROM metric_samples WHERE machine_id = ? AND occurred_at >= ? ORDER BY occurred_at ASC LIMIT ?`, machineID, sinceISO, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*MetricSample
	for rows.Next() {
		m, err := scanMetric(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// Sparkline returns the most recent n metric samples, ascending.
func (s *Store) Sparkline(ctx context.Context, machineID string, n int) ([]*MetricSample, error) {
	if n <= 0 {
		n = 30
	}
	rows, err := s.db.QueryContext(ctx, `SELECT `+metricCols+` FROM (SELECT `+metricCols+` FROM metric_samples WHERE machine_id = ? ORDER BY occurred_at DESC LIMIT ?) ORDER BY occurred_at ASC`, machineID, n)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*MetricSample
	for rows.Next() {
		m, err := scanMetric(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// ServiceSeverityCounts returns counts of enabled services by severity bucket.
func (s *Store) ServiceSeverityCounts(ctx context.Context, machineID string) (map[string]int, error) {
	svcs, err := s.ListServicesByMachine(ctx, machineID)
	if err != nil {
		return nil, err
	}
	out := map[string]int{"normal": 0, "info": 0, "warning": 0, "error": 0, "unknown": 0}
	for _, svc := range svcs {
		if !svc.Enabled {
			continue
		}
		out[svc.Severity]++
	}
	return out, nil
}

const logEntrySelect = `
		SELECT e.event_id, e.severity, e.occurred_at, e.payload_json,
			IFNULL(e.service_id,''), IFNULL(sv.service_key,''), IFNULL(sv.name,''), IFNULL(r.run_key,'')
		FROM events e
		LEFT JOIN services sv ON sv.id = e.service_id
		LEFT JOIN runs r ON r.id = e.run_id`

// RecentMachineLogs returns recent warning/error log.append entries for a machine.
func (s *Store) RecentMachineLogs(ctx context.Context, machineID string, limit int) ([]LogEntry, error) {
	rows, err := s.db.QueryContext(ctx, logEntrySelect+`
		WHERE e.machine_id = ? AND e.event_type = 'log.append' AND e.severity IN ('warning','error')
		ORDER BY e.occurred_at DESC LIMIT ?`, machineID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanLogRows(rows)
}

// ListMachineLogs returns log.append entries for a machine (all severities),
// newest first, optionally before a cursor (occurred_at).
func (s *Store) ListMachineLogs(ctx context.Context, machineID, beforeISO string, limit int) ([]LogEntry, error) {
	return s.ListMachineLogsExcluding(ctx, machineID, beforeISO, limit, nil)
}

// ListMachineLogsExcluding is ListMachineLogs minus service_key / source values.
func (s *Store) ListMachineLogsExcluding(ctx context.Context, machineID, beforeISO string, limit int, exclude []string) ([]LogEntry, error) {
	if limit <= 0 || limit > 200 {
		limit = 30
	}
	where := `e.machine_id = ? AND e.event_type = 'log.append'`
	args := []any{machineID}
	if beforeISO != "" {
		where += ` AND e.occurred_at < ?`
		args = append(args, beforeISO)
	}
	for _, key := range exclude {
		if key == "" {
			continue
		}
		where += ` AND IFNULL(sv.service_key,'') != ? AND IFNULL(json_extract(e.payload_json, '$.source'), '') != ?`
		args = append(args, key, key)
	}
	args = append(args, limit)
	rows, err := s.db.QueryContext(ctx, logEntrySelect+`
		WHERE `+where+`
		ORDER BY e.occurred_at DESC LIMIT ?`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanLogRows(rows)
}

// ListServiceLogs returns log.append entries for a service, newest first,
// optionally before a cursor (occurred_at).
func (s *Store) ListServiceLogs(ctx context.Context, serviceID, beforeISO string, limit int) ([]LogEntry, error) {
	if limit <= 0 {
		limit = 100
	}
	if limit > 200 {
		limit = 200
	}
	var rows *sql.Rows
	var err error
	if beforeISO == "" {
		rows, err = s.db.QueryContext(ctx, logEntrySelect+`
			WHERE e.service_id = ? AND e.event_type = 'log.append'
			ORDER BY e.occurred_at DESC LIMIT ?`, serviceID, limit)
	} else {
		rows, err = s.db.QueryContext(ctx, logEntrySelect+`
			WHERE e.service_id = ? AND e.event_type = 'log.append' AND e.occurred_at < ?
			ORDER BY e.occurred_at DESC LIMIT ?`, serviceID, beforeISO, limit)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanLogRows(rows)
}

func scanLogRows(rows *sql.Rows) ([]LogEntry, error) {
	var out []LogEntry
	for rows.Next() {
		var eid, sev, occ, payload, sid, skey, sname, rkey string
		if err := rows.Scan(&eid, &sev, &occ, &payload, &sid, &skey, &sname, &rkey); err != nil {
			return nil, err
		}
		var lp event.LogPayload
		_ = json.Unmarshal([]byte(payload), &lp)
		out = append(out, LogEntry{
			EventID: eid, Markdown: lp.Markdown, Severity: sev, Source: lp.Source,
			OccurredAt: occ, ServiceID: sid, ServiceKey: skey, ServiceName: sname, RunKey: rkey,
		})
	}
	return out, rows.Err()
}

// GetPinnedLog returns the pinned log for a service, or ErrNotFound.
func (s *Store) GetPinnedLog(ctx context.Context, serviceID string) (*PinnedLog, error) {
	row := s.db.QueryRowContext(ctx, `SELECT service_id, event_id, markdown, severity, occurred_at, updated_at FROM pinned_logs WHERE service_id = ?`, serviceID)
	var p PinnedLog
	err := row.Scan(&p.ServiceID, &p.EventID, &p.Markdown, &p.Severity, &p.OccurredAt, &p.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return &p, err
}

// ListPinnedLogsByMachine returns pinned logs for all services on a machine.
func (s *Store) ListPinnedLogsByMachine(ctx context.Context, machineID string) ([]PinnedLog, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT p.service_id, p.event_id, p.markdown, p.severity, p.occurred_at, p.updated_at, IFNULL(sv.service_key,''), IFNULL(sv.name,'')
		FROM pinned_logs p
		JOIN services sv ON sv.id = p.service_id
		WHERE sv.machine_id = ? AND sv.deleted_at IS NULL
		ORDER BY p.updated_at DESC`, machineID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PinnedLog
	for rows.Next() {
		var p PinnedLog
		if err := rows.Scan(&p.ServiceID, &p.EventID, &p.Markdown, &p.Severity, &p.OccurredAt, &p.UpdatedAt, &p.ServiceKey, &p.ServiceName); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// ListStatuses returns current status items for a service.
func (s *Store) ListStatuses(ctx context.Context, serviceID string) ([]CurrentStatus, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT service_id, status_key, label, value_json, value_type, unit, severity, display_format, sort_order, occurred_at, updated_at FROM current_status WHERE service_id = ? ORDER BY sort_order, status_key`, serviceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []CurrentStatus
	for rows.Next() {
		var c CurrentStatus
		if err := rows.Scan(&c.ServiceID, &c.StatusKey, &c.Label, &c.ValueJSON, &c.ValueType, &c.Unit, &c.Severity, &c.DisplayFormat, &c.SortOrder, &c.OccurredAt, &c.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// ListStatusesByMachine returns current status items for every service on a machine.
func (s *Store) ListStatusesByMachine(ctx context.Context, machineID string) ([]CurrentStatus, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT c.service_id, c.status_key, c.label, c.value_json, c.value_type, c.unit, c.severity, c.display_format, c.sort_order, c.occurred_at, c.updated_at, IFNULL(sv.service_key,''), IFNULL(sv.name,'')
		FROM current_status c
		JOIN services sv ON sv.id = c.service_id
		WHERE sv.machine_id = ? AND sv.deleted_at IS NULL AND sv.enabled = 1
		ORDER BY sv.sort_order, sv.name, c.sort_order, c.status_key`, machineID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []CurrentStatus
	for rows.Next() {
		var c CurrentStatus
		if err := rows.Scan(&c.ServiceID, &c.StatusKey, &c.Label, &c.ValueJSON, &c.ValueType, &c.Unit, &c.Severity, &c.DisplayFormat, &c.SortOrder, &c.OccurredAt, &c.UpdatedAt, &c.ServiceKey, &c.ServiceName); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// ListRuns returns runs for a service, newest first.
func (s *Store) ListRuns(ctx context.Context, serviceID string, limit int) ([]*Run, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, service_id, run_key, status, summary, started_at, finished_at, provider, provider_agent_id, provider_run_id, duration_ms, input_tokens, output_tokens, metadata_json, created_at, updated_at
		FROM runs WHERE service_id = ? ORDER BY created_at DESC LIMIT ?`, serviceID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Run
	for rows.Next() {
		var r Run
		if err := rows.Scan(&r.ID, &r.ServiceID, &r.RunKey, &r.Status, &r.Summary, &r.StartedAt, &r.FinishedAt, &r.Provider, &r.ProviderAgentID, &r.ProviderRunID, &r.DurationMs, &r.InputTokens, &r.OutputTokens, &r.MetadataJSON, &r.CreatedAt, &r.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, &r)
	}
	return out, rows.Err()
}

// ListActiveRunsByMachine returns non-terminal runs for a machine, newest first.
func (s *Store) ListActiveRunsByMachine(ctx context.Context, machineID string) ([]ActiveRun, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT r.id, r.service_id, sv.service_key, sv.name, r.run_key, r.status, r.summary, r.started_at, r.created_at
		FROM runs r
		JOIN services sv ON sv.id = r.service_id
		WHERE sv.machine_id = ? AND sv.deleted_at IS NULL
		  AND r.status IN ('queued','running','waiting_input','blocked')
		ORDER BY r.updated_at DESC`, machineID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ActiveRun
	for rows.Next() {
		var r ActiveRun
		if err := rows.Scan(&r.ID, &r.ServiceID, &r.ServiceKey, &r.ServiceName, &r.RunKey, &r.Status, &r.Summary, &r.StartedAt, &r.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	if out == nil {
		out = []ActiveRun{}
	}
	return out, rows.Err()
}
