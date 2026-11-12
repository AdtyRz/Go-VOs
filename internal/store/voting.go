package store

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"govoting/internal/models"
)

func (s *Store) CreateVotingSession(ctx context.Context, tokenHash, sessionHash []byte, csrf string, ttl time.Duration, now time.Time) (*models.VotingSession, *models.Election, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, nil, err
	}
	defer tx.Rollback()

	var tokenID, electionID int64
	var tokenStatus string
	var tokenExpires sql.NullTime
	err = tx.QueryRowContext(ctx, `
		SELECT id, election_id, status, expires_at
		FROM voting_tokens WHERE token_hash=? FOR UPDATE`, tokenHash).
		Scan(&tokenID, &electionID, &tokenStatus, &tokenExpires)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil, ErrTokenInvalid
	}
	if err != nil {
		return nil, nil, err
	}
	if tokenStatus != string(models.TokenUnused) {
		return nil, nil, ErrTokenInvalid
	}
	if tokenExpires.Valid && !tokenExpires.Time.After(now) {
		return nil, nil, ErrTokenInvalid
	}

	election, err := scanElection(tx.QueryRowContext(ctx, `
		SELECT `+electionColumns+`,
		       (SELECT COUNT(*) FROM candidates c WHERE c.election_id = elections.id AND c.is_active = 1)
		FROM elections WHERE id=? FOR UPDATE`, electionID))
	if err != nil {
		return nil, nil, err
	}
	if election.Status != models.ElectionActive || now.Before(election.StartAt) || !now.Before(election.EndAt) {
		return nil, nil, ErrElectionClosed
	}
	if tokenExpires.Valid && tokenExpires.Time.Before(election.EndAt) {
		ttl = minDuration(ttl, tokenExpires.Time.Sub(now))
	}

	if _, err := tx.ExecContext(ctx, `DELETE FROM voting_sessions WHERE token_id=? AND consumed_at IS NULL`, tokenID); err != nil {
		return nil, nil, err
	}

	expiresAt := now.Add(ttl)
	if election.EndAt.Before(expiresAt) {
		expiresAt = election.EndAt
	}
	res, err := tx.ExecContext(ctx, `
		INSERT INTO voting_sessions (election_id, token_id, session_hash, csrf_token, expires_at)
		VALUES (?, ?, ?, ?, ?)`, electionID, tokenID, sessionHash, csrf, expiresAt.UTC())
	if err != nil {
		return nil, nil, err
	}
	sessionID, err := res.LastInsertId()
	if err != nil {
		return nil, nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, nil, err
	}

	return &models.VotingSession{
		ID:         sessionID,
		ElectionID: electionID,
		TokenID:    &tokenID,
		CSRF:       csrf,
		ExpiresAt:  expiresAt,
		CreatedAt:  now,
	}, election, nil
}

func minDuration(a, b time.Duration) time.Duration {
	if b < a {
		return b
	}
	return a
}

func (s *Store) GetVotingSession(ctx context.Context, sessionHash []byte, now time.Time) (*models.VotingSession, *models.Election, error) {
	var sess models.VotingSession
	var tokenID sql.NullInt64
	var consumed sql.NullTime
	err := s.db.QueryRowContext(ctx, `
		SELECT id, election_id, token_id, csrf_token, expires_at, consumed_at, created_at
		FROM voting_sessions
		WHERE session_hash=? AND consumed_at IS NULL AND expires_at > ?`, sessionHash, now.UTC()).
		Scan(&sess.ID, &sess.ElectionID, &tokenID, &sess.CSRF, &sess.ExpiresAt, &consumed, &sess.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil, ErrSessionInvalid
	}
	if err != nil {
		return nil, nil, err
	}
	sess.TokenID = nullableID(tokenID)
	sess.ConsumedAt = nullableTime(consumed)

	election, err := s.GetElectionByID(ctx, sess.ElectionID)
	if err != nil {
		return nil, nil, err
	}
	if election.Status != models.ElectionActive || now.Before(election.StartAt) || !now.Before(election.EndAt) {
		return nil, nil, ErrElectionClosed
	}
	return &sess, election, nil
}

func (s *Store) GetElectionByID(ctx context.Context, id int64) (*models.Election, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT `+electionColumns+`,
		       (SELECT COUNT(*) FROM candidates c WHERE c.election_id = elections.id AND c.is_active = 1)
		FROM elections WHERE id=?`, id)
	return scanElection(row)
}

func (s *Store) SubmitVote(ctx context.Context, sessionHash []byte, candidateID int64, now time.Time) (int64, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	var sessionID, electionID int64
	var tokenID sql.NullInt64
	var expiresAt time.Time
	var consumedAt sql.NullTime
	err = tx.QueryRowContext(ctx, `
		SELECT id, election_id, token_id, expires_at, consumed_at
		FROM voting_sessions WHERE session_hash=? FOR UPDATE`, sessionHash).
		Scan(&sessionID, &electionID, &tokenID, &expiresAt, &consumedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrSessionInvalid
	}
	if err != nil {
		return 0, err
	}
	if consumedAt.Valid || !expiresAt.After(now) || !tokenID.Valid {
		return 0, ErrSessionInvalid
	}

	var tokenElectionID int64
	var tokenStatus string
	var tokenExpires sql.NullTime
	err = tx.QueryRowContext(ctx, `
		SELECT election_id, status, expires_at FROM voting_tokens WHERE id=? FOR UPDATE`, tokenID.Int64).
		Scan(&tokenElectionID, &tokenStatus, &tokenExpires)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrSessionInvalid
	}
	if err != nil {
		return 0, err
	}
	if tokenElectionID != electionID || tokenStatus != string(models.TokenUnused) ||
		(tokenExpires.Valid && !tokenExpires.Time.After(now)) {
		return 0, ErrSessionInvalid
	}

	var electionStatus models.ElectionStatus
	var startAt, endAt time.Time
	err = tx.QueryRowContext(ctx, `
		SELECT status, start_at, end_at FROM elections WHERE id=? FOR UPDATE`, electionID).
		Scan(&electionStatus, &startAt, &endAt)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrElectionClosed
	}
	if err != nil {
		return 0, err
	}
	if electionStatus != models.ElectionActive || now.Before(startAt) || !now.Before(endAt) {
		return 0, ErrElectionClosed
	}

	var candidateElectionID int64
	var isActive bool
	err = tx.QueryRowContext(ctx, `
		SELECT election_id, is_active FROM candidates WHERE id=?`, candidateID).
		Scan(&candidateElectionID, &isActive)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrCandidateInvalid
	}
	if err != nil {
		return 0, err
	}
	if candidateElectionID != electionID || !isActive {
		return 0, ErrCandidateInvalid
	}

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO votes (election_id, candidate_id, created_at) VALUES (?, ?, ?)`,
		electionID, candidateID, now.UTC()); err != nil {
		return 0, err
	}

	res, err := tx.ExecContext(ctx, `
		UPDATE voting_tokens SET status='USED', used_at=? WHERE id=? AND status='UNUSED'`,
		now.UTC(), tokenID.Int64)
	if err != nil {
		return 0, err
	}
	if affected, _ := res.RowsAffected(); affected != 1 {
		return 0, ErrAlreadyUsed
	}

	if _, err := tx.ExecContext(ctx, `
		UPDATE voting_sessions SET consumed_at=?, token_id=NULL WHERE id=? AND consumed_at IS NULL`,
		now.UTC(), sessionID); err != nil {
		return 0, err
	}

	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return electionID, nil
}

func (s *Store) CancelVotingSession(ctx context.Context, sessionHash []byte) error {
	_, err := s.db.ExecContext(ctx, `
		DELETE FROM voting_sessions WHERE session_hash=? AND consumed_at IS NULL`, sessionHash)
	return err
}

func (s *Store) CleanupVotingSessions(ctx context.Context, now time.Time) (int64, error) {
	res, err := s.db.ExecContext(ctx, `
		DELETE FROM voting_sessions
		WHERE expires_at <= ? OR (consumed_at IS NOT NULL AND consumed_at <= ?)`,
		now.UTC(), now.UTC().Add(-24*time.Hour))
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
