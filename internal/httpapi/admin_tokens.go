package httpapi

import (
	"encoding/csv"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

func (s *Server) apiAdminTokensGenerate(w http.ResponseWriter, r *http.Request) {
	var input struct {
		ElectionID int64  `json:"election_id"`
		Count      int    `json:"count"`
		ExpiresAt  string `json:"expires_at"`
	}
	if err := decodeJSON(w, r, &input, 1024); err != nil {
		writeAPIError(w, http.StatusBadRequest, "VALIDATION", "Permintaan generate token tidak valid.")
		return
	}
	election, err := s.selectedElectionForAPI(r, input.ElectionID)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	if input.Count < 1 || input.Count > 1000 {
		writeAPIError(w, http.StatusBadRequest, "VALIDATION", "Jumlah token harus antara 1 dan 1000.")
		return
	}
	var expiresAt *time.Time
	if strings.TrimSpace(input.ExpiresAt) != "" {
		t, err := parseFlexibleTime(input.ExpiresAt, s.cfg.Timezone)
		if err != nil {
			writeAPIError(w, http.StatusBadRequest, "VALIDATION", "Waktu kedaluwarsa token tidak valid.")
			return
		}
		if !t.After(time.Now().UTC()) {
			writeAPIError(w, http.StatusBadRequest, "VALIDATION", "Waktu kedaluwarsa token harus di masa depan.")
			return
		}
		expiresAt = &t
	}
	tokens, err := s.st.GenerateTokens(r.Context(), election.ID, input.Count, expiresAt, s.cfg.TokenSecret)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	effectiveExpiry := election.EndAt
	if expiresAt != nil {
		effectiveExpiry = *expiresAt
	}
	s.audit(r, "TOKEN_GENERATE", "election", &election.ID, map[string]any{
		"count":      len(tokens),
		"expires_at": effectiveExpiry,
	})
	writeData(w, http.StatusCreated, map[string]any{
		"tokens":        tokens,
		"count":         len(tokens),
		"expires_at":    effectiveExpiry,
		"election_id":   election.ID,
		"election_name": election.Name,
		"period":        election.Period,
		"warning":       "Token asli hanya ditampilkan sekali. Simpan atau cetak sekarang.",
	})
}

func (s *Server) apiAdminTokensList(w http.ResponseWriter, r *http.Request) {
	election, err := s.selectedElectionForAPI(r, 0)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	now := time.Now().UTC()
	stats, err := s.st.TokenStats(r.Context(), election.ID, now)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	page, err := s.st.ListTokens(r.Context(), election.ID, r.URL.Query().Get("status"), parsePage(r), 25, now)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeData(w, http.StatusOK, map[string]any{"stats": stats, "tokens": page})
}

func (s *Server) apiAdminTokenStats(w http.ResponseWriter, r *http.Request) {
	election, err := s.selectedElectionForAPI(r, 0)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	stats, err := s.st.TokenStats(r.Context(), election.ID, time.Now().UTC())
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeData(w, http.StatusOK, stats)
}

func (s *Server) apiAdminTokensRevoke(w http.ResponseWriter, r *http.Request) {
	var input struct {
		ElectionID int64   `json:"election_id"`
		IDs        []int64 `json:"ids"`
		All        bool    `json:"all"`
	}
	if err := decodeJSON(w, r, &input, 64*1024); err != nil {
		writeAPIError(w, http.StatusBadRequest, "VALIDATION", "Permintaan tidak valid.")
		return
	}
	election, err := s.selectedElectionForAPI(r, input.ElectionID)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	var changed int64
	if input.All {
		changed, err = s.st.RevokeAllUnused(r.Context(), election.ID)
	} else {
		changed, err = s.st.RevokeTokens(r.Context(), election.ID, input.IDs, time.Now().UTC())
	}
	if err != nil {
		writeStoreError(w, err)
		return
	}
	s.audit(r, "TOKEN_REVOKE", "election", &election.ID, map[string]any{
		"count": changed,
		"all":   input.All,
	})
	writeData(w, http.StatusOK, map[string]any{"revoked": changed})
}

func (s *Server) apiAdminTokensExport(w http.ResponseWriter, r *http.Request) {
	election, err := s.selectedElectionForAPI(r, 0)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	rows, err := s.st.TokenExportRows(r.Context(), election.ID, time.Now().UTC())
	if err != nil {
		writeStoreError(w, err)
		return
	}
	s.audit(r, "TOKEN_EXPORT", "election", &election.ID, map[string]any{"count": len(rows)})
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="token-metadata-%d-%s.csv"`, election.ID, time.Now().In(s.cfg.Timezone).Format("20060102")))
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write([]byte{0xEF, 0xBB, 0xBF})
	cw := csv.NewWriter(w)
	_ = cw.Write([]string{"ID Token", "Status", "Kedaluwarsa", "Dipakai Pada", "Dibuat Pada"})
	format := func(t *time.Time) string {
		if t == nil {
			return ""
		}
		return t.In(s.cfg.Timezone).Format("2006-01-02 15:04")
	}
	for _, token := range rows {
		_ = cw.Write([]string{
			strconv.FormatInt(token.ID, 10),
			string(token.Status),
			format(token.ExpiresAt),
			format(token.UsedAt),
			token.CreatedAt.In(s.cfg.Timezone).Format("2006-01-02 15:04"),
		})
	}
	cw.Flush()
}
