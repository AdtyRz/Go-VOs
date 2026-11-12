package httpapi

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"govoting/internal/models"
	"govoting/internal/store"
)

func currentSession(r *http.Request) *models.Session {
	return sessionFrom(r)
}

func (s *Server) adminElection(r *http.Request, rawID string) (*models.Election, error) {
	sess := currentSession(r)
	if sess == nil || sess.Admin == nil {
		return nil, store.ErrNotFound
	}
	id, ok := parseID(rawID)
	if !ok {
		return nil, store.ErrNotFound
	}
	return s.st.GetElection(r.Context(), sess.Admin.SchoolID, id)
}

func (s *Server) electionIDFromRequest(r *http.Request, jsonFallback int64) (int64, error) {
	raw := r.URL.Query().Get("election_id")
	if raw == "" {
		raw = r.FormValue("election_id")
	}
	if raw != "" {
		id, ok := parseID(raw)
		if !ok {
			return 0, store.ErrNotFound
		}
		return id, nil
	}
	if jsonFallback > 0 {
		return jsonFallback, nil
	}
	return 0, store.ErrNotFound
}

func trimmed(value string, max int, field string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", fmt.Errorf("%w: %s wajib diisi", store.ErrInvalid, field)
	}
	if utf8.RuneCountInString(value) > max {
		return "", fmt.Errorf("%w: %s terlalu panjang (maksimal %d karakter)", store.ErrInvalid, field, max)
	}
	return value, nil
}

func trimmedOptional(value string, max int, field string) (string, error) {
	value = strings.TrimSpace(value)
	if utf8.RuneCountInString(value) > max {
		return "", fmt.Errorf("%w: %s terlalu panjang (maksimal %d karakter)", store.ErrInvalid, field, max)
	}
	return value, nil
}

func parseFlexibleTime(value string, loc *time.Location) (time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, errors.New("waktu wajib diisi")
	}
	layouts := []string{
		"2006-01-02T15:04",
		"2006-01-02T15:04:05",
		"2006-01-02 15:04",
	}
	for _, layout := range layouts {
		if t, err := time.ParseInLocation(layout, value, loc); err == nil {
			return t.UTC(), nil
		}
	}
	if t, err := time.Parse(time.RFC3339, value); err == nil {
		return t.UTC(), nil
	}
	return time.Time{}, errors.New("format waktu tidak valid")
}

func basicEmailValid(email string) bool {
	if email == "" {
		return true
	}
	if strings.ContainsAny(email, " \t\r\n") || !strings.Contains(email, "@") {
		return false
	}
	return len(email) <= 160
}

func basicWebsiteValid(url string) bool {
	if url == "" {
		return true
	}
	return (strings.HasPrefix(url, "https://") || strings.HasPrefix(url, "http://")) && len(url) <= 255
}
