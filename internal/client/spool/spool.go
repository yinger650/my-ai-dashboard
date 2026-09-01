// Package spool implements the client's durable local event queue (SQLite).
package spool

import (
	"database/sql"
	"fmt"
	"time"

	_ "modernc.org/sqlite"

	"agentboard/internal/shared"
)

// Spool is a persistent local event queue.
type Spool struct {
	db *sql.DB
}

// Open opens (creating if needed) the spool database and its schema.
func Open(path string) (*Spool, error) {
	dsn := fmt.Sprintf("file:%s?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=synchronous(NORMAL)", path)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	schema := `
	CREATE TABLE IF NOT EXISTS spool_events (
	  event_id TEXT PRIMARY KEY,
	  created_at TEXT NOT NULL,
	  next_attempt_at TEXT NOT NULL,
	  attempt_count INTEGER NOT NULL DEFAULT 0,
	  event_type TEXT NOT NULL DEFAULT '',
	  payload_json TEXT NOT NULL,
	  size_bytes INTEGER NOT NULL,
	  last_error TEXT
	);
	CREATE TABLE IF NOT EXISTS client_state (
	  key TEXT PRIMARY KEY,
	  value_json TEXT NOT NULL,
	  updated_at TEXT NOT NULL
	);`
	if _, err := db.Exec(schema); err != nil {
		_ = db.Close()
		return nil, err
	}
	return &Spool{db: db}, nil
}

// Close closes the spool.
func (s *Spool) Close() error { return s.db.Close() }

// Enqueue persists an event. Duplicate event_ids are ignored.
func (s *Spool) Enqueue(eventID, eventType, payloadJSON string) error {
	now := shared.FormatTime(shared.NowUTC())
	_, err := s.db.Exec(`
		INSERT INTO spool_events (event_id, created_at, next_attempt_at, event_type, payload_json, size_bytes)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(event_id) DO NOTHING`,
		eventID, now, now, eventType, payloadJSON, len(payloadJSON))
	return err
}

// QueuedEvent is a spooled event ready to send.
type QueuedEvent struct {
	EventID string
	Payload string
	Size    int
}

// Batch returns up to limit events (max total bytes) whose next_attempt_at is due.
func (s *Spool) Batch(limit, maxBytes int) ([]QueuedEvent, error) {
	now := shared.FormatTime(shared.NowUTC())
	rows, err := s.db.Query(`SELECT event_id, payload_json, size_bytes FROM spool_events WHERE next_attempt_at <= ? ORDER BY created_at ASC LIMIT ?`, now, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []QueuedEvent
	total := 0
	for rows.Next() {
		var q QueuedEvent
		if err := rows.Scan(&q.EventID, &q.Payload, &q.Size); err != nil {
			return nil, err
		}
		if len(out) > 0 && total+q.Size > maxBytes {
			break
		}
		total += q.Size
		out = append(out, q)
	}
	return out, rows.Err()
}

// Delete removes events (accepted/duplicate/dead-lettered).
func (s *Spool) Delete(ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	stmt, err := tx.Prepare(`DELETE FROM spool_events WHERE event_id = ?`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, id := range ids {
		if _, err := stmt.Exec(id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// MarkRetry schedules a retry for events after a backoff delay.
func (s *Spool) MarkRetry(ids []string, delay time.Duration, lastErr string) error {
	if len(ids) == 0 {
		return nil
	}
	next := shared.FormatTime(shared.NowUTC().Add(delay))
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	stmt, err := tx.Prepare(`UPDATE spool_events SET next_attempt_at = ?, attempt_count = attempt_count + 1, last_error = ? WHERE event_id = ?`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, id := range ids {
		if _, err := stmt.Exec(next, lastErr, id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// Count returns the number of spooled events.
func (s *Spool) Count() (int, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM spool_events`).Scan(&n)
	return n, err
}

// Trim enforces the max queue size, dropping oldest metric then heartbeat
// events first (spec 14.3). Returns the number dropped.
func (s *Spool) Trim(maxEvents int) (int, error) {
	n, err := s.Count()
	if err != nil || n <= maxEvents {
		return 0, err
	}
	toDrop := n - maxEvents
	res, err := s.db.Exec(`
		DELETE FROM spool_events WHERE event_id IN (
			SELECT event_id FROM spool_events
			ORDER BY CASE event_type
				WHEN 'metric.sample' THEN 0
				WHEN 'machine.heartbeat' THEN 1
				ELSE 2 END ASC, created_at ASC
			LIMIT ?)`, toDrop)
	if err != nil {
		return 0, err
	}
	dropped, _ := res.RowsAffected()
	return int(dropped), nil
}

// GetState reads a client_state value. ok is false when the key is missing.
func (s *Spool) GetState(key string) (string, bool, error) {
	var v string
	err := s.db.QueryRow(`SELECT value_json FROM client_state WHERE key = ?`, key).Scan(&v)
	if err == sql.ErrNoRows {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return v, true, nil
}

// SetState upserts a client_state value.
func (s *Spool) SetState(key, valueJSON string) error {
	now := shared.FormatTime(shared.NowUTC())
	_, err := s.db.Exec(`
		INSERT INTO client_state (key, value_json, updated_at)
		VALUES (?, ?, ?)
		ON CONFLICT(key) DO UPDATE SET value_json = excluded.value_json, updated_at = excluded.updated_at`,
		key, valueJSON, now)
	return err
}

// DeleteState removes a client_state key.
func (s *Spool) DeleteState(key string) error {
	_, err := s.db.Exec(`DELETE FROM client_state WHERE key = ?`, key)
	return err
}
