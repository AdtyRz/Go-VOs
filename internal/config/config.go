package config

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	AppEnv       string
	Addr         string
	BaseURL      string
	Timezone     *time.Location
	DatabaseDSN  string
	TokenSecret  []byte
	CookieSecure bool
	TrustProxy   bool
	AutoMigrate  bool
	UploadDir    string
	SessionTTL   time.Duration

	SeedDemo       bool
	SchoolName     string
	SchoolSlug     string
	SchoolEmail    string
	SchoolAddress  string
	SchoolPhone    string
	SchoolWebsite  string
	AdminName      string
	AdminUsername  string
	AdminEmail     string
	AdminPassword  string
	AdminGenerated bool
}

func Load() (*Config, error) {
	loadDotEnv(".env")

	tzName := env("APP_TIMEZONE", "Asia/Jakarta")
	tz, err := time.LoadLocation(tzName)
	if err != nil {
		return nil, fmt.Errorf("APP_TIMEZONE tidak valid: %w", err)
	}

	secretHex := strings.TrimSpace(os.Getenv("TOKEN_SECRET"))
	secret, err := hex.DecodeString(secretHex)
	if err != nil || len(secret) < 32 {
		return nil, errors.New("TOKEN_SECRET wajib berupa minimal 64 karakter hex (openssl rand -hex 32)")
	}

	sessionTTL := 12 * time.Hour
	if raw := os.Getenv("SESSION_TTL"); raw != "" {
		if sessionTTL, err = time.ParseDuration(raw); err != nil {
			return nil, fmt.Errorf("SESSION_TTL tidak valid: %w", err)
		}
	}

	cfg := &Config{
		AppEnv:        env("APP_ENV", "development"),
		Addr:          env("APP_ADDR", ":8080"),
		BaseURL:       strings.TrimRight(env("APP_BASE_URL", "http://localhost:8080"), "/"),
		Timezone:      tz,
		DatabaseDSN:   os.Getenv("DB_DSN"),
		TokenSecret:   secret,
		CookieSecure:  envBool("COOKIE_SECURE", false),
		TrustProxy:    envBool("TRUST_PROXY", false),
		AutoMigrate:   envBool("AUTO_MIGRATE", true),
		UploadDir:     env("UPLOAD_DIR", "./storage/uploads"),
		SessionTTL:    sessionTTL,
		SeedDemo:      envBool("SEED_DEMO", false),
		SchoolName:    env("SCHOOL_NAME", ""),
		SchoolSlug:    env("SCHOOL_SLUG", ""),
		SchoolEmail:   env("SCHOOL_EMAIL", ""),
		SchoolAddress: env("SCHOOL_ADDRESS", ""),
		SchoolPhone:   env("SCHOOL_PHONE", ""),
		SchoolWebsite: env("SCHOOL_WEBSITE", ""),
		AdminName:     env("ADMIN_NAME", "Administrator"),
		AdminUsername: env("ADMIN_USERNAME", ""),
		AdminEmail:    env("ADMIN_EMAIL", ""),
		AdminPassword: os.Getenv("ADMIN_PASSWORD"),
	}

	if cfg.DatabaseDSN == "" {
		return nil, errors.New("DB_DSN wajib diisi")
	}
	if cfg.AppEnv == "production" && !cfg.CookieSecure {
		return nil, errors.New("COOKIE_SECURE harus true pada APP_ENV=production")
	}
	if len(cfg.SchoolName) < 2 {
		return nil, errors.New("SCHOOL_NAME wajib diisi saat inisialisasi")
	}
	if cfg.SchoolSlug == "" {
		cfg.SchoolSlug = slugify(cfg.SchoolName)
	}
	if cfg.AdminUsername == "" && cfg.AdminPassword == "" && cfg.SeedDemo {

		buf := make([]byte, 18)
		if _, err := rand.Read(buf); err != nil {
			return nil, err
		}
		cfg.AdminUsername = "admin"
		cfg.AdminPassword = hex.EncodeToString(buf)
		cfg.AdminEmail = "admin@localhost"
		cfg.AdminGenerated = true
	}
	return cfg, nil
}

func env(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

func envBool(key string, fallback bool) bool {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	v, err := strconv.ParseBool(raw)
	if err != nil {
		return fallback
	}
	return v
}

func slugify(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	dash := false
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			dash = false
		case r == ' ' || r == '-' || r == '_' || r == '.':
			if !dash && b.Len() > 0 {
				b.WriteByte('-')
				dash = true
			}
		}
	}
	return strings.Trim(b.String(), "-")
}

func loadDotEnv(path string) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if len(value) >= 2 && (value[0] == '"' && value[len(value)-1] == '"' || value[0] == '\'' && value[len(value)-1] == '\'') {
			value = value[1 : len(value)-1]
		}
		if key == "" {
			continue
		}
		if _, exists := os.LookupEnv(key); !exists {
			_ = os.Setenv(key, value)
		}
	}
}
