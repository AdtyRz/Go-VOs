package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"govoting/internal/models"
)

type Store struct {
	db *sql.DB
}

func New(db *sql.DB) *Store { return &Store{db: db} }

func (s *Store) DB() *sql.DB { return s.db }

func (s *Store) Ping(ctx context.Context) error {
	return s.db.PingContext(ctx)
}

func nullableTime(t sql.NullTime) *time.Time {
	if !t.Valid {
		return nil
	}
	v := t.Time
	return &v
}

func nullableID(v sql.NullInt64) *int64 {
	if !v.Valid {
		return nil
	}
	id := v.Int64
	return &id
}

func firstNonEmpty(v ...string) string {
	for _, s := range v {
		if s != "" {
			return s
		}
	}
	return ""
}

func decodeSettings(raw []byte) (footer, tagline string) {
	if len(raw) == 0 {
		return "", ""
	}
	var payload struct {
		FooterText string `json:"footer_text"`
		Tagline    string `json:"tagline"`
	}
	_ = json.Unmarshal(raw, &payload)
	return payload.FooterText, payload.Tagline
}

func encodeSettings(footer, tagline string) ([]byte, error) {
	return json.Marshal(map[string]string{
		"footer_text": footer,
		"tagline":     tagline,
	})
}

func (s *Store) GetSchool(ctx context.Context, id int64) (*models.School, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, name, slug, logo_path, address, email, phone, website,
		       theme, primary_color, secondary_color, settings_json, created_at, updated_at
		FROM schools WHERE id = ?`, id)
	return scanSchool(row)
}

func (s *Store) GetDefaultSchool(ctx context.Context) (*models.School, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, name, slug, logo_path, address, email, phone, website,
		       theme, primary_color, secondary_color, settings_json, created_at, updated_at
		FROM schools ORDER BY id ASC LIMIT 1`)
	school, err := scanSchool(row)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	return school, err
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanSchool(row rowScanner) (*models.School, error) {
	var school models.School
	var settings []byte
	err := row.Scan(
		&school.ID, &school.Name, &school.Slug, &school.LogoPath, &school.Address,
		&school.Email, &school.Phone, &school.Website, &school.Theme,
		&school.PrimaryColor, &school.SecondaryColor, &settings,
		&school.CreatedAt, &school.UpdatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, err
	}
	school.SettingsJSON = settings
	school.FooterText, school.Tagline = decodeSettings(settings)
	return &school, nil
}

func (s *Store) UpdateSchool(ctx context.Context, id int64, in models.SchoolInput) error {
	settings, err := encodeSettings(in.FooterText, in.Tagline)
	if err != nil {
		return err
	}
	res, err := s.db.ExecContext(ctx, `
		UPDATE schools SET name=?, logo_path=?, address=?, email=?, phone=?, website=?,
		       theme=?, primary_color=?, secondary_color=?, settings_json=?
		WHERE id=?`,
		in.Name, in.LogoPath, in.Address, in.Email, in.Phone, in.Website,
		in.Theme, in.PrimaryColor, in.SecondaryColor, settings, id)
	if err != nil {
		return err
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		var exists int
		if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM schools WHERE id=?`, id).Scan(&exists); err != nil {
			return err
		}
		if exists == 0 {
			return ErrNotFound
		}
	}
	return nil
}

func (s *Store) CountSchools(ctx context.Context) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM schools`).Scan(&n)
	return n, err
}

func (s *Store) CreateSchool(ctx context.Context, name, slug, email, address, phone, website string) (int64, error) {
	res, err := s.db.ExecContext(ctx, `
		INSERT INTO schools (name, slug, email, address, phone, website, theme, primary_color, secondary_color)
		VALUES (?, ?, ?, ?, ?, ?, 'batik', '#7a4f2a', '#c9a227')`,
		name, slug, email, address, phone, website)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func nowUTC() time.Time { return time.Now().UTC() }
