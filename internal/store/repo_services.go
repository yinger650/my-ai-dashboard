package store

import (
	"context"
	"database/sql"
	"errors"

	"agentboard/internal/shared"
)

const serviceCols = `id, machine_id, service_key, name, type, description, current_state, state_summary, severity, ttl_seconds, last_seen_at, last_run_at, enabled, sort_order, metadata_json, created_at, updated_at`

func scanService(sc interface{ Scan(...any) error }) (*Service, error) {
	var s Service
	err := sc.Scan(&s.ID, &s.MachineID, &s.ServiceKey, &s.Name, &s.Type, &s.Description, &s.CurrentState,
		&s.StateSummary, &s.Severity, &s.TTLSeconds, &s.LastSeenAt, &s.LastRunAt, &s.Enabled, &s.SortOrder,
		&s.MetadataJSON, &s.CreatedAt, &s.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &s, nil
}

// CreateService inserts a new service.
func (s *Store) CreateService(ctx context.Context, svc *Service) error {
	now := shared.FormatTime(shared.NowUTC())
	if svc.ID == "" {
		svc.ID = shared.NewID()
	}
	svc.CreatedAt, svc.UpdatedAt = now, now
	if svc.MetadataJSON == "" {
		svc.MetadataJSON = "{}"
	}
	if svc.CurrentState == "" {
		svc.CurrentState = "unknown"
	}
	if svc.Severity == "" {
		svc.Severity = "unknown"
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO services (id, machine_id, service_key, name, type, description, current_state, state_summary, severity, ttl_seconds, enabled, sort_order, metadata_json, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		svc.ID, svc.MachineID, svc.ServiceKey, svc.Name, svc.Type, svc.Description, svc.CurrentState,
		svc.StateSummary, svc.Severity, svc.TTLSeconds, boolToInt(svc.Enabled), svc.SortOrder, svc.MetadataJSON, svc.CreatedAt, svc.UpdatedAt)
	return err
}

// GetServiceByID returns a non-deleted service by id.
func (s *Store) GetServiceByID(ctx context.Context, id string) (*Service, error) {
	row := s.db.QueryRowContext(ctx, `SELECT `+serviceCols+` FROM services WHERE id = ? AND deleted_at IS NULL`, id)
	svc, err := scanService(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return svc, err
}

// GetServiceByKey returns a non-deleted service by (machine, service_key).
func (s *Store) GetServiceByKey(ctx context.Context, machineID, key string) (*Service, error) {
	row := s.db.QueryRowContext(ctx, `SELECT `+serviceCols+` FROM services WHERE machine_id = ? AND service_key = ? AND deleted_at IS NULL`, machineID, key)
	svc, err := scanService(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return svc, err
}

// ListServicesByMachine returns non-deleted services for a machine.
func (s *Store) ListServicesByMachine(ctx context.Context, machineID string) ([]*Service, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+serviceCols+` FROM services WHERE machine_id = ? AND deleted_at IS NULL ORDER BY sort_order, name`, machineID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Service
	for rows.Next() {
		svc, err := scanService(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, svc)
	}
	return out, rows.Err()
}

// UpdateServiceState updates the current projected state of a service.
func (s *Store) UpdateServiceState(ctx context.Context, id, state, summary, severity string, ttl *int, lastSeenAt string) error {
	now := shared.FormatTime(shared.NowUTC())
	_, err := s.db.ExecContext(ctx, `
		UPDATE services SET current_state = ?, state_summary = ?, severity = ?, ttl_seconds = COALESCE(?, ttl_seconds), last_seen_at = ?, updated_at = ?
		WHERE id = ?`,
		state, summary, severity, ttl, lastSeenAt, now, id)
	return err
}

// UpdateServiceLastRun sets last_run_at.
func (s *Store) UpdateServiceLastRun(ctx context.Context, id, at string) error {
	now := shared.FormatTime(shared.NowUTC())
	_, err := s.db.ExecContext(ctx, `UPDATE services SET last_run_at = ?, updated_at = ? WHERE id = ?`, at, now, id)
	return err
}

// UpdateServiceFields updates admin-editable service fields.
func (s *Store) UpdateServiceFields(ctx context.Context, id, name, description string, enabled bool, sortOrder int, metadataJSON string) error {
	now := shared.FormatTime(shared.NowUTC())
	_, err := s.db.ExecContext(ctx, `UPDATE services SET name = ?, description = ?, enabled = ?, sort_order = ?, metadata_json = ?, updated_at = ? WHERE id = ? AND deleted_at IS NULL`,
		name, description, boolToInt(enabled), sortOrder, metadataJSON, now, id)
	return err
}

// SoftDeleteService marks a service deleted.
func (s *Store) SoftDeleteService(ctx context.Context, id string) error {
	now := shared.FormatTime(shared.NowUTC())
	_, err := s.db.ExecContext(ctx, `UPDATE services SET deleted_at = ?, updated_at = ? WHERE id = ?`, now, now, id)
	return err
}
