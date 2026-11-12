package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"govoting/internal/models"
	"govoting/internal/security"
)

func (s *Store) GenerateTokens(ctx context.Context, electionID int64, count int, expiresAt *time.Time, secret []byte) ([]string, error) {
	if count < 1 || count > security.MaxBulkTokenCount {
		return nil, fmt.Errorf("%w: jumlah token harus 1-%d", ErrInvalid, security.MaxBulkTokenCount)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	var status models.ElectionStatus
	var electionEnd time.Time
	err = tx.QueryRowContext(ctx, `
		SELECT status, end_at FROM elections WHERE id=? FOR UPDATE`, electionID).Scan(&status, &electionEnd)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if status == models.ElectionClosed || status == models.ElectionArchived {
		return nil, ErrElectionClosed
	}
	if expiresAt == nil {
		expiresAt = &electionEnd
	}
	if !expiresAt.After(time.Now().UTC()) {
		return nil, fmt.Errorf("%w: waktu kedaluwarsa token sudah lewat", ErrInvalid)
	}

	plain := make([]string, 0, count)
	for i := 0; i < count; i++ {
		inserted := false
		for attempt := 0; attempt < 25; attempt++ {
			raw, err := security.GenerateToken()
			if err != nil {
				return nil, err
			}
			hash := security.HashToken(secret, raw)
			_, err = tx.ExecContext(ctx, `
				INSERT INTO voting_tokens (election_id, token_hash, status, expires_at)
				VALUES (?, ?, 'UNUSED', ?)`, electionID, hash, expiresAt.UTC())
			if err == nil {
				plain = append(plain, raw)
				inserted = true
				break
			}
			if isDuplicate(err) {
				continue
			}
			return nil, err
		}
		if !inserted {
			return nil, fmt.Errorf("%w: gagal membuat token unik, coba lagi", ErrConflict)
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return plain, nil
}

const tokenEffectiveStatus = `CASE WHEN status='UNUSED' AND expires_at IS NOT NULL AND expires_at <= ? THEN 'EXPIRED' ELSE status END`

func (s *Store) ListTokens(ctx context.Context, electionID int64, status string, page, perPage int, now time.Time) (*models.TokenPage, error) {
	if page < 1 {
		page = 1
	}
	if perPage < 10 {
		perPage = 25
	}
	if perPage > 100 {
		perPage = 100
	}
	status = strings.ToUpper(strings.TrimSpace(status))
	validStatus := status == "" || status == "UNUSED" || status == "USED" || status == "REVOKED" || status == "EXPIRED"

	filterSQL := ""
	countArgs := []any{electionID}
	if validStatus && status != "" {
		filterSQL = ` AND (` + tokenEffectiveStatus + `) = ?`
		countArgs = append(countArgs, now.UTC(), status)
	}

	var total int64
	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM voting_tokens WHERE election_id=?`+filterSQL, countArgs...).Scan(&total); err != nil {
		return nil, err
	}
	pages := int((total + int64(perPage) - 1) / int64(perPage))
	if pages == 0 {
		pages = 1
	}
	if page > pages {
		page = pages
	}

	query := `SELECT id, election_id, ` + tokenEffectiveStatus + ` AS effective_status, expires_at, used_at, created_at
		FROM voting_tokens WHERE election_id=?` + filterSQL + ` ORDER BY id DESC LIMIT ? OFFSET ?`
	dataArgs := []any{now.UTC(), electionID}
	if validStatus && status != "" {
		dataArgs = append(dataArgs, now.UTC(), status)
	}
	dataArgs = append(dataArgs, perPage, (page-1)*perPage)
	rows, err := s.db.QueryContext(ctx, query, dataArgs...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := &models.TokenPage{Page: page, Pages: pages, Total: total, PerPage: perPage}
	for rows.Next() {
		var t models.Token
		var statusVal string
		var expires, used sql.NullTime
		if err := rows.Scan(&t.ID, &t.ElectionID, &statusVal, &expires, &used, &t.CreatedAt); err != nil {
			return nil, err
		}
		t.Status = models.TokenStatus(statusVal)
		t.ExpiresAt = nullableTime(expires)
		t.UsedAt = nullableTime(used)
		out.Tokens = append(out.Tokens, t)
	}
	return out, rows.Err()
}

func (s *Store) TokenStats(ctx context.Context, electionID int64, now time.Time) (*models.TokenStats, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*),
		       COALESCE(SUM(CASE WHEN status='UNUSED' AND (expires_at IS NULL OR expires_at > ?) THEN 1 ELSE 0 END),0),
		       COALESCE(SUM(CASE WHEN status='USED' THEN 1 ELSE 0 END),0),
		       COALESCE(SUM(CASE WHEN status='REVOKED' THEN 1 ELSE 0 END),0),
		       COALESCE(SUM(CASE WHEN status='EXPIRED' OR (status='UNUSED' AND expires_at IS NOT NULL AND expires_at <= ?) THEN 1 ELSE 0 END),0)
		FROM voting_tokens WHERE election_id=?`, now.UTC(), now.UTC(), electionID)
	var st models.TokenStats
	if err := row.Scan(&st.Total, &st.Available, &st.Used, &st.Revoked, &st.Expired); err != nil {
		return nil, err
	}
	return &st, nil
}

func (s *Store) RevokeTokens(ctx context.Context, electionID int64, ids []int64, now time.Time) (int64, error) {
	_ = now
	if len(ids) == 0 {
		return 0, fmt.Errorf("%w: pilih minimal satu token", ErrInvalid)
	}
	if len(ids) > 500 {
		return 0, fmt.Errorf("%w: maksimal 500 token per aksi", ErrInvalid)
	}
	holders := make([]string, len(ids))
	args := make([]any, 0, len(ids)+2)
	args = append(args, electionID)
	for i, id := range ids {
		holders[i] = "?"
		args = append(args, id)
	}
	query := `UPDATE voting_tokens SET status='REVOKED'
		WHERE election_id=? AND status='UNUSED' AND id IN (` + strings.Join(holders, ",") + `)`
	res, err := s.db.ExecContext(ctx, query, args...)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (s *Store) RevokeAllUnused(ctx context.Context, electionID int64) (int64, error) {
	res, err := s.db.ExecContext(ctx, `
		UPDATE voting_tokens SET status='REVOKED'
		WHERE election_id=? AND status='UNUSED'`, electionID)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (s *Store) TokenExportRows(ctx context.Context, electionID int64, now time.Time) ([]models.Token, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, election_id, `+tokenEffectiveStatus+` AS effective_status, expires_at, used_at, created_at
		FROM voting_tokens WHERE election_id=? ORDER BY id ASC`, now.UTC(), electionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.Token
	for rows.Next() {
		var t models.Token
		var status string
		var exp, used sql.NullTime
		if err := rows.Scan(&t.ID, &t.ElectionID, &status, &exp, &used, &t.CreatedAt); err != nil {
			return nil, err
		}
		t.Status = models.TokenStatus(status)
		t.ExpiresAt = nullableTime(exp)
		t.UsedAt = nullableTime(used)
		out = append(out, t)
	}
	return out, rows.Err()
}
