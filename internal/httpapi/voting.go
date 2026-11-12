package httpapi

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"govoting/internal/models"
	"govoting/internal/security"
	"govoting/internal/store"
)

type studentHomeData struct {
	BasePage
	IsOpen         bool
	CandidateCount int
	Notice         string
	NoticeKind     string
	ResultsVisible bool
	ResultsFinal   bool
	ResultsLive    bool
	Results        *publicResultsPayload
}

func (s *Server) studentHome(w http.ResponseWriter, r *http.Request) {
	base, err := s.siteBase(r.Context(), "Pemilihan OSIS", "home")
	if err != nil {
		http.Error(w, "Aplikasi belum siap.", http.StatusInternalServerError)
		return
	}
	now := time.Now().UTC()
	data := studentHomeData{BasePage: *base}
	election, err := s.st.GetPreferredElection(r.Context(), base.School.ID, now)
	if err == nil {
		data.Election = election
		data.IsOpen = election.IsOpen(now)
		data.CandidateCount = election.CandidateCount
		if publicResultsVisible(election, now) {
			payload, err := s.buildPublicResults(r.Context(), election, now)
			if err != nil {
				http.Error(w, "Gagal memuat hasil pemilihan.", http.StatusInternalServerError)
				return
			}
			data.ResultsVisible = true
			data.ResultsFinal = payload.ResultsFinal
			data.ResultsLive = data.IsOpen && election.ResultsVisibility == "LIVE"
			data.Results = payload
		}
	} else if !errors.Is(err, store.ErrNotFound) && !errors.Is(err, store.ErrNoActiveElection) {
		http.Error(w, "Gagal memuat pemilihan.", http.StatusInternalServerError)
		return
	}

	if r.URL.Query().Get("status") == "success" {
		data.Notice = "Suara Anda sudah tercatat. Terima kasih sudah menggunakan hak suara."
		data.NoticeKind = "success"
	}
	s.renderer.Render(w, http.StatusOK, "student/home", data)
}

type studentTokenData struct {
	BasePage
	IsOpen     bool
	Notice     string
	NoticeKind string
}

func (s *Server) studentToken(w http.ResponseWriter, r *http.Request) {
	base, err := s.siteBase(r.Context(), "Masukkan Token", "token")
	if err != nil {
		http.Error(w, "Aplikasi belum siap.", http.StatusInternalServerError)
		return
	}
	now := time.Now().UTC()
	data := studentTokenData{BasePage: *base}
	election, err := s.st.GetPreferredElection(r.Context(), base.School.ID, now)
	if err == nil {
		data.Election = election
		data.IsOpen = election.IsOpen(now)
	} else if !errors.Is(err, store.ErrNotFound) && !errors.Is(err, store.ErrNoActiveElection) {
		http.Error(w, "Gagal memuat pemilihan.", http.StatusInternalServerError)
		return
	}

	switch r.URL.Query().Get("status") {
	case "expired":
		data.Notice = "Sesi memilih sudah berakhir. Masukkan token kembali untuk melanjutkan."
		data.NoticeKind = "warning"
	case "closed":
		data.Notice = "Pemilihan belum dibuka atau sudah ditutup."
		data.NoticeKind = "warning"
	case "error":
		data.Notice = "Terjadi kendala. Silakan coba lagi atau hubungi panitia."
		data.NoticeKind = "warning"
	}
	s.renderer.Render(w, http.StatusOK, "student/token", data)
}

type studentBallotData struct {
	BasePage
	Candidates []models.Candidate
	ExpiresAt  time.Time
}

func (s *Server) studentBallot(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie(studentCookieName)
	if err != nil || cookie.Value == "" {
		http.Redirect(w, r, "/token?status=expired", http.StatusSeeOther)
		return
	}
	now := time.Now().UTC()
	sess, election, err := s.st.GetVotingSession(r.Context(), security.HashSession(cookie.Value), now)
	if err != nil {
		s.clearCookie(w, studentCookieName)
		status := "expired"
		if errors.Is(err, store.ErrElectionClosed) {
			status = "closed"
		}
		http.Redirect(w, r, "/token?status="+status, http.StatusSeeOther)
		return
	}
	school, err := s.st.GetDefaultSchool(r.Context())
	if err != nil || school.ID != election.SchoolID {
		s.clearCookie(w, studentCookieName)
		http.Redirect(w, r, "/token?status=error", http.StatusSeeOther)
		return
	}
	candidates, err := s.st.ListCandidates(r.Context(), election.ID, true)
	if err != nil {
		http.Error(w, "Gagal memuat pasangan calon.", http.StatusInternalServerError)
		return
	}
	s.renderer.Render(w, http.StatusOK, "student/ballot", studentBallotData{
		BasePage: BasePage{
			Title:       "Pilih Pasangan Calon",
			Page:        "ballot",
			School:      school,
			Election:    election,
			CSRF:        sess.CSRF,
			BaseURL:     s.cfg.BaseURL,
			CurrentYear: time.Now().In(s.cfg.Timezone).Year(),
		},
		Candidates: candidates,
		ExpiresAt:  sess.ExpiresAt,
	})
}

func (s *Server) studentSuccess(w http.ResponseWriter, r *http.Request) {
	s.clearCookie(w, studentCookieName)
	base, err := s.siteBase(r.Context(), "Suara Berhasil Direkam", "success")
	if err != nil {
		http.Error(w, "Aplikasi belum siap.", http.StatusInternalServerError)
		return
	}
	s.renderer.Render(w, http.StatusOK, "student/success", *base)
}

func (s *Server) apiPublicSchool(w http.ResponseWriter, r *http.Request) {
	school, err := s.st.GetDefaultSchool(r.Context())
	if err != nil {
		writeAPIError(w, http.StatusServiceUnavailable, "NOT_READY", "Profil sekolah belum tersedia.")
		return
	}
	writeData(w, http.StatusOK, map[string]any{
		"name":            school.Name,
		"slug":            school.Slug,
		"logo_path":       school.LogoPath,
		"address":         school.Address,
		"email":           school.Email,
		"phone":           school.Phone,
		"website":         school.Website,
		"theme":           school.Theme,
		"primary_color":   school.PrimaryColor,
		"secondary_color": school.SecondaryColor,
		"footer_text":     school.FooterText,
		"tagline":         school.Tagline,
	})
}

func (s *Server) apiPublicElection(w http.ResponseWriter, r *http.Request) {
	school, err := s.st.GetDefaultSchool(r.Context())
	if err != nil {
		writeAPIError(w, http.StatusServiceUnavailable, "NOT_READY", "Sekolah belum tersedia.")
		return
	}
	election, err := s.st.GetActiveElection(r.Context(), school.ID, time.Now().UTC())
	if err != nil {
		if errors.Is(err, store.ErrNoActiveElection) {
			writeData(w, http.StatusOK, map[string]any{"election": nil})
			return
		}
		writeAPIError(w, http.StatusInternalServerError, "INTERNAL", "Gagal memuat pemilihan.")
		return
	}
	writeData(w, http.StatusOK, map[string]any{
		"election": map[string]any{
			"id":                 election.ID,
			"name":               election.Name,
			"period":             election.Period,
			"description":        election.Description,
			"start_at":           election.StartAt,
			"end_at":             election.EndAt,
			"status":             election.Status,
			"candidate_count":    election.CandidateCount,
			"results_visibility": election.ResultsVisibility,
		},
	})
}

func (s *Server) apiPublicCandidates(w http.ResponseWriter, r *http.Request) {
	school, err := s.st.GetDefaultSchool(r.Context())
	if err != nil {
		writeAPIError(w, http.StatusServiceUnavailable, "NOT_READY", "Sekolah belum tersedia.")
		return
	}
	election, err := s.st.GetActiveElection(r.Context(), school.ID, time.Now().UTC())
	if err != nil {
		if errors.Is(err, store.ErrNoActiveElection) {
			writeData(w, http.StatusOK, map[string]any{"candidates": []any{}})
			return
		}
		writeAPIError(w, http.StatusInternalServerError, "INTERNAL", "Gagal memuat pemilihan.")
		return
	}
	candidates, err := s.st.ListCandidates(r.Context(), election.ID, true)
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "INTERNAL", "Gagal memuat pasangan calon.")
		return
	}
	out := make([]map[string]any, 0, len(candidates))
	for _, c := range candidates {
		out = append(out, map[string]any{
			"id":                 c.ID,
			"candidate_number":   c.CandidateNumber,
			"chairman_name":      c.ChairmanName,
			"vice_chairman_name": c.ViceChairmanName,
			"photo_path":         c.PhotoPath,
			"vision":             c.Vision,
			"mission":            c.Mission,
		})
	}
	writeData(w, http.StatusOK, map[string]any{"candidates": out})
}

func (s *Server) apiValidateToken(w http.ResponseWriter, r *http.Request) {
	device := s.deviceID(w, r)
	now := time.Now().UTC()
	if ok, retry := s.tokenIPLimiter.Allow(rateKeyIP(s.clientIP(r))); !ok {
		w.Header().Set("Retry-After", retrySeconds(retry))
		writeAPIError(w, http.StatusTooManyRequests, "RATE_LIMITED", "Terlalu banyak percobaan. Tunggu sebentar lalu coba lagi.")
		return
	}
	if ok, retry := s.tokenDeviceLimiter.Allow(rateKeyDevice(device)); !ok {
		w.Header().Set("Retry-After", retrySeconds(retry))
		writeAPIError(w, http.StatusTooManyRequests, "RATE_LIMITED", "Terlalu banyak percobaan. Tunggu sebentar lalu coba lagi.")
		return
	}

	var input struct {
		Token string `json:"token"`
	}
	if err := decodeJSON(w, r, &input, 512); err != nil {
		writeAPIError(w, http.StatusBadRequest, "VALIDATION", "Masukkan token 5 karakter.")
		return
	}
	token, valid := security.NormalizeToken(input.Token)
	if !valid {
		writeAPIError(w, http.StatusUnauthorized, "TOKEN_INVALID", "Token yang digunakan salah, coba lagi.")
		return
	}

	school, err := s.st.GetDefaultSchool(r.Context())
	if err != nil {
		writeAPIError(w, http.StatusServiceUnavailable, "NOT_READY", "Aplikasi belum siap.")
		return
	}
	_, err = s.st.GetActiveElection(r.Context(), school.ID, now)
	if err != nil {
		if errors.Is(err, store.ErrNoActiveElection) {
			writeAPIError(w, http.StatusConflict, "ELECTION_CLOSED", "Pemilihan belum dibuka atau sudah ditutup.")
			return
		}
		writeAPIError(w, http.StatusInternalServerError, "INTERNAL", "Gagal memeriksa pemilihan.")
		return
	}

	if old, err := r.Cookie(studentCookieName); err == nil && old.Value != "" {
		_ = s.st.CancelVotingSession(r.Context(), security.HashSession(old.Value))
	}
	sessionRaw, err := security.RandomURLToken(32)
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "INTERNAL", "Gagal membuat sesi memilih.")
		return
	}
	csrf, err := security.RandomHex(32)
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "INTERNAL", "Gagal membuat sesi memilih.")
		return
	}
	ttl := 10 * time.Minute
	sess, _, err := s.st.CreateVotingSession(r.Context(), security.HashToken(s.cfg.TokenSecret, token), security.HashSession(sessionRaw), csrf, ttl, now)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	s.setStudentCookie(w, sessionRaw, time.Until(sess.ExpiresAt)+15*time.Second)
	writeData(w, http.StatusOK, map[string]any{"redirect": "/vote/ballot"})
}

func (s *Server) apiSubmitVote(w http.ResponseWriter, r *http.Request) {
	device := s.deviceID(w, r)
	if ok, retry := s.submitIPLimiter.Allow(rateKeyIP(s.clientIP(r))); !ok {
		w.Header().Set("Retry-After", retrySeconds(retry))
		writeAPIError(w, http.StatusTooManyRequests, "RATE_LIMITED", "Terlalu banyak percobaan. Tunggu sebentar lalu coba lagi.")
		return
	}
	if ok, retry := s.submitDeviceLimiter.Allow(rateKeyDevice(device)); !ok {
		w.Header().Set("Retry-After", retrySeconds(retry))
		writeAPIError(w, http.StatusTooManyRequests, "RATE_LIMITED", "Terlalu banyak percobaan. Tunggu sebentar lalu coba lagi.")
		return
	}

	cookie, err := r.Cookie(studentCookieName)
	if err != nil || cookie.Value == "" {
		writeAPIError(w, http.StatusUnauthorized, "SESSION_INVALID", "Sesi memilih sudah berakhir. Silakan masukkan token kembali.")
		return
	}
	hash := security.HashSession(cookie.Value)
	now := time.Now().UTC()
	sess, _, err := s.st.GetVotingSession(r.Context(), hash, now)
	if err != nil {
		s.clearCookie(w, studentCookieName)
		writeStoreError(w, err)
		return
	}
	if !security.ConstantTimeEqual(r.Header.Get("X-CSRF-Token"), sess.CSRF) {
		writeAPIError(w, http.StatusForbidden, "CSRF", "Permintaan tidak dapat diverifikasi. Muat ulang halaman.")
		return
	}

	var input struct {
		CandidateID int64 `json:"candidate_id"`
	}
	if err := decodeJSON(w, r, &input, 512); err != nil || input.CandidateID <= 0 {
		writeAPIError(w, http.StatusBadRequest, "VALIDATION", "Pilih satu pasangan calon terlebih dahulu.")
		return
	}

	electionID, err := s.st.SubmitVote(r.Context(), hash, input.CandidateID, now)
	if err != nil {
		if errors.Is(err, store.ErrSessionInvalid) || errors.Is(err, store.ErrElectionClosed) {
			s.clearCookie(w, studentCookieName)
		}
		writeStoreError(w, err)
		return
	}
	s.clearCookie(w, studentCookieName)
	s.broadcastLive(r.Context(), electionID)
	writeData(w, http.StatusOK, map[string]any{"redirect": "/?status=success"})
}

func (s *Server) apiCancelVote(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie(studentCookieName)
	if err != nil || cookie.Value == "" {
		s.clearCookie(w, studentCookieName)
		writeData(w, http.StatusOK, map[string]any{"redirect": "/token"})
		return
	}
	hash := security.HashSession(cookie.Value)
	if sess, _, err := s.st.GetVotingSession(r.Context(), hash, time.Now().UTC()); err == nil {
		if !security.ConstantTimeEqual(r.Header.Get("X-CSRF-Token"), sess.CSRF) {
			writeAPIError(w, http.StatusForbidden, "CSRF", "Permintaan tidak dapat diverifikasi.")
			return
		}
	}
	_ = s.st.CancelVotingSession(r.Context(), hash)
	s.clearCookie(w, studentCookieName)
	writeData(w, http.StatusOK, map[string]any{"redirect": "/token"})
}

func retrySeconds(d time.Duration) string {
	seconds := int(d.Seconds())
	if seconds < 1 {
		seconds = 1
	}
	return strconv.Itoa(seconds)
}
