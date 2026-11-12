package store

import (
	"context"
	"sort"
	"time"

	"govoting/internal/models"
)

func (s *Store) CountVotes(ctx context.Context, electionID int64) (int64, error) {
	var n int64
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM votes WHERE election_id=?`, electionID).Scan(&n)
	return n, err
}

func (s *Store) GetResults(ctx context.Context, election *models.Election) (*models.ElectionResults, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT c.id, c.candidate_number, c.chairman_name, c.vice_chairman_name, c.photo_path,
		       c.is_active, COUNT(v.id) AS votes
		FROM candidates c
		LEFT JOIN votes v ON v.candidate_id = c.id AND v.election_id = c.election_id
		WHERE c.election_id = ?
		GROUP BY c.id, c.candidate_number, c.chairman_name, c.vice_chairman_name, c.photo_path, c.is_active
		ORDER BY c.candidate_number ASC`, election.ID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var candidates []models.ResultCandidate
	var total int64
	for rows.Next() {
		var c models.ResultCandidate
		if err := rows.Scan(&c.CandidateID, &c.CandidateNumber, &c.ChairmanName, &c.ViceChairmanName,
			&c.PhotoPath, &c.IsActive, &c.Votes); err != nil {
			return nil, err
		}
		total += c.Votes
		candidates = append(candidates, c)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	for i := range candidates {
		if total > 0 {
			candidates[i].Percentage = float64(candidates[i].Votes) / float64(total) * 100
		}
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].Votes == candidates[j].Votes {
			return candidates[i].CandidateNumber < candidates[j].CandidateNumber
		}
		return candidates[i].Votes > candidates[j].Votes
	})
	rank := 0
	prevVotes := int64(-1)
	for i := range candidates {
		if candidates[i].Votes != prevVotes {
			rank = i + 1
			prevVotes = candidates[i].Votes
		}
		candidates[i].Rank = rank
	}

	return &models.ElectionResults{
		Election:   *election,
		Candidates: candidates,
		TotalVotes: total,
		UpdatedAt:  time.Now().UTC(),
	}, nil
}

func (s *Store) DashboardStats(ctx context.Context, electionID int64, now time.Time) (*models.DashboardStats, error) {
	tokens, err := s.TokenStats(ctx, electionID, now)
	if err != nil {
		return nil, err
	}
	votes, err := s.CountVotes(ctx, electionID)
	if err != nil {
		return nil, err
	}
	candidates, err := s.CountCandidates(ctx, electionID, true)
	if err != nil {
		return nil, err
	}
	stats := &models.DashboardStats{
		TokenStats:     *tokens,
		Votes:          votes,
		CandidateCount: candidates,
	}
	if tokens.Total > 0 {
		stats.Participation = float64(tokens.Used) / float64(tokens.Total) * 100
	}
	return stats, nil
}
