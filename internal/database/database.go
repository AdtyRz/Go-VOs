package database

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"
	"log/slog"
	"sort"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

func Open(ctx context.Context, dsn string) (*sql.DB, error) {
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, fmt.Errorf("buka database: %w", err)
	}
	db.SetMaxOpenConns(30)
	db.SetMaxIdleConns(10)
	db.SetConnMaxLifetime(30 * time.Minute)
	db.SetConnMaxIdleTime(5 * time.Minute)

	var pingErr error
	for attempt := 1; attempt <= 20; attempt++ {
		pingCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
		pingErr = db.PingContext(pingCtx)
		cancel()
		if pingErr == nil {
			return db, nil
		}
		if ctx.Err() != nil {
			break
		}
		slog.Warn("database belum siap", "attempt", attempt, "error", pingErr)
		select {
		case <-ctx.Done():
		case <-time.After(2 * time.Second):
		}
	}
	db.Close()
	return nil, fmt.Errorf("koneksi database gagal: %w", pingErr)
}

func Migrate(ctx context.Context, db *sql.DB, files fs.FS) error {
	if _, err := db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version VARCHAR(180) NOT NULL,
			applied_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			PRIMARY KEY (version)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`); err != nil {
		return fmt.Errorf("buat schema_migrations: %w", err)
	}

	entries, err := fs.ReadDir(files, ".")
	if err != nil {
		return err
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".sql") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)

	for _, name := range names {
		var exists int
		err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM schema_migrations WHERE version = ?`, name).Scan(&exists)
		if err != nil {
			return err
		}
		if exists > 0 {
			continue
		}
		body, err := fs.ReadFile(files, name)
		if err != nil {
			return err
		}
		for i, stmt := range splitSQL(string(body)) {
			if _, err := db.ExecContext(ctx, stmt); err != nil {
				return fmt.Errorf("migrasi %s statement %d: %w", name, i+1, err)
			}
		}
		if _, err := db.ExecContext(ctx, `INSERT INTO schema_migrations (version) VALUES (?)`, name); err != nil {
			return err
		}
		slog.Info("migrasi diterapkan", "file", name)
	}
	return nil
}

func splitSQL(input string) []string {
	var out []string
	var b strings.Builder
	var quote rune
	lineStart := true
	runes := []rune(input)

	for i := 0; i < len(runes); i++ {
		r := runes[i]
		if lineStart && quote == 0 {

			if r == ' ' || r == '\t' || r == '\r' {
				continue
			}
			if r == '-' && i+1 < len(runes) && runes[i+1] == '-' {
				for i < len(runes) && runes[i] != '\n' {
					i++
				}
				lineStart = true
				continue
			}
			lineStart = false
		}

		switch {
		case quote != 0:
			b.WriteRune(r)
			if r == quote {

				if i+1 < len(runes) && runes[i+1] == quote {
					b.WriteRune(runes[i+1])
					i++
				} else {
					quote = 0
				}
			}
		case r == '\'' || r == '"' || r == '`':
			quote = r
			b.WriteRune(r)
		case r == ';':
			stmt := strings.TrimSpace(b.String())
			if stmt != "" {
				out = append(out, stmt)
			}
			b.Reset()
			lineStart = true
		default:
			b.WriteRune(r)
			lineStart = r == '\n'
		}
	}
	if stmt := strings.TrimSpace(b.String()); stmt != "" {
		out = append(out, stmt)
	}
	return out
}
