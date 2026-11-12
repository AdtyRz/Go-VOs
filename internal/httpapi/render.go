package httpapi

import (
	"bytes"
	"fmt"
	"html/template"
	"io/fs"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"govoting/internal/models"
)

type pageSpec struct {
	layout  string
	content string
}

type Renderer struct {
	pages map[string]*template.Template
	specs map[string]pageSpec
	loc   *time.Location
}

func NewRenderer(files fs.FS, loc *time.Location) (*Renderer, error) {
	partials, err := fs.Glob(files, "templates/partials/*.html")
	if err != nil {
		return nil, err
	}
	specs := map[string]pageSpec{
		"student/home":     {"templates/layouts/student.html", "templates/student/home.html"},
		"student/token":    {"templates/layouts/student.html", "templates/student/token.html"},
		"student/ballot":   {"templates/layouts/student.html", "templates/student/ballot.html"},
		"student/success":  {"templates/layouts/student.html", "templates/student/success.html"},
		"admin/login":      {"templates/layouts/auth.html", "templates/admin/login.html"},
		"admin/dashboard":  {"templates/layouts/admin.html", "templates/admin/dashboard.html"},
		"admin/candidates": {"templates/layouts/admin.html", "templates/admin/candidates.html"},
		"admin/tokens":     {"templates/layouts/admin.html", "templates/admin/tokens.html"},
		"admin/results":    {"templates/layouts/admin.html", "templates/admin/results.html"},
		"admin/elections":  {"templates/layouts/admin.html", "templates/admin/elections.html"},
		"admin/school":     {"templates/layouts/admin.html", "templates/admin/school.html"},
		"admin/audit":      {"templates/layouts/admin.html", "templates/admin/audit.html"},
		"error":            {"templates/layouts/student.html", "templates/student/error.html"},
	}

	formatTime := func(value any, empty, layout string) string {
		var t time.Time
		switch v := value.(type) {
		case time.Time:
			t = v
		case *time.Time:
			if v == nil {
				return empty
			}
			t = *v
		default:
			return empty
		}
		if t.IsZero() {
			return empty
		}
		return t.In(loc).Format(layout)
	}
	funcs := template.FuncMap{
		"date": func(t any) string {
			return formatTime(t, "-", "02 Jan 2006")
		},
		"datetime": func(t any) string {
			return formatTime(t, "-", "02 Jan 2006 - 15:04")
		},
		"shortdatetime": func(t any) string {
			return formatTime(t, "-", "02/01/2006 15:04")
		},
		"clock": func(t any) string {
			return formatTime(t, "-", "15:04")
		},
		"inputdate": func(t any) string {
			return formatTime(t, "", "2006-01-02T15:04")
		},
		"dateinput": func(t any) string {
			return formatTime(t, "", "2006-01-02")
		},
		"isodatetime": func(t any) string {
			return formatTime(t, "", time.RFC3339)
		},
		"pct": func(v float64) string {
			return strconv.FormatFloat(v, 'f', 1, 64) + "%"
		},
		"initials": initials,
		"splitlines": func(s string) []string {
			normalized := strings.ReplaceAll(s, "\r\n", "\n")
			var out []string
			for _, line := range strings.Split(normalized, "\n") {
				line = strings.TrimSpace(strings.TrimPrefix(line, "-"))
				if line != "" {
					out = append(out, line)
				}
			}
			return out
		},
		"add": func(a, b int) int { return a + b },
		"seq": func(from, to int) []int {
			var out []int
			for i := from; i <= to; i++ {
				out = append(out, i)
			}
			return out
		},
		"statusclass": func(v any) string {
			return strings.ToLower(strings.ReplaceAll(fmt.Sprint(v), "_", "-"))
		},
		"themelabel": models.ThemeLabel,
		"truncate": func(s string, n int) string {
			r := []rune(strings.TrimSpace(s))
			if len(r) <= n {
				return string(r)
			}
			return string(r[:n]) + "..."
		},
		"actionlabel": actionLabel,
		"initial": func(s string) string {
			r := []rune(strings.TrimSpace(s))
			if len(r) == 0 {
				return "?"
			}
			return strings.ToUpper(string(r[0]))
		},
	}

	renderer := &Renderer{pages: map[string]*template.Template{}, specs: specs, loc: loc}
	for name, spec := range specs {
		names := append([]string{spec.layout, spec.content}, partials...)
		tpl, err := template.New("layout").Funcs(funcs).Option("missingkey=zero").ParseFS(files, names...)
		if err != nil {
			return nil, fmt.Errorf("parse template %s: %w", name, err)
		}
		renderer.pages[name] = tpl
	}
	return renderer, nil
}

func (r *Renderer) Render(w http.ResponseWriter, status int, page string, data any) {
	tpl, ok := r.pages[page]
	if !ok {
		slog.Error("template tidak dikenal", "page", page)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	var buf bytes.Buffer
	if err := tpl.ExecuteTemplate(&buf, "layout", data); err != nil {
		slog.Error("render template", "page", page, "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_, _ = buf.WriteTo(w)
}

func initials(name string) string {
	parts := strings.Fields(strings.TrimSpace(name))
	if len(parts) == 0 {
		return "?"
	}
	var b strings.Builder
	for i, part := range parts {
		if i >= 2 {
			break
		}
		r := []rune(part)
		if len(r) > 0 {
			b.WriteString(strings.ToUpper(string(r[0])))
		}
	}
	return b.String()
}

func actionLabel(action string) string {
	labels := map[string]string{
		"LOGIN":               "Login admin",
		"LOGOUT":              "Logout admin",
		"ADMIN_BOOTSTRAP":     "Admin awal dibuat",
		"CANDIDATE_CREATE":    "Tambah pasangan calon",
		"CANDIDATE_UPDATE":    "Ubah pasangan calon",
		"CANDIDATE_DELETE":    "Hapus pasangan calon",
		"TOKEN_GENERATE":      "Buat token",
		"TOKEN_REVOKE":        "Cabut token",
		"TOKEN_EXPORT":        "Unduh daftar token",
		"ELECTION_CREATE":     "Buat pemilihan",
		"ELECTION_UPDATE":     "Ubah pemilihan",
		"ELECTION_SCHEDULE":   "Jadwalkan pemilihan",
		"ELECTION_START":      "Mulai pemilihan",
		"ELECTION_PAUSE":      "Jeda pemilihan",
		"ELECTION_CLOSE":      "Tutup pemilihan",
		"ELECTION_ARCHIVE":    "Arsipkan pemilihan",
		"ELECTION_AUTO_CLOSE": "Pemilihan ditutup otomatis",
		"ELECTION_AUTO_START": "Pemilihan dimulai otomatis",
		"SCHOOL_UPDATE":       "Ubah profil sekolah",
		"SCHOOL_LOGO":         "Ubah logo sekolah",
		"RESULTS_EXPORT":      "Unduh laporan hasil",
		"DEMO_SEED":           "Data demo dibuat",
	}
	if label, ok := labels[action]; ok {
		return label
	}
	return action
}
