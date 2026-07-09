package api

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/rusik69/govnohub/internal/repo"
)

func (s *Server) handleListCollaborators(w http.ResponseWriter, r *http.Request) {
	repository, ok := s.getRepo(w, r)
	if !ok {
		return
	}
	collabs, err := s.repos.ListCollaborators(r.Context(), repository.ID)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if collabs == nil {
		collabs = []repo.Collaborator{}
	}
	jsonOK(w, collabs)
}

func (s *Server) handleAddCollaborator(w http.ResponseWriter, r *http.Request) {
	repository, ok := s.getRepoWrite(w, r)
	if !ok {
		return
	}
	username := chi.URLParam(r, "username")
	u, err := s.auth.GetUserByUsername(r.Context(), username)
	if err != nil {
		jsonError(w, http.StatusNotFound, "user not found")
		return
	}
	var req struct {
		Permission string `json:"permission"`
	}
	json.NewDecoder(r.Body).Decode(&req)
	if err := s.repos.AddCollaborator(r.Context(), repository.ID, u.ID, req.Permission); err != nil {
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	jsonOK(w, map[string]string{"status": "added", "username": username})
}

func (s *Server) handleRemoveCollaborator(w http.ResponseWriter, r *http.Request) {
	repository, ok := s.getRepoWrite(w, r)
	if !ok {
		return
	}
	username := chi.URLParam(r, "username")
	u, err := s.auth.GetUserByUsername(r.Context(), username)
	if err != nil {
		jsonError(w, http.StatusNotFound, "user not found")
		return
	}
	if err := s.repos.RemoveCollaborator(r.Context(), repository.ID, u.ID); err != nil {
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	jsonOK(w, map[string]string{"status": "removed"})
}

func (s *Server) handleListProtectedBranches(w http.ResponseWriter, r *http.Request) {
	repository, ok := s.getRepo(w, r)
	if !ok {
		return
	}
	rules, err := s.repos.ListProtectedBranches(r.Context(), repository.ID)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if rules == nil {
		rules = []repo.ProtectedBranch{}
	}
	jsonOK(w, rules)
}
