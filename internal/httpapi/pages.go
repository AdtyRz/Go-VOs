package httpapi

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"govoting/internal/models"
	"govoting/internal/store"
)

type BasePage struct {
	Title       string
	Page        string
	BodyClass   string
	School      *models.School
	Election    *models.Election
	CSRF        string
	Scripts     []string
	BaseURL     string
	CurrentYear int
	Flash       string
	FlashKind   string
}

type AdminBase struct {
	BasePage
	Admin       *models.Admin
	Elections   []models.Election
	ElectionErr string
}

func (s *Server) siteBase(ctx context.Context, title, page string, scripts ...string) (*BasePage, error) {
	school, err := s.st.GetDefaultSchool(ctx)
	if err != nil {
		return nil, err
	}
	return &BasePage{
		Title:       title,
		Page:        page,
		School:      school,
		Scripts:     scripts,
		BaseURL:     s.cfg.BaseURL,
		CurrentYear: time.Now().In(s.cfg.Timezone).Year(),
	}, nil
}

func (s *Server) adminBase(r *http.Request, title, page string, scripts ...string) (*AdminBase, error) {
	sess := sessionFrom(r)
	if sess == nil || sess.Admin == nil {
		return nil, fmt.Errorf("no admin session")
	}
	school, err := s.st.GetSchool(r.Context(), sess.Admin.SchoolID)
	if err != nil {
		return nil, err
	}
	elections, err := s.st.ListElections(r.Context(), school.ID)
	if err != nil {
		return nil, err
	}
	base := &AdminBase{
		BasePage: BasePage{
			Title:       title,
			Page:        page,
			School:      school,
			CSRF:        sess.CSRF,
			Scripts:     scripts,
			BaseURL:     s.cfg.BaseURL,
			CurrentYear: time.Now().In(s.cfg.Timezone).Year(),
		},
		Admin:     sess.Admin,
		Elections: elections,
	}

	selected, err := s.selectElection(r, school.ID, elections)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			base.ElectionErr = "Pemilihan tidak ditemukan."
			return base, nil
		}
		return nil, err
	}
	base.Election = selected
	return base, nil
}

func (s *Server) selectElection(r *http.Request, schoolID int64, elections []models.Election) (*models.Election, error) {
	raw := r.URL.Query().Get("election_id")
	if raw != "" {
		id, ok := parseID(raw)
		if !ok {
			return nil, store.ErrNotFound
		}
		return s.st.GetElection(r.Context(), schoolID, id)
	}
	preferred, err := s.st.GetPreferredElection(r.Context(), schoolID, time.Now().UTC())
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return preferred, nil
}

func (s *Server) renderPage(w http.ResponseWriter, status int, page string, data any) {
	s.renderer.Render(w, status, page, data)
}

func (s *Server) renderUserError(w http.ResponseWriter, r *http.Request, status int, title, message string) {
	base, err := s.siteBase(r.Context(), title, "error")
	if err != nil {
		http.Error(w, message, status)
		return
	}
	s.renderer.Render(w, status, "error", struct {
		BasePage
		Message string
	}{BasePage: *base, Message: message})
}

func (s *Server) themeCSS(w http.ResponseWriter, r *http.Request) {
	school, err := s.st.GetDefaultSchool(r.Context())
	if err != nil {
		http.Error(w, "theme unavailable", http.StatusInternalServerError)
		return
	}
	primary := validHexColor(school.PrimaryColor, "#7a4f2a")
	secondary := validHexColor(school.SecondaryColor, "#c9a227")
	etag := `"` + strings.TrimPrefix(primary, "#") + strings.TrimPrefix(secondary, "#") + `"`
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	w.Header().Set("Content-Type", "text/css; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("ETag", etag)
	fmt.Fprintf(w, ":root{--brand-primary:%s;--brand-secondary:%s;}\n", primary, secondary)
}

func (s *Server) audit(r *http.Request, action, entityType string, entityID *int64, metadata any) {
	sess := sessionFrom(r)
	if sess == nil || sess.Admin == nil {
		return
	}
	adminID := sess.Admin.ID
	if err := s.st.RecordAudit(r.Context(), sess.Admin.SchoolID, &adminID, action, entityType, entityID, metadata); err != nil {
		slog.Error("tulis audit log", "action", action, "error", err)
	}
}
