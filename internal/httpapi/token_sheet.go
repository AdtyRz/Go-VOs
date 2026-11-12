package httpapi

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/go-pdf/fpdf"

	"govoting/internal/models"
)

const (
	tokenSheetPerPage = 28
	tokenSheetCols    = 4
	tokenSheetCardW   = 44.25
	tokenSheetCardH   = 32.0
	tokenSheetGapX    = 3.0
	tokenSheetGapY    = 2.5
	tokenSheetMarginX = 12.0
	tokenSheetTop     = 42.0
)

func (s *Server) buildTokenSheetPDF(school *models.School, election *models.Election, tokens []string, expiresAt time.Time) ([]byte, int, error) {
	regular, err := readEmbedded(s.files, "static/fonts/dmsans-regular.ttf")
	if err != nil {
		return nil, 0, err
	}
	bold, err := readEmbedded(s.files, "static/fonts/dmsans-bold.ttf")
	if err != nil {
		return nil, 0, err
	}

	loc := s.cfg.Timezone
	if loc == nil {
		loc = time.Local
	}
	now := time.Now().In(loc)
	primary := mustRGB(school.PrimaryColor, rgb{122, 79, 42})
	ink := rgb{32, 29, 26}
	line := mix(primary, rgb{255, 255, 255}, 0.68)
	headerText := contrastFor(primary)

	pdf := fpdf.New("P", "mm", "A4", "")
	pdf.SetMargins(tokenSheetMarginX, 12, tokenSheetMarginX)
	pdf.SetAutoPageBreak(false, 0)
	pdf.SetCompression(true)
	pdf.SetTitle("Kartu Token Pemilih - "+election.Name, true)
	pdf.SetAuthor(school.Name, true)
	pdf.SetCreator("Go-Voting", true)
	pdf.AddUTF8FontFromBytes("dmsans", "", regular)
	pdf.AddUTF8FontFromBytes("dmsans", "B", bold)
	pdf.AliasNbPages("{nb}")

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
		pdf.SetY(-10)
		pdf.SetDrawColor(line.R, line.G, line.B)
		pdf.SetLineWidth(0.2)
		pdf.Line(tokenSheetMarginX, pdf.GetY()-2, 198, pdf.GetY()-2)
		pdf.SetFont("dmsans", "", 7)
		pdf.SetTextColor(118, 112, 106)
		pdf.SetX(tokenSheetMarginX)
		pdf.CellFormat(120, 4, "Go-Voting - dicetak "+now.Format("02 Jan 2006 15:04"), "", 0, "L", false, 0, "")
		pdf.CellFormat(66, 4, "Halaman "+strconv.Itoa(pdf.PageNo())+" dari {nb}", "", 0, "R", false, 0, "")
	})

	drawHeader := func() {
		pdf.SetFillColor(primary.R, primary.G, primary.B)
		pdf.Rect(0, 0, 210, 34, "F")
		if logoName != "" {
			pdf.ImageOptions(logoName, 181, 8, 17, 17, false, fpdf.ImageOptions{ImageType: logoType}, 0, "")
		}
		pdf.SetTextColor(headerText.R, headerText.G, headerText.B)
		pdf.SetXY(12, 7)
		pdf.SetFont("dmsans", "B", 7.5)
		pdf.CellFormat(160, 4, "KARTU TOKEN PEMILIH", "", 0, "L", false, 0, "")
		pdf.SetXY(12, 12)
		pdf.SetFont("dmsans", "B", 15)
		pdf.CellFormat(164, 7, fitPDFText(pdf, cleanPDFText(school.Name), 164), "", 0, "L", false, 0, "")
		pdf.SetXY(12, 21)
		pdf.SetFont("dmsans", "", 9)
		pdf.CellFormat(164, 5, fitPDFText(pdf, cleanPDFText(election.Name), 164), "", 0, "L", false, 0, "")
		pdf.SetXY(12, 27)
		pdf.SetFont("dmsans", "", 7.5)
		meta := fmt.Sprintf("%d token - Berlaku hingga %s - Satu kartu untuk satu pemilih, sekali pakai.", len(tokens), expiresAt.In(loc).Format("02 Jan 2006 15:04"))
		pdf.CellFormat(178, 4, fitPDFText(pdf, meta, 178), "", 0, "L", false, 0, "")
	}

	drawCard := func(x, y float64, token string, number int) {
		pdf.SetFillColor(255, 255, 255)
		pdf.SetDrawColor(line.R, line.G, line.B)
		pdf.SetLineWidth(0.25)
		pdf.SetDashPattern([]float64{1.1, 1.1}, 0)
		pdf.Rect(x, y, tokenSheetCardW, tokenSheetCardH, "DF")
		pdf.SetDashPattern([]float64{}, 0)

		pdf.SetFillColor(primary.R, primary.G, primary.B)
		pdf.Rect(x, y, tokenSheetCardW, 2.2, "F")

		pdf.SetTextColor(118, 112, 106)
		pdf.SetFont("dmsans", "B", 6)
		pdf.SetXY(x+3.5, y+4)
		pdf.CellFormat(22, 3, "TOKEN PEMILIH", "", 0, "L", false, 0, "")
		pdf.SetFont("dmsans", "", 6)
		pdf.SetXY(x+tokenSheetCardW-15, y+4)
		pdf.CellFormat(11, 3, fmt.Sprintf("%03d", number), "", 0, "R", false, 0, "")

		pdf.SetTextColor(primary.R, primary.G, primary.B)
		pdf.SetFont("dmsans", "B", 25)
		pdf.SetXY(x, y+7)
		pdf.CellFormat(tokenSheetCardW, 12, token, "", 0, "C", false, 0, "")

		pdf.SetTextColor(ink.R, ink.G, ink.B)
		pdf.SetFont("dmsans", "", 5.8)
		pdf.SetXY(x+2.5, y+23)
		pdf.CellFormat(tokenSheetCardW-5, 3.5, "Berlaku s.d. "+expiresAt.In(loc).Format("02 Jan 2006"), "", 0, "C", false, 0, "")
	}

	for i, token := range tokens {
		if i%tokenSheetPerPage == 0 {
			pdf.AddPage()
			drawHeader()
		}
		position := i % tokenSheetPerPage
		row := position / tokenSheetCols
		col := position % tokenSheetCols
		x := tokenSheetMarginX + float64(col)*(tokenSheetCardW+tokenSheetGapX)
		y := tokenSheetTop + float64(row)*(tokenSheetCardH+tokenSheetGapY)
		drawCard(x, y, token, i+1)
	}

	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		return nil, 0, err
	}
	return buf.Bytes(), pdf.PageCount(), nil
}

func cleanPDFText(text string) string {
	return strings.Join(strings.Fields(text), " ")
}

func fitPDFText(pdf *fpdf.Fpdf, text string, maxWidth float64) string {
	if text == "" || pdf.GetStringWidth(text) <= maxWidth {
		return text
	}
	runes := []rune(text)
	suffix := "..."
	for len(runes) > 0 {
		runes = runes[:len(runes)-1]
		candidate := strings.TrimSpace(string(runes)) + suffix
		if pdf.GetStringWidth(candidate) <= maxWidth {
			return candidate
		}
	}
	return suffix
}
