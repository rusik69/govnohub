package api

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/rusik69/govnohub/internal/auth"
)

func (s *Server) handleCreateSSHKey(w http.ResponseWriter, r *http.Request) {
	if !s.requireJWT(w, r) {
		return
	}
	var req struct {
		Title string `json:"title"`
		Key   string `json:"key"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, http.StatusBadRequest, "invalid body")
		return
	}
	key, err := s.auth.AddSSHKey(r.Context(), userIDFrom(r.Context()), req.Title, req.Key)
	if err != nil {
		switch err {
		case auth.ErrInvalidSSHKey:
			jsonError(w, http.StatusBadRequest, err.Error())
		case auth.ErrDuplicateSSHKey:
			jsonError(w, http.StatusConflict, err.Error())
		default:
			jsonError(w, http.StatusInternalServerError, err.Error())
		}
		return
	}
	jsonOK(w, key)
}

func (s *Server) handleListSSHKeys(w http.ResponseWriter, r *http.Request) {
	if !s.requireJWT(w, r) {
		return
	}
	keys, err := s.auth.ListSSHKeys(r.Context(), userIDFrom(r.Context()))
	if err != nil {
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	jsonOK(w, keys)
}

func (s *Server) handleDeleteSSHKey(w http.ResponseWriter, r *http.Request) {
	if !s.requireJWT(w, r) {
		return
	}
	keyID, err := uuid.Parse(chi.URLParam(r, "keyID"))
	if err != nil {
		jsonError(w, http.StatusBadRequest, "invalid key id")
		return
	}
	if err := s.auth.DeleteSSHKey(r.Context(), userIDFrom(r.Context()), keyID); err != nil {
		jsonError(w, http.StatusNotFound, err.Error())
		return
	}
	jsonOK(w, map[string]string{"status": "deleted"})
}

func (s *Server) requireJWT(w http.ResponseWriter, r *http.Request) bool {
	if isPATAuth(r.Context()) {
		jsonError(w, http.StatusForbidden, "use account login to manage SSH keys")
		return false
	}
	return true
}
