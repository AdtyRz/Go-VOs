package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"govoting/internal/config"
	"govoting/internal/ws"
	"govoting/web"
)

func newTestServer(t *testing.T) *Server {
	t.Helper()
	cfg := &config.Config{
		AppEnv:       "test",
		BaseURL:      "http://localhost:8080",
		Timezone:     time.UTC,
		TokenSecret:  []byte("0123456789abcdef0123456789abcdef"),
		CookieSecure: false,
	}
	server, err := New(cfg, nil, ws.NewHub(), web.FS)
	if err != nil {
		t.Fatalf("buat server uji: %v", err)
	}
	return server
}

func TestSecurityHeadersOnEveryResponse(t *testing.T) {
	server := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/me", nil)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, mau 401", rec.Code)
	}
	headers := map[string]string{
		"X-Content-Type-Options":     "nosniff",
		"X-Frame-Options":            "DENY",
		"Referrer-Policy":            "strict-origin-when-cross-origin",
		"Cross-Origin-Opener-Policy": "same-origin",
	}
	for name, want := range headers {
		if got := rec.Header().Get(name); got != want {
			t.Fatalf("%s = %q, mau %q", name, got, want)
		}
	}
	csp := rec.Header().Get("Content-Security-Policy")
	if csp == "" {
		t.Fatal("Content-Security-Policy tidak ada")
	}
	for _, directive := range []string{"default-src 'self'", "script-src 'self'", "object-src 'none'", "frame-ancestors 'none'"} {
		if !containsDirective(csp, directive) {
			t.Fatalf("CSP tidak memuat %q: %s", directive, csp)
		}
	}
}

func TestAdminAPIReturnsJSONWhenUnauthenticated(t *testing.T) {
	server := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/dashboard", nil)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, mau 401", rec.Code)
	}
	var payload struct {
		OK    bool `json:"ok"`
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("respons bukan JSON: %v", err)
	}
	if payload.OK || payload.Error.Code != "UNAUTHORIZED" {
		t.Fatalf("respons tidak sesuai: %+v", payload)
	}
}

func TestAdminPageRedirectsToLogin(t *testing.T) {
	server := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/admin", nil)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, mau 303", rec.Code)
	}
	if location := rec.Header().Get("Location"); location != "/admin/login" {
		t.Fatalf("Location = %q, mau /admin/login", location)
	}
}

func TestCrossSiteWriteIsRejected(t *testing.T) {
	server := newTestServer(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/voting/validate-token", nil)
	req.Header.Set("Sec-Fetch-Site", "cross-site")
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, mau 403", rec.Code)
	}
	var payload struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("respons bukan JSON: %v", err)
	}
	if payload.Error.Code != "ORIGIN" {
		t.Fatalf("kode galat = %q, mau ORIGIN", payload.Error.Code)
	}
}

func TestStaticAndFaviconRoutes(t *testing.T) {
	server := newTestServer(t)
	handler := server.Handler()

	req := httptest.NewRequest(http.MethodGet, "/static/css/base.css", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("CSS statis status = %d, mau 200", rec.Code)
	}
	if cache := rec.Header().Get("Cache-Control"); cache == "" {
		t.Fatal("CSS statis tidak memiliki Cache-Control")
	}

	req = httptest.NewRequest(http.MethodGet, "/static/tidak-ada.css", nil)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("aset hilang status = %d, mau 404", rec.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/favicon.ico", nil)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusMovedPermanently {
		t.Fatalf("favicon status = %d, mau 301", rec.Code)
	}
}

func TestSessionCookiesAreHardened(t *testing.T) {
	server := newTestServer(t)
	rec := httptest.NewRecorder()
	server.setAdminCookie(rec, "sesi-admin", time.Hour)
	server.setStudentCookie(rec, "sesi-siswa", time.Hour)
	cookies := rec.Result().Cookies()
	if len(cookies) != 2 {
		t.Fatalf("jumlah cookie = %d, mau 2", len(cookies))
	}
	admin, student := cookies[0], cookies[1]
	if admin.Name != adminCookieName || !admin.HttpOnly || admin.SameSite != http.SameSiteLaxMode || admin.Path != "/" {
		t.Fatalf("cookie admin tidak aman: %+v", admin)
	}
	if student.Name != studentCookieName || !student.HttpOnly || student.SameSite != http.SameSiteStrictMode || student.Path != "/" {
		t.Fatalf("cookie siswa tidak aman: %+v", student)
	}
}

func containsDirective(policy, want string) bool {
	for _, part := range splitPolicy(policy) {
		if part == want {
			return true
		}
	}
	return false
}

func splitPolicy(policy string) []string {
	var parts []string
	current := ""
	for _, r := range policy {
		if r == ';' {
			parts = append(parts, trimSpace(current))
			current = ""
			continue
		}
		current += string(r)
	}
	if current != "" {
		parts = append(parts, trimSpace(current))
	}
	return parts
}

func trimSpace(s string) string {
	start, end := 0, len(s)
	for start < end && (s[start] == ' ' || s[start] == '\t') {
		start++
	}
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t') {
		end--
	}
	return s[start:end]
}
