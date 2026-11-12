package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"govoting/internal/config"
	"govoting/internal/models"
	"govoting/internal/security"
	"govoting/internal/store"
)

func Seed(ctx context.Context, cfg *config.Config, st *store.Store) error {
	for _, sub := range []string{"candidates", "school"} {
		if err := os.MkdirAll(filepath.Join(cfg.UploadDir, sub), 0o750); err != nil {
			return fmt.Errorf("buat folder upload: %w", err)
		}
	}

	school, err := st.GetDefaultSchool(ctx)
	if errors.Is(err, store.ErrNotFound) {
		id, err := st.CreateSchool(ctx, cfg.SchoolName, cfg.SchoolSlug, cfg.SchoolEmail, cfg.SchoolAddress, cfg.SchoolPhone, cfg.SchoolWebsite)
		if err != nil {
			return fmt.Errorf("seed sekolah: %w", err)
		}
		school, err = st.GetSchool(ctx, id)
		if err != nil {
			return err
		}
		slog.Info("sekolah awal dibuat", "name", school.Name, "slug", school.Slug)
	} else if err != nil {
		return err
	}

	count, err := st.CountAdmins(ctx)
	if err != nil {
		return err
	}
	if count == 0 {
		if cfg.AdminUsername == "" || cfg.AdminPassword == "" {
			return errors.New("belum ada admin. Isi ADMIN_USERNAME dan ADMIN_PASSWORD di .env untuk membuat admin pertama")
		}
		if len(cfg.AdminPassword) < 8 {
			return errors.New("ADMIN_PASSWORD minimal 8 karakter")
		}
		hash, err := security.HashPassword(cfg.AdminPassword)
		if err != nil {
			return err
		}
		adminID, err := st.CreateAdmin(ctx, school.ID, cfg.AdminName, cfg.AdminUsername, cfg.AdminEmail, hash)
		if err != nil {
			return fmt.Errorf("seed admin: %w", err)
		}
		_ = st.RecordAudit(ctx, school.ID, nil, "ADMIN_BOOTSTRAP", "admin", &adminID, map[string]any{"username": cfg.AdminUsername})
		if cfg.AdminGenerated {
			slog.Warn("admin demo dibuat, catat sekarang karena password hanya ditampilkan sekali", "username", cfg.AdminUsername, "password", cfg.AdminPassword)
		} else {
			slog.Info("admin awal dibuat", "username", cfg.AdminUsername)
		}
	}

	if !cfg.SeedDemo {
		return nil
	}

	elections, err := st.ListElections(ctx, school.ID)
	if err != nil {
		return err
	}
	if len(elections) > 0 {
		return nil
	}

	now := time.Now().UTC()
	input := models.ElectionInput{
		Name:              "Pemilihan Ketua & Wakil Ketua OSIS",
		Period:            "2026/2027",
		Description:       "Satu suara untuk satu masa depan. Pilih pasangan yang paling mewakili suara siswa.",
		StartAt:           now.Add(-1 * time.Hour),
		EndAt:             now.AddDate(0, 0, 14),
		ResultsVisibility: "LIVE",
	}
	electionID, err := st.CreateElection(ctx, school.ID, input)
	if err != nil {
		return fmt.Errorf("seed pemilihan demo: %w", err)
	}

	candidates := []models.CandidateInput{
		{
			CandidateNumber:  1,
			ChairmanName:     "Nadia Putri Ramadhani",
			ViceChairmanName: "Raka Aditya Pratama",
			Vision:           "Mewujudkan OSIS yang responsif, terbuka, dan benar-benar dekat dengan seluruh siswa.",
			Mission:          "Membuka kanal aspirasi mingguan di setiap tingkat.\nMenjalankan program pendampingan belajar lintas kelas.\nMemublikasikan laporan kegiatan dan anggaran secara berkala.",
			IsActive:         true,
		},
		{
			CandidateNumber:  2,
			ChairmanName:     "Bagas Dwi Santoso",
			ViceChairmanName: "Salsabila Nur Aini",
			Vision:           "Menjadi jembatan antara siswa dan sekolah melalui budaya digital yang sehat dan produktif.",
			Mission:          "Mengadakan kelas literasi digital dan keamanan data.\nMembangun pusat informasi kegiatan siswa yang mudah diakses.\nMemperluas program kolaborasi antar-ekstrakurikuler.",
			IsActive:         true,
		},
		{
			CandidateNumber:  3,
			ChairmanName:     "Kirana Maharani",
			ViceChairmanName: "Farel Adi Nugroho",
			Vision:           "Merawat kreativitas dan kebersamaan siswa melalui kegiatan seni, budaya, dan pelayanan.",
			Mission:          "Menghidupkan kembali panggung seni dan budaya sekolah.\nMembuat ruang kolaborasi untuk komunitas kreatif siswa.\nMenguatkan gerakan kepedulian sosial di lingkungan sekolah.",
			IsActive:         true,
		},
	}
	for _, candidate := range candidates {
		if _, err := st.CreateCandidate(ctx, school.ID, electionID, candidate); err != nil {
			return fmt.Errorf("seed kandidat demo: %w", err)
		}
	}
	if err := st.StartElection(ctx, school.ID, electionID, now); err != nil {
		return fmt.Errorf("aktifkan pemilihan demo: %w", err)
	}
	if err := st.RecordAudit(ctx, school.ID, nil, "DEMO_SEED", "election", &electionID, map[string]any{"candidates": len(candidates)}); err != nil {
		return err
	}
	slog.Info("data pemilihan demo dibuat", "election_id", electionID, "candidates", len(candidates))
	slog.Info("belum ada token demo; login ke /admin lalu buat token dari menu Token")
	return nil
}
