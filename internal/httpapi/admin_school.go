package httpapi

import (
	"errors"
	"io"
	"net/http"
	"strings"

	"govoting/internal/models"
	"govoting/internal/upload"
)

func (s *Server) apiAdminSchool(w http.ResponseWriter, r *http.Request) {
	sess := currentSession(r)
	school, err := s.st.GetSchool(r.Context(), sess.Admin.SchoolID)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeData(w, http.StatusOK, map[string]any{"school": school})
}

func (s *Server) apiAdminSchoolUpdate(w http.ResponseWriter, r *http.Request) {
	sess := currentSession(r)
	school, err := s.st.GetSchool(r.Context(), sess.Admin.SchoolID)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	var input struct {
		Name           string `json:"name"`
		Address        string `json:"address"`
		Email          string `json:"email"`
		Phone          string `json:"phone"`
		Website        string `json:"website"`
		Theme          string `json:"theme"`
		PrimaryColor   string `json:"primary_color"`
		SecondaryColor string `json:"secondary_color"`
		FooterText     string `json:"footer_text"`
		Tagline        string `json:"tagline"`
	}
	if err := decodeJSON(w, r, &input, 16*1024); err != nil {
		writeAPIError(w, http.StatusBadRequest, "VALIDATION", "Data sekolah tidak valid.")
		return
	}
	name, err := trimmed(input.Name, 160, "nama sekolah")
	if err != nil {
		writeStoreError(w, err)
		return
	}
	address, err := trimmedOptional(input.Address, 255, "alamat")
	if err != nil {
		writeStoreError(w, err)
		return
	}
	email, err := trimmedOptional(input.Email, 160, "email")
	if err != nil || !basicEmailValid(email) {
		writeAPIError(w, http.StatusBadRequest, "VALIDATION", "Alamat email sekolah tidak valid.")
		return
	}
	phone, err := trimmedOptional(input.Phone, 50, "nomor telepon")
	if err != nil {
		writeStoreError(w, err)
		return
	}
	website, err := trimmedOptional(input.Website, 255, "website")
	if err != nil || !basicWebsiteValid(website) {
		writeAPIError(w, http.StatusBadRequest, "VALIDATION", "Website harus diawali http:// atau https://.")
		return
	}
	if !models.ValidTheme(input.Theme) {
		writeAPIError(w, http.StatusBadRequest, "VALIDATION", "Tema visual tidak dikenal.")
		return
	}
	if !hexColor.MatchString(input.PrimaryColor) || !hexColor.MatchString(input.SecondaryColor) {
		writeAPIError(w, http.StatusBadRequest, "VALIDATION", "Warna harus dalam format hex seperti #7a4f2a.")
		return
	}
	footer, err := trimmedOptional(input.FooterText, 500, "footer")
	if err != nil {
		writeStoreError(w, err)
		return
	}
	tagline, err := trimmedOptional(input.Tagline, 200, "tagline")
	if err != nil {
		writeStoreError(w, err)
		return
	}

	update := models.SchoolInput{
		Name:           name,
		LogoPath:       school.LogoPath,
		Address:        address,
		Email:          email,
		Phone:          phone,
		Website:        website,
		Theme:          input.Theme,
		PrimaryColor:   strings.ToLower(input.PrimaryColor),
		SecondaryColor: strings.ToLower(input.SecondaryColor),
		FooterText:     footer,
		Tagline:        tagline,
	}
	if err := s.st.UpdateSchool(r.Context(), school.ID, update); err != nil {
		writeStoreError(w, err)
		return
	}
	s.audit(r, "SCHOOL_UPDATE", "school", &school.ID, map[string]any{"theme": input.Theme})
	updated, err := s.st.GetSchool(r.Context(), school.ID)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeData(w, http.StatusOK, map[string]any{"school": updated})
}

func (s *Server) apiAdminSchoolLogo(w http.ResponseWriter, r *http.Request) {
	sess := currentSession(r)
	school, err := s.st.GetSchool(r.Context(), sess.Admin.SchoolID)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4<<20)
	if err := r.ParseMultipartForm(2 << 20); err != nil {
		writeAPIError(w, http.StatusBadRequest, "VALIDATION", "Upload logo tidak valid.")
		return
	}
	file, header, err := r.FormFile("logo")
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "VALIDATION", "Pilih file logo terlebih dahulu.")
		return
	}
	defer file.Close()
	if header.Size > upload.MaxImageBytes {
		writeAPIError(w, http.StatusBadRequest, "VALIDATION", "Ukuran logo maksimal 3 MB.")
		return
	}
	data, err := io.ReadAll(io.LimitReader(file, upload.MaxImageBytes+1))
	if err != nil || len(data) > upload.MaxImageBytes {
		writeAPIError(w, http.StatusBadRequest, "VALIDATION", "Ukuran logo maksimal 3 MB.")
		return
	}
	path, err := upload.Save(data, s.cfg.UploadDir, "school", 800, true)
	if err != nil {
		if errors.Is(err, upload.ErrInvalidImage) {
			writeAPIError(w, http.StatusBadRequest, "VALIDATION", "Logo harus berupa JPG, PNG, atau WebP valid.")
			return
		}
		writeStoreError(w, err)
		return
	}
	update := models.SchoolInput{
		Name:           school.Name,
		LogoPath:       path,
		Address:        school.Address,
		Email:          school.Email,
		Phone:          school.Phone,
		Website:        school.Website,
		Theme:          school.Theme,
		PrimaryColor:   school.PrimaryColor,
		SecondaryColor: school.SecondaryColor,
		FooterText:     school.FooterText,
		Tagline:        school.Tagline,
	}
	if err := s.st.UpdateSchool(r.Context(), school.ID, update); err != nil {
		s.removeUpload(path)
		writeStoreError(w, err)
		return
	}
	if school.LogoPath != "" {
		s.removeUpload(school.LogoPath)
	}
	s.audit(r, "SCHOOL_LOGO", "school", &school.ID, nil)
	writeData(w, http.StatusOK, map[string]any{"logo_path": path})
}

func (s *Server) apiAdminSchoolLogoDelete(w http.ResponseWriter, r *http.Request) {
	sess := currentSession(r)
	school, err := s.st.GetSchool(r.Context(), sess.Admin.SchoolID)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	update := models.SchoolInput{
		Name:           school.Name,
		LogoPath:       "",
		Address:        school.Address,
		Email:          school.Email,
		Phone:          school.Phone,
		Website:        school.Website,
		Theme:          school.Theme,
		PrimaryColor:   school.PrimaryColor,
		SecondaryColor: school.SecondaryColor,
		FooterText:     school.FooterText,
		Tagline:        school.Tagline,
	}
	if err := s.st.UpdateSchool(r.Context(), school.ID, update); err != nil {
		writeStoreError(w, err)
		return
	}
	if school.LogoPath != "" {
		s.removeUpload(school.LogoPath)
	}
	s.audit(r, "SCHOOL_LOGO", "school", &school.ID, map[string]any{"removed": true})
	writeData(w, http.StatusOK, map[string]any{"logo_path": ""})
}
