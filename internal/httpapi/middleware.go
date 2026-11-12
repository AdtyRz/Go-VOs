package httpapi

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"runtime/debug"
	"strconv"
	"strings"
	"time"

	"govoting/internal/models"
	"govoting/internal/security"
	"govoting/internal/store"
)

type ctxKey int

const (
	sessionKey ctxKey = iota
)

func sessionFrom(r *http.Request) *models.Session {
	sess, _ := r.Context().Value(sessionKey).(*models.Session)
	return sess
}

func withSession(r *http.Request, sess *models.Session) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), sessionKey, sess))
}

func (s *Server) adminSession(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(adminCookieName)
		if err == nil && cookie.Value != "" {
			sess, lookupErr := s.st.GetSession(r.Context(), security.HashSession(cookie.Value), time.Now().UTC())
			if lookupErr == nil {
				s.st.TouchSession(r.Context(), sess.ID, time.Now().UTC())
				r = withSession(r, sess)
			} else {
				if !errors.Is(lookupErr, store.ErrNotFound) && !errors.Is(lookupErr, store.ErrInactive) {
					slog.Error("load sesi admin", "error", lookupErr)
				}
				s.clearCookie(w, adminCookieName)
			}
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) requireAdminPage(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sess := sessionFrom(r)
		if sess == nil || sess.Admin == nil {
			http.Redirect(w, r, "/admin/login", http.StatusSeeOther)
			return
		}
		s.clearCookieIfLegacy(w, r)
		next.ServeHTTP(w, r)
	})
}

func (s *Server) requireAdminAPI(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sess := sessionFrom(r)
		if sess == nil || sess.Admin == nil {
			writeAPIError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Sesi berakhir. Silakan login kembali.")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) requireAdminWS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sess := sessionFrom(r)
		if sess == nil || sess.Admin == nil {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) csrfAPI(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sess := sessionFrom(r)
		if sess == nil {
			writeAPIError(w, http.StatusForbidden, "CSRF", "Sesi tidak valid. Muat ulang halaman.")
			return
		}
		token := r.Header.Get("X-CSRF-Token")
		if token == "" {
			token = r.FormValue("_csrf")
		}
		if token == "" || !security.ConstantTimeEqual(token, sess.CSRF) {
			writeAPIError(w, http.StatusForbidden, "CSRF", "Permintaan tidak dapat diverifikasi. Muat ulang halaman.")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) originCheck(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
			next.ServeHTTP(w, r)
			return
		}
		if site := r.Header.Get("Sec-Fetch-Site"); site == "cross-site" {
			writeAPIError(w, http.StatusForbidden, "ORIGIN", "Permintaan lintas situs ditolak.")
			return
		}
		if origin := r.Header.Get("Origin"); origin != "" && origin != "null" {
			u, err := url.Parse(origin)
			if err != nil || !sameHost(u.Host, r.Host) {
				writeAPIError(w, http.StatusForbidden, "ORIGIN", "Asal permintaan tidak diizinkan.")
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func sameHost(a, b string) bool {
	ah, _, errA := net.SplitHostPort(a)
	bh, _, errB := net.SplitHostPort(b)
	if errA != nil {
		ah = a
	}
	if errB != nil {
		bh = b
	}
	return strings.EqualFold(ah, bh) || strings.EqualFold(a, b)
}

func (s *Server) securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		h.Set("Permissions-Policy", "camera=(), microphone=(), geolocation=(), payment=()")
		h.Set("Cross-Origin-Opener-Policy", "same-origin")
		h.Set("Content-Security-Policy",
			"default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' data:; font-src 'self'; connect-src 'self' ws: wss:; object-src 'none'; base-uri 'none'; form-action 'self'; frame-ancestors 'none'")
		if s.cfg.AppEnv == "production" && s.cfg.CookieSecure {
			h.Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}
		next.ServeHTTP(w, r)
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (r *statusRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

func (r *statusRecorder) Write(b []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	n, err := r.ResponseWriter.Write(b)
	r.bytes += n
	return n, err
}

func (r *statusRecorder) Unwrap() http.ResponseWriter {
	return r.ResponseWriter
}

func (r *statusRecorder) Flush() {
	if flusher, ok := r.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

func (r *statusRecorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	hijacker, ok := r.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, fmt.Errorf("response writer does not support hijacking")
	}
	return hijacker.Hijack()
}

func (s *Server) logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/static/") || strings.HasPrefix(r.URL.Path, "/uploads/") || r.URL.Path == "/healthz" {
			next.ServeHTTP(w, r)
			return
		}
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w}
		next.ServeHTTP(rec, r)
		status := rec.status
		if status == 0 {
			status = http.StatusOK
		}

		slog.Info("http",
			"method", r.Method,
			"path", r.URL.Path,
			"status", status,
			"duration_ms", time.Since(start).Milliseconds(),
		)
	})
}

func (s *Server) recoverPanic(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				slog.Error("panic", "error", rec, "stack", string(debug.Stack()))
				writeAPIError(w, http.StatusInternalServerError, "INTERNAL", "Terjadi kesalahan pada server.")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func (s *Server) clientIP(r *http.Request) string {
	if s.cfg.TrustProxy {
		if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
			first := strings.TrimSpace(strings.Split(forwarded, ",")[0])
			if first != "" {
				return first
			}
		}
		if realIP := strings.TrimSpace(r.Header.Get("X-Real-IP")); realIP != "" {
			return realIP
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func (s *Server) deviceID(w http.ResponseWriter, r *http.Request) string {
	if cookie, err := r.Cookie("osis_device"); err == nil && len(cookie.Value) >= 16 && len(cookie.Value) <= 80 {
		return cookie.Value
	}
	raw, err := security.RandomURLToken(18)
	if err != nil {
		return s.clientIP(r)
	}
	http.SetCookie(w, &http.Cookie{
		Name:     "osis_device",
		Value:    raw,
		Path:     "/",
		MaxAge:   365 * 24 * 60 * 60,
		HttpOnly: true,
		Secure:   s.cfg.CookieSecure,
		SameSite: http.SameSiteLaxMode,
	})
	return raw
}

func rateKeyIP(ip string) string {
	sum := sha256.Sum256([]byte(ip))
	return "ip:" + hex.EncodeToString(sum[:8])
}

func rateKeyDevice(device string) string {
	sum := sha256.Sum256([]byte(device))
	return "dev:" + hex.EncodeToString(sum[:8])
}

func (s *Server) setAdminCookie(w http.ResponseWriter, raw string, ttl time.Duration) {
	http.SetCookie(w, &http.Cookie{
		Name:     adminCookieName,
		Value:    raw,
		Path:     "/",
		MaxAge:   int(ttl.Seconds()),
		HttpOnly: true,
		Secure:   s.cfg.CookieSecure,
		SameSite: http.SameSiteLaxMode,
	})
}

func (s *Server) setStudentCookie(w http.ResponseWriter, raw string, ttl time.Duration) {
	http.SetCookie(w, &http.Cookie{
		Name:     studentCookieName,
		Value:    raw,
		Path:     "/",
		MaxAge:   int(ttl.Seconds()),
		HttpOnly: true,
		Secure:   s.cfg.CookieSecure,
		SameSite: http.SameSiteStrictMode,
	})
}

func (s *Server) clearCookie(w http.ResponseWriter, name string) {
	http.SetCookie(w, &http.Cookie{
		Name:     name,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		Expires:  time.Unix(1, 0),
		HttpOnly: true,
		Secure:   s.cfg.CookieSecure,
		SameSite: http.SameSiteLaxMode,
	})
}

func (s *Server) clearCookieIfLegacy(w http.ResponseWriter, r *http.Request) {}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	_ = enc.Encode(payload)
}

func writeData(w http.ResponseWriter, status int, data any) {
	writeJSON(w, status, map[string]any{"ok": true, "data": data})
}

func writeAPIError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{
		"ok": false,
		"error": map[string]string{
			"code":    code,
			"message": message,
		},
	})
}

func writeStoreError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, store.ErrTokenInvalid), errors.Is(err, store.ErrAlreadyUsed):
		writeAPIError(w, http.StatusUnauthorized, "TOKEN_INVALID", "Token tidak valid atau sudah digunakan. Hubungi panitia bila Anda merasa ini keliru.")
	case errors.Is(err, store.ErrSessionInvalid):
		writeAPIError(w, http.StatusUnauthorized, "SESSION_INVALID", "Sesi memilih sudah berakhir. Silakan masukkan token kembali.")
	case errors.Is(err, store.ErrElectionClosed):
		writeAPIError(w, http.StatusConflict, "ELECTION_CLOSED", "Pemilihan sedang tidak berlangsung. Suara tidak dapat dikirim.")
	case errors.Is(err, store.ErrCandidateInvalid):
		writeAPIError(w, http.StatusBadRequest, "CANDIDATE_INVALID", "Pasangan calon tidak tersedia.")
	case errors.Is(err, store.ErrNotFound):
		writeAPIError(w, http.StatusNotFound, "NOT_FOUND", "Data tidak ditemukan.")
	case errors.Is(err, store.ErrInvalid):
		writeAPIError(w, http.StatusBadRequest, "VALIDATION", cleanError(err))
	case errors.Is(err, store.ErrConflict), errors.Is(err, store.ErrVotesExist),
		errors.Is(err, store.ErrElectionLocked), errors.Is(err, store.ErrTokenRevoked):
		msg := cleanError(err)
		if errors.Is(err, store.ErrVotesExist) {
			msg = "Pasangan calon sudah memiliki suara dan tidak dapat dihapus."
		}
		if errors.Is(err, store.ErrElectionLocked) {
			msg = "Data pemilihan tidak dapat diubah pada status ini."
		}
		writeAPIError(w, http.StatusConflict, "CONFLICT", msg)
	default:
		slog.Error("API error", "error", err)
		writeAPIError(w, http.StatusInternalServerError, "INTERNAL", "Terjadi kesalahan pada server.")
	}
}

func cleanError(err error) string {
	msg := err.Error()
	for _, prefix := range []string{"data tidak valid: ", "data bertentangan: ", "token sudah dicabut: "} {
		msg = strings.TrimPrefix(msg, prefix)
	}
	return msg
}

func decodeJSON(w http.ResponseWriter, r *http.Request, dst any, maxBytes int64) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return err
	}
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		return errors.New("body hanya boleh berisi satu objek JSON")
	}
	return nil
}

func parseID(raw string) (int64, bool) {
	id, err := strconv.ParseInt(raw, 10, 64)
	return id, err == nil && id > 0
}

func parsePage(r *http.Request) int {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		return 1
	}
	return page
}

var hexColor = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

func validHexColor(v, fallback string) string {
	if hexColor.MatchString(v) {
		return strings.ToLower(v)
	}
	return fallback
}
