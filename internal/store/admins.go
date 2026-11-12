package store

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"govoting/internal/models"
)

type AdminAuth struct {
	Admin            *models.Admin
	PasswordHash     string
	LockedUntil      *time.Time
	FailedLoginCount int
}

func scanAdmin(row rowScanner, withAuth bool) (*AdminAuth, error) {
	var auth AdminAuth
	var admin models.Admin
	var email sql.NullString
	var lastLogin sql.NullTime
	var locked sql.NullTime
	dest := []any{
		&admin.ID, &admin.SchoolID, &admin.Name, &admin.Username, &email,
		&admin.Role, &admin.IsActive, &lastLogin, &admin.CreatedAt,
	}
	if withAuth {
		dest = append(dest, &auth.PasswordHash, &locked, &auth.FailedLoginCount)
	}
	err := row.Scan(dest...)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	admin.Email = email.String
	admin.LastLoginAt = nullableTime(lastLogin)
	auth.Admin = &admin
	auth.LockedUntil = nullableTime(locked)
	return &auth, nil
}

func (s *Store) GetAdminAuthByLogin(ctx context.Context, login string) (*AdminAuth, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, school_id, name, username, email, role, is_active, last_login_at,
		       created_at, password_hash, locked_until, failed_login_count
		FROM admins
		WHERE username = ? OR (email IS NOT NULL AND email <> '' AND email = ?)
		LIMIT 1`, login, login)
	return scanAdmin(row, true)
}

func (s *Store) GetAdminByID(ctx context.Context, id int64) (*models.Admin, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, school_id, name, username, email, role, is_active, last_login_at, created_at
		FROM admins WHERE id = ?`, id)
	auth, err := scanAdmin(row, false)
	if err != nil {
		return nil, err
	}
	return auth.Admin, nil
}

func (s *Store) CountAdmins(ctx context.Context) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM admins`).Scan(&n)
	return n, err
}

func (s *Store) CreateAdmin(ctx context.Context, schoolID int64, name, username, email, passwordHash string) (int64, error) {
	var emailVal any
	if email != "" {
		emailVal = email
	}
	res, err := s.db.ExecContext(ctx, `
		INSERT INTO admins (school_id, name, username, email, password_hash, role, is_active)
		VALUES (?, ?, ?, ?, ?, 'superadmin', 1)`,
		schoolID, name, username, emailVal, passwordHash)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *Store) MarkLoginSuccess(ctx context.Context, adminID int64) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE admins SET last_login_at = ?, failed_login_count = 0, locked_until = NULL
		WHERE id = ?`, nowUTC(), adminID)
	return err
}

func (s *Store) MarkLoginFailure(ctx context.Context, adminID int64, maxAttempts int, lockFor time.Duration) error {
	lockUntil := time.Now().UTC().Add(lockFor)
	_, err := s.db.ExecContext(ctx, `
		UPDATE admins
		SET locked_until = CASE WHEN failed_login_count + 1 >= ? THEN ? ELSE locked_until END,
		    failed_login_count = failed_login_count + 1
		WHERE id = ?`, maxAttempts, lockUntil, adminID)
	return err
}

func (s *Store) CreateAdminSession(ctx context.Context, sessionHash []byte, adminID *int64, csrf string, expiresAt time.Time) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO admin_sessions (session_hash, admin_id, csrf_token, expires_at, last_seen_at)
		VALUES (?, ?, ?, ?, ?)`, sessionHash, adminID, csrf, expiresAt.UTC(), nowUTC())
	return err
}

func (s *Store) ReplaceLoginSession(ctx context.Context, oldHash, newHash []byte, adminID int64, csrf string, expiresAt time.Time) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `DELETE FROM admin_sessions WHERE session_hash = ?`, oldHash); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO admin_sessions (session_hash, admin_id, csrf_token, expires_at, last_seen_at)
		VALUES (?, ?, ?, ?, ?)`, newHash, adminID, csrf, expiresAt.UTC(), nowUTC()); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) GetSession(ctx context.Context, sessionHash []byte, now time.Time) (*models.Session, error) {
	var sess models.Session
	var adminID sql.NullInt64
	err := s.db.QueryRowContext(ctx, `
		SELECT id, admin_id, csrf_token, expires_at
		FROM admin_sessions
		WHERE session_hash = ? AND expires_at > ?`, sessionHash, now.UTC()).
		Scan(&sess.ID, &adminID, &sess.CSRF, &sess.ExpiresAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	sess.AdminID = nullableID(adminID)
	if sess.AdminID != nil {
		admin, err := s.GetAdminByID(ctx, *sess.AdminID)
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				return nil, ErrNotFound
			}
			return nil, err
		}
		if !admin.IsActive {
			return nil, ErrInactive
		}
		sess.Admin = admin
	}
	return &sess, nil
}

func (s *Store) TouchSession(ctx context.Context, sessionID int64, now time.Time) {

	_, _ = s.db.ExecContext(ctx, `
		UPDATE admin_sessions SET last_seen_at = ?
		WHERE id = ? AND last_seen_at < ?`, now.UTC(), sessionID, now.UTC().Add(-5*time.Minute))
}

func (s *Store) DeleteSession(ctx context.Context, sessionHash []byte) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM admin_sessions WHERE session_hash = ?`, sessionHash)
	return err
}

func (s *Store) CleanupExpiredSessions(ctx context.Context, now time.Time) (int64, error) {
	res, err := s.db.ExecContext(ctx, `DELETE FROM admin_sessions WHERE expires_at <= ?`, now.UTC())
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
