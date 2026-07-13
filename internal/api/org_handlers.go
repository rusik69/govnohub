package api

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/rusik69/govnohub/internal/org"
)

func (s *Server) WithOrg(orgSvc *org.Service) *Server {
	s.org = orgSvc
	return s
}

func (s *Server) registerOrgRoutes(r chi.Router) {
	r.Get("/orgs", s.handleListOrgs)
	r.Post("/orgs", s.handleCreateOrg)
	r.Post("/orgs/{org}/members", s.handleAddOrgMember)
	r.Post("/orgs/{org}/teams", s.handleCreateTeam)
	r.Post("/orgs/{org}/teams/{team}/members", s.handleAddTeamMember)
}

func (s *Server) handleListOrgs(w http.ResponseWriter, r *http.Request) {
	orgs, err := s.org.List(r.Context())
	if err != nil {
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	jsonOK(w, orgs)
}

func (s *Server) handleCreateOrg(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name        string `json:"name"`
		DisplayName string `json:"display_name"`
		Description string `json:"description"`
	}
	json.NewDecoder(r.Body).Decode(&req)
	o, err := s.org.Create(r.Context(), req.Name, req.DisplayName, req.Description)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := s.org.AddMember(r.Context(), o.ID, userIDFrom(r.Context()), "admin"); err != nil {
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	jsonOK(w, o)
}

func (s *Server) handleAddOrgMember(w http.ResponseWriter, r *http.Request) {
	ownerType, ownerID, err := s.repos.ResolveOwnerID(r.Context(), chi.URLParam(r, "org"))
	if err != nil || ownerType != "org" {
		jsonError(w, http.StatusNotFound, "org not found")
		return
	}
	var req struct {
		UserID uuid.UUID `json:"user_id"`
		Role   string    `json:"role"`
	}
	json.NewDecoder(r.Body).Decode(&req)
	if err := s.org.AddMember(r.Context(), ownerID, req.UserID, req.Role); err != nil {
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	jsonOK(w, map[string]string{"status": "added"})
}

func (s *Server) handleCreateTeam(w http.ResponseWriter, r *http.Request) {
	ownerType, ownerID, err := s.repos.ResolveOwnerID(r.Context(), chi.URLParam(r, "org"))
	if err != nil || ownerType != "org" {
		jsonError(w, http.StatusNotFound, "org not found")
		return
	}
	var req struct{ Name, Description string }
	json.NewDecoder(r.Body).Decode(&req)
	t, err := s.org.CreateTeam(r.Context(), ownerID, req.Name, req.Description)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	jsonOK(w, t)
}

func (s *Server) handleAddTeamMember(w http.ResponseWriter, r *http.Request) {
	var teamID uuid.UUID
	if err := s.pool.QueryRow(r.Context(), `
		SELECT t.id FROM teams t JOIN orgs o ON o.id=t.org_id
		WHERE o.name=$1 AND t.name=$2`, chi.URLParam(r, "org"), chi.URLParam(r, "team")).Scan(&teamID); err != nil {
		jsonError(w, http.StatusNotFound, "team not found")
		return
	}
	var req struct{ UserID uuid.UUID `json:"user_id"` }
	json.NewDecoder(r.Body).Decode(&req)
	if err := s.org.AddTeamMember(r.Context(), teamID, req.UserID); err != nil {
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	jsonOK(w, map[string]string{"status": "added"})
}
