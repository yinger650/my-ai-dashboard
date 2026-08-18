package store

import (
	"context"
	"database/sql"
	"errors"

	"agentboard/internal/shared"
)

const machineCols = `id, machine_key, name, kind, description, os, arch, hostname, collector_version, boot_id, heartbeat_interval_seconds, last_seen_at, last_event_at, enabled, auto_create_services, metadata_json, created_at, updated_at, deleted_at`

func scanMachine(sc interface{ Scan(...any) error }) (*Machine, error) {
	var m Machine
	err := sc.Scan(&m.ID, &m.MachineKey, &m.Name, &m.Kind, &m.Description, &m.OS, &m.Arch, &m.Hostname,
		&m.CollectorVersion, &m.BootID, &m.HeartbeatIntervalSeconds, &m.LastSeenAt, &m.LastEventAt,
		&m.Enabled, &m.AutoCreateServices, &m.MetadataJSON, &m.CreatedAt, &m.UpdatedAt, &m.DeletedAt)
	if err != nil {
		return nil, err
	}
	return &m, nil
}

// CreateMachine inserts a new machine.
func (s *Store) CreateMachine(ctx context.Context, m *Machine) error {
	now := shared.FormatTime(shared.NowUTC())
	if m.ID == "" {
		m.ID = shared.NewID()
	}
	m.CreatedAt, m.UpdatedAt = now, now
	if m.HeartbeatIntervalSeconds == 0 {
		m.HeartbeatIntervalSeconds = 30
	}
	if m.MetadataJSON == "" {
		m.MetadataJSON = "{}"
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO machines (id, machine_key, name, kind, description, heartbeat_interval_seconds, enabled, auto_create_services, metadata_json, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		m.ID, m.MachineKey, m.Name, m.Kind, m.Description, m.HeartbeatIntervalSeconds,
		boolToInt(m.Enabled), boolToInt(m.AutoCreateServices), m.MetadataJSON, m.CreatedAt, m.UpdatedAt)
	return err
}

// GetMachineByID returns a machine by id (including soft-deleted).
func (s *Store) GetMachineByID(ctx context.Context, id string) (*Machine, error) {
	row := s.db.QueryRowContext(ctx, `SELECT `+machineCols+` FROM machines WHERE id = ?`, id)
	m, err := scanMachine(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return m, err
}

// GetMachineByKey returns a non-deleted machine by machine_key.
func (s *Store) GetMachineByKey(ctx context.Context, key string) (*Machine, error) {
	row := s.db.QueryRowContext(ctx, `SELECT `+machineCols+` FROM machines WHERE machine_key = ? AND deleted_at IS NULL`, key)
	m, err := scanMachine(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return m, err
}

// ListMachines returns all non-deleted machines ordered by name.
func (s *Store) ListMachines(ctx context.Context, includeDisabled bool) ([]*Machine, error) {
	q := `SELECT ` + machineCols + ` FROM machines WHERE deleted_at IS NULL`
	if !includeDisabled {
		q += ` AND enabled = 1`
	}
	q += ` ORDER BY name`
	rows, err := s.db.QueryContext(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Machine
	for rows.Next() {
		m, err := scanMachine(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// UpdateMachineFields updates admin-editable machine fields.
func (s *Store) UpdateMachineFields(ctx context.Context, id, name, kind, description string, enabled bool, metadataJSON string) error {
	now := shared.FormatTime(shared.NowUTC())
	_, err := s.db.ExecContext(ctx, `
		UPDATE machines SET name = ?, kind = ?, description = ?, enabled = ?, metadata_json = ?, updated_at = ?
		WHERE id = ? AND deleted_at IS NULL`,
		name, kind, description, boolToInt(enabled), metadataJSON, now, id)
	return err
}

// SoftDeleteMachine marks a machine deleted and revokes its tokens.
func (s *Store) SoftDeleteMachine(ctx context.Context, id string) error {
	now := shared.FormatTime(shared.NowUTC())
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `UPDATE machines SET deleted_at = ?, updated_at = ? WHERE id = ?`, now, now, id); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE api_tokens SET enabled = 0, revoked_at = ? WHERE machine_id = ? AND revoked_at IS NULL`, now, id); err != nil {
		return err
	}
	return tx.Commit()
}

// UpdateMachineHeartbeat updates identity + last_seen from a heartbeat/event.
func (s *Store) UpdateMachineHeartbeat(ctx context.Context, id string, os, arch, hostname, collectorVersion, bootID *string, interval int, occurredAt string) error {
	now := shared.FormatTime(shared.NowUTC())
	_, err := s.db.ExecContext(ctx, `
		UPDATE machines SET
			os = COALESCE(?, os),
			arch = COALESCE(?, arch),
			hostname = COALESCE(?, hostname),
			collector_version = COALESCE(?, collector_version),
			boot_id = COALESCE(?, boot_id),
			heartbeat_interval_seconds = CASE WHEN ? > 0 THEN ? ELSE heartbeat_interval_seconds END,
			last_seen_at = ?,
			last_event_at = ?,
			updated_at = ?
		WHERE id = ?`,
		os, arch, hostname, collectorVersion, bootID, interval, interval, now, occurredAt, now, id)
	return err
}

// TouchMachineSeen updates last_seen/last_event on any accepted event.
func (s *Store) TouchMachineSeen(ctx context.Context, id, occurredAt string) error {
	now := shared.FormatTime(shared.NowUTC())
	_, err := s.db.ExecContext(ctx, `UPDATE machines SET last_seen_at = ?, last_event_at = ?, updated_at = ? WHERE id = ?`, now, occurredAt, now, id)
	return err
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
