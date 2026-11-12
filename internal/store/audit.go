package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"govoting/internal/models"
)

func (s *Store) RecordAudit(ctx context.Context, schoolID int64, adminID *int64, action, entityType string, entityID *int64, metadata any) error {
	var meta []byte
	if metadata != nil {
		var err error
		meta, err = json.Marshal(metadata)
		if err != nil {
			return err
		}
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO audit_logs (school_id, admin_id, action, entity_type, entity_id, metadata_json, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		schoolID, adminID, action, entityType, entityID, meta, time.Now().UTC())
	return err
}

func (s *Store) ListAudit(ctx context.Context, schoolID int64, page, perPage int) ([]models.AuditLog, int64, error) {
	if page < 1 {
		page = 1
	}
	if perPage < 10 {
		perPage = 30
	}
	if perPage > 100 {
		perPage = 100
	}
	var total int64
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM audit_logs WHERE school_id=?`, schoolID).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT a.id, a.admin_id, COALESCE(ad.name, 'Sistem'), a.action, a.entity_type,
		       a.entity_id, a.metadata_json, a.created_at
		FROM audit_logs a
		LEFT JOIN admins ad ON ad.id = a.admin_id
		WHERE a.school_id = ?
		ORDER BY a.id DESC LIMIT ? OFFSET ?`, schoolID, perPage, (page-1)*perPage)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []models.AuditLog
	for rows.Next() {
		var log models.AuditLog
		var adminID, entityID sql.NullInt64
		var meta []byte
		if err := rows.Scan(&log.ID, &adminID, &log.AdminName, &log.Action, &log.EntityType,
			&entityID, &meta, &log.CreatedAt); err != nil {
			return nil, 0, err
		}
		log.AdminID = nullableID(adminID)
		log.EntityID = nullableID(entityID)
		log.Metadata = meta
		out = append(out, log)
	}
	return out, total, rows.Err()
}
