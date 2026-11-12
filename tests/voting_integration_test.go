package tests

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"govoting/internal/database"
	"govoting/internal/models"
	"govoting/internal/security"
	"govoting/internal/store"
	"govoting/migrations"

	_ "github.com/go-sql-driver/mysql"
)

type fixture struct {
	st          *store.Store
	db          *sql.DB
	schoolID    int64
	electionID  int64
	candidateA  int64
	candidateB  int64
	tokenSecret []byte
}

func testDatabase(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("TEST_DB_DSN")
	if dsn == "" {
		t.Skip("TEST_DB_DSN belum diatur; lewati uji integrasi MySQL")
	}
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Fatalf("buka database uji: %v", err)
	}
	db.SetMaxOpenConns(24)
	db.SetMaxIdleConns(8)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := database.Migrate(ctx, db, migrations.FS); err != nil {
		db.Close()
		t.Fatalf("migrasi database uji: %v", err)
	}
	return db
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	db := testDatabase(t)
	ctx := context.Background()
	st := store.New(db)
	suffix, err := security.RandomHex(8)
	if err != nil {
		t.Fatalf("buat akhiran unik: %v", err)
	}
	schoolID, err := st.CreateSchool(ctx, "Sekolah Uji "+suffix, "sekolah-uji-"+suffix, "", "", "", "")
	if err != nil {
		db.Close()
		t.Fatalf("buat sekolah uji: %v", err)
	}
	now := time.Now().UTC()
	electionID, err := st.CreateElection(ctx, schoolID, models.ElectionInput{
		Name:              "Pemilihan Uji",
		Period:            "2026/2027",
		Description:       "Data uji integrasi.",
		StartAt:           now.Add(-30 * time.Minute),
		EndAt:             now.Add(2 * time.Hour),
		ResultsVisibility: "LIVE",
	})
	if err != nil {
		t.Fatalf("buat pemilihan uji: %v", err)
	}
	candidateA, err := st.CreateCandidate(ctx, schoolID, electionID, models.CandidateInput{
		CandidateNumber:  1,
		ChairmanName:     "Calon Uji Satu",
		ViceChairmanName: "Wakil Uji Satu",
		Vision:           "Visi uji.",
		Mission:          "Misi uji.",
		IsActive:         true,
	})
	if err != nil {
		t.Fatalf("buat calon pertama: %v", err)
	}
	candidateB, err := st.CreateCandidate(ctx, schoolID, electionID, models.CandidateInput{
		CandidateNumber:  2,
		ChairmanName:     "Calon Uji Dua",
		ViceChairmanName: "Wakil Uji Dua",
		Vision:           "Visi uji.",
		Mission:          "Misi uji.",
		IsActive:         true,
	})
	if err != nil {
		t.Fatalf("buat calon kedua: %v", err)
	}
	if err := st.StartElection(ctx, schoolID, electionID, now); err != nil {
		t.Fatalf("aktifkan pemilihan uji: %v", err)
	}
	f := &fixture{
		st:          st,
		db:          db,
		schoolID:    schoolID,
		electionID:  electionID,
		candidateA:  candidateA,
		candidateB:  candidateB,
		tokenSecret: []byte("secret-uji-integrasi-yang-panjang"),
	}
	t.Cleanup(func() {
		ctx := context.Background()
		_, _ = db.ExecContext(ctx, `DELETE FROM votes WHERE election_id=?`, electionID)
		_, _ = db.ExecContext(ctx, `DELETE FROM schools WHERE id=?`, schoolID)
		db.Close()
	})
	return f
}

func (f *fixture) tokenSession(t *testing.T) (sessionHash []byte, token string, tokenID int64) {
	t.Helper()
	ctx := context.Background()
	tokens, err := f.st.GenerateTokens(ctx, f.electionID, 1, nil, f.tokenSecret)
	if err != nil {
		t.Fatalf("buat token: %v", err)
	}
	token = tokens[0]
	tokenHash := security.HashToken(f.tokenSecret, token)
	if err := f.db.QueryRowContext(ctx, `SELECT id FROM voting_tokens WHERE token_hash=?`, tokenHash).Scan(&tokenID); err != nil {
		t.Fatalf("baca id token: %v", err)
	}
	rawSession, err := security.RandomURLToken(24)
	if err != nil {
		t.Fatalf("buat sesi: %v", err)
	}
	sessionHash = security.HashSession(rawSession)
	now := time.Now().UTC()
	if _, _, err := f.st.CreateVotingSession(ctx, tokenHash, sessionHash, "csrf-uji", 10*time.Minute, now); err != nil {
		t.Fatalf("buat sesi voting: %v", err)
	}
	return sessionHash, token, tokenID
}

func TestConcurrentSubmitVoteUsesTokenOnce(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	sessionHash, _, tokenID := f.tokenSession(t)
	now := time.Now().UTC()

	const workers = 16
	start := make(chan struct{})
	errs := make(chan error, workers)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, err := f.st.SubmitVote(ctx, sessionHash, f.candidateA, now)
			errs <- err
		}()
	}
	close(start)
	wg.Wait()
	close(errs)

	successes := 0
	for err := range errs {
		if err == nil {
			successes++
			continue
		}
		if !errors.Is(err, store.ErrSessionInvalid) && !errors.Is(err, store.ErrAlreadyUsed) {
			t.Fatalf("galat tak terduga saat kirim suara: %v", err)
		}
	}
	if successes != 1 {
		t.Fatalf("suara berhasil = %d, mau tepat 1", successes)
	}

	var votes int
	if err := f.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM votes WHERE election_id=?`, f.electionID).Scan(&votes); err != nil {
		t.Fatalf("hitung suara: %v", err)
	}
	if votes != 1 {
		t.Fatalf("jumlah suara tersimpan = %d, mau 1", votes)
	}
	var status string
	if err := f.db.QueryRowContext(ctx, `SELECT status FROM voting_tokens WHERE id=?`, tokenID).Scan(&status); err != nil {
		t.Fatalf("baca status token: %v", err)
	}
	if status != string(models.TokenUsed) {
		t.Fatalf("status token = %q, mau USED", status)
	}
	if _, err := f.st.SubmitVote(ctx, sessionHash, f.candidateB, now); !errors.Is(err, store.ErrSessionInvalid) {
		t.Fatalf("kirim ulang sesi yang sama: %v, mau ErrSessionInvalid", err)
	}
}

func TestUsedTokenCannotStartNewSession(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	sessionHash, token, _ := f.tokenSession(t)
	now := time.Now().UTC()
	if _, err := f.st.SubmitVote(ctx, sessionHash, f.candidateA, now); err != nil {
		t.Fatalf("kirim suara: %v", err)
	}
	tokenHash := security.HashToken(f.tokenSecret, token)
	newSession := security.HashSession("sesi-baru-setelah-dipakai")
	if _, _, err := f.st.CreateVotingSession(ctx, tokenHash, newSession, "csrf-baru", 10*time.Minute, now); !errors.Is(err, store.ErrTokenInvalid) {
		t.Fatalf("token terpakai membuat sesi baru: %v, mau ErrTokenInvalid", err)
	}
}

func TestCancelVotingSessionInvalidatesSession(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	sessionHash, _, tokenID := f.tokenSession(t)
	if err := f.st.CancelVotingSession(ctx, sessionHash); err != nil {
		t.Fatalf("batalkan sesi: %v", err)
	}
	if _, err := f.st.SubmitVote(ctx, sessionHash, f.candidateA, time.Now().UTC()); !errors.Is(err, store.ErrSessionInvalid) {
		t.Fatalf("kirim suara dari sesi batal: %v, mau ErrSessionInvalid", err)
	}
	var votes int
	if err := f.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM votes WHERE election_id=?`, f.electionID).Scan(&votes); err != nil {
		t.Fatalf("hitung suara: %v", err)
	}
	if votes != 0 {
		t.Fatalf("sesi batal menyimpan %d suara", votes)
	}
	var status string
	if err := f.db.QueryRowContext(ctx, `SELECT status FROM voting_tokens WHERE id=?`, tokenID).Scan(&status); err != nil {
		t.Fatalf("baca status token: %v", err)
	}
	if status != string(models.TokenUnused) {
		t.Fatalf("token berubah menjadi %q setelah sesi batal", status)
	}
}

func TestVotingSessionExpiry(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	tokens, err := f.st.GenerateTokens(ctx, f.electionID, 1, nil, f.tokenSecret)
	if err != nil {
		t.Fatalf("buat token: %v", err)
	}
	sessionHash := security.HashSession("sesi-kedaluwarsa")
	now := time.Now().UTC()
	if _, _, err := f.st.CreateVotingSession(ctx, security.HashToken(f.tokenSecret, tokens[0]), sessionHash, "csrf", 10*time.Minute, now); err != nil {
		t.Fatalf("buat sesi: %v", err)
	}
	if _, err := f.db.ExecContext(ctx, `UPDATE voting_sessions SET expires_at=? WHERE session_hash=?`, now.Add(-time.Minute), sessionHash); err != nil {
		t.Fatalf("kedaluwarsakan sesi: %v", err)
	}
	if _, _, err := f.st.GetVotingSession(ctx, sessionHash, time.Now().UTC()); !errors.Is(err, store.ErrSessionInvalid) {
		t.Fatalf("sesi kedaluwarsa diterima: %v", err)
	}
	if _, err := f.st.SubmitVote(ctx, sessionHash, f.candidateA, time.Now().UTC()); !errors.Is(err, store.ErrSessionInvalid) {
		t.Fatalf("kirim suara dari sesi kedaluwarsa: %v", err)
	}
}

func TestAdminSessionValidation(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	hash, err := security.HashPassword("kata-sandi-admin-uji")
	if err != nil {
		t.Fatalf("hash password admin: %v", err)
	}
	adminID, err := f.st.CreateAdmin(ctx, f.schoolID, "Admin Uji", "admin-uji", "", hash)
	if err != nil {
		t.Fatalf("buat admin: %v", err)
	}
	sessionHash := security.HashSession("sesi-admin-uji")
	expiresAt := time.Now().UTC().Add(time.Hour)
	if err := f.st.CreateAdminSession(ctx, sessionHash, &adminID, "csrf-admin", expiresAt); err != nil {
		t.Fatalf("buat sesi admin: %v", err)
	}
	sess, err := f.st.GetSession(ctx, sessionHash, time.Now().UTC())
	if err != nil {
		t.Fatalf("baca sesi admin: %v", err)
	}
	if sess.Admin == nil || sess.Admin.ID != adminID {
		t.Fatalf("admin sesi tidak sesuai: %+v", sess.Admin)
	}
	if _, err := f.db.ExecContext(ctx, `UPDATE admins SET is_active=0 WHERE id=?`, adminID); err != nil {
		t.Fatalf("nonaktifkan admin: %v", err)
	}
	if _, err := f.st.GetSession(ctx, sessionHash, time.Now().UTC()); !errors.Is(err, store.ErrInactive) {
		t.Fatalf("sesi admin nonaktif: %v, mau ErrInactive", err)
	}
	if err := f.st.DeleteSession(ctx, sessionHash); err != nil {
		t.Fatalf("hapus sesi admin: %v", err)
	}
	if _, err := f.st.GetSession(ctx, sessionHash, time.Now().UTC()); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("sesi terhapus masih terbaca: %v", err)
	}
	if err := f.st.CreateAdminSession(ctx, security.HashSession("sesi-kedaluwarsa-admin"), &adminID, "csrf", time.Now().UTC().Add(-time.Minute)); err != nil {
		t.Fatalf("buat sesi kedaluwarsa: %v", err)
	}
	if _, err := f.st.GetSession(ctx, security.HashSession("sesi-kedaluwarsa-admin"), time.Now().UTC()); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("sesi admin kedaluwarsa diterima: %v", err)
	}
}

func TestVotesSchemaInMySQLHasNoVoterColumns(t *testing.T) {
	db := testDatabase(t)
	defer db.Close()
	ctx := context.Background()
	rows, err := db.QueryContext(ctx, `
		SELECT column_name FROM information_schema.columns
		WHERE table_schema=DATABASE() AND table_name='votes'
		ORDER BY ordinal_position`)
	if err != nil {
		t.Fatalf("baca kolom votes: %v", err)
	}
	defer rows.Close()
	var columns []string
	for rows.Next() {
		var column string
		if err := rows.Scan(&column); err != nil {
			t.Fatalf("scan kolom: %v", err)
		}
		columns = append(columns, column)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterasi kolom: %v", err)
	}
	want := []string{"id", "election_id", "candidate_id", "created_at"}
	if fmt.Sprint(columns) != fmt.Sprint(want) {
		t.Fatalf("kolom votes = %v, mau %v", columns, want)
	}
}
