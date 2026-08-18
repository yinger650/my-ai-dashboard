package store

import (
	"context"

	"agentboard/internal/shared"
)

// InsertAccessLog writes an access log row.
func (s *Store) InsertAccessLog(ctx context.Context, a *AccessLog) error {
	if a.ID == "" {
		a.ID = shared.NewID()
	}
	if a.OccurredAt == "" {
		a.OccurredAt = shared.FormatTime(shared.NowUTC())
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO access_logs (id, occurred_at, request_id, actor_type, actor_id, method, path, status_code, ip, user_agent, bytes_in, duration_ms, result, reason, is_abnormal)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		a.ID, a.OccurredAt, a.RequestID, a.ActorType, a.ActorID, a.Method, a.Path, a.StatusCode, a.IP, a.UserAgent,
		a.BytesIn, a.DurationMs, a.Result, a.Reason, boolToInt(a.IsAbnormal))
	return err
}

// ListAccessLogs returns access logs, newest first, with an opaque cursor
// (the occurred_at of the last row seen).
func (s *Store) ListAccessLogs(ctx context.Context, abnormalOnly bool, beforeISO string, limit int) ([]*AccessLog, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	q := `SELECT id, occurred_at, request_id, actor_type, actor_id, method, path, status_code, ip, user_agent, bytes_in, duration_ms, result, reason, is_abnormal FROM access_logs WHERE 1=1`
	args := []any{}
	if abnormalOnly {
		q += ` AND is_abnormal = 1`
	}
	if beforeISO != "" {
		q += ` AND occurred_at < ?`
		args = append(args, beforeISO)
	}
	q += ` ORDER BY occurred_at DESC LIMIT ?`
	args = append(args, limit)
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*AccessLog
	for rows.Next() {
		var a AccessLog
		var abn int
		if err := rows.Scan(&a.ID, &a.OccurredAt, &a.RequestID, &a.ActorType, &a.ActorID, &a.Method, &a.Path, &a.StatusCode, &a.IP, &a.UserAgent, &a.BytesIn, &a.DurationMs, &a.Result, &a.Reason, &abn); err != nil {
			return nil, err
		}
		a.IsAbnormal = abn == 1
		out = append(out, &a)
	}
	return out, rows.Err()
}

// AbnormalCountSince returns the number of abnormal access rows since a time.
func (s *Store) AbnormalCountSince(ctx context.Context, sinceISO string) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM access_logs WHERE is_abnormal = 1 AND occurred_at >= ?`, sinceISO).Scan(&n)
	return n, err
}
