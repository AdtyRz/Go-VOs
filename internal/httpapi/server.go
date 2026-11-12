package httpapi

import (
	"context"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"govoting/internal/config"
	"govoting/internal/security"
	"govoting/internal/store"
	"govoting/internal/ws"
)

const (
	adminCookieName   = "osis_admin"
	studentCookieName = "osis_vote"
	adminSessionCtx   = "admin_session"
)

type Server struct {
	cfg      *config.Config
	st       *store.Store
	hub      *ws.Hub
	renderer *Renderer
	static   fs.FS
	files    fs.FS
	started  time.Time

	loginIPLimiter      *security.Limiter
	loginUserLimiter    *security.Limiter
	tokenIPLimiter      *security.Limiter
	tokenDeviceLimiter  *security.Limiter
	submitIPLimiter     *security.Limiter
	submitDeviceLimiter *security.Limiter
	reauthLimiter       *security.Limiter
	dummyPasswordHash   string
	argonSem            chan struct{}
}

func New(cfg *config.Config, st *store.Store, hub *ws.Hub, files fs.FS) (*Server, error) {
	staticFS, err := fs.Sub(files, "static")
	if err != nil {
		return nil, err
	}
	renderer, err := NewRenderer(files, cfg.Timezone)
	if err != nil {
		return nil, err
	}
	dummyHash, err := security.HashPassword("dummy-password-for-constant-time-login")
	if err != nil {
		return nil, err
	}
	return &Server{
		cfg:                 cfg,
		st:                  st,
		hub:                 hub,
		renderer:            renderer,
		static:              staticFS,
		files:               files,
		started:             time.Now(),
		loginIPLimiter:      security.NewLimiter(20, 15*time.Minute),
		loginUserLimiter:    security.NewLimiter(6, 15*time.Minute),
		tokenIPLimiter:      security.NewLimiter(240, 5*time.Minute),
		tokenDeviceLimiter:  security.NewLimiter(10, 5*time.Minute),
		submitIPLimiter:     security.NewLimiter(200, 10*time.Minute),
		submitDeviceLimiter: security.NewLimiter(12, 10*time.Minute),
		reauthLimiter:       security.NewLimiter(5, 10*time.Minute),
		dummyPasswordHash:   dummyHash,
		argonSem:            make(chan struct{}, 2),
	}, nil
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	mux.Handle("GET /static/", s.staticHandler())
	mux.HandleFunc("GET /theme.css", s.themeCSS)
	mux.Handle("GET /uploads/", s.uploadsHandler())
	mux.HandleFunc("GET /healthz", s.health)
	mux.HandleFunc("GET /favicon.ico", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/static/images/favicon.svg", http.StatusMovedPermanently)
	})

	mux.HandleFunc("GET /{$}", s.studentHome)
	mux.HandleFunc("GET /token", s.studentToken)
	mux.Handle("GET /vote", http.RedirectHandler("/token", http.StatusSeeOther))
	mux.HandleFunc("GET /vote/ballot", s.studentBallot)
	mux.HandleFunc("GET /vote/selesai", s.studentSuccess)

	mux.HandleFunc("GET /api/v1/public/school", s.apiPublicSchool)
	mux.HandleFunc("GET /api/v1/public/election", s.apiPublicElection)
	mux.HandleFunc("GET /api/v1/public/candidates", s.apiPublicCandidates)
	mux.HandleFunc("GET /api/v1/public/results", s.apiPublicResults)
	mux.HandleFunc("POST /api/v1/voting/validate-token", s.apiValidateToken)
	mux.HandleFunc("POST /api/v1/voting/submit", s.apiSubmitVote)
	mux.HandleFunc("POST /api/v1/voting/cancel", s.apiCancelVote)

	mux.HandleFunc("GET /ws/public/results", s.wsPublicResults)

	mux.Handle("GET /admin/login", s.adminSession(http.HandlerFunc(s.adminLoginPage)))
	mux.Handle("POST /api/v1/admin/login", s.adminSession(http.HandlerFunc(s.apiAdminLogin)))
	mux.Handle("POST /api/v1/admin/logout", s.adminSession(s.requireAdminAPI(s.csrfAPI(http.HandlerFunc(s.apiAdminLogout)))))
	mux.Handle("GET /api/v1/admin/me", s.adminSession(s.requireAdminAPI(http.HandlerFunc(s.apiAdminMe))))

	mux.Handle("GET /admin", s.adminSession(s.requireAdminPage(http.HandlerFunc(s.adminDashboardPage))))
	mux.Handle("GET /admin/candidates", s.adminSession(s.requireAdminPage(http.HandlerFunc(s.adminCandidatesPage))))
	mux.Handle("GET /admin/tokens", s.adminSession(s.requireAdminPage(http.HandlerFunc(s.adminTokensPage))))
	mux.Handle("GET /admin/results", s.adminSession(s.requireAdminPage(http.HandlerFunc(s.adminResultsPage))))
	mux.Handle("GET /admin/elections", s.adminSession(s.requireAdminPage(http.HandlerFunc(s.adminElectionsPage))))
	mux.Handle("GET /admin/school", s.adminSession(s.requireAdminPage(http.HandlerFunc(s.adminSchoolPage))))
	mux.Handle("GET /admin/audit", s.adminSession(s.requireAdminPage(http.HandlerFunc(s.adminAuditPage))))

	admin := func(handler http.HandlerFunc) http.Handler {
		return s.adminSession(s.requireAdminAPI(s.csrfAPI(handler)))
	}
	mux.Handle("GET /api/v1/admin/dashboard", admin(s.apiAdminDashboard))
	mux.Handle("GET /api/v1/admin/candidates", admin(s.apiAdminCandidates))
	mux.Handle("POST /api/v1/admin/candidates", admin(s.apiAdminCandidateCreate))
	mux.Handle("PUT /api/v1/admin/candidates/{id}", admin(s.apiAdminCandidateUpdate))
	mux.Handle("DELETE /api/v1/admin/candidates/{id}", admin(s.apiAdminCandidateDelete))
	mux.Handle("POST /api/v1/admin/tokens/generate", admin(s.apiAdminTokensGenerate))
	mux.Handle("GET /api/v1/admin/tokens", admin(s.apiAdminTokensList))
	mux.Handle("GET /api/v1/admin/tokens/statistics", admin(s.apiAdminTokenStats))
	mux.Handle("POST /api/v1/admin/tokens/revoke", admin(s.apiAdminTokensRevoke))
	mux.Handle("POST /api/v1/admin/tokens/print", admin(s.apiAdminTokensPrint))
	mux.Handle("GET /api/v1/admin/tokens/export", admin(s.apiAdminTokensExport))
	mux.Handle("GET /api/v1/admin/results", admin(s.apiAdminResults))
	mux.Handle("GET /api/v1/admin/results/statistics", admin(s.apiAdminResults))
	mux.Handle("GET /api/v1/admin/results/export", admin(s.apiAdminResultsExport))
	mux.Handle("GET /api/v1/admin/school", admin(s.apiAdminSchool))
	mux.Handle("PUT /api/v1/admin/school", admin(s.apiAdminSchoolUpdate))
	mux.Handle("POST /api/v1/admin/school/logo", admin(s.apiAdminSchoolLogo))
	mux.Handle("DELETE /api/v1/admin/school/logo", admin(s.apiAdminSchoolLogoDelete))
	mux.Handle("GET /api/v1/admin/elections", admin(s.apiAdminElections))
	mux.Handle("POST /api/v1/admin/elections", admin(s.apiAdminElectionCreate))
	mux.Handle("PUT /api/v1/admin/elections/{id}", admin(s.apiAdminElectionUpdate))
	mux.Handle("POST /api/v1/admin/elections/{id}/visibility", admin(s.apiAdminElectionVisibility))
	mux.Handle("POST /api/v1/admin/elections/{id}/schedule", admin(s.apiAdminElectionAction("schedule")))
	mux.Handle("POST /api/v1/admin/elections/{id}/start", admin(s.apiAdminElectionAction("start")))
	mux.Handle("POST /api/v1/admin/elections/{id}/pause", admin(s.apiAdminElectionAction("pause")))
	mux.Handle("POST /api/v1/admin/elections/{id}/close", admin(s.apiAdminElectionClose))
	mux.Handle("POST /api/v1/admin/elections/{id}/archive", admin(s.apiAdminElectionAction("archive")))
	mux.Handle("GET /api/v1/admin/audit", admin(s.apiAdminAudit))

	mux.Handle("GET /ws/admin/results", s.adminSession(s.requireAdminWS(http.HandlerFunc(s.wsAdminResults))))

	return s.recoverPanic(s.logRequests(s.securityHeaders(s.originCheck(mux))))
}

func (s *Server) StartScheduler(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(20 * time.Second)
		defer ticker.Stop()
		s.schedulerTick(ctx)
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.schedulerTick(ctx)
			}
		}
	}()
}

func (s *Server) schedulerTick(ctx context.Context) {
	now := time.Now().UTC()
	changed, err := s.st.AutoTransitionElections(ctx, now)
	if err != nil {
		slog.Error("scheduler pemilihan", "error", err)
	} else {
		for _, electionID := range changed {
			election, err := s.st.GetElectionByID(ctx, electionID)
			if err != nil {
				continue
			}
			action := "ELECTION_AUTO_START"
			if election.Status == "CLOSED" {
				action = "ELECTION_AUTO_CLOSE"
			}
			_ = s.st.RecordAudit(ctx, election.SchoolID, nil, action, "election", &election.ID, nil)
			s.broadcastLive(ctx, electionID)
		}
	}

	if time.Now().Unix()%40 < 20 {
		if _, err := s.st.CleanupExpiredSessions(ctx, now); err != nil {
			slog.Error("cleanup sesi admin", "error", err)
		}
		if _, err := s.st.CleanupVotingSessions(ctx, now); err != nil {
			slog.Error("cleanup sesi voting", "error", err)
		}
	}
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if err := s.st.Ping(ctx); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"ok": false, "status": "database unavailable"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "status": "ok", "uptime_seconds": int(time.Since(s.started).Seconds())})
}

func (s *Server) staticHandler() http.Handler {
	fileServer := http.StripPrefix("/static/", http.FileServer(http.FS(s.static)))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		clean := path.Clean(strings.TrimPrefix(r.URL.Path, "/static/"))
		if clean == "." || strings.HasSuffix(r.URL.Path, "/") {
			http.NotFound(w, r)
			return
		}
		if _, err := fs.Stat(s.static, clean); err != nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Cache-Control", "public, max-age=86400")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		fileServer.ServeHTTP(w, r)
	})
}

func (s *Server) uploadsHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rel := strings.TrimPrefix(r.URL.Path, "/uploads/")
		clean := path.Clean(rel)
		if clean == "." || strings.HasPrefix(clean, "..") || strings.Contains(clean, "/../") {
			http.NotFound(w, r)
			return
		}
		ext := strings.ToLower(path.Ext(clean))
		if ext != ".jpg" && ext != ".png" {
			http.NotFound(w, r)
			return
		}
		full := filepath.Join(s.cfg.UploadDir, filepath.FromSlash(clean))
		info, err := os.Stat(full)
		if err != nil || info.IsDir() {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Cache-Control", "public, max-age=604800")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Content-Disposition", "inline")
		http.ServeFile(w, r, full)
	})
}
