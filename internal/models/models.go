package models

import (
	"encoding/json"
	"time"
)

const (
	ThemeBatik     = "batik"
	ThemeIndonesia = "indonesia"
	ThemeWayang    = "wayang"
	ThemeGlass     = "glass"
)

var Themes = []string{ThemeBatik, ThemeIndonesia, ThemeWayang, ThemeGlass}

func ValidTheme(v string) bool {
	for _, t := range Themes {
		if t == v {
			return true
		}
	}
	return false
}

func ThemeLabel(v string) string {
	switch v {
	case ThemeBatik:
		return "Batik"
	case ThemeIndonesia:
		return "Indonesia"
	case ThemeWayang:
		return "Wayang"
	case ThemeGlass:
		return "Modern Glass"
	default:
		return v
	}
}

type School struct {
	ID             int64     `json:"id"`
	Name           string    `json:"name"`
	Slug           string    `json:"slug"`
	LogoPath       string    `json:"logo_path"`
	Address        string    `json:"address"`
	Email          string    `json:"email"`
	Phone          string    `json:"phone"`
	Website        string    `json:"website"`
	Theme          string    `json:"theme"`
	PrimaryColor   string    `json:"primary_color"`
	SecondaryColor string    `json:"secondary_color"`
	FooterText     string    `json:"footer_text"`
	Tagline        string    `json:"tagline"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`

	SettingsJSON json.RawMessage `json:"-"`
}

type Admin struct {
	ID           int64      `json:"id"`
	SchoolID     int64      `json:"school_id"`
	Name         string     `json:"name"`
	Username     string     `json:"username"`
	Email        string     `json:"email"`
	Role         string     `json:"role"`
	IsActive     bool       `json:"is_active"`
	LastLoginAt  *time.Time `json:"last_login_at"`
	PasswordHash string     `json:"-"`
	CreatedAt    time.Time  `json:"created_at"`
}

type Session struct {
	ID        int64
	AdminID   *int64
	Admin     *Admin
	CSRF      string
	ExpiresAt time.Time
}

type ElectionStatus string

const (
	ElectionDraft     ElectionStatus = "DRAFT"
	ElectionScheduled ElectionStatus = "SCHEDULED"
	ElectionActive    ElectionStatus = "ACTIVE"
	ElectionPaused    ElectionStatus = "PAUSED"
	ElectionClosed    ElectionStatus = "CLOSED"
	ElectionArchived  ElectionStatus = "ARCHIVED"
)

type Election struct {
	ID                int64          `json:"id"`
	SchoolID          int64          `json:"school_id"`
	Name              string         `json:"name"`
	Period            string         `json:"period"`
	Description       string         `json:"description"`
	StartAt           time.Time      `json:"start_at"`
	EndAt             time.Time      `json:"end_at"`
	Status            ElectionStatus `json:"status"`
	ResultsVisibility string         `json:"results_visibility"`
	CandidateCount    int            `json:"candidate_count"`
	CreatedAt         time.Time      `json:"created_at"`
	UpdatedAt         time.Time      `json:"updated_at"`
}

func (e Election) StatusLabel() string {
	switch e.Status {
	case ElectionDraft:
		return "Draft"
	case ElectionScheduled:
		return "Terjadwal"
	case ElectionActive:
		return "Berlangsung"
	case ElectionPaused:
		return "Dijeda"
	case ElectionClosed:
		return "Ditutup"
	case ElectionArchived:
		return "Diarsipkan"
	default:
		return string(e.Status)
	}
}

func (e Election) IsOpen(now time.Time) bool {
	return e.Status == ElectionActive && !now.Before(e.StartAt) && now.Before(e.EndAt)
}

func (e Election) IsEditable() bool {
	return e.Status == ElectionDraft || e.Status == ElectionScheduled
}

type Candidate struct {
	ID               int64     `json:"id"`
	ElectionID       int64     `json:"election_id"`
	CandidateNumber  int       `json:"candidate_number"`
	ChairmanName     string    `json:"chairman_name"`
	ViceChairmanName string    `json:"vice_chairman_name"`
	PhotoPath        string    `json:"photo_path"`
	Vision           string    `json:"vision"`
	Mission          string    `json:"mission"`
	IsActive         bool      `json:"is_active"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

type TokenStatus string

const (
	TokenUnused  TokenStatus = "UNUSED"
	TokenUsed    TokenStatus = "USED"
	TokenRevoked TokenStatus = "REVOKED"
	TokenExpired TokenStatus = "EXPIRED"
)

type Token struct {
	ID         int64       `json:"id"`
	ElectionID int64       `json:"election_id"`
	Status     TokenStatus `json:"status"`
	ExpiresAt  *time.Time  `json:"expires_at"`
	UsedAt     *time.Time  `json:"used_at"`
	CreatedAt  time.Time   `json:"created_at"`
}

type TokenStats struct {
	Total     int64 `json:"total"`
	Available int64 `json:"available"`
	Used      int64 `json:"used"`
	Revoked   int64 `json:"revoked"`
	Expired   int64 `json:"expired"`
}

type TokenPage struct {
	Tokens  []Token `json:"tokens"`
	Page    int     `json:"page"`
	Pages   int     `json:"pages"`
	Total   int64   `json:"total"`
	PerPage int     `json:"per_page"`
}

type DashboardStats struct {
	TokenStats
	Votes          int64   `json:"votes"`
	CandidateCount int     `json:"candidate_count"`
	Participation  float64 `json:"participation"`
}

type ResultCandidate struct {
	CandidateID      int64   `json:"candidate_id"`
	CandidateNumber  int     `json:"candidate_number"`
	ChairmanName     string  `json:"chairman_name"`
	ViceChairmanName string  `json:"vice_chairman_name"`
	PhotoPath        string  `json:"photo_path"`
	Votes            int64   `json:"votes"`
	Percentage       float64 `json:"percentage"`
	Rank             int     `json:"rank"`
	IsActive         bool    `json:"is_active"`
}

type ElectionResults struct {
	Election   Election          `json:"election"`
	Candidates []ResultCandidate `json:"candidates"`
	TotalVotes int64             `json:"total_votes"`
	UpdatedAt  time.Time         `json:"updated_at"`
}

type AuditLog struct {
	ID         int64           `json:"id"`
	AdminID    *int64          `json:"admin_id"`
	AdminName  string          `json:"admin_name"`
	Action     string          `json:"action"`
	EntityType string          `json:"entity_type"`
	EntityID   *int64          `json:"entity_id"`
	Metadata   json.RawMessage `json:"metadata"`
	CreatedAt  time.Time       `json:"created_at"`
}

type VotingSession struct {
	ID         int64
	ElectionID int64
	TokenID    *int64
	CSRF       string
	ExpiresAt  time.Time
	ConsumedAt *time.Time
	CreatedAt  time.Time
}

type ElectionInput struct {
	Name              string
	Period            string
	Description       string
	StartAt           time.Time
	EndAt             time.Time
	ResultsVisibility string
}

type CandidateInput struct {
	CandidateNumber  int
	ChairmanName     string
	ViceChairmanName string
	PhotoPath        string
	Vision           string
	Mission          string
	IsActive         bool
}

type SchoolInput struct {
	Name           string
	LogoPath       string
	Address        string
	Email          string
	Phone          string
	Website        string
	Theme          string
	PrimaryColor   string
	SecondaryColor string
	FooterText     string
	Tagline        string
}
