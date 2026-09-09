package workspace

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	_ "modernc.org/sqlite"

	"agentboard/internal/shared"
	"agentboard/internal/store"
)

// Control is the Feishu edition catalog: users, slugs, sessions.
type Control struct {
	db *sql.DB
}

// User is a Feishu-identified owner of one workspace.
type User struct {
	ID            string
	FeishuOpenID  string
	FeishuUnionID string
	TenantKey     string
	DisplayName   string
	AvatarURL     string
	CreatedAt     string
	UpdatedAt     string
}

// Workspace is a per-user AgentBoard Personal SQLite instance.
type Workspace struct {
	ID          string
	Slug        string
	OwnerUserID string
	CreatedAt   string
}

// OpenControl opens (and migrates) the control-plane SQLite database.
func OpenControl(path string) (*Control, error) {
	dsn := fmt.Sprintf("file:%s?_pragma=journal_mode(WAL)&_pragma=foreign_keys(ON)&_pragma=busy_timeout(5000)&_pragma=synchronous(NORMAL)", path)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(time.Hour)
	c := &Control{db: db}
	if err := c.migrate(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return c, nil
}

func (c *Control) Close() error { return c.db.Close() }

func (c *Control) Ping(ctx context.Context) error { return c.db.PingContext(ctx) }

func (c *Control) migrate() error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS users (
			id TEXT PRIMARY KEY,
			feishu_open_id TEXT NOT NULL UNIQUE,
			feishu_union_id TEXT NOT NULL DEFAULT '',
			tenant_key TEXT NOT NULL DEFAULT '',
			display_name TEXT NOT NULL DEFAULT '',
			avatar_url TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS workspaces (
			id TEXT PRIMARY KEY,
			slug TEXT NOT NULL UNIQUE,
			owner_user_id TEXT NOT NULL REFERENCES users(id),
			created_at TEXT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_workspaces_owner ON workspaces(owner_user_id)`,
		`CREATE TABLE IF NOT EXISTS user_sessions (
			id TEXT PRIMARY KEY,
			user_id TEXT NOT NULL REFERENCES users(id),
			workspace_id TEXT NOT NULL REFERENCES workspaces(id),
			token_hash TEXT NOT NULL UNIQUE,
			csrf_token_hash TEXT NOT NULL,
			created_at TEXT NOT NULL,
			expires_at TEXT NOT NULL,
			last_seen_at TEXT NOT NULL,
			ip TEXT,
			user_agent TEXT
		)`,
		`CREATE INDEX IF NOT EXISTS idx_user_sessions_expires ON user_sessions(expires_at)`,
		`CREATE TABLE IF NOT EXISTS oauth_states (
			state TEXT PRIMARY KEY,
			created_at TEXT NOT NULL,
			expires_at TEXT NOT NULL
		)`,
	}
	for _, q := range stmts {
		if _, err := c.db.Exec(q); err != nil {
			return fmt.Errorf("control migrate: %w", err)
		}
	}
	return nil
}

func (c *Control) GetUserByOpenID(ctx context.Context, openID string) (*User, error) {
	row := c.db.QueryRowContext(ctx, `
		SELECT id, feishu_open_id, feishu_union_id, tenant_key, display_name, avatar_url, created_at, updated_at
		FROM users WHERE feishu_open_id = ?`, openID)
	return scanUser(row)
}

func (c *Control) GetUserByID(ctx context.Context, id string) (*User, error) {
	row := c.db.QueryRowContext(ctx, `
		SELECT id, feishu_open_id, feishu_union_id, tenant_key, display_name, avatar_url, created_at, updated_at
		FROM users WHERE id = ?`, id)
	return scanUser(row)
}

func scanUser(row *sql.Row) (*User, error) {
	var u User
	err := row.Scan(&u.ID, &u.FeishuOpenID, &u.FeishuUnionID, &u.TenantKey, &u.DisplayName, &u.AvatarURL, &u.CreatedAt, &u.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, store.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
}

func (c *Control) InsertUser(ctx context.Context, u *User) error {
	_, err := c.db.ExecContext(ctx, `
		INSERT INTO users (id, feishu_open_id, feishu_union_id, tenant_key, display_name, avatar_url, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		u.ID, u.FeishuOpenID, u.FeishuUnionID, u.TenantKey, u.DisplayName, u.AvatarURL, u.CreatedAt, u.UpdatedAt)
	return err
}

func (c *Control) UpdateUserProfile(ctx context.Context, id, name, avatar, tenantKey string) error {
	now := shared.FormatTime(shared.NowUTC())
	_, err := c.db.ExecContext(ctx, `
		UPDATE users SET display_name = ?, avatar_url = ?, tenant_key = ?, updated_at = ? WHERE id = ?`,
		name, avatar, tenantKey, now, id)
	return err
}

func (c *Control) GetWorkspaceByOwner(ctx context.Context, userID string) (*Workspace, error) {
	row := c.db.QueryRowContext(ctx, `SELECT id, slug, owner_user_id, created_at FROM workspaces WHERE owner_user_id = ?`, userID)
	return scanWorkspace(row)
}

func (c *Control) GetWorkspaceBySlug(ctx context.Context, slug string) (*Workspace, error) {
	row := c.db.QueryRowContext(ctx, `SELECT id, slug, owner_user_id, created_at FROM workspaces WHERE slug = ?`, slug)
	return scanWorkspace(row)
}

func scanWorkspace(row *sql.Row) (*Workspace, error) {
	var w Workspace
	err := row.Scan(&w.ID, &w.Slug, &w.OwnerUserID, &w.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, store.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &w, nil
}

func (c *Control) SlugTaken(ctx context.Context, slug string) (bool, error) {
	var n int
	err := c.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM workspaces WHERE slug = ?`, slug).Scan(&n)
	return n > 0, err
}

func (c *Control) InsertWorkspace(ctx context.Context, w *Workspace) error {
	_, err := c.db.ExecContext(ctx, `
		INSERT INTO workspaces (id, slug, owner_user_id, created_at) VALUES (?, ?, ?, ?)`,
		w.ID, w.Slug, w.OwnerUserID, w.CreatedAt)
	return err
}

func (c *Control) ListSlugs(ctx context.Context) ([]string, error) {
	rows, err := c.db.QueryContext(ctx, `SELECT slug FROM workspaces`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var slug string
		if err := rows.Scan(&slug); err != nil {
			return nil, err
		}
		out = append(out, slug)
	}
	return out, rows.Err()
}

func (c *Control) PutOAuthState(ctx context.Context, state string, ttl time.Duration) error {
	now := time.Now().UTC()
	_, err := c.db.ExecContext(ctx, `INSERT INTO oauth_states (state, created_at, expires_at) VALUES (?, ?, ?)`,
		state, shared.FormatTime(now), shared.FormatTime(now.Add(ttl)))
	return err
}

func (c *Control) ConsumeOAuthState(ctx context.Context, state string) error {
	now := shared.FormatTime(shared.NowUTC())
	res, err := c.db.ExecContext(ctx, `DELETE FROM oauth_states WHERE state = ? AND expires_at > ?`, state, now)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return store.ErrNotFound
	}
	return nil
}

func (c *Control) CreateSession(ctx context.Context, sess *store.Session) error {
	_, err := c.db.ExecContext(ctx, `
		INSERT INTO user_sessions (id, user_id, workspace_id, token_hash, csrf_token_hash, created_at, expires_at, last_seen_at, ip, user_agent)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		sess.ID, sess.UserID, sess.WorkspaceID, sess.TokenHash, sess.CSRFTokenHash,
		sess.CreatedAt, sess.ExpiresAt, sess.LastSeenAt, sess.IP, sess.UserAgent)
	return err
}

func (c *Control) GetSessionByTokenHash(ctx context.Context, tokenHash string) (*store.Session, error) {
	row := c.db.QueryRowContext(ctx, `
		SELECT s.id, s.user_id, s.workspace_id, w.slug, u.display_name, s.token_hash, s.csrf_token_hash,
		       s.created_at, s.expires_at, s.last_seen_at, s.ip, s.user_agent
		FROM user_sessions s
		JOIN workspaces w ON w.id = s.workspace_id
		JOIN users u ON u.id = s.user_id
		WHERE s.token_hash = ?`, tokenHash)
	var sess store.Session
	err := row.Scan(&sess.ID, &sess.UserID, &sess.WorkspaceID, &sess.WorkspaceSlug, &sess.DisplayName,
		&sess.TokenHash, &sess.CSRFTokenHash, &sess.CreatedAt, &sess.ExpiresAt, &sess.LastSeenAt, &sess.IP, &sess.UserAgent)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, store.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &sess, nil
}

func (c *Control) UpdateSessionCSRF(ctx context.Context, sessionID, csrfHash string) error {
	_, err := c.db.ExecContext(ctx, `UPDATE user_sessions SET csrf_token_hash = ? WHERE id = ?`, csrfHash, sessionID)
	return err
}

func (c *Control) DeleteSession(ctx context.Context, id string) error {
	_, err := c.db.ExecContext(ctx, `DELETE FROM user_sessions WHERE id = ?`, id)
	return err
}

func (c *Control) DeleteExpiredSessions(ctx context.Context) (int64, error) {
	now := shared.FormatTime(shared.NowUTC())
	res, err := c.db.ExecContext(ctx, `DELETE FROM user_sessions WHERE expires_at < ?`, now)
	if err != nil {
		return 0, err
	}
	n, err := res.RowsAffected()
	_, _ = c.db.ExecContext(ctx, `DELETE FROM oauth_states WHERE expires_at < ?`, now)
	return n, err
}

func newID() string { return uuid.NewString() }
