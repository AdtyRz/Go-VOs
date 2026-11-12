package tests

import (
	"io/fs"
	"strings"
	"testing"

	"govoting/migrations"
)

func votesTableDefinition(t *testing.T) string {
	t.Helper()
	files, err := fs.Glob(migrations.FS, "*.sql")
	if err != nil {
		t.Fatalf("baca migrasi: %v", err)
	}
	for _, name := range files {
		body, err := fs.ReadFile(migrations.FS, name)
		if err != nil {
			t.Fatalf("baca %s: %v", name, err)
		}
		sql := string(body)
		start := strings.Index(sql, "CREATE TABLE IF NOT EXISTS votes (")
		if start == -1 {
			continue
		}
		rest := sql[start:]
		end := strings.Index(rest, ") ENGINE=")
		if end == -1 {
			t.Fatalf("definisi tabel votes tidak lengkap di %s", name)
		}
		return rest[:end]
	}
	t.Fatal("tabel votes tidak ditemukan di migrasi")
	return ""
}

func TestVotesTableHasNoVoterIdentifiers(t *testing.T) {
	definition := strings.ToLower(votesTableDefinition(t))
	columns := make(map[string]bool)
	for _, line := range strings.Split(definition, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "create table") || strings.HasPrefix(line, "primary key") ||
			strings.HasPrefix(line, "key ") || strings.HasPrefix(line, "unique key") ||
			strings.HasPrefix(line, "constraint") || line == "(" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		columns[fields[0]] = true
	}
	want := map[string]bool{"id": true, "election_id": true, "candidate_id": true, "created_at": true}
	if len(columns) != len(want) {
		t.Fatalf("kolom tabel votes berubah: %v", columns)
	}
	for column := range want {
		if !columns[column] {
			t.Fatalf("kolom wajib %q tidak ada pada tabel votes", column)
		}
	}
	forbidden := []string{"token", "session", "student", "siswa", "user", "device", "ip", "agent", "nis", "nisn", "fingerprint"}
	for column := range columns {
		for _, term := range forbidden {
			if strings.Contains(column, term) {
				t.Fatalf("kolom %q dapat membuka identitas pemilih", column)
			}
		}
	}
}

func TestVoteInsertDoesNotReferenceVoter(t *testing.T) {
	body, err := fs.ReadFile(migrations.FS, "001_initial.sql")
	if err != nil {
		t.Fatalf("baca migrasi: %v", err)
	}
	definition := strings.ToLower(string(body))
	index := strings.Index(definition, "create table if not exists votes (")
	if index == -1 {
		t.Fatal("tabel votes tidak ditemukan")
	}
	end := strings.Index(definition[index:], ") engine=")
	if end == -1 {
		t.Fatal("definisi tabel votes tidak lengkap")
	}
	block := definition[index : index+end]
	for _, term := range []string{"foreign key (token", "foreign key (session", "foreign key (student", "ip_address"} {
		if strings.Contains(block, term) {
			t.Fatalf("tabel votes memuat referensi pemilih: %s", term)
		}
	}
}
