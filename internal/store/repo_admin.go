package store

import (
	"context"
	"database/sql"
	"errors"

	"agentboard/internal/shared"
)

// ErrNotFound is returned when a requested row does not exist.
var ErrNotFound = errors.New("not found")

// GetAdminCredentials returns the single admin credential row, or ErrNotFound.
func (s *Store) GetAdminCredentials(ctx context.Context) (*AdminCredentials, error) {
	row := s.db.QueryRowContext(ctx, `SELECT password_hash, totp_secret_encrypted, failed_attempts, locked_until, updated_at FROM admin_credentials WHERE id = 1`)
	var a AdminCredentials
	err := row.Scan(&a.PasswordHash, &a.TOTPSecretEncrypted, &a.FailedAttempts, &a.LockedUntil, &a.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &a, nil
}

// AdminInitialized reports whether the admin password has been set.
func (s *Store) AdminInitialized(ctx context.Context) (bool, error) {
	_, err := s.GetAdminCredentials(ctx)
	if errors.Is(err, ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// SetAdminPassword inserts or replaces the admin password hash.
func (s *Store) SetAdminPassword(ctx context.Context, hash string) error {
	now := shared.FormatTime(shared.NowUTC())
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO admin_credentials (id, password_hash, failed_attempts, updated_at)
		VALUES (1, ?, 0, ?)
		ON CONFLICT(id) DO UPDATE SET password_hash = excluded.password_hash, failed_attempts = 0, locked_until = NULL, updated_at = excluded.updated_at`,
		hash, now)
	return err
}

// SetFailedAttempts updates the lockout counter and optional locked_until.
func (s *Store) SetFailedAttempts(ctx context.Context, attempts int, lockedUntil *string) error {
	now := shared.FormatTime(shared.NowUTC())
	_, err := s.db.ExecContext(ctx, `UPDATE admin_credentials SET failed_attempts = ?, locked_until = ?, updated_at = ? WHERE id = 1`, attempts, lockedUntil, now)
	return err
}

// CreateSession inserts a new admin session.
func (s *Store) CreateSession(ctx context.Context, sess *Session) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO admin_sessions (id, token_hash, csrf_token_hash, created_at, expires_at, last_seen_at, ip, user_agent)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		sess.ID, sess.TokenHash, sess.CSRFTokenHash, sess.CreatedAt, sess.ExpiresAt, sess.LastSeenAt, sess.IP, sess.UserAgent)
	return err
}

// GetSessionByTokenHash returns the session for a session-cookie token hash.
func (s *Store) GetSessionByTokenHash(ctx context.Context, tokenHash string) (*Session, error) {
	row := s.db.QueryRowContext(ctx, `SELECT id, token_hash, csrf_token_hash, created_at, expires_at, last_seen_at, ip, user_agent FROM admin_sessions WHERE token_hash = ?`, tokenHash)
	var sess Session
	err := row.Scan(&sess.ID, &sess.TokenHash, &sess.CSRFTokenHash, &sess.CreatedAt, &sess.ExpiresAt, &sess.LastSeenAt, &sess.IP, &sess.UserAgent)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &sess, nil
}

// TouchSession updates last_seen_at for a session.
func (s *Store) TouchSession(ctx context.Context, id string) error {
	now := shared.FormatTime(shared.NowUTC())
	_, err := s.db.ExecContext(ctx, `UPDATE admin_sessions SET last_seen_at = ? WHERE id = ?`, now, id)
	return err
}

// DeleteSession removes a session by id.
func (s *Store) DeleteSession(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM admin_sessions WHERE id = ?`, id)
	return err
}

// DeleteExpiredSessions removes sessions past their expiry.
func (s *Store) DeleteExpiredSessions(ctx context.Context) (int64, error) {
	now := shared.FormatTime(shared.NowUTC())
	res, err := s.db.ExecContext(ctx, `DELETE FROM admin_sessions WHERE expires_at < ?`, now)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// GetSetting returns a JSON setting value by key.
func (s *Store) GetSetting(ctx context.Context, key string) (string, error) {
	row := s.db.QueryRowContext(ctx, `SELECT value_json FROM settings WHERE key = ?`, key)
	var v string
	err := row.Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	return v, err
}

// SetSetting upserts a JSON setting value.
func (s *Store) SetSetting(ctx context.Context, key, valueJSON string) error {
	now := shared.FormatTime(shared.NowUTC())
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO settings (key, value_json, updated_at) VALUES (?, ?, ?)
		ON CONFLICT(key) DO UPDATE SET value_json = excluded.value_json, updated_at = excluded.updated_at`,
		key, valueJSON, now)
	return err
}

// AllSettings returns all settings as a key->value_json map.
func (s *Store) AllSettings(ctx context.Context) (map[string]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT key, value_json FROM settings`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, err
		}
		out[k] = v
	}
	return out, rows.Err()
}
