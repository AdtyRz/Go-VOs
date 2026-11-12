package httpapi

import (
	"net/http"
)

func (s *Server) apiAdminAudit(w http.ResponseWriter, r *http.Request) {
	sess := currentSession(r)
	pageNum := parsePage(r)
	logs, total, err := s.st.ListAudit(r.Context(), sess.Admin.SchoolID, pageNum, 50)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	pages := int((total + 49) / 50)
	if pages < 1 {
		pages = 1
	}
	writeData(w, http.StatusOK, map[string]any{
		"logs":  logs,
		"total": total,
		"page":  pageNum,
		"pages": pages,
	})
}
