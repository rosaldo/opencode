package server

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"
)

// ShareResponse is returned when a session is shared.
type ShareResponse struct {
	ID        string `json:"id"`
	SessionID string `json:"session_id"`
	URL       string `json:"url"`
}

// handleShare creates a share link for a session.
// POST /api/share {"session_id": "..."}
func (s *Server) handleShare(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		SessionID string `json:"session_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.SessionID == "" {
		http.Error(w, "missing session_id", http.StatusBadRequest)
		return
	}
	if s.cfg.Sessions != nil {
		if _, err := s.cfg.Sessions.Get(r.Context(), body.SessionID); err != nil {
			http.Error(w, "unknown session", http.StatusNotFound)
			return
		}
	}
	id := newShareID()
	s.share.create(id, body.SessionID)
	writeJSON(w, http.StatusOK, ShareResponse{
		ID:        id,
		SessionID: body.SessionID,
		URL:       s.URL() + "/s/" + id,
	})
}

// handleShareByID serves the shared session payload, and DELETE invalidates it.
// GET/DELETE /api/share/{id}
func (s *Server) handleShareByID(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/share/")
	if id == "" {
		http.Error(w, "missing share id", http.StatusBadRequest)
		return
	}
	sessionID, ok := s.share.get(id)
	if !ok {
		http.Error(w, "share not found", http.StatusNotFound)
		return
	}
	switch r.Method {
	case http.MethodDelete:
		s.share.remove(id)
		writeJSON(w, http.StatusOK, map[string]string{"status": "unshared"})
	case http.MethodGet:
		if s.cfg.Sessions == nil || s.cfg.Messages == nil {
			http.Error(w, "sessions unavailable", http.StatusServiceUnavailable)
			return
		}
		sess, err := s.cfg.Sessions.Get(r.Context(), sessionID)
		if err != nil {
			http.Error(w, "session gone", http.StatusNotFound)
			return
		}
		msgs, err := s.cfg.Messages.List(r.Context(), sessionID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"session": sess, "messages": msgs})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func newShareID() string {
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
