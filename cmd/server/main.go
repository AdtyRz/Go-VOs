package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"govoting/internal/bootstrap"
	"govoting/internal/config"
	"govoting/internal/database"
	"govoting/internal/httpapi"
	"govoting/internal/store"
	"govoting/internal/ws"
	"govoting/migrations"
	"govoting/web"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("konfigurasi tidak valid", "error", err)
		os.Exit(1)
	}
	setupLogger(cfg.AppEnv)
	slog.Info("memulai Go-Voting", "env", cfg.AppEnv, "addr", cfg.Addr)

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	db, err := database.Open(ctx, cfg.DatabaseDSN)
	if err != nil {
		slog.Error("database", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	if cfg.AutoMigrate {
		if err := database.Migrate(ctx, db, migrations.FS); err != nil {
			slog.Error("migrasi gagal", "error", err)
			os.Exit(1)
		}
	}

	st := store.New(db)
	if err := bootstrap.Seed(ctx, cfg, st); err != nil {
		slog.Error("seed gagal", "error", err)
		os.Exit(1)
	}

	hub := ws.NewHub()
	api, err := httpapi.New(cfg, st, hub, web.FS)
	if err != nil {
		slog.Error("server init", "error", err)
		os.Exit(1)
	}

	srv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           api.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    1 << 20,
		ErrorLog:          slog.NewLogLogger(slog.Default().Handler(), slog.LevelError),
	}

	api.StartScheduler(ctx)

	errCh := make(chan error, 1)
	go func() {
		slog.Info("server siap", "url", cfg.BaseURL)
		errCh <- srv.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		slog.Info("menghentikan server")
	case err := <-errCh:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("server berhenti", "error", err)
			os.Exit(1)
		}
	}

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer shutdownCancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Error("shutdown", "error", err)
	}
	hub.CloseAll()
	slog.Info("server berhenti dengan aman")
}

func setupLogger(env string) {
	level := slog.LevelInfo
	if env == "development" {
		level = slog.LevelDebug
	}
	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level})
	slog.SetDefault(slog.New(handler))
}
