package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"govoting/internal/models"
	"govoting/internal/security"
	"govoting/internal/store"
)

type adminLoginData struct {
	BasePage
	Error string
}

func (s *Server) adminLoginPage(w http.ResponseWriter, r *http.Request) {
	sess := sessionFrom(r)
	if sess != nil && sess.Admin != nil {
		http.Redirect(w, r, "/admin", http.StatusSeeOther)
		return
	}
	if sess == nil {
		var err error
		sess, err = s.createAnonymousSession(w, r)
		if err != nil {
			http.Error(w, "Gagal membuat sesi login.", http.StatusInternalServerError)
			return
		}
	}
	school, err := s.st.GetDefaultSchool(r.Context())
	if err != nil {
		http.Error(w, "Profil sekolah belum tersedia.", http.StatusServiceUnavailable)
		return
	}
	s.renderer.Render(w, http.StatusOK, "admin/login", adminLoginData{
		BasePage: BasePage{
			Title:       "Masuk Panitia",
			Page:        "login",
			School:      school,
			CSRF:        sess.CSRF,
			BaseURL:     s.cfg.BaseURL,
			CurrentYear: time.Now().In(s.cfg.Timezone).Year(),
		},
	})
}

func (s *Server) createAnonymousSession(w http.ResponseWriter, r *http.Request) (*models.Session, error) {
	raw, err := security.RandomURLToken(32)
	if err != nil {
		return nil, err
	}
	csrf, err := security.RandomHex(32)
	if err != nil {
		return nil, err
	}
	ttl := time.Hour
	expires := time.Now().UTC().Add(ttl)
	if err := s.st.CreateAdminSession(r.Context(), security.HashSession(raw), nil, csrf, expires); err != nil {
		return nil, err
	}
	s.setAdminCookie(w, raw, ttl)
	return &models.Session{CSRF: csrf, ExpiresAt: expires}, nil
}

func (s *Server) apiAdminLogin(w http.ResponseWriter, r *http.Request) {
	sess := sessionFrom(r)
	if sess == nil {
		writeAPIError(w, http.StatusForbidden, "SESSION_REQUIRED", "Muat ulang halaman login lalu coba lagi.")
		return
	}
	if !security.ConstantTimeEqual(r.Header.Get("X-CSRF-Token"), sess.CSRF) {
		writeAPIError(w, http.StatusForbidden, "CSRF", "Permintaan tidak dapat diverifikasi. Muat ulang halaman.")
		return
	}
	ipKey := rateKeyIP(s.clientIP(r))
	if ok, retry := s.loginIPLimiter.Allow(ipKey); !ok {
		w.Header().Set("Retry-After", retrySeconds(retry))
		writeAPIError(w, http.StatusTooManyRequests, "RATE_LIMITED", "Terlalu banyak percobaan login. Coba lagi nanti.")
		return
	}

	var input struct {
		Login    string `json:"login"`
		Password string `json:"password"`
	}
	if err := decodeJSON(w, r, &input, 1024); err != nil {
		writeAPIError(w, http.StatusBadRequest, "VALIDATION", "Masukkan username dan password.")
		return
	}
	input.Login = strings.TrimSpace(input.Login)
	if input.Login == "" || input.Password == "" || len(input.Login) > 160 || len(input.Password) > 512 {
		writeAPIError(w, http.StatusUnauthorized, "INVALID_CREDENTIALS", "Username atau password salah.")
		return
	}

	userKey := strings.ToLower(input.Login)
	if ok, retry := s.loginUserLimiter.Allow("user:" + userKey); !ok {
		w.Header().Set("Retry-After", retrySeconds(retry))
		writeAPIError(w, http.StatusTooManyRequests, "RATE_LIMITED", "Terlalu banyak percobaan login. Coba lagi nanti.")
		return
	}

	auth, err := s.st.GetAdminAuthByLogin(r.Context(), input.Login)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		writeStoreError(w, err)
		return
	}

	hashToCheck := s.dummyPasswordHash
	var adminID int64
	exists := auth != nil
	if exists {
		hashToCheck = auth.PasswordHash
		adminID = auth.Admin.ID
	}
	valid := s.verifyPasswordLimited(r.Context(), input.Password, hashToCheck)
	now := time.Now().UTC()
	locked := exists && auth.LockedUntil != nil && auth.LockedUntil.After(now)
	inactive := exists && !auth.Admin.IsActive
	if !exists || !valid || locked || inactive {
		if exists && valid {

		} else if exists {
			maxAttempts := 5
			if err := s.st.MarkLoginFailure(r.Context(), adminID, maxAttempts, 15*time.Minute); err != nil {

			}
		}
		writeAPIError(w, http.StatusUnauthorized, "INVALID_CREDENTIALS", "Username atau password salah.")
		return
	}

	newRaw, err := security.RandomURLToken(32)
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "INTERNAL", "Gagal membuat sesi.")
		return
	}
	newCSRF, err := security.RandomHex(32)
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "INTERNAL", "Gagal membuat sesi.")
		return
	}
	expires := now.Add(s.cfg.SessionTTL)
	var oldHash []byte
	if cookie, err := r.Cookie(adminCookieName); err == nil {
		oldHash = security.HashSession(cookie.Value)
	}
	if err := s.st.ReplaceLoginSession(r.Context(), oldHash, security.HashSession(newRaw), adminID, newCSRF, expires); err != nil {
		writeStoreError(w, err)
		return
	}
	if err := s.st.MarkLoginSuccess(r.Context(), adminID); err != nil {

	}
	s.setAdminCookie(w, newRaw, s.cfg.SessionTTL)
	s.loginIPLimiter.Reset(ipKey)
	s.loginUserLimiter.Reset("user:" + userKey)

	_ = s.st.RecordAudit(r.Context(), auth.Admin.SchoolID, &adminID, "LOGIN", "admin", &adminID, nil)
	writeData(w, http.StatusOK, map[string]any{"redirect": "/admin"})
}

func (s *Server) apiAdminLogout(w http.ResponseWriter, r *http.Request) {
	sess := sessionFrom(r)
	if sess != nil && sess.Admin != nil {
		adminID := sess.Admin.ID
		_ = s.st.RecordAudit(r.Context(), sess.Admin.SchoolID, &adminID, "LOGOUT", "admin", &adminID, nil)
	}
	if cookie, err := r.Cookie(adminCookieName); err == nil {
		_ = s.st.DeleteSession(r.Context(), security.HashSession(cookie.Value))
	}
	s.clearCookie(w, adminCookieName)
	writeData(w, http.StatusOK, map[string]any{"redirect": "/admin/login"})
}

func (s *Server) apiAdminMe(w http.ResponseWriter, r *http.Request) {
	sess := sessionFrom(r)
	if sess == nil || sess.Admin == nil {
		writeAPIError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Sesi berakhir.")
		return
	}
	writeData(w, http.StatusOK, map[string]any{
		"id":         sess.Admin.ID,
		"name":       sess.Admin.Name,
		"username":   sess.Admin.Username,
		"email":      sess.Admin.Email,
		"role":       sess.Admin.Role,
		"expires_at": sess.ExpiresAt,
	})
}

func (s *Server) verifyPasswordLimited(ctx context.Context, password, hash string) bool {
	select {
	case s.argonSem <- struct{}{}:
		defer func() { <-s.argonSem }()
	case <-ctx.Done():
		return false
	case <-time.After(3 * time.Second):
		return false
	}
	ok, err := security.VerifyPassword(password, hash)
	return err == nil && ok
}
