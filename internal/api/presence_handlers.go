package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

// handlePresenceHeartbeat reports that the authenticated user is viewing a resource.
func (s *Server) handlePresenceHeartbeat(w http.ResponseWriter, r *http.Request) {
	userID := userIDFrom(r.Context())
	user, err := s.auth.GetUser(r.Context(), userID)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, "user not found")
		return
	}
	resource := chi.URLParam(r, "owner") + "/" + chi.URLParam(r, "repo")
	s.presence.Heartbeat(userID.String(), user.Username, resource)
	jsonOK(w, map[string]string{"status": "ok"})
}

// handlePresence returns the list of active viewers for a repository resource.
func (s *Server) handlePresence(w http.ResponseWriter, r *http.Request) {
	resource := chi.URLParam(r, "owner") + "/" + chi.URLParam(r, "repo")
	viewers := s.presence.GetActiveViewers(resource)
	if viewers == nil {
		viewers = []string{}
	}
	jsonOK(w, map[string]interface{}{
		"viewers": viewers,
		"count":   len(viewers),
	})
}
