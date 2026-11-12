package httpapi

import (
	"net/http"
	"net/url"
	"time"

	"github.com/gorilla/websocket"

	"govoting/internal/ws"
)

func (s *Server) wsAdminResults(w http.ResponseWriter, r *http.Request) {
	election, err := s.selectedElectionForAPI(r, 0)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	sess := currentSession(r)
	if sess == nil || sess.Admin == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	payload, err := s.buildLivePayload(r, election)
	if err != nil {
		writeStoreError(w, err)
		return
	}

	upgrader := websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 4096,
		CheckOrigin: func(r *http.Request) bool {
			origin := r.Header.Get("Origin")
			if origin == "" {
				return true
			}
			u, err := url.Parse(origin)
			if err != nil {
				return false
			}
			return sameHost(u.Host, r.Host)
		},
	}
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	conn.SetReadLimit(1024)
	client := s.hub.NewClient(conn, sess.Admin.ID, sess.Admin.SchoolID, election.ID)
	if !s.hub.Register(client) {
		_ = conn.WriteControl(websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseTryAgainLater, "terlalu banyak koneksi"),
			time.Now().Add(2*time.Second))
		_ = conn.Close()
		return
	}
	s.hub.Send(client, ws.Event{Event: "snapshot", Data: payload})
}
