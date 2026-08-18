package store

import (
	"context"
	"database/sql"
	"errors"

	"agentboard/internal/shared"
)

const artifactCols = `id, upload_event_id, machine_id, service_id, stored_name, original_name, mime_type, size_bytes, sha256, created_at, deleted_at`

func scanArtifact(sc interface{ Scan(...any) error }) (*Artifact, error) {
	var a Artifact
	err := sc.Scan(&a.ID, &a.UploadEventID, &a.MachineID, &a.ServiceID, &a.StoredName, &a.OriginalName, &a.MIMEType, &a.SizeBytes, &a.SHA256, &a.CreatedAt, &a.DeletedAt)
	if err != nil {
		return nil, err
	}
	return &a, nil
}

// InsertArtifact writes a new artifact row.
func (s *Store) InsertArtifact(ctx context.Context, a *Artifact) error {
	if a.ID == "" {
		a.ID = shared.NewID()
	}
	if a.CreatedAt == "" {
		a.CreatedAt = shared.FormatTime(shared.NowUTC())
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO artifacts (id, upload_event_id, machine_id, service_id, stored_name, original_name, mime_type, size_bytes, sha256, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		a.ID, a.UploadEventID, a.MachineID, a.ServiceID, a.StoredName, a.OriginalName, a.MIMEType, a.SizeBytes, a.SHA256, a.CreatedAt)
	return err
}

// GetArtifact returns a non-deleted artifact by id.
func (s *Store) GetArtifact(ctx context.Context, id string) (*Artifact, error) {
	row := s.db.QueryRowContext(ctx, `SELECT `+artifactCols+` FROM artifacts WHERE id = ? AND deleted_at IS NULL`, id)
	a, err := scanArtifact(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return a, err
}

// ListArtifactsByService returns non-deleted artifacts for a service, newest first.
func (s *Store) ListArtifactsByService(ctx context.Context, serviceID string, limit int) ([]*Artifact, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := s.db.QueryContext(ctx, `SELECT `+artifactCols+` FROM artifacts WHERE service_id = ? AND deleted_at IS NULL ORDER BY created_at DESC LIMIT ?`, serviceID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Artifact
	for rows.Next() {
		a, err := scanArtifact(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// ArtifactBytesUsed returns the sum of undeleted artifact sizes.
func (s *Store) ArtifactBytesUsed(ctx context.Context) (int64, error) {
	row := s.db.QueryRowContext(ctx, `SELECT COALESCE(SUM(size_bytes), 0) FROM artifacts WHERE deleted_at IS NULL`)
	var n int64
	err := row.Scan(&n)
	return n, err
}

// SoftDeleteArtifact marks an artifact deleted.
func (s *Store) SoftDeleteArtifact(ctx context.Context, id string) error {
	now := shared.FormatTime(shared.NowUTC())
	res, err := s.db.ExecContext(ctx, `UPDATE artifacts SET deleted_at = ? WHERE id = ? AND deleted_at IS NULL`, now, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}
