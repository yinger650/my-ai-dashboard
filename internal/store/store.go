// Package store owns the SQLite connection pool, migrations and repositories.
package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/pressly/goose/v3"
	_ "modernc.org/sqlite"

	"agentboard/migrations"
)

// Store wraps the database handle and exposes repository methods.
type Store struct {
	db *sql.DB
}

// Open opens the SQLite database at path with the pragmas required by the spec
// and returns a ready-to-use Store. Migrations are NOT run here; call Migrate.
func Open(path string) (*Store, error) {
	// modernc.org/sqlite accepts pragmas via the connection string.
	dsn := fmt.Sprintf("file:%s?_pragma=journal_mode(WAL)&_pragma=foreign_keys(ON)&_pragma=busy_timeout(5000)&_pragma=synchronous(NORMAL)", path)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	// SQLite has a single writer. Serialize all access through one connection
	// so concurrent writers queue in Go's pool instead of racing to SQLITE_BUSY.
	// busy_timeout (set in the DSN) remains a backstop. This is ample for the
	// personal-scale target in spec section 3.
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(time.Hour)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	return &Store{db: db}, nil
}

// DB exposes the underlying handle for advanced use (health checks, tests).
func (s *Store) DB() *sql.DB { return s.db }

// Close closes the database.
func (s *Store) Close() error { return s.db.Close() }

// Migrate runs all embedded goose migrations to the latest version.
func (s *Store) Migrate() error {
	goose.SetBaseFS(migrations.FS)
	goose.SetLogger(goose.NopLogger())
	if err := goose.SetDialect("sqlite3"); err != nil {
		return err
	}
	return goose.Up(s.db, ".")
}

// Ping verifies database connectivity within a short timeout.
func (s *Store) Ping(ctx context.Context) error {
	return s.db.PingContext(ctx)
}
