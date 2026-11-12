package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"time"

	"github.com/gorilla/websocket"

	"govoting/internal/models"
	"govoting/internal/store"
	"govoting/internal/ws"
)

type publicResultsPayload struct {
	ElectionID   int64                    `json:"election_id"`
	ElectionName string                   `json:"election_name"`
	Period       string                   `json:"period"`
	ResultsFinal bool                     `json:"results_final"`
	Candidates   []models.ResultCandidate `json:"candidates"`
	TotalVotes   int64                    `json:"total_votes"`
	UpdatedAt    time.Time                `json:"updated_at"`
}

type publicResultsResponse struct {
	Visible bool                  `json:"visible"`
	Results *publicResultsPayload `json:"results,omitempty"`
}

func publicResultsVisible(election *models.Election, now time.Time) bool {
	if election == nil {
		return false
	}
	switch election.ResultsVisibility {
	case "LIVE":
		switch election.Status {
		case models.ElectionActive, models.ElectionPaused, models.ElectionClosed, models.ElectionArchived:
			return true
		}
	case "FINAL":
		if election.Status == models.ElectionClosed || election.Status == models.ElectionArchived {
			return true
		}
		if election.Status == models.ElectionActive && !now.Before(election.EndAt) {
			return true
		}
	}
	return false
}

func publicResultsFinal(election *models.Election, now time.Time) bool {
	if election == nil {
		return false
	}
	if election.Status == models.ElectionClosed || election.Status == models.ElectionArchived {
		return true
	}
	return !election.EndAt.IsZero() && !now.Before(election.EndAt)
}

func (s *Server) publicResultsElection(ctx context.Context, now time.Time) (*models.Election, bool, error) {
	school, err := s.st.GetDefaultSchool(ctx)
	if err != nil {
		return nil, false, err
	}
	election, err := s.st.GetPreferredElection(ctx, school.ID, now)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) || errors.Is(err, store.ErrNoActiveElection) {
			return nil, false, nil
		}
		return nil, false, err
	}
	return election, publicResultsVisible(election, now), nil
}

func (s *Server) buildPublicResults(ctx context.Context, election *models.Election, now time.Time) (*publicResultsPayload, error) {
	results, err := s.st.GetResults(ctx, election)
	if err != nil {
		return nil, err
	}
	return &publicResultsPayload{
		ElectionID:   election.ID,
		ElectionName: election.Name,
		Period:       election.Period,
		ResultsFinal: publicResultsFinal(election, now),
		Candidates:   results.Candidates,
		TotalVotes:   results.TotalVotes,
		UpdatedAt:    results.UpdatedAt,
	}, nil
}

func (s *Server) apiPublicResults(w http.ResponseWriter, r *http.Request) {
	now := time.Now().UTC()
	election, visible, err := s.publicResultsElection(r.Context(), now)
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "INTERNAL", "Gagal memuat hasil pemilihan.")
		return
	}
	if !visible {
		writeData(w, http.StatusOK, publicResultsResponse{Visible: false})
		return
	}
	payload, err := s.buildPublicResults(r.Context(), election, now)
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "INTERNAL", "Gagal memuat hasil pemilihan.")
		return
	}
	writeData(w, http.StatusOK, publicResultsResponse{Visible: true, Results: payload})
}

func (s *Server) wsPublicResults(w http.ResponseWriter, r *http.Request) {
	now := time.Now().UTC()
	election, visible, err := s.publicResultsElection(r.Context(), now)
	if err != nil {
		http.Error(w, "Hasil belum tersedia.", http.StatusServiceUnavailable)
		return
	}
	if !visible || election.ResultsVisibility != "LIVE" || election.Status != models.ElectionActive {
		http.Error(w, "Hasil live tidak tersedia.", http.StatusForbidden)
		return
	}
	payload, err := s.buildPublicResults(r.Context(), election, now)
	if err != nil {
		http.Error(w, "Hasil belum tersedia.", http.StatusServiceUnavailable)
		return
	}

	upgrader := websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 4096,
		CheckOrigin: func(r *http.Request) bool {
			origin := r.Header.Get("Origin")
			if origin == "" {
				return true
			}
			u, err := url.Parse(origin)
			if err != nil {
				return false
			}
			return sameHost(u.Host, r.Host)
		},
	}
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	conn.SetReadLimit(1024)
	client := s.hub.NewClient(conn, 0, election.SchoolID, election.ID)
	if !s.hub.Register(client) {
		_ = conn.WriteControl(websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseTryAgainLater, "terlalu banyak koneksi"),
			time.Now().Add(2*time.Second))
		_ = conn.Close()
		return
	}
	s.hub.Send(client, ws.Event{Event: "snapshot", Data: payload})
}
