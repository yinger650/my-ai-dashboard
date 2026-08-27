package store

import (
	"context"
	"database/sql"
	"time"

	"agentboard/internal/shared"
)

const (
	// DefaultEventQuotaBytes is 5 GiB of event payload storage.
	DefaultEventQuotaBytes int64 = 5 * 1024 * 1024 * 1024
	retentionBatch               = 2000
	quotaLoopMax                 = 40
)

// RetentionPolicy is the time + size cap for server-side history.
type RetentionPolicy struct {
	EventDays  int
	MetricDays int
	AccessDays int
	QuotaBytes int64
}

// RetentionResult is a single maintenance pass.
type RetentionResult struct {
	ExpiredSessions int64 `json:"expired_sessions"`
	EventsDeleted   int64 `json:"events_deleted"`
	AccessDeleted   int64 `json:"access_deleted"`
	RunsDeleted     int64 `json:"runs_deleted"`
	UsageDeleted    int64 `json:"usage_deleted"`
	QuotaDeleted    int64 `json:"quota_deleted"`
	EventsBytes     int64 `json:"events_bytes"`
}

// ApplyRetention deletes expired sessions, aged history, and oldest events
// when the payload store is over QuotaBytes. Current-state pins are kept.
func (s *Store) ApplyRetention(ctx context.Context, p RetentionPolicy) (RetentionResult, error) {
	var out RetentionResult
	n, err := s.DeleteExpiredSessions(ctx)
	if err != nil {
		return out, err
	}
	out.ExpiredSessions = n

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
