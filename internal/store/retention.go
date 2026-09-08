package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"agentboard/internal/event"
	"agentboard/internal/shared"
)

const (
	// DefaultEventQuotaBytes is 5 GiB of event payload storage.
	DefaultEventQuotaBytes int64 = 5 * 1024 * 1024 * 1024
	// DefaultStaleRunIdle is how long a non-agent run may go without activity
	// before maintenance closes it as timed_out.
	DefaultStaleRunIdle = 24 * time.Hour
	// DefaultAgentStaleRunIdle is how long an agent run may go without
	// log.append / log.pin / run.transition before it is failed as interrupted.
	DefaultAgentStaleRunIdle = 30 * time.Minute
	retentionBatch           = 2000
	quotaLoopMax             = 40
	staleRunCloseLog         = "超过 1 天没有新日志，已自动关闭。"
	agentInterruptedLog      = "任务被打断（无后续上报）"
)

// RetentionPolicy is the time + size cap for server-side history.
type RetentionPolicy struct {
	EventDays    int
	MetricDays   int
	AccessDays   int
	QuotaBytes   int64
	StaleRunIdle time.Duration // 0 means DefaultStaleRunIdle; negative skips close
}

// RetentionResult is a single maintenance pass.
type RetentionResult struct {
	ExpiredSessions int64 `json:"expired_sessions"`
	EventsDeleted   int64 `json:"events_deleted"`
	AccessDeleted   int64 `json:"access_deleted"`
	RunsDeleted     int64 `json:"runs_deleted"`
	RunsClosed      int64 `json:"runs_closed"`
	UsageDeleted    int64 `json:"usage_deleted"`
	QuotaDeleted    int64 `json:"quota_deleted"`
	EventsBytes     int64 `json:"events_bytes"`
}

// ApplyRetention deletes expired sessions, aged history, and oldest events
// when the payload store is over QuotaBytes. Current-state pins are kept.
// Non-terminal runs are closed first (agent ~30m interrupted, others StaleRunIdle)
// so last-log detection still sees the original events.
func (s *Store) ApplyRetention(ctx context.Context, p RetentionPolicy) (RetentionResult, error) {
	var out RetentionResult
	n, err := s.DeleteExpiredSessions(ctx)
	if err != nil {
		return out, err
	}
	out.ExpiredSessions = n

	if p.StaleRunIdle >= 0 {
		idle := p.StaleRunIdle
		if idle == 0 {
			idle = DefaultStaleRunIdle
		}
		n, err = s.CloseStaleRuns(ctx, idle)
		if err != nil {
			return out, err
		}
		out.RunsClosed = n
	}

	now := shared.NowUTC()
	if p.EventDays > 0 {
		n, err = s.deleteEventsBefore(ctx, now.AddDate(0, 0, -p.EventDays))
		if err != nil {
			return out, err
		}
		out.EventsDeleted = n
	}
	if p.MetricDays > 0 && (p.EventDays == 0 || p.MetricDays < p.EventDays) {
		// Metrics ride events via ON DELETE CASCADE. A shorter metric window
		// still needs a dedicated delete of metric.sample events.
		n, err = s.deleteEventsBeforeOfType(ctx, now.AddDate(0, 0, -p.MetricDays), "metric.sample")
		if err != nil {
			return out, err
		}
		out.EventsDeleted += n
	}
	if p.AccessDays > 0 {
		n, err = s.deleteAccessBefore(ctx, now.AddDate(0, 0, -p.AccessDays))
		if err != nil {
			return out, err
		}
		out.AccessDeleted = n
	}
	if p.EventDays > 0 {
		n, err = s.deleteRunsBefore(ctx, now.AddDate(0, 0, -p.EventDays))
		if err != nil {
			return out, err
		}
		out.RunsDeleted = n
		n, err = s.deleteTokenUsageBefore(ctx, now.AddDate(0, 0, -p.EventDays))
		if err != nil {
			return out, err
		}
		out.UsageDeleted = n
	}

	if p.QuotaBytes > 0 {
		n, bytes, err := s.enforceEventQuota(ctx, p.QuotaBytes)
		if err != nil {
			return out, err
		}
		out.QuotaDeleted = n
		out.EventsBytes = bytes
	} else {
		out.EventsBytes, _ = s.eventPayloadBytes(ctx)
	}

	_, _ = s.db.ExecContext(ctx, `PRAGMA optimize`)
	_, _ = s.db.ExecContext(ctx, `PRAGMA wal_checkpoint(PASSIVE)`)
	return out, nil
}

func (s *Store) deleteEventsBefore(ctx context.Context, cutoff time.Time) (int64, error) {
	return s.deleteOldestEvents(ctx, shared.FormatTime(cutoff), "", 0)
}

func (s *Store) deleteEventsBeforeOfType(ctx context.Context, cutoff time.Time, eventType string) (int64, error) {
	return s.deleteOldestEvents(ctx, shared.FormatTime(cutoff), eventType, 0)
}

func (s *Store) deleteOldestEvents(ctx context.Context, beforeISO, eventType string, limit int) (int64, error) {
	var total int64
	for {
		if err := ctx.Err(); err != nil {
			return total, err
		}
		q := `DELETE FROM events WHERE event_id IN (
			SELECT e.event_id FROM events e
			WHERE e.occurred_at < ?
			  AND e.event_id NOT IN (SELECT event_id FROM pinned_logs)`
		args := []any{beforeISO}
		if eventType != "" {
			q += ` AND e.event_type = ?`
			args = append(args, eventType)
		}
		q += ` ORDER BY e.occurred_at ASC LIMIT ?)`
		batch := retentionBatch
		if limit > 0 && int(total)+batch > limit {
			batch = limit - int(total)
			if batch <= 0 {
				return total, nil
			}
		}
		args = append(args, batch)
		res, err := s.db.ExecContext(ctx, q, args...)
		if err != nil {
			return total, err
		}
		n, _ := res.RowsAffected()
		total += n
		if n == 0 || n < int64(batch) {
			return total, nil
		}
		if limit > 0 && int(total) >= limit {
			return total, nil
		}
	}
}

func (s *Store) deleteAccessBefore(ctx context.Context, cutoff time.Time) (int64, error) {
	res, err := s.db.ExecContext(ctx, `DELETE FROM access_logs WHERE occurred_at < ?`, shared.FormatTime(cutoff))
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (s *Store) deleteRunsBefore(ctx context.Context, cutoff time.Time) (int64, error) {
	res, err := s.db.ExecContext(ctx, `
		DELETE FROM runs WHERE updated_at < ?
		  AND id NOT IN (SELECT DISTINCT run_id FROM events WHERE run_id IS NOT NULL)`,
		shared.FormatTime(cutoff))
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (s *Store) deleteTokenUsageBefore(ctx context.Context, cutoff time.Time) (int64, error) {
	day := cutoff.UTC().Format("2006-01-02")
	res, err := s.db.ExecContext(ctx, `DELETE FROM token_daily_usage WHERE usage_date < ?`, day)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (s *Store) eventPayloadBytes(ctx context.Context) (int64, error) {
	var n sql.NullInt64
	err := s.db.QueryRowContext(ctx, `SELECT COALESCE(SUM(LENGTH(payload_json)), 0) FROM events`).Scan(&n)
	if err != nil {
		return 0, err
	}
	return n.Int64, nil
}

func (s *Store) eventCount(ctx context.Context) (int64, error) {
	var n int64
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM events`).Scan(&n)
	return n, err
}

func (s *Store) enforceEventQuota(ctx context.Context, quota int64) (deleted, bytes int64, err error) {
	bytes, err = s.eventPayloadBytes(ctx)
	if err != nil {
		return 0, 0, err
	}
	if bytes <= quota {
		return 0, bytes, nil
	}
	target := quota * 9 / 10
	if target < 1 {
		target = quota
	}
	for i := 0; i < quotaLoopMax && bytes > target; i++ {
		count, err := s.eventCount(ctx)
		if err != nil || count == 0 {
			return deleted, bytes, err
		}
		avg := bytes / count
		if avg < 1 {
			avg = 1
		}
		need := int((bytes-target)/avg + 1)
		if need < 1 {
			need = 1
		}
		if need > retentionBatch {
			need = retentionBatch
		}
		cutoff := shared.FormatTime(shared.NowUTC().Add(time.Hour))
		n, err := s.deleteOldestEvents(ctx, cutoff, "log.append", need)
		if err != nil {
			return deleted, bytes, err
		}
		if n == 0 {
			n, err = s.deleteOldestEvents(ctx, cutoff, "", need)
			if err != nil {
				return deleted, bytes, err
			}
		}
		if n == 0 {
			break
		}
		deleted += n
		bytes, err = s.eventPayloadBytes(ctx)
		if err != nil {
			return deleted, bytes, err
		}
	}
	return deleted, bytes, nil
}

type staleRunRow struct {
	ID          string
	ServiceID   string
	Status      string
	CreatedAt   string
	StartedAt   sql.NullString
	MachineID   string
	ServiceType string
}

type staleCloseKind struct {
	agent  bool
	idle   time.Duration
	reason string
	log    string
}

// CloseStaleRuns marks idle non-terminal runs finished.
// Agent services: after DefaultAgentStaleRunIdle with no log.append / log.pin /
// run.transition, status becomes failed (queued → cancelled) with
// 「任务被打断（无后续上报）」. Other service types keep `idle` (default 24h)
// and close as timed_out. Does not bump machine last_seen.
func (s *Store) CloseStaleRuns(ctx context.Context, idle time.Duration) (int64, error) {
	if idle <= 0 {
		idle = DefaultStaleRunIdle
	}
	return s.closeStaleRuns(ctx, idle, DefaultAgentStaleRunIdle)
}

// CloseStaleAgentRuns only closes idle agent runs. Used by the short ticker
// so interrupted coding sessions fail without waiting for hourly retention.
func (s *Store) CloseStaleAgentRuns(ctx context.Context) (int64, error) {
	return s.closeStaleRuns(ctx, 0, DefaultAgentStaleRunIdle)
}

func (s *Store) closeStaleRuns(ctx context.Context, jobIdle, agentIdle time.Duration) (int64, error) {
	now := shared.NowUTC()
	nowISO := shared.FormatTime(now)

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()

	var kinds []staleCloseKind
	if jobIdle > 0 {
		kinds = append(kinds, staleCloseKind{agent: false, idle: jobIdle, reason: "no_logs", log: staleRunCloseLog})
	}
	if agentIdle > 0 {
		kinds = append(kinds, staleCloseKind{agent: true, idle: agentIdle, reason: "interrupted", log: agentInterruptedLog})
	}

	touched := map[string]struct{}{}
	var closed int64
	for _, kind := range kinds {
		rows, err := listStaleRunsTx(ctx, tx, now.Add(-kind.idle), kind.agent)
		if err != nil {
			return 0, err
		}
		for _, r := range rows {
			to := staleCloseStatus(r.Status, kind.agent)
			if !event.AllowedTransition(r.Status, to) {
				continue
			}
			n, err := closeOneStaleRunTx(ctx, tx, r, to, kind, now, nowISO)
			if err != nil {
				return 0, err
			}
			if n {
				touched[r.ServiceID] = struct{}{}
				closed++
			}
		}
	}
	for sid := range touched {
		if err := s.refreshActiveRunSummaryTx(ctx, tx, sid, nowISO); err != nil {
			return 0, err
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return closed, nil
}

func listStaleRunsTx(ctx context.Context, tx *sql.Tx, cutoff time.Time, agent bool) ([]staleRunRow, error) {
	typePred := "sv.type <> 'agent'"
	if agent {
		typePred = "sv.type = 'agent'"
	}
	q := `
		SELECT r.id, r.service_id, r.status, r.created_at, r.started_at, sv.machine_id, sv.type
		FROM runs r
		JOIN services sv ON sv.id = r.service_id
		WHERE r.status IN ('queued','running','waiting_input','blocked')
		  AND sv.deleted_at IS NULL
		  AND ` + typePred + `
		  AND COALESCE(
		    (SELECT MAX(e.occurred_at) FROM events e
		     WHERE e.run_id = r.id AND e.event_type IN ('log.append','log.pin','run.transition')),
		    r.created_at
		  ) < ?`
	rows, err := tx.QueryContext(ctx, q, shared.FormatTime(cutoff))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var stale []staleRunRow
	for rows.Next() {
		var r staleRunRow
		if err := rows.Scan(&r.ID, &r.ServiceID, &r.Status, &r.CreatedAt, &r.StartedAt, &r.MachineID, &r.ServiceType); err != nil {
			return nil, err
		}
		stale = append(stale, r)
	}
	return stale, rows.Err()
}

func closeOneStaleRunTx(ctx context.Context, tx *sql.Tx, r staleRunRow, to string, kind staleCloseKind, now time.Time, nowISO string) (bool, error) {
	start := r.CreatedAt
	if r.StartedAt.Valid && r.StartedAt.String != "" {
		start = r.StartedAt.String
	}
	dur := durationMsSince(start, now)
	summary := ""
	if kind.agent {
		summary = kind.log
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE runs SET status = ?, summary = COALESCE(NULLIF(?, ''), summary),
			finished_at = COALESCE(finished_at, ?),
			duration_ms = COALESCE(duration_ms, ?), updated_at = ?
		WHERE id = ? AND status = ?`,
		to, summary, nowISO, dur, nowISO, r.ID, r.Status); err != nil {
		return false, err
	}
	rtPayload, err := json.Marshal(event.RunTransition{
		Status:     to,
		Summary:    summary,
		FinishedAt: nowISO,
		DurationMs: dur,
		Metadata: map[string]any{
			"closed_by": "maintenance",
			"reason":    kind.reason,
		},
	})
	if err != nil {
		return false, err
	}
	sid, rid := r.ServiceID, r.ID
	if err := insertMaintenanceEventTx(ctx, tx, r.MachineID, &sid, &rid, event.TypeRunTransition, event.RunSeverity(to), nowISO, rtPayload); err != nil {
		return false, err
	}
	logSev := "warning"
	if kind.agent {
		logSev = "error"
	}
	logPayload, err := json.Marshal(event.LogPayload{
		Markdown: kind.log,
		Severity: logSev,
		Source:   "board-server",
	})
	if err != nil {
		return false, err
	}
	if err := insertMaintenanceEventTx(ctx, tx, r.MachineID, &sid, &rid, event.TypeLogAppend, logSev, nowISO, logPayload); err != nil {
		return false, err
	}
	return true, nil
}

func staleCloseStatus(from string, agent bool) string {
	if from == "queued" || from == "waiting_input" {
		return "cancelled"
	}
	if agent {
		return "failed"
	}
	return "timed_out"
}

func durationMsSince(start string, now time.Time) *int64 {
	t, err := shared.ParseTime(start)
	if err != nil {
		return nil
	}
	ms := now.Sub(t).Milliseconds()
	if ms < 0 {
		ms = 0
	}
	return &ms
}

func insertMaintenanceEventTx(ctx context.Context, tx *sql.Tx, machineID string, serviceID, runID *string, eventType, severity, at string, payload []byte) error {
	_, err := tx.ExecContext(ctx, `
		INSERT INTO events (id, event_id, machine_id, service_id, run_id, event_type, severity, occurred_at, received_at, payload_json)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		shared.NewID(), shared.NewID(), machineID, serviceID, runID, eventType, severity, at, at, string(nonNullJSON(payload)))
	return err
}
