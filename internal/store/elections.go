package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"govoting/internal/models"
)

const electionColumns = `id, school_id, name, period, description, start_at, end_at, status, results_visibility, created_at, updated_at`

func scanElection(row rowScanner) (*models.Election, error) {
	var e models.Election
	var description sql.NullString
	err := row.Scan(&e.ID, &e.SchoolID, &e.Name, &e.Period, &description, &e.StartAt, &e.EndAt,
		&e.Status, &e.ResultsVisibility, &e.CreatedAt, &e.UpdatedAt, &e.CandidateCount)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	e.Description = description.String
	return &e, nil
}

func (s *Store) ListElections(ctx context.Context, schoolID int64) ([]models.Election, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT `+electionColumns+`,
		       (SELECT COUNT(*) FROM candidates c WHERE c.election_id = elections.id AND c.is_active = 1) AS candidate_count
		FROM elections
		WHERE school_id = ?
		ORDER BY FIELD(status,'ACTIVE','PAUSED','SCHEDULED','DRAFT','CLOSED','ARCHIVED'), start_at DESC, id DESC`, schoolID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.Election
	for rows.Next() {
		var e models.Election
		var description sql.NullString
		if err := rows.Scan(&e.ID, &e.SchoolID, &e.Name, &e.Period, &description, &e.StartAt,
			&e.EndAt, &e.Status, &e.ResultsVisibility, &e.CreatedAt, &e.UpdatedAt, &e.CandidateCount); err != nil {
			return nil, err
		}
		e.Description = description.String
		out = append(out, e)
	}
	return out, rows.Err()
}

func (s *Store) GetElection(ctx context.Context, schoolID, id int64) (*models.Election, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT `+electionColumns+`,
		       (SELECT COUNT(*) FROM candidates c WHERE c.election_id = elections.id AND c.is_active = 1)
		FROM elections WHERE id = ? AND school_id = ?`, id, schoolID)
	var e models.Election
	var description sql.NullString
	err := row.Scan(&e.ID, &e.SchoolID, &e.Name, &e.Period, &description, &e.StartAt, &e.EndAt,
		&e.Status, &e.ResultsVisibility, &e.CreatedAt, &e.UpdatedAt, &e.CandidateCount)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	e.Description = description.String
	return &e, nil
}

func (s *Store) GetActiveElection(ctx context.Context, schoolID int64, now time.Time) (*models.Election, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT `+electionColumns+`,
		       (SELECT COUNT(*) FROM candidates c WHERE c.election_id = elections.id AND c.is_active = 1)
		FROM elections
		WHERE school_id = ? AND status = 'ACTIVE' AND start_at <= ? AND end_at > ?
		ORDER BY id DESC LIMIT 1`, schoolID, now.UTC(), now.UTC())
	var e models.Election
	var description sql.NullString
	err := row.Scan(&e.ID, &e.SchoolID, &e.Name, &e.Period, &description, &e.StartAt, &e.EndAt,
		&e.Status, &e.ResultsVisibility, &e.CreatedAt, &e.UpdatedAt, &e.CandidateCount)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNoActiveElection
		}
		return nil, err
	}
	e.Description = description.String
	return &e, nil
}

func (s *Store) GetPreferredElection(ctx context.Context, schoolID int64, now time.Time) (*models.Election, error) {
	active, err := s.GetActiveElection(ctx, schoolID, now)
	if err == nil {
		return active, nil
	}
	if !errors.Is(err, ErrNoActiveElection) {
		return nil, err
	}
	row := s.db.QueryRowContext(ctx, `
		SELECT `+electionColumns+`,
		       (SELECT COUNT(*) FROM candidates c WHERE c.election_id = elections.id AND c.is_active = 1)
		FROM elections
		WHERE school_id = ?
		ORDER BY (status <> 'ARCHIVED') DESC, start_at DESC, id DESC LIMIT 1`, schoolID)
	var e models.Election
	var description sql.NullString
	err = row.Scan(&e.ID, &e.SchoolID, &e.Name, &e.Period, &description, &e.StartAt, &e.EndAt,
		&e.Status, &e.ResultsVisibility, &e.CreatedAt, &e.UpdatedAt, &e.CandidateCount)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	e.Description = description.String
	return &e, nil
}

func (s *Store) CreateElection(ctx context.Context, schoolID int64, in models.ElectionInput) (int64, error) {
	res, err := s.db.ExecContext(ctx, `
		INSERT INTO elections (school_id, name, period, description, start_at, end_at, status, results_visibility)
		VALUES (?, ?, ?, ?, ?, ?, 'DRAFT', ?)`,
		schoolID, in.Name, in.Period, in.Description, in.StartAt.UTC(), in.EndAt.UTC(), in.ResultsVisibility)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *Store) UpdateElection(ctx context.Context, schoolID, id int64, in models.ElectionInput) error {
	res, err := s.db.ExecContext(ctx, `
		UPDATE elections
		SET name=?, period=?, description=?, start_at=?, end_at=?, results_visibility=?
		WHERE id=? AND school_id=? AND status IN ('DRAFT','SCHEDULED')`,
		in.Name, in.Period, in.Description, in.StartAt.UTC(), in.EndAt.UTC(),
		in.ResultsVisibility, id, schoolID)
	if err != nil {
		return err
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		var status models.ElectionStatus
		err := s.db.QueryRowContext(ctx, `SELECT status FROM elections WHERE id=? AND school_id=?`, id, schoolID).Scan(&status)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if status != models.ElectionDraft && status != models.ElectionScheduled {
			return ErrElectionLocked
		}
	}
	return nil
}

func (s *Store) countActiveCandidates(ctx context.Context, electionID int64) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM candidates WHERE election_id=? AND is_active=1`, electionID).Scan(&n)
	return n, err
}

func (s *Store) SetElectionScheduled(ctx context.Context, schoolID, id int64, now time.Time) error {
	e, err := s.GetElection(ctx, schoolID, id)
	if err != nil {
		return err
	}
	if e.Status != models.ElectionDraft {
		return ErrConflict
	}
	if !e.EndAt.After(now) {
		return fmt.Errorf("%w: waktu selesai sudah lewat", ErrInvalid)
	}
	n, err := s.countActiveCandidates(ctx, id)
	if err != nil {
		return err
	}
	if n < 2 {
		return fmt.Errorf("%w: minimal 2 pasangan calon aktif", ErrInvalid)
	}
	res, err := s.db.ExecContext(ctx, `
		UPDATE elections SET status='SCHEDULED'
		WHERE id=? AND school_id=? AND status='DRAFT'`, id, schoolID)
	if err != nil {
		return err
	}
	if affected, _ := res.RowsAffected(); affected == 0 {
		return ErrConflict
	}
	return nil
}

func (s *Store) StartElection(ctx context.Context, schoolID, id int64, now time.Time) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var status models.ElectionStatus
	var endAt time.Time
	err = tx.QueryRowContext(ctx, `
		SELECT status, end_at FROM elections WHERE id=? AND school_id=? FOR UPDATE`, id, schoolID).
		Scan(&status, &endAt)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if !endAt.After(now) {
		return fmt.Errorf("%w: waktu selesai sudah lewat", ErrInvalid)
	}
	if status == models.ElectionActive {
		return nil
	}
	if status == models.ElectionClosed || status == models.ElectionArchived {
		return ErrElectionClosed
	}
	var activeCount int
	if err := tx.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM elections
		WHERE school_id=? AND id<>? AND status='ACTIVE'`, schoolID, id).Scan(&activeCount); err != nil {
		return err
	}
	if activeCount > 0 {
		return fmt.Errorf("%w: masih ada pemilihan lain yang berlangsung", ErrConflict)
	}
	var candidates int
	if err := tx.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM candidates WHERE election_id=? AND is_active=1`, id).Scan(&candidates); err != nil {
		return err
	}
	if candidates < 2 {
		return fmt.Errorf("%w: minimal 2 pasangan calon aktif", ErrInvalid)
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE elections
		SET status='ACTIVE', start_at = CASE WHEN start_at > ? THEN ? ELSE start_at END
		WHERE id=?`, now.UTC(), now.UTC(), id); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) PauseElection(ctx context.Context, schoolID, id int64) error {
	res, err := s.db.ExecContext(ctx, `
		UPDATE elections SET status='PAUSED'
		WHERE id=? AND school_id=? AND status='ACTIVE'`, id, schoolID)
	if err != nil {
		return err
	}
	if affected, _ := res.RowsAffected(); affected == 0 {
		return ErrConflict
	}
	return nil
}

func (s *Store) CloseElection(ctx context.Context, schoolID, id int64, now time.Time) error {
	res, err := s.db.ExecContext(ctx, `
		UPDATE elections
		SET status='CLOSED', end_at = CASE WHEN end_at > ? THEN ? ELSE end_at END
		WHERE id=? AND school_id=? AND status IN ('DRAFT','SCHEDULED','ACTIVE','PAUSED')`,
		now.UTC(), now.UTC(), id, schoolID)
	if err != nil {
		return err
	}
	if affected, _ := res.RowsAffected(); affected == 0 {
		var status models.ElectionStatus
		err := s.db.QueryRowContext(ctx, `SELECT status FROM elections WHERE id=? AND school_id=?`, id, schoolID).Scan(&status)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return ErrConflict
	}
	return nil
}

func (s *Store) ArchiveElection(ctx context.Context, schoolID, id int64) error {
	res, err := s.db.ExecContext(ctx, `
		UPDATE elections SET status='ARCHIVED'
		WHERE id=? AND school_id=? AND status='CLOSED'`, id, schoolID)
	if err != nil {
		return err
	}
	if affected, _ := res.RowsAffected(); affected == 0 {
		return ErrConflict
	}
	return nil
}

func (s *Store) AutoTransitionElections(ctx context.Context, now time.Time) ([]int64, error) {
	var changed []int64

	rows, err := s.db.QueryContext(ctx, `SELECT id FROM elections WHERE status='ACTIVE' AND end_at <= ?`, now.UTC())
	if err != nil {
		return nil, err
	}
	var toClose []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		toClose = append(toClose, id)
	}
	rows.Close()
	for _, id := range toClose {
		if _, err := s.db.ExecContext(ctx, `UPDATE elections SET status='CLOSED' WHERE id=? AND status='ACTIVE'`, id); err != nil {
			return nil, err
		}
		changed = append(changed, id)
	}

	type sched struct {
		id       int64
		schoolID int64
	}
	rows, err = s.db.QueryContext(ctx, `
		SELECT id, school_id FROM elections
		WHERE status='SCHEDULED' AND start_at <= ? AND end_at > ?`, now.UTC(), now.UTC())
	if err != nil {
		return nil, err
	}
	var scheduled []sched
	for rows.Next() {
		var item sched
		if err := rows.Scan(&item.id, &item.schoolID); err != nil {
			rows.Close()
			return nil, err
		}
		scheduled = append(scheduled, item)
	}
	rows.Close()
	for _, item := range scheduled {
		var active int
		if err := s.db.QueryRowContext(ctx, `
			SELECT COUNT(*) FROM elections WHERE school_id=? AND status='ACTIVE'`, item.schoolID).Scan(&active); err != nil {
			return nil, err
		}
		if active == 0 {
			res, err := s.db.ExecContext(ctx, `
				UPDATE elections SET status='ACTIVE'
				WHERE id=? AND status='SCHEDULED'`, item.id)
			if err != nil {
				return nil, err
			}
			if n, _ := res.RowsAffected(); n > 0 {
				changed = append(changed, item.id)
			}
		}
	}

	if _, err := s.db.ExecContext(ctx, `
		UPDATE voting_tokens SET status='EXPIRED'
		WHERE status='UNUSED' AND expires_at IS NOT NULL AND expires_at <= ?`, now.UTC()); err != nil {
		return nil, err
	}
	return changed, nil
}
