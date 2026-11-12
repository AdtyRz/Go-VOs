package httpapi

import (
	"log/slog"
	"net/http"
	"time"

	"govoting/internal/models"
)

type dashboardPageData struct {
	AdminBase
	Stats   models.DashboardStats
	Results models.ElectionResults
	Recent  []models.AuditLog
}

func (s *Server) adminDashboardPage(w http.ResponseWriter, r *http.Request) {
	base, err := s.adminBase(r, "Ringkasan Pemilihan", "dashboard", "dashboard.js")
	if err != nil {
		http.Error(w, "Gagal memuat dashboard.", http.StatusInternalServerError)
		return
	}
	data := dashboardPageData{AdminBase: *base}
	if base.Election != nil {
		stats, err := s.st.DashboardStats(r.Context(), base.Election.ID, time.Now().UTC())
		if err != nil {
			slog.Error("dashboard stats", "error", err)
		} else {
			data.Stats = *stats
		}
		results, err := s.st.GetResults(r.Context(), base.Election)
		if err != nil {
			slog.Error("dashboard results", "error", err)
		} else {
			data.Results = *results
		}
	}
	recent, _, err := s.st.ListAudit(r.Context(), base.School.ID, 1, 6)
	if err == nil {
		data.Recent = recent
	}
	s.renderer.Render(w, http.StatusOK, "admin/dashboard", data)
}

type candidatesPageData struct {
	AdminBase
	Candidates []models.Candidate
	CanEdit    bool
}

func (s *Server) adminCandidatesPage(w http.ResponseWriter, r *http.Request) {
	base, err := s.adminBase(r, "Pasangan Calon", "candidates", "candidates.js")
	if err != nil {
		http.Error(w, "Gagal memuat data.", http.StatusInternalServerError)
		return
	}
	data := candidatesPageData{AdminBase: *base}
	if base.Election != nil {
		data.CanEdit = base.Election.IsEditable()
		candidates, err := s.st.ListCandidates(r.Context(), base.Election.ID, false)
		if err != nil {
			http.Error(w, "Gagal memuat pasangan calon.", http.StatusInternalServerError)
			return
		}
		data.Candidates = candidates
	}
	s.renderer.Render(w, http.StatusOK, "admin/candidates", data)
}

type tokensPageData struct {
	AdminBase
	Stats   models.TokenStats
	Tokens  *models.TokenPage
	Filter  string
	PageNum int
}

func (s *Server) adminTokensPage(w http.ResponseWriter, r *http.Request) {
	base, err := s.adminBase(r, "Token Pemilih", "tokens", "tokens.js")
	if err != nil {
		http.Error(w, "Gagal memuat data.", http.StatusInternalServerError)
		return
	}
	data := tokensPageData{AdminBase: *base, Filter: r.URL.Query().Get("status"), PageNum: parsePage(r)}
	if base.Election != nil {
		now := time.Now().UTC()
		stats, err := s.st.TokenStats(r.Context(), base.Election.ID, now)
		if err != nil {
			http.Error(w, "Gagal memuat statistik token.", http.StatusInternalServerError)
			return
		}
		data.Stats = *stats
		page, err := s.st.ListTokens(r.Context(), base.Election.ID, data.Filter, data.PageNum, 25, now)
		if err != nil {
			http.Error(w, "Gagal memuat token.", http.StatusInternalServerError)
			return
		}
		data.Tokens = page
		data.PageNum = page.Page
	}
	s.renderer.Render(w, http.StatusOK, "admin/tokens", data)
}

type resultsPageData struct {
	AdminBase
	Stats   models.DashboardStats
	Results models.ElectionResults
}

func (s *Server) adminResultsPage(w http.ResponseWriter, r *http.Request) {
	base, err := s.adminBase(r, "Hasil Pemilihan", "results", "results.js")
	if err != nil {
		http.Error(w, "Gagal memuat data.", http.StatusInternalServerError)
		return
	}
	data := resultsPageData{AdminBase: *base}
	if base.Election != nil {
		now := time.Now().UTC()
		stats, err := s.st.DashboardStats(r.Context(), base.Election.ID, now)
		if err != nil {
			http.Error(w, "Gagal memuat statistik.", http.StatusInternalServerError)
			return
		}
		results, err := s.st.GetResults(r.Context(), base.Election)
		if err != nil {
			http.Error(w, "Gagal memuat hasil.", http.StatusInternalServerError)
			return
		}
		data.Stats = *stats
		data.Results = *results
	}
	s.renderer.Render(w, http.StatusOK, "admin/results", data)
}

type electionsPageData struct {
	AdminBase
	HasActive bool
}

func (s *Server) adminElectionsPage(w http.ResponseWriter, r *http.Request) {
	base, err := s.adminBase(r, "Periode Pemilihan", "elections", "elections.js")
	if err != nil {
		http.Error(w, "Gagal memuat data.", http.StatusInternalServerError)
		return
	}
	data := electionsPageData{AdminBase: *base}
	for _, election := range base.Elections {
		if election.Status == models.ElectionActive {
			data.HasActive = true
			break
		}
	}
	s.renderer.Render(w, http.StatusOK, "admin/elections", data)
}

type schoolPageData struct {
	AdminBase
	Themes []string
}

func (s *Server) adminSchoolPage(w http.ResponseWriter, r *http.Request) {
	base, err := s.adminBase(r, "Identitas Sekolah", "school", "school.js")
	if err != nil {
		http.Error(w, "Gagal memuat data.", http.StatusInternalServerError)
		return
	}
	s.renderer.Render(w, http.StatusOK, "admin/school", schoolPageData{
		AdminBase: *base,
		Themes:    models.Themes,
	})
}

type auditPageData struct {
	AdminBase
	Logs    []models.AuditLog
	PageNum int
	Pages   int
	Total   int64
}

func (s *Server) adminAuditPage(w http.ResponseWriter, r *http.Request) {
	base, err := s.adminBase(r, "Jejak Audit", "audit")
	if err != nil {
		http.Error(w, "Gagal memuat data.", http.StatusInternalServerError)
		return
	}
	pageNum := parsePage(r)
	logs, total, err := s.st.ListAudit(r.Context(), base.School.ID, pageNum, 30)
	if err != nil {
		http.Error(w, "Gagal memuat audit log.", http.StatusInternalServerError)
		return
	}
	pages := int((total + 29) / 30)
	if pages < 1 {
		pages = 1
	}
	if pageNum > pages {
		pageNum = pages
	}
	s.renderer.Render(w, http.StatusOK, "admin/audit", auditPageData{
		AdminBase: *base,
		Logs:      logs,
		PageNum:   pageNum,
		Pages:     pages,
		Total:     total,
	})
}
