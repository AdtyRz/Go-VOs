package httpapi

import (
	"errors"
	"io"
	"net/http"
	"strconv"

	"govoting/internal/models"
	"govoting/internal/store"
	"govoting/internal/upload"
)

func (s *Server) apiAdminCandidates(w http.ResponseWriter, r *http.Request) {
	election, err := s.selectedElectionForAPI(r, 0)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	candidates, err := s.st.ListCandidates(r.Context(), election.ID, false)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeData(w, http.StatusOK, map[string]any{
		"election":   election,
		"candidates": candidates,
		"can_edit":   election.IsEditable(),
	})
}

func (s *Server) selectedElectionForAPI(r *http.Request, fallbackID int64) (*models.Election, error) {
	sess := currentSession(r)
	if sess == nil || sess.Admin == nil {
		return nil, store.ErrNotFound
	}
	id, err := s.electionIDFromRequest(r, fallbackID)
	if err != nil {
		return nil, err
	}
	return s.st.GetElection(r.Context(), sess.Admin.SchoolID, id)
}

func (s *Server) apiAdminCandidateCreate(w http.ResponseWriter, r *http.Request) {
	input, election, newPhoto, err := s.readCandidateRequest(w, r, nil)
	if err != nil {
		s.handleCandidateError(w, newPhoto, err)
		return
	}
	id, err := s.st.CreateCandidate(r.Context(), election.SchoolID, election.ID, input)
	if err != nil {
		s.removeUpload(newPhoto)
		writeStoreError(w, err)
		return
	}
	s.audit(r, "CANDIDATE_CREATE", "candidate", &id, map[string]any{
		"election_id":      election.ID,
		"candidate_number": input.CandidateNumber,
		"chairman":         input.ChairmanName,
	})
	candidate, err := s.st.GetCandidate(r.Context(), election.ID, id)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeData(w, http.StatusCreated, map[string]any{"candidate": candidate})
}

func (s *Server) apiAdminCandidateUpdate(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(r.PathValue("id"))
	if !ok {
		writeAPIError(w, http.StatusBadRequest, "VALIDATION", "ID kandidat tidak valid.")
		return
	}
	election, err := s.selectedElectionForAPI(r, 0)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	existing, err := s.st.GetCandidate(r.Context(), election.ID, id)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	input, _, newPhoto, err := s.readCandidateRequest(w, r, existing)
	if err != nil {
		s.handleCandidateError(w, newPhoto, err)
		return
	}
	if err := s.st.UpdateCandidate(r.Context(), election.SchoolID, election.ID, id, input); err != nil {
		s.removeUpload(newPhoto)
		writeStoreError(w, err)
		return
	}
	if existing.PhotoPath != "" && existing.PhotoPath != input.PhotoPath {
		s.removeUpload(existing.PhotoPath)
	}
	s.audit(r, "CANDIDATE_UPDATE", "candidate", &id, map[string]any{
		"election_id":      election.ID,
		"candidate_number": input.CandidateNumber,
		"chairman":         input.ChairmanName,
	})
	candidate, err := s.st.GetCandidate(r.Context(), election.ID, id)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeData(w, http.StatusOK, map[string]any{"candidate": candidate})
}

func (s *Server) apiAdminCandidateDelete(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(r.PathValue("id"))
	if !ok {
		writeAPIError(w, http.StatusBadRequest, "VALIDATION", "ID kandidat tidak valid.")
		return
	}
	election, err := s.selectedElectionForAPI(r, 0)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	candidate, err := s.st.GetCandidate(r.Context(), election.ID, id)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	if err := s.st.DeleteCandidate(r.Context(), election.SchoolID, election.ID, id); err != nil {
		writeStoreError(w, err)
		return
	}
	if candidate.PhotoPath != "" {
		s.removeUpload(candidate.PhotoPath)
	}
	s.audit(r, "CANDIDATE_DELETE", "candidate", &id, map[string]any{
		"election_id":      election.ID,
		"candidate_number": candidate.CandidateNumber,
	})
	writeData(w, http.StatusOK, map[string]any{"deleted": true})
}

func (s *Server) readCandidateRequest(w http.ResponseWriter, r *http.Request, existing *models.Candidate) (models.CandidateInput, *models.Election, string, error) {
	var input models.CandidateInput
	r.Body = http.MaxBytesReader(w, r.Body, 6<<20)
	if err := r.ParseMultipartForm(4 << 20); err != nil {
		return input, nil, "", store.ErrInvalid
	}
	electionID, err := s.electionIDFromRequest(r, 0)
	if err != nil {
		return input, nil, "", err
	}
	election, err := s.adminElection(r, strconv.FormatInt(electionID, 10))
	if err != nil {
		return input, nil, "", err
	}
	if !election.IsEditable() {
		return input, nil, "", store.ErrElectionLocked
	}

	number, err := strconv.Atoi(r.FormValue("candidate_number"))
	if err != nil || number < 1 || number > 99 {
		return input, nil, "", errors.New("nomor urut harus berupa angka 1-99")
	}
	chairman, err := trimmed(r.FormValue("chairman_name"), 140, "nama ketua")
	if err != nil {
		return input, nil, "", err
	}
	vice, err := trimmed(r.FormValue("vice_chairman_name"), 140, "nama wakil ketua")
	if err != nil {
		return input, nil, "", err
	}
	vision, err := trimmed(r.FormValue("vision"), 4000, "visi")
	if err != nil {
		return input, nil, "", err
	}
	mission, err := trimmed(r.FormValue("mission"), 4000, "misi")
	if err != nil {
		return input, nil, "", err
	}

	input = models.CandidateInput{
		CandidateNumber:  number,
		ChairmanName:     chairman,
		ViceChairmanName: vice,
		Vision:           vision,
		Mission:          mission,
		IsActive:         r.FormValue("is_active") == "true" || r.FormValue("is_active") == "on" || r.FormValue("is_active") == "1",
	}
	if existing != nil {
		input.PhotoPath = existing.PhotoPath
	}
	if r.FormValue("remove_photo") == "true" {
		input.PhotoPath = ""
	}

	newPhoto := ""
	file, header, err := r.FormFile("photo")
	if err == nil {
		defer file.Close()
		if header.Size > upload.MaxImageBytes {
			return input, election, "", upload.ErrInvalidImage
		}
		data, err := io.ReadAll(io.LimitReader(file, upload.MaxImageBytes+1))
		if err != nil || len(data) > upload.MaxImageBytes {
			return input, election, "", upload.ErrInvalidImage
		}
		path, err := upload.Save(data, s.cfg.UploadDir, "candidates", 1600, false)
		if err != nil {
			return input, election, "", err
		}
		newPhoto = path
		input.PhotoPath = path
	} else if !errors.Is(err, http.ErrMissingFile) {
		return input, election, "", err
	}
	return input, election, newPhoto, nil
}

func (s *Server) handleCandidateError(w http.ResponseWriter, newPhoto string, err error) {
	s.removeUpload(newPhoto)
	if errors.Is(err, upload.ErrInvalidImage) || errors.Is(err, store.ErrInvalid) {
		msg := "Data pasangan calon tidak valid."
		if errors.Is(err, upload.ErrInvalidImage) {
			msg = "Foto harus berupa JPG, PNG, atau WebP valid dengan ukuran maksimal 3 MB."
		} else if err.Error() != store.ErrInvalid.Error() {
			msg = cleanError(err)
		}
		writeAPIError(w, http.StatusBadRequest, "VALIDATION", msg)
		return
	}
	if err.Error() == "nomor urut harus berupa angka 1-99" {
		writeAPIError(w, http.StatusBadRequest, "VALIDATION", err.Error())
		return
	}
	writeStoreError(w, err)
}

func (s *Server) removeUpload(webPath string) {
	upload.Remove(s.cfg.UploadDir, webPath)
}
