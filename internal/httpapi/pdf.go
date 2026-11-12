package httpapi

import (
	"bytes"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/go-pdf/fpdf"

	"govoting/internal/models"
)

func (s *Server) exportResultsPDF(w http.ResponseWriter, school *models.School, election *models.Election, results *models.ElectionResults, stats *models.DashboardStats) {
	regular, err := readEmbedded(s.files, "static/fonts/dmsans-regular.ttf")
	if err != nil {
		slog.Error("pdf font", "error", err)
		writeAPIError(w, http.StatusInternalServerError, "INTERNAL", "Gagal memuat font laporan.")
		return
	}
	bold, err := readEmbedded(s.files, "static/fonts/dmsans-bold.ttf")
	if err != nil {
		slog.Error("pdf font", "error", err)
		writeAPIError(w, http.StatusInternalServerError, "INTERNAL", "Gagal memuat font laporan.")
		return
	}

	primary := mustRGB(school.PrimaryColor, rgb{122, 79, 42})
	ink := rgb{32, 29, 26}
	soft := mix(primary, rgb{255, 255, 255}, 0.92)
	line := mix(primary, rgb{255, 255, 255}, 0.7)
	headerText := contrastFor(primary)

	pdf := fpdf.New("P", "mm", "A4", "")
	pdf.SetMargins(15, 20, 15)
	pdf.SetAutoPageBreak(true, 22)
	pdf.SetTitle(election.Name+" - Hasil Pemilihan", true)
	pdf.SetAuthor(school.Name, true)
	pdf.SetCreator("Go-Voting", true)
	pdf.AddUTF8FontFromBytes("dmsans", "", regular)
	pdf.AddUTF8FontFromBytes("dmsans", "B", bold)

	logoName := ""
	logoType := "PNG"
	if school.LogoPath != "" {
		if data, imgType, err := s.readLogo(school.LogoPath); err == nil {
			logoName = "school-logo"
			logoType = imgType
			pdf.RegisterImageOptionsReader(logoName, fpdf.ImageOptions{ImageType: imgType}, bytes.NewReader(data))
		}
	}

	pdf.SetFooterFunc(func() {
		pdf.SetY(-18)
		pdf.SetDrawColor(line.R, line.G, line.B)
		pdf.SetLineWidth(0.2)
		pdf.Line(15, pdf.GetY()-2, 195, pdf.GetY()-2)
		pdf.SetFont("dmsans", "", 8)
		pdf.SetTextColor(110, 105, 100)
		pdf.CellFormat(90, 6, "Dihasilkan oleh Go-Voting - "+time.Now().In(s.cfg.Timezone).Format("02 Jan 2006 15:04"), "", 0, "L", false, 0, "")
		pdf.CellFormat(90, 6, fmt.Sprintf("Halaman %d", pdf.PageNo()), "", 0, "R", false, 0, "")
	})

	pdf.AddPage()
	pageWidth, _ := pdf.GetPageSize()
	pdf.SetFillColor(primary.R, primary.G, primary.B)
	pdf.Rect(0, 0, pageWidth, 34, "F")
	if logoName != "" {
		pdf.ImageOptions(logoName, pageWidth-35, 7, 20, 20, false, fpdf.ImageOptions{ImageType: logoType}, 0, "")
	}
	pdf.SetTextColor(headerText.R, headerText.G, headerText.B)
	pdf.SetFont("dmsans", "B", 8)
	pdf.SetXY(15, 9)
	pdf.CellFormat(130, 5, "LAPORAN RESMI PEMILIHAN", "", 0, "L", false, 0, "")
	pdf.SetXY(15, 15)
	pdf.SetFont("dmsans", "B", 18)
	pdf.CellFormat(150, 9, school.Name, "", 0, "L", false, 0, "")
	pdf.SetFont("dmsans", "", 8)
	pdf.SetXY(15, 25)
	pdf.CellFormat(150, 4, "Dokumen hasil pemungutan suara elektronik", "", 0, "L", false, 0, "")

	pdf.SetY(43)
	pdf.SetTextColor(ink.R, ink.G, ink.B)
	pdf.SetFont("dmsans", "B", 16)
	pdf.MultiCell(180, 7, election.Name, "", "L", false)
	pdf.SetFont("dmsans", "", 10)
	pdf.SetTextColor(105, 100, 95)
	pdf.CellFormat(180, 6, "Periode "+election.Period+"  -  Status: "+election.StatusLabel(), "", 1, "L", false, 0, "")
	pdf.Ln(3)

	label := func(text string) {
		pdf.SetFont("dmsans", "", 8)
		pdf.SetTextColor(125, 120, 115)
		pdf.CellFormat(35, 5, strings.ToUpper(text), "", 0, "L", false, 0, "")
	}
	value := func(width float64, text string, newline bool) {
		pdf.SetFont("dmsans", "", 9.5)
		pdf.SetTextColor(ink.R, ink.G, ink.B)
		border := ""
		ln := 0
		if newline {
			ln = 1
		}
		pdf.CellFormat(width, 5, text, border, ln, "L", false, 0, "")
	}
	label("Mulai")
	value(70, election.StartAt.In(s.cfg.Timezone).Format("02 Jan 2006, 15:04"), false)
	label("Selesai")
	value(70, election.EndAt.In(s.cfg.Timezone).Format("02 Jan 2006, 15:04"), true)
	label("Total token")
	value(70, strconv.FormatInt(stats.Total, 10)+" diterbitkan - "+strconv.FormatInt(stats.Used, 10)+" digunakan", false)
	label("Partisipasi")
	value(70, fmt.Sprintf("%.1f%%", stats.Participation), true)
	label("Suara sah")
	value(70, strconv.FormatInt(results.TotalVotes, 10), false)
	label("Pasangan calon")
	value(70, strconv.Itoa(stats.CandidateCount), true)
	pdf.Ln(5)

	y := pdf.GetY()
	boxW := 42.0
	boxGap := 4.0
	boxH := 21.0
	boxes := []struct {
		label string
		value string
	}{
		{"SUARA MASUK", strconv.FormatInt(results.TotalVotes, 10)},
		{"TOKEN DIPAKAI", strconv.FormatInt(stats.Used, 10)},
		{"TOKEN TERSEDIA", strconv.FormatInt(stats.Available, 10)},
		{"PARTISIPASI", fmt.Sprintf("%.1f%%", stats.Participation)},
	}
	for i, box := range boxes {
		x := 15 + float64(i)*(boxW+boxGap)
		pdf.SetFillColor(soft.R, soft.G, soft.B)
		pdf.SetDrawColor(line.R, line.G, line.B)
		pdf.SetLineWidth(0.2)
		pdf.Rect(x, y, boxW, boxH, "DF")
		pdf.SetFont("dmsans", "", 7)
		pdf.SetTextColor(120, 115, 110)
		pdf.SetXY(x+4, y+4)
		pdf.CellFormat(boxW-8, 4, box.label, "", 0, "L", false, 0, "")
		pdf.SetFont("dmsans", "B", 15)
		pdf.SetTextColor(primary.R, primary.G, primary.B)
		pdf.SetXY(x+4, y+9)
		pdf.CellFormat(boxW-8, 8, box.value, "", 0, "L", false, 0, "")
	}
	pdf.SetY(y + boxH + 10)

	pdf.SetFont("dmsans", "B", 10)
	pdf.SetTextColor(ink.R, ink.G, ink.B)
	pdf.CellFormat(180, 7, "Perolehan Suara", "", 1, "L", false, 0, "")
	pdf.Ln(1)
	colW := []float64{14, 18, 62, 46, 20, 20}
	headers := []string{"Rank", "No.", "Calon Ketua", "Calon Wakil", "Suara", "Persen"}
	pdf.SetFillColor(primary.R, primary.G, primary.B)
	pdf.SetTextColor(headerText.R, headerText.G, headerText.B)
	pdf.SetFont("dmsans", "B", 8)
	for i, h := range headers {
		align := "L"
		if i >= 4 {
			align = "R"
		}
		pdf.CellFormat(colW[i], 8, h, "0", 0, align, true, 0, "")
	}
	pdf.Ln(-1)
	pdf.SetTextColor(ink.R, ink.G, ink.B)
	pdf.SetDrawColor(line.R, line.G, line.B)
	for _, c := range results.Candidates {
		if pdf.GetY() > 250 {
			pdf.AddPage()
		}
		pdf.SetFont("dmsans", "B", 9)
		pdf.SetFillColor(255, 255, 255)
		pdf.CellFormat(colW[0], 9, "#"+strconv.Itoa(c.Rank), "B", 0, "L", false, 0, "")
		pdf.SetFont("dmsans", "B", 9)
		pdf.CellFormat(colW[1], 9, fmt.Sprintf("%02d", c.CandidateNumber), "B", 0, "L", false, 0, "")
		pdf.SetFont("dmsans", "B", 9)
		pdf.CellFormat(colW[2], 9, c.ChairmanName, "B", 0, "L", false, 0, "")
		pdf.SetFont("dmsans", "", 9)
		pdf.CellFormat(colW[3], 9, c.ViceChairmanName, "B", 0, "L", false, 0, "")
		pdf.SetFont("dmsans", "B", 9)
		pdf.CellFormat(colW[4], 9, strconv.FormatInt(c.Votes, 10), "B", 0, "R", false, 0, "")
		pdf.CellFormat(colW[5], 9, fmt.Sprintf("%.1f%%", c.Percentage), "B", 0, "R", false, 0, "")
		pdf.Ln(-1)
	}
	if len(results.Candidates) == 0 {
		pdf.SetFont("dmsans", "", 9)
		pdf.CellFormat(180, 10, "Belum ada pasangan calon.", "B", 1, "L", false, 0, "")
	}
	pdf.Ln(6)

	if len(results.Candidates) > 0 && results.TotalVotes > 0 {
		winner := results.Candidates[0]
		pdf.SetFillColor(soft.R, soft.G, soft.B)
		pdf.SetDrawColor(primary.R, primary.G, primary.B)
		pdf.SetLineWidth(0.5)
		y = pdf.GetY()
		pdf.Rect(15, y, 180, 25, "DF")
		pdf.SetXY(21, y+4)
		pdf.SetFont("dmsans", "", 7.5)
		pdf.SetTextColor(120, 115, 110)
		pdf.CellFormat(160, 4, "PEROLEHAN SUARA TERTINGGI", "", 1, "L", false, 0, "")
		pdf.SetX(21)
		pdf.SetFont("dmsans", "B", 12)
		pdf.SetTextColor(primary.R, primary.G, primary.B)
		pdf.CellFormat(160, 7, fmt.Sprintf("Paslon %02d - %s & %s", winner.CandidateNumber, winner.ChairmanName, winner.ViceChairmanName), "", 1, "L", false, 0, "")
		pdf.SetX(21)
		pdf.SetFont("dmsans", "", 8.5)
		pdf.SetTextColor(ink.R, ink.G, ink.B)
		pdf.CellFormat(160, 5, fmt.Sprintf("%d suara (%.1f%% dari %d suara sah)", winner.Votes, winner.Percentage, results.TotalVotes), "", 0, "L", false, 0, "")
		pdf.Ln(15)
	}

	if pdf.GetY() > 235 {
		pdf.AddPage()
	}
	pdf.Ln(4)
	pdf.SetFont("dmsans", "", 8.5)
	pdf.SetTextColor(105, 100, 95)
	pdf.CellFormat(90, 5, "Diverifikasi oleh panitia pemilihan", "", 0, "L", false, 0, "")
	pdf.CellFormat(90, 5, "Dihasilkan pada "+time.Now().In(s.cfg.Timezone).Format("02 Jan 2006, 15:04"), "", 1, "R", false, 0, "")

	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		slog.Error("pdf output", "error", err)
		writeAPIError(w, http.StatusInternalServerError, "INTERNAL", "Gagal membuat PDF.")
		return
	}
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="hasil-pemilihan-%d-%s.pdf"`, election.ID, time.Now().In(s.cfg.Timezone).Format("20060102")))
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Length", strconv.Itoa(buf.Len()))
	_, _ = w.Write(buf.Bytes())
}

func readEmbedded(files fs.FS, name string) ([]byte, error) {
	return fs.ReadFile(files, name)
}

func (s *Server) readLogo(webPath string) ([]byte, string, error) {
	clean := strings.TrimPrefix(webPath, "/uploads/")
	if strings.HasPrefix(clean, "..") {
		return nil, "", fmt.Errorf("invalid logo path")
	}
	full := filepath.Join(s.cfg.UploadDir, filepath.FromSlash(clean))
	data, err := os.ReadFile(full)
	if err != nil {
		return nil, "", err
	}
	imageType := "PNG"
	if strings.HasSuffix(strings.ToLower(clean), ".jpg") || strings.HasSuffix(strings.ToLower(clean), ".jpeg") {
		imageType = "JPG"
	}
	return data, imageType, nil
}

type rgb struct{ R, G, B int }

func mustRGB(hex string, fallback rgb) rgb {
	hex = strings.TrimPrefix(strings.TrimSpace(hex), "#")
	if len(hex) != 6 {
		return fallback
	}
	v, err := strconv.ParseUint(hex, 16, 32)
	if err != nil {
		return fallback
	}
	return rgb{R: int(v >> 16 & 0xff), G: int(v >> 8 & 0xff), B: int(v & 0xff)}
}

func mix(a, b rgb, weightB float64) rgb {
	return rgb{
		R: int(float64(a.R)*(1-weightB) + float64(b.R)*weightB),
		G: int(float64(a.G)*(1-weightB) + float64(b.G)*weightB),
		B: int(float64(a.B)*(1-weightB) + float64(b.B)*weightB),
	}
}

func contrastFor(bg rgb) rgb {

	lum := (0.299*float64(bg.R) + 0.587*float64(bg.G) + 0.114*float64(bg.B)) / 255
	if lum > 0.62 {
		return rgb{25, 22, 20}
	}
	return rgb{255, 255, 255}
}
