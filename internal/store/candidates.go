package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"govoting/internal/models"
)

func scanCandidate(row rowScanner) (*models.Candidate, error) {
	var c models.Candidate
	err := row.Scan(&c.ID, &c.ElectionID, &c.CandidateNumber, &c.ChairmanName, &c.ViceChairmanName,
		&c.PhotoPath, &c.Vision, &c.Mission, &c.IsActive, &c.CreatedAt, &c.UpdatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &c, nil
}

const candidateColumns = `id, election_id, candidate_number, chairman_name, vice_chairman_name, photo_path, vision, mission, is_active, created_at, updated_at`

func (s *Store) ListCandidates(ctx context.Context, electionID int64, onlyActive bool) ([]models.Candidate, error) {
	query := `SELECT ` + candidateColumns + ` FROM candidates WHERE election_id=?`
	if onlyActive {
		query += ` AND is_active=1`
	}
	query += ` ORDER BY candidate_number ASC`
	rows, err := s.db.QueryContext(ctx, query, electionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.Candidate
	for rows.Next() {
		c, err := scanCandidate(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *c)
	}
	return out, rows.Err()
}

func (s *Store) GetCandidate(ctx context.Context, electionID, id int64) (*models.Candidate, error) {
	row := s.db.QueryRowContext(ctx, `SELECT `+candidateColumns+` FROM candidates WHERE id=? AND election_id=?`, id, electionID)
	return scanCandidate(row)
}

func (s *Store) lockEditableElection(ctx context.Context, tx *sql.Tx, schoolID, electionID int64, now time.Time) error {
	var status models.ElectionStatus
	err := tx.QueryRowContext(ctx, `
		SELECT status FROM elections WHERE id=? AND school_id=? FOR UPDATE`, electionID, schoolID).Scan(&status)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if status != models.ElectionDraft && status != models.ElectionScheduled {
		return ErrElectionLocked
	}
	return nil
}

func (s *Store) CreateCandidate(ctx context.Context, schoolID, electionID int64, in models.CandidateInput) (int64, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	if err := s.lockEditableElection(ctx, tx, schoolID, electionID, time.Now().UTC()); err != nil {
		return 0, err
	}
	if in.CandidateNumber < 1 || in.CandidateNumber > 99 {
		return 0, fmt.Errorf("%w: nomor urut harus 1-99", ErrInvalid)
	}
	res, err := tx.ExecContext(ctx, `
		INSERT INTO candidates (election_id, candidate_number, chairman_name, vice_chairman_name,
		                        photo_path, vision, mission, is_active)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		electionID, in.CandidateNumber, in.ChairmanName, in.ViceChairmanName,
		in.PhotoPath, in.Vision, in.Mission, in.IsActive)
	if err != nil {
		if isDuplicate(err) {
			return 0, fmt.Errorf("%w: nomor urut sudah dipakai", ErrConflict)
		}
		return 0, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	return id, tx.Commit()
}

func (s *Store) UpdateCandidate(ctx context.Context, schoolID, electionID, id int64, in models.CandidateInput) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := s.lockEditableElection(ctx, tx, schoolID, electionID, time.Now().UTC()); err != nil {
		return err
	}
	if in.CandidateNumber < 1 || in.CandidateNumber > 99 {
		return fmt.Errorf("%w: nomor urut harus 1-99", ErrInvalid)
	}
	res, err := tx.ExecContext(ctx, `
		UPDATE candidates
		SET candidate_number=?, chairman_name=?, vice_chairman_name=?, photo_path=?,
		    vision=?, mission=?, is_active=?
		WHERE id=? AND election_id=?`,
		in.CandidateNumber, in.ChairmanName, in.ViceChairmanName, in.PhotoPath,
		in.Vision, in.Mission, in.IsActive, id, electionID)
	if err != nil {
		if isDuplicate(err) {
			return fmt.Errorf("%w: nomor urut sudah dipakai", ErrConflict)
		}
		return err
	}
	if affected, _ := res.RowsAffected(); affected == 0 {
		var exists int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM candidates WHERE id=? AND election_id=?`, id, electionID).Scan(&exists); err != nil {
			return err
		}
		if exists == 0 {
			return ErrNotFound
		}
	}
	return tx.Commit()
}

func (s *Store) DeleteCandidate(ctx context.Context, schoolID, electionID, id int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := s.lockEditableElection(ctx, tx, schoolID, electionID, time.Now().UTC()); err != nil {
		return err
	}
	var votes int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM votes WHERE candidate_id=?`, id).Scan(&votes); err != nil {
		return err
	}
	if votes > 0 {
		return ErrVotesExist
	}
	res, err := tx.ExecContext(ctx, `DELETE FROM candidates WHERE id=? AND election_id=?`, id, electionID)
	if err != nil {
		return err
	}
	if affected, _ := res.RowsAffected(); affected == 0 {
		return ErrNotFound
	}
	return tx.Commit()
}

func (s *Store) CountCandidates(ctx context.Context, electionID int64, onlyActive bool) (int, error) {
	query := `SELECT COUNT(*) FROM candidates WHERE election_id=?`
	if onlyActive {
		query += ` AND is_active=1`
	}
	var n int
	err := s.db.QueryRowContext(ctx, query, electionID).Scan(&n)
	return n, err
}
