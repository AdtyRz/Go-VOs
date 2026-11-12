package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"govoting/internal/models"
	"govoting/internal/store"
)

func TestPublicResultsVisible(t *testing.T) {
	now := time.Date(2026, 11, 12, 10, 0, 0, 0, time.UTC)
	cases := []struct {
		name     string
		election *models.Election
		want     bool
	}{
		{"nil", nil, false},
		{"live aktif", &models.Election{ResultsVisibility: "LIVE", Status: models.ElectionActive, EndAt: now.Add(time.Hour)}, true},
		{"live dijeda", &models.Election{ResultsVisibility: "LIVE", Status: models.ElectionPaused, EndAt: now.Add(time.Hour)}, true},
		{"live ditutup", &models.Election{ResultsVisibility: "LIVE", Status: models.ElectionClosed, EndAt: now.Add(-time.Hour)}, true},
		{"live terjadwal", &models.Election{ResultsVisibility: "LIVE", Status: models.ElectionScheduled, EndAt: now.Add(time.Hour)}, false},
		{"final aktif", &models.Election{ResultsVisibility: "FINAL", Status: models.ElectionActive, EndAt: now.Add(time.Hour)}, false},
		{"final lewat waktu", &models.Election{ResultsVisibility: "FINAL", Status: models.ElectionActive, EndAt: now.Add(-time.Minute)}, true},
		{"final ditutup", &models.Election{ResultsVisibility: "FINAL", Status: models.ElectionClosed, EndAt: now.Add(-time.Hour)}, true},
		{"final terjadwal", &models.Election{ResultsVisibility: "FINAL", Status: models.ElectionScheduled, EndAt: now.Add(time.Hour)}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := publicResultsVisible(tc.election, now); got != tc.want {
				t.Fatalf("publicResultsVisible = %v, mau %v", got, tc.want)
			}
		})
	}
}

func TestPublicResultsFinal(t *testing.T) {
	now := time.Date(2026, 11, 12, 10, 0, 0, 0, time.UTC)
	cases := []struct {
		name     string
		election *models.Election
		want     bool
	}{
		{"nil", nil, false},
		{"aktif", &models.Election{Status: models.ElectionActive, EndAt: now.Add(time.Hour)}, false},
		{"aktif lewat waktu", &models.Election{Status: models.ElectionActive, EndAt: now.Add(-time.Minute)}, true},
		{"ditutup", &models.Election{Status: models.ElectionClosed, EndAt: now.Add(time.Hour)}, true},
		{"diarsipkan", &models.Election{Status: models.ElectionArchived, EndAt: now.Add(time.Hour)}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := publicResultsFinal(tc.election, now); got != tc.want {
				t.Fatalf("publicResultsFinal = %v, mau %v", got, tc.want)
			}
		})
	}
}

func TestInvalidTokenMessage(t *testing.T) {
	rec := httptest.NewRecorder()
	writeStoreError(rec, store.ErrTokenInvalid)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, mau 401", rec.Code)
	}
	var payload struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("respons bukan JSON: %v", err)
	}
	if payload.Error.Code != "TOKEN_INVALID" {
		t.Fatalf("kode = %q, mau TOKEN_INVALID", payload.Error.Code)
	}
	if payload.Error.Message != "Token yang digunakan salah, coba lagi." {
		t.Fatalf("pesan = %q", payload.Error.Message)
	}
}
