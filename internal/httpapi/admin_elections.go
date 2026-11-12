package httpapi

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"govoting/internal/models"
	"govoting/internal/store"
)

type electionRequest struct {
	Name              string `json:"name"`
	Period            string `json:"period"`
	Description       string `json:"description"`
	StartAt           string `json:"start_at"`
	EndAt             string `json:"end_at"`
	ResultsVisibility string `json:"results_visibility"`
}

func (s *Server) parseElectionRequest(w http.ResponseWriter, r *http.Request, rejectPast bool) (models.ElectionInput, error) {
	var req electionRequest
	var input models.ElectionInput
	if err := decodeJSON(w, r, &req, 16*1024); err != nil {
		return input, store.ErrInvalid
	}
	name, err := trimmed(req.Name, 180, "nama pemilihan")
	if err != nil {
		return input, err
	}
	period, err := trimmed(req.Period, 80, "periode")
	if err != nil {
		return input, err
	}
	description, err := trimmedOptional(req.Description, 2000, "deskripsi")
	if err != nil {
		return input, err
	}
	start, err := parseFlexibleTime(req.StartAt, s.cfg.Timezone)
	if err != nil {
		return input, err
	}
	end, err := parseFlexibleTime(req.EndAt, s.cfg.Timezone)
	if err != nil {
		return input, err
	}
	if !end.After(start) {
		return input, errors.New("waktu selesai harus setelah waktu mulai")
	}
	if rejectPast && !end.After(time.Now().UTC()) {
		return input, errors.New("waktu selesai harus di masa depan")
	}
	visibility := strings.ToUpper(strings.TrimSpace(req.ResultsVisibility))
	if visibility == "" {
		visibility = "LIVE"
	}
	if visibility != "LIVE" && visibility != "FINAL" {
		return input, errors.New("mode hasil harus LIVE atau FINAL")
	}
	return models.ElectionInput{
		Name:              name,
		Period:            period,
		Description:       description,
		StartAt:           start,
		EndAt:             end,
		ResultsVisibility: visibility,
	}, nil
}

func (s *Server) apiAdminElections(w http.ResponseWriter, r *http.Request) {
	sess := currentSession(r)
	elections, err := s.st.ListElections(r.Context(), sess.Admin.SchoolID)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeData(w, http.StatusOK, map[string]any{"elections": elections})
}

func (s *Server) apiAdminElectionCreate(w http.ResponseWriter, r *http.Request) {
	sess := currentSession(r)
	r.Body = http.MaxBytesReader(w, r.Body, 32*1024)
	input, err := s.parseElectionRequest(w, r, false)
	if err != nil {
		s.writeElectionInputError(w, err)
		return
	}
	id, err := s.st.CreateElection(r.Context(), sess.Admin.SchoolID, input)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	s.audit(r, "ELECTION_CREATE", "election", &id, map[string]any{
		"name":   input.Name,
		"period": input.Period,
	})
	election, err := s.st.GetElection(r.Context(), sess.Admin.SchoolID, id)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeData(w, http.StatusCreated, map[string]any{"election": election})
}

func (s *Server) apiAdminElectionUpdate(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(r.PathValue("id"))
	if !ok {
		writeAPIError(w, http.StatusBadRequest, "VALIDATION", "ID pemilihan tidak valid.")
		return
	}
	sess := currentSession(r)
	election, err := s.st.GetElection(r.Context(), sess.Admin.SchoolID, id)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	if !election.IsEditable() {
		writeStoreError(w, store.ErrElectionLocked)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 32*1024)
	input, err := s.parseElectionRequest(w, r, false)
	if err != nil {
		s.writeElectionInputError(w, err)
		return
	}
	if err := s.st.UpdateElection(r.Context(), sess.Admin.SchoolID, id, input); err != nil {
		writeStoreError(w, err)
		return
	}
	s.audit(r, "ELECTION_UPDATE", "election", &id, map[string]any{"name": input.Name})
	updated, err := s.st.GetElection(r.Context(), sess.Admin.SchoolID, id)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeData(w, http.StatusOK, map[string]any{"election": updated})
}

func (s *Server) apiAdminElectionAction(action string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := parseID(r.PathValue("id"))
		if !ok {
			writeAPIError(w, http.StatusBadRequest, "VALIDATION", "ID pemilihan tidak valid.")
			return
		}
		sess := currentSession(r)
		election, err := s.st.GetElection(r.Context(), sess.Admin.SchoolID, id)
		if err != nil {
			writeStoreError(w, err)
			return
		}
		now := time.Now().UTC()
		auditAction := "ELECTION_UPDATE"
		switch action {
		case "schedule":
			err = s.st.SetElectionScheduled(r.Context(), sess.Admin.SchoolID, id, now)
			auditAction = "ELECTION_SCHEDULE"
		case "start":
			err = s.st.StartElection(r.Context(), sess.Admin.SchoolID, id, now)
			auditAction = "ELECTION_START"
		case "pause":
			err = s.st.PauseElection(r.Context(), sess.Admin.SchoolID, id)
			auditAction = "ELECTION_PAUSE"
		case "archive":
			err = s.st.ArchiveElection(r.Context(), sess.Admin.SchoolID, id)
			auditAction = "ELECTION_ARCHIVE"
		default:
			writeAPIError(w, http.StatusNotFound, "NOT_FOUND", "Aksi tidak dikenal.")
			return
		}
		if err != nil {
			writeStoreError(w, err)
			return
		}
		s.audit(r, auditAction, "election", &id, map[string]any{"status": action})
		s.broadcastLive(r.Context(), id)
		updated, err := s.st.GetElection(r.Context(), sess.Admin.SchoolID, id)
		if err != nil {
			writeStoreError(w, err)
			return
		}
		_ = election
		writeData(w, http.StatusOK, map[string]any{"election": updated})
	}
}

func (s *Server) apiAdminElectionClose(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(r.PathValue("id"))
	if !ok {
		writeAPIError(w, http.StatusBadRequest, "VALIDATION", "ID pemilihan tidak valid.")
		return
	}
	sess := currentSession(r)
	election, err := s.st.GetElection(r.Context(), sess.Admin.SchoolID, id)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	var input struct {
		Password string `json:"password"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 2048)
	if err := decodeJSON(w, r, &input, 2048); err != nil || input.Password == "" {
		writeAPIError(w, http.StatusBadRequest, "VALIDATION", "Masukkan password administrator untuk menutup pemilihan.")
		return
	}
	key := "reauth:" + strings.ToLower(sess.Admin.Username)
	if ok, retry := s.reauthLimiter.Allow(key); !ok {
		w.Header().Set("Retry-After", retrySeconds(retry))
		writeAPIError(w, http.StatusTooManyRequests, "RATE_LIMITED", "Terlalu banyak percobaan verifikasi. Coba lagi nanti.")
		return
	}
	auth, err := s.st.GetAdminAuthByLogin(r.Context(), sess.Admin.Username)
	if err != nil || auth.Admin.ID != sess.Admin.ID || !s.verifyPasswordLimited(r.Context(), input.Password, auth.PasswordHash) {
		writeAPIError(w, http.StatusForbidden, "REAUTH_FAILED", "Password tidak sesuai.")
		return
	}
	s.reauthLimiter.Reset(key)
	if err := s.st.CloseElection(r.Context(), sess.Admin.SchoolID, id, time.Now().UTC()); err != nil {
		writeStoreError(w, err)
		return
	}
	s.audit(r, "ELECTION_CLOSE", "election", &id, map[string]any{"previous_status": election.Status})
	s.broadcastLive(r.Context(), id)
	updated, err := s.st.GetElection(r.Context(), sess.Admin.SchoolID, id)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeData(w, http.StatusOK, map[string]any{"election": updated})
}

func (s *Server) writeElectionInputError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, store.ErrInvalid):
		writeAPIError(w, http.StatusBadRequest, "VALIDATION", "Data pemilihan tidak lengkap atau tidak valid.")
	case err != nil:
		msg := err.Error()
		if msg == "format waktu tidak valid" || strings.Contains(msg, "waktu selesai") || strings.Contains(msg, "mode hasil") || strings.Contains(msg, "waktu wajib") {
			writeAPIError(w, http.StatusBadRequest, "VALIDATION", msg)
			return
		}
		writeStoreError(w, err)
	}
}
