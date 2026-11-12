package httpapi

import (
	"bytes"
	"fmt"
	"testing"
	"time"

	"govoting/internal/models"
)

func TestBuildTokenSheetPDFPagination(t *testing.T) {
	server := newTestServer(t)
	school := &models.School{
		Name:         "SMA Negeri 1 Nusantara",
		PrimaryColor: "#7a4f2a",
	}
	election := &models.Election{
		ID:     1,
		Name:   "Pemilihan Ketua dan Wakil Ketua OSIS",
		Period: "2026/2027",
		EndAt:  time.Date(2026, 11, 13, 15, 0, 0, 0, time.UTC),
	}
	cases := []struct {
		count int
		pages int
	}{
		{1, 1},
		{25, 1},
		{tokenSheetPerPage, 1},
		{tokenSheetPerPage + 1, 2},
		{1000, 36},
	}
	for _, tc := range cases {
		tokens := make([]string, 0, tc.count)
		for i := 0; i < tc.count; i++ {
			tokens = append(tokens, fmt.Sprintf("%05d", i))
		}
		data, pages, err := server.buildTokenSheetPDF(school, election, tokens, election.EndAt)
		if err != nil {
			t.Fatalf("%d token: %v", tc.count, err)
		}
		if !bytes.HasPrefix(data, []byte("%PDF-")) || len(data) < 1024 {
			t.Fatalf("%d token: PDF tidak valid, ukuran %d", tc.count, len(data))
		}
		if pages != tc.pages {
			t.Fatalf("%d token: halaman = %d, mau %d", tc.count, pages, tc.pages)
		}
	}
}

func TestBuildTokenSheetPDFAcceptsLongIdentity(t *testing.T) {
	server := newTestServer(t)
	school := &models.School{
		Name:         "Sekolah Menengah Atas Negeri 1 Nusantara Raya Kabupaten Kepulauan Seribu",
		PrimaryColor: "#1b5e20",
	}
	election := &models.Election{
		ID:    2,
		Name:  "Pemilihan Ketua dan Wakil Ketua Organisasi Siswa Intra Sekolah Tahun Pelajaran 2026/2027",
		EndAt: time.Date(2026, 12, 31, 23, 59, 0, 0, time.UTC),
	}
	data, pages, err := server.buildTokenSheetPDF(school, election, []string{"AB7CD"}, election.EndAt)
	if err != nil {
		t.Fatalf("buat PDF: %v", err)
	}
	if pages != 1 {
		t.Fatalf("halaman = %d, mau 1", pages)
	}
	if !bytes.HasPrefix(data, []byte("%PDF-")) {
		t.Fatal("hasil bukan PDF")
	}
}
