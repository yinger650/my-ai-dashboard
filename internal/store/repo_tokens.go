package store

import (
	"context"
	"database/sql"
	"errors"

	"agentboard/internal/shared"
)

const tokenCols = `id, name, token_prefix, token_hash, scope, machine_id, service_id, ip_allowlist_json, requests_per_minute, bytes_per_day, allow_artifact_download, last_used_at, last_used_ip, expires_at, enabled, created_at, revoked_at`

func scanToken(sc interface{ Scan(...any) error }) (*Token, error) {
	var t Token
	err := sc.Scan(&t.ID, &t.Name, &t.TokenPrefix, &t.TokenHash, &t.Scope, &t.MachineID, &t.ServiceID,
		&t.IPAllowlistJSON, &t.RequestsPerMinute, &t.BytesPerDay, &t.AllowArtifactDownload, &t.LastUsedAt,
		&t.LastUsedIP, &t.ExpiresAt, &t.Enabled, &t.CreatedAt, &t.RevokedAt)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

// CreateToken inserts a new API token row (hash + prefix only).
func (s *Store) CreateToken(ctx context.Context, t *Token) error {
	now := shared.FormatTime(shared.NowUTC())
	if t.ID == "" {
		t.ID = shared.NewID()
	}
	t.CreatedAt = now
	if t.IPAllowlistJSON == "" {
		t.IPAllowlistJSON = "[]"
	}
	if t.RequestsPerMinute == 0 {
		t.RequestsPerMinute = 120
	}
	if t.BytesPerDay == 0 {
		t.BytesPerDay = 104857600
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO api_tokens (id, name, token_prefix, token_hash, scope, machine_id, service_id, ip_allowlist_json, requests_per_minute, bytes_per_day, allow_artifact_download, enabled, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 1, ?)`,
		t.ID, t.Name, t.TokenPrefix, t.TokenHash, t.Scope, t.MachineID, t.ServiceID, t.IPAllowlistJSON,
		t.RequestsPerMinute, t.BytesPerDay, boolToInt(t.AllowArtifactDownload), t.CreatedAt)
	return err
}

// GetTokenByHash returns an enabled, non-revoked token by hash.
func (s *Store) GetTokenByHash(ctx context.Context, hash string) (*Token, error) {
	row := s.db.QueryRowContext(ctx, `SELECT `+tokenCols+` FROM api_tokens WHERE token_hash = ?`, hash)
	t, err := scanToken(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return t, err
}

// ListTokens returns all tokens (metadata only).
func (s *Store) ListTokens(ctx context.Context) ([]*Token, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+tokenCols+` FROM api_tokens ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Token
	for rows.Next() {
		t, err := scanToken(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// GetTokenByID returns a token by id.
func (s *Store) GetTokenByID(ctx context.Context, id string) (*Token, error) {
	row := s.db.QueryRowContext(ctx, `SELECT `+tokenCols+` FROM api_tokens WHERE id = ?`, id)
	t, err := scanToken(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return t, err
}

// RevokeToken disables and marks a token revoked.
func (s *Store) RevokeToken(ctx context.Context, id string) error {
	now := shared.FormatTime(shared.NowUTC())
	_, err := s.db.ExecContext(ctx, `UPDATE api_tokens SET enabled = 0, revoked_at = ? WHERE id = ? AND revoked_at IS NULL`, now, id)
	return err
}

// TouchTokenUsed records last-used timestamp and IP.
func (s *Store) TouchTokenUsed(ctx context.Context, id, ip string) error {
	now := shared.FormatTime(shared.NowUTC())
	_, err := s.db.ExecContext(ctx, `UPDATE api_tokens SET last_used_at = ?, last_used_ip = ? WHERE id = ?`, now, ip, id)
	return err
}
