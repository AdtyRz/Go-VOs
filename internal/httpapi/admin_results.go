package httpapi

import (
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"govoting/internal/models"
	"govoting/internal/store"
	"govoting/internal/ws"
)

type livePayload struct {
	Election       models.Election          `json:"election"`
	Candidates     []models.ResultCandidate `json:"candidates"`
	TotalVotes     int64                    `json:"total_votes"`
	TokenStats     models.TokenStats        `json:"token_stats"`
	Participation  float64                  `json:"participation"`
	CandidateCount int                      `json:"candidate_count"`
	UpdatedAt      time.Time                `json:"updated_at"`
}

func (s *Server) buildLivePayload(r *http.Request, election *models.Election) (*livePayload, error) {
	now := time.Now().UTC()
	results, err := s.st.GetResults(r.Context(), election)
	if err != nil {
		return nil, err
	}
	stats, err := s.st.DashboardStats(r.Context(), election.ID, now)
	if err != nil {
		return nil, err
	}
	return &livePayload{
		Election:       *election,
		Candidates:     results.Candidates,
		TotalVotes:     results.TotalVotes,
		TokenStats:     stats.TokenStats,
		Participation:  stats.Participation,
		CandidateCount: stats.CandidateCount,
		UpdatedAt:      time.Now().UTC(),
	}, nil
}

func (s *Server) broadcastLive(ctx context.Context, electionID int64) {
	election, err := s.st.GetElectionByID(ctx, electionID)
	if err != nil {
		slog.Error("broadcast live: election", "error", err)
		return
	}
	payload, err := s.buildLivePayloadCtx(ctx, election)
	if err != nil {
		slog.Error("broadcast live: payload", "error", err)
		return
	}
	s.hub.Broadcast(electionID, ws.Event{Event: "vote_update", Data: payload})
}

func (s *Server) buildLivePayloadCtx(ctx context.Context, election *models.Election) (*livePayload, error) {
	now := time.Now().UTC()
	results, err := s.st.GetResults(ctx, election)
	if err != nil {
		return nil, err
	}
	stats, err := s.st.DashboardStats(ctx, election.ID, now)
	if err != nil {
		return nil, err
	}
	return &livePayload{
		Election:       *election,
		Candidates:     results.Candidates,
		TotalVotes:     results.TotalVotes,
		TokenStats:     stats.TokenStats,
		Participation:  stats.Participation,
		CandidateCount: stats.CandidateCount,
		UpdatedAt:      time.Now().UTC(),
	}, nil
}

func (s *Server) apiAdminDashboard(w http.ResponseWriter, r *http.Request) {
	election, err := s.selectedElectionForAPI(r, 0)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeData(w, http.StatusOK, map[string]any{"election": nil, "stats": models.DashboardStats{}})
			return
		}
		writeStoreError(w, err)
		return
	}
	payload, err := s.buildLivePayload(r, election)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeData(w, http.StatusOK, payload)
}

func (s *Server) apiAdminResults(w http.ResponseWriter, r *http.Request) {
	election, err := s.selectedElectionForAPI(r, 0)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	payload, err := s.buildLivePayload(r, election)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeData(w, http.StatusOK, payload)
}

func (s *Server) apiAdminResultsExport(w http.ResponseWriter, r *http.Request) {
	election, err := s.selectedElectionForAPI(r, 0)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	sess := currentSession(r)
	school, err := s.st.GetSchool(r.Context(), sess.Admin.SchoolID)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	results, err := s.st.GetResults(r.Context(), election)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	stats, err := s.st.DashboardStats(r.Context(), election.ID, time.Now().UTC())
	if err != nil {
		writeStoreError(w, err)
		return
	}

	format := strings.ToLower(r.URL.Query().Get("format"))
	if format == "" {
		format = "csv"
	}
	s.audit(r, "RESULTS_EXPORT", "election", &election.ID, map[string]any{"format": format})
	switch format {
	case "csv":
		s.exportResultsCSV(w, school, election, results, stats)
	case "pdf":
		s.exportResultsPDF(w, school, election, results, stats)
	default:
		writeAPIError(w, http.StatusBadRequest, "VALIDATION", "Format laporan tidak dikenal.")
	}
}

func csvText(value string) string {
	if value == "" {
		return value
	}
	switch value[0] {
	case '=', '+', '-', '@', '\t', '\r':
		return "'" + value
	}
	return value
}

func (s *Server) exportResultsCSV(w http.ResponseWriter, school *models.School, election *models.Election, results *models.ElectionResults, stats *models.DashboardStats) {
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="hasil-pemilihan-%d-%s.csv"`, election.ID, time.Now().In(s.cfg.Timezone).Format("20060102")))
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write([]byte{0xEF, 0xBB, 0xBF})
	cw := csv.NewWriter(w)
	_ = cw.Write([]string{"LAPORAN HASIL PEMILIHAN"})
	_ = cw.Write([]string{"Sekolah", csvText(school.Name)})
	_ = cw.Write([]string{"Pemilihan", csvText(election.Name)})
	_ = cw.Write([]string{"Periode", csvText(election.Period)})
	_ = cw.Write([]string{"Mulai", election.StartAt.In(s.cfg.Timezone).Format("02-01-2006 15:04")})
	_ = cw.Write([]string{"Selesai", election.EndAt.In(s.cfg.Timezone).Format("02-01-2006 15:04")})
	_ = cw.Write([]string{"Status", csvText(election.StatusLabel())})
	_ = cw.Write([]string{"Total token diterbitkan", strconv.FormatInt(stats.Total, 10)})
	_ = cw.Write([]string{"Total token digunakan", strconv.FormatInt(stats.Used, 10)})
	_ = cw.Write([]string{"Total suara sah", strconv.FormatInt(results.TotalVotes, 10)})
	_ = cw.Write([]string{"Partisipasi", fmt.Sprintf("%.2f%%", stats.Participation)})
	_ = cw.Write([]string{})
	_ = cw.Write([]string{"Peringkat", "Nomor Urut", "Calon Ketua", "Calon Wakil Ketua", "Suara", "Persentase"})
	for _, c := range results.Candidates {
		_ = cw.Write([]string{
			strconv.Itoa(c.Rank),
			strconv.Itoa(c.CandidateNumber),
			csvText(c.ChairmanName),
			csvText(c.ViceChairmanName),
			strconv.FormatInt(c.Votes, 10),
			fmt.Sprintf("%.2f%%", c.Percentage),
		})
	}
	if len(results.Candidates) > 0 {
		_ = cw.Write([]string{})
		winner := results.Candidates[0]
		_ = cw.Write([]string{"PEROLEHAN SUARA TERTINGGI", fmt.Sprintf("Paslon %02d", winner.CandidateNumber), csvText(winner.ChairmanName + " & " + winner.ViceChairmanName), strconv.FormatInt(winner.Votes, 10), fmt.Sprintf("%.2f%%", winner.Percentage)})
	}
	cw.Flush()
}
