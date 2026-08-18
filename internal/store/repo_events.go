package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"agentboard/internal/event"
	"agentboard/internal/shared"
)

// IngestAuth carries the resolved authorization context for an event.
type IngestAuth struct {
	MachineID          string
	ServiceID          *string // non-nil for service-scoped tokens
	AutoCreateServices bool
}

// IngestResult is the per-event outcome.
type IngestResult struct {
	Status   string // accepted | duplicate | rejected
	Code     string
	Message  string
	Abnormal bool
}

func rejected(code, msg string, abnormal bool) IngestResult {
	return IngestResult{Status: "rejected", Code: code, Message: msg, Abnormal: abnormal}
}

// IngestEvent performs the full spec 13.2 transaction for one event.
func (s *Store) IngestEvent(ctx context.Context, env *event.Envelope, auth IngestAuth, receivedAt string) (IngestResult, error) {
	if !event.KnownType(env.EventType) {
		return rejected("unsupported_event_type", "unknown event type", true), nil
	}
	if !shared.IsUUID(env.EventID) {
		return rejected("validation_failed", "event_id must be a UUID", true), nil
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return IngestResult{}, err
	}
	defer func() { _ = tx.Rollback() }()

	// Idempotency check on event_id.
	var existing int
	if err := tx.QueryRowContext(ctx, `SELECT 1 FROM events WHERE event_id = ?`, env.EventID).Scan(&existing); err == nil {
		return IngestResult{Status: "duplicate"}, nil
	} else if !errors.Is(err, sql.ErrNoRows) {
		return IngestResult{}, err
	}

	res, err := s.projectTx(ctx, tx, env, auth, receivedAt)
	if err != nil {
		return IngestResult{}, err
	}
	if res.Status == "rejected" {
		return res, nil
	}
	if err := tx.Commit(); err != nil {
		return IngestResult{}, err
	}
	return res, nil
}

func (s *Store) projectTx(ctx context.Context, tx *sql.Tx, env *event.Envelope, auth IngestAuth, receivedAt string) (IngestResult, error) {
	var serviceID *string
	var runID *string
	severity := "info"

	switch env.EventType {
	case event.TypeHeartbeat, event.TypeMetricSample, event.TypePortSnapshot, event.TypeServiceSnapshot:
		// machine-scoped; service_key must be empty
		if env.ServiceKey != "" {
			return rejected("validation_failed", "service_key must be empty for machine events", true), nil
		}

	case event.TypeServiceState, event.TypeStatusUpsert, event.TypeLogAppend, event.TypeLogPin, event.TypeRunTransition:
		sid, res, err := s.resolveService(ctx, tx, env, auth)
		if err != nil {
			return IngestResult{}, err
		}
		if res.Status == "rejected" {
			return res, nil
		}
		serviceID = sid

	case event.TypeCollectorNotice:
		if env.ServiceKey != "" {
			sid, res, err := s.resolveService(ctx, tx, env, auth)
			if err != nil {
				return IngestResult{}, err
			}
			if res.Status == "rejected" {
				return res, nil
			}
			serviceID = sid
		} else if auth.ServiceID != nil {
			serviceID = auth.ServiceID
		}
	}

	// Type-specific projection (before inserting the event row where a FK is needed).
	switch env.EventType {
	case event.TypeHeartbeat:
		var hb event.Heartbeat
		if err := json.Unmarshal(env.Payload, &hb); err != nil {
			return rejected("validation_failed", "invalid heartbeat payload", true), nil
		}
		if err := s.updateMachineFromHeartbeatTx(ctx, tx, auth.MachineID, &hb, env); err != nil {
			return IngestResult{}, err
		}

	case event.TypeServiceState:
		var ss event.ServiceState
		if err := json.Unmarshal(env.Payload, &ss); err != nil {
			return rejected("validation_failed", "invalid service.state payload", true), nil
		}
		if ss.Severity == "" {
			ss.Severity = "unknown"
		}
		if !event.ValidSeverity(ss.Severity) {
			return rejected("validation_failed", "invalid severity", true), nil
		}
		severity = ss.Severity
		if _, err := tx.ExecContext(ctx, `UPDATE services SET current_state = ?, state_summary = ?, severity = ?, ttl_seconds = COALESCE(?, ttl_seconds), last_seen_at = ?, updated_at = ? WHERE id = ?`,
			ss.State, ss.Summary, ss.Severity, ss.TTLSeconds, env.OccurredAt, receivedAt, *serviceID); err != nil {
			return IngestResult{}, err
		}

	case event.TypeStatusUpsert:
		var su event.StatusUpsert
		if err := json.Unmarshal(env.Payload, &su); err != nil {
			return rejected("validation_failed", "invalid status.upsert payload", true), nil
		}
		if len(su.Items) == 0 || len(su.Items) > 50 {
			return rejected("validation_failed", "status items must be 1..50", true), nil
		}
		severity = "info"
		for _, it := range su.Items {
			if !event.ValidSeverity(it.Severity) {
				return rejected("validation_failed", "invalid status severity", true), nil
			}
			severity = maxSeverity(severity, it.Severity)
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO current_status (service_id, status_key, label, value_json, value_type, unit, severity, display_format, sort_order, occurred_at, updated_at)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
				ON CONFLICT(service_id, status_key) DO UPDATE SET label=excluded.label, value_json=excluded.value_json, value_type=excluded.value_type, unit=excluded.unit, severity=excluded.severity, display_format=excluded.display_format, sort_order=excluded.sort_order, occurred_at=excluded.occurred_at, updated_at=excluded.updated_at`,
				*serviceID, it.Key, it.Label, string(nonNullJSON(it.Value)), it.ValueType, nullStr(it.Unit), it.Severity, defaultStr(it.DisplayFormat, "text"), it.SortOrder, env.OccurredAt, receivedAt); err != nil {
				return IngestResult{}, err
			}
		}

	case event.TypeLogAppend, event.TypeLogPin:
		var lp event.LogPayload
		if err := json.Unmarshal(env.Payload, &lp); err != nil {
			return rejected("validation_failed", "invalid log payload", true), nil
		}
		if lp.Severity == "" {
			lp.Severity = "info"
		}
		if !event.ValidSeverity(lp.Severity) {
			return rejected("validation_failed", "invalid severity", true), nil
		}
		if len(lp.Markdown) > 64*1024 {
			return rejected("payload_too_large", "markdown too large", true), nil
		}
		severity = lp.Severity

	case event.TypeRunTransition:
		var rt event.RunTransition
		if err := json.Unmarshal(env.Payload, &rt); err != nil {
			return rejected("validation_failed", "invalid run.transition payload", true), nil
		}
		if env.RunKey == "" {
			return rejected("validation_failed", "run_key required", true), nil
		}
		if !event.ValidRunStatus(rt.Status) {
			return rejected("validation_failed", "invalid run status", true), nil
		}
		rid, res, err := s.upsertRunTx(ctx, tx, *serviceID, env.RunKey, &rt)
		if err != nil {
			return IngestResult{}, err
		}
		if res.Status == "rejected" {
			return res, nil
		}
		runID = &rid
		severity = event.RunSeverity(rt.Status)
		if _, err := tx.ExecContext(ctx, `UPDATE services SET last_run_at = ?, updated_at = ? WHERE id = ?`, env.OccurredAt, receivedAt, *serviceID); err != nil {
			return IngestResult{}, err
		}

	case event.TypeCollectorNotice:
		var cn event.CollectorNotice
		if err := json.Unmarshal(env.Payload, &cn); err != nil {
			return rejected("validation_failed", "invalid collector.notice payload", true), nil
		}
		if cn.Severity != "" && !event.ValidSeverity(cn.Severity) {
			return rejected("validation_failed", "invalid severity", true), nil
		}
		if cn.Severity != "" {
			severity = cn.Severity
		}

	case event.TypeServiceSnapshot:
		var snap event.ServiceSnapshot
		if err := json.Unmarshal(env.Payload, &snap); err != nil {
			return rejected("validation_failed", "invalid machine.service_snapshot payload", true), nil
		}
		if errRes, err := s.projectServiceSnapshotTx(ctx, tx, env, auth, receivedAt, &snap); err != nil {
			return IngestResult{}, err
		} else if errRes.Status == "rejected" {
			return errRes, nil
		}
	}

	// Insert the immutable event row.
	id := shared.NewID()
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO events (id, event_id, machine_id, service_id, run_id, event_type, severity, occurred_at, received_at, boot_id, sequence, payload_json)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, env.EventID, auth.MachineID, serviceID, runID, env.EventType, severity, env.OccurredAt, receivedAt,
		nullStr(env.BootID), env.Sequence, string(nonNullJSON(env.Payload))); err != nil {
		return IngestResult{}, err
	}

	// metric.sample projection needs the event row (FK on event_id).
	if env.EventType == event.TypeMetricSample {
		var ms event.MetricSample
		if err := json.Unmarshal(env.Payload, &ms); err != nil {
			return rejected("validation_failed", "invalid metric.sample payload", true), nil
		}
		extra, _ := json.Marshal(map[string]any{"filesystems": ms.Filesystems, "interfaces": ms.Interfaces})
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO metric_samples (id, event_id, machine_id, occurred_at, cpu_percent, load1, load5, load15, memory_used_bytes, memory_total_bytes, swap_used_bytes, swap_total_bytes, disk_read_bps, disk_write_bps, network_rx_bps, network_tx_bps, root_disk_used_bytes, root_disk_total_bytes, extra_json)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			shared.NewID(), env.EventID, auth.MachineID, env.OccurredAt, ms.CPUPercent, ms.Load1, ms.Load5, ms.Load15,
			ms.MemoryUsedBytes, ms.MemoryTotalBytes, ms.SwapUsedBytes, ms.SwapTotalBytes, ms.DiskReadBps, ms.DiskWriteBps,
			ms.NetworkRxBps, ms.NetworkTxBps, ms.RootDiskUsedBytes, ms.RootDiskTotalBytes, string(extra)); err != nil {
			return IngestResult{}, err
		}
	}

	// log.pin projection references the event_id (FK).
	if env.EventType == event.TypeLogPin {
		var lp event.LogPayload
		_ = json.Unmarshal(env.Payload, &lp)
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO pinned_logs (service_id, event_id, markdown, severity, occurred_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?)
			ON CONFLICT(service_id) DO UPDATE SET event_id=excluded.event_id, markdown=excluded.markdown, severity=excluded.severity, occurred_at=excluded.occurred_at, updated_at=excluded.updated_at`,
			*serviceID, env.EventID, lp.Markdown, severity, env.OccurredAt, receivedAt); err != nil {
			return IngestResult{}, err
		}
	}

	// Update machine last_seen for all accepted events.
	if _, err := tx.ExecContext(ctx, `UPDATE machines SET last_seen_at = ?, last_event_at = ?, updated_at = ? WHERE id = ?`, receivedAt, env.OccurredAt, receivedAt, auth.MachineID); err != nil {
		return IngestResult{}, err
	}

	return IngestResult{Status: "accepted"}, nil
}

// resolveService resolves and (when permitted) auto-creates the target service.
func (s *Store) resolveService(ctx context.Context, tx *sql.Tx, env *event.Envelope, auth IngestAuth) (*string, IngestResult, error) {
	if auth.ServiceID != nil {
		// Service token: service is fixed; service_key must match or be empty.
		if env.ServiceKey != "" {
			var key string
			if err := tx.QueryRowContext(ctx, `SELECT service_key FROM services WHERE id = ?`, *auth.ServiceID).Scan(&key); err != nil {
				return nil, IngestResult{}, err
			}
			if key != env.ServiceKey {
				return nil, rejected("forbidden", "service_key does not match token", true), nil
			}
		}
		return auth.ServiceID, IngestResult{Status: "accepted"}, nil
	}

	if env.ServiceKey == "" {
		return nil, rejected("validation_failed", "service_key required", true), nil
	}

	var sid string
	err := tx.QueryRowContext(ctx, `SELECT id FROM services WHERE machine_id = ? AND service_key = ? AND deleted_at IS NULL`, auth.MachineID, env.ServiceKey).Scan(&sid)
	if err == nil {
		return &sid, IngestResult{Status: "accepted"}, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, IngestResult{}, err
	}

	// Not found: only service.state and run.transition may auto-create.
	if !auth.AutoCreateServices {
		return nil, rejected("not_found", "service not found", false), nil
	}
	name, stype := "", ""
	switch env.EventType {
	case event.TypeServiceState:
		var ss event.ServiceState
		_ = json.Unmarshal(env.Payload, &ss)
		name, stype = ss.Name, ss.Type
	case event.TypeRunTransition:
		var rt event.RunTransition
		_ = json.Unmarshal(env.Payload, &rt)
		name, stype = rt.ServiceName, rt.ServiceType
	default:
		return nil, rejected("not_found", "service not found", false), nil
	}
	if name == "" || !event.ValidServiceType(stype) {
		return nil, rejected("validation_failed", "cannot auto-create service without valid name/type", true), nil
	}
	sid = shared.NewID()
	now := shared.FormatTime(shared.NowUTC())
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO services (id, machine_id, service_key, name, type, current_state, severity, enabled, metadata_json, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, 'unknown', 'unknown', 1, '{}', ?, ?)`,
		sid, auth.MachineID, env.ServiceKey, name, stype, now, now); err != nil {
		return nil, IngestResult{}, err
	}
	return &sid, IngestResult{Status: "accepted"}, nil
}

func (s *Store) upsertRunTx(ctx context.Context, tx *sql.Tx, serviceID, runKey string, rt *event.RunTransition) (string, IngestResult, error) {
	now := shared.FormatTime(shared.NowUTC())
	var id, curStatus string
	err := tx.QueryRowContext(ctx, `SELECT id, status FROM runs WHERE service_id = ? AND run_key = ?`, serviceID, runKey).Scan(&id, &curStatus)
	if errors.Is(err, sql.ErrNoRows) {
		id = shared.NewID()
		meta, _ := json.Marshal(rt.Metadata)
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO runs (id, service_id, run_key, status, summary, started_at, finished_at, provider, provider_agent_id, provider_run_id, duration_ms, input_tokens, output_tokens, metadata_json, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			id, serviceID, runKey, rt.Status, rt.Summary, nullStr(rt.StartedAt), nullStr(rt.FinishedAt), nullStr(rt.Provider),
			nullStr(rt.ProviderAgentID), nullStr(rt.ProviderRunID), rt.DurationMs, rt.InputTokens, rt.OutputTokens, string(nonNullObj(meta)), now, now); err != nil {
			return "", IngestResult{}, err
		}
		return id, IngestResult{Status: "accepted"}, nil
	}
	if err != nil {
		return "", IngestResult{}, err
	}
	// Existing run.
	if curStatus == rt.Status && event.IsTerminal(curStatus) {
		// Idempotent terminal re-send.
		return id, IngestResult{Status: "accepted"}, nil
	}
	if !event.AllowedTransition(curStatus, rt.Status) {
		return "", rejected("invalid_transition", fmt.Sprintf("cannot transition %s -> %s", curStatus, rt.Status), true), nil
	}
	meta, _ := json.Marshal(rt.Metadata)
	if _, err := tx.ExecContext(ctx, `
		UPDATE runs SET status = ?, summary = COALESCE(NULLIF(?, ''), summary), started_at = COALESCE(?, started_at), finished_at = COALESCE(?, finished_at),
			provider = COALESCE(?, provider), provider_agent_id = COALESCE(?, provider_agent_id), provider_run_id = COALESCE(?, provider_run_id),
			duration_ms = COALESCE(?, duration_ms), input_tokens = COALESCE(?, input_tokens), output_tokens = COALESCE(?, output_tokens),
			metadata_json = ?, updated_at = ?
		WHERE id = ?`,
		rt.Status, rt.Summary, nullStr(rt.StartedAt), nullStr(rt.FinishedAt), nullStr(rt.Provider), nullStr(rt.ProviderAgentID),
		nullStr(rt.ProviderRunID), rt.DurationMs, rt.InputTokens, rt.OutputTokens, string(nonNullObj(meta)), now, id); err != nil {
		return "", IngestResult{}, err
	}
	return id, IngestResult{Status: "accepted"}, nil
}

func (s *Store) projectServiceSnapshotTx(ctx context.Context, tx *sql.Tx, env *event.Envelope, auth IngestAuth, receivedAt string, snap *event.ServiceSnapshot) (IngestResult, error) {
	units := snap.Units
	if len(units) == 0 {
		units = snap.Services
	}
	if len(units) > 200 {
		return rejected("validation_failed", "snapshot units must be <= 200", true), nil
	}
	now := shared.FormatTime(shared.NowUTC())
	for _, u := range units {
		key := u.Unit
		if key == "" {
			continue
		}
		if !event.ValidServiceKey(key) {
			continue
		}
		state, summary, sev := event.UnitProjection(u.Active, u.Sub, "")
		name := u.Name
		if name == "" {
			name = u.Description
		}
		if name == "" {
			name = key
		}

		var sid string
		err := tx.QueryRowContext(ctx, `SELECT id FROM services WHERE machine_id = ? AND service_key = ? AND deleted_at IS NULL`, auth.MachineID, key).Scan(&sid)
		if errors.Is(err, sql.ErrNoRows) {
			if !auth.AutoCreateServices {
				continue
			}
			sid = shared.NewID()
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO services (id, machine_id, service_key, name, type, current_state, state_summary, severity, enabled, metadata_json, last_seen_at, created_at, updated_at)
				VALUES (?, ?, ?, ?, 'daemon', ?, ?, ?, 1, '{}', ?, ?, ?)`,
				sid, auth.MachineID, key, name, state, summary, sev, env.OccurredAt, now, now); err != nil {
				return IngestResult{}, err
			}
			continue
		}
		if err != nil {
			return IngestResult{}, err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE services SET current_state = ?, state_summary = ?, severity = ?, last_seen_at = ?, updated_at = ? WHERE id = ?`,
			state, summary, sev, env.OccurredAt, receivedAt, sid); err != nil {
			return IngestResult{}, err
		}
	}
	return IngestResult{Status: "accepted"}, nil
}

func (s *Store) updateMachineFromHeartbeatTx(ctx context.Context, tx *sql.Tx, machineID string, hb *event.Heartbeat, env *event.Envelope) error {
	now := shared.FormatTime(shared.NowUTC())
	_, err := tx.ExecContext(ctx, `
		UPDATE machines SET
			os = COALESCE(NULLIF(?, ''), os),
			arch = COALESCE(NULLIF(?, ''), arch),
			hostname = COALESCE(NULLIF(?, ''), hostname),
			collector_version = COALESCE(NULLIF(?, ''), collector_version),
			boot_id = COALESCE(NULLIF(?, ''), boot_id),
			heartbeat_interval_seconds = CASE WHEN ? > 0 THEN ? ELSE heartbeat_interval_seconds END,
			last_seen_at = ?, last_event_at = ?, updated_at = ?
		WHERE id = ?`,
		hb.OS, hb.Arch, hb.Hostname, hb.CollectorVersion, env.BootID,
		hb.HeartbeatIntervalSeconds, hb.HeartbeatIntervalSeconds, now, env.OccurredAt, now, machineID)
	return err
}

func maxSeverity(a, b string) string {
	rank := map[string]int{"normal": 0, "info": 1, "unknown": 1, "warning": 2, "error": 3}
	if rank[b] > rank[a] {
		return b
	}
	return a
}

func nullStr(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func defaultStr(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

func nonNullJSON(r json.RawMessage) json.RawMessage {
	if len(r) == 0 {
		return json.RawMessage(`null`)
	}
	return r
}

func nonNullObj(r json.RawMessage) json.RawMessage {
	if len(r) == 0 || string(r) == "null" {
		return json.RawMessage(`{}`)
	}
	return r
}
