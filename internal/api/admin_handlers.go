package api

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/rusik69/govnohub/internal/auth"
)

func (s *Server) registerAdminRoutes(r chi.Router) {
	r.Route("/admin", func(r chi.Router) {
		r.Use(s.requireAdmin)
		r.Get("/users", s.handleListUsers)
		r.Post("/users", s.handleAdminCreateUser)
		r.Delete("/users/{userID}", s.handleDeleteUser)
	})
}

func (s *Server) requireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isPATAuth(r.Context()) {
			jsonError(w, http.StatusForbidden, "admin access requires account login")
			return
		}
		ok, err := s.auth.IsAdmin(r.Context(), userIDFrom(r.Context()))
		if err != nil {
			jsonError(w, http.StatusUnauthorized, err.Error())
			return
		}
		if !ok {
			jsonError(w, http.StatusForbidden, "admin access required")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) handleListUsers(w http.ResponseWriter, r *http.Request) {
	users, err := s.auth.ListUsers(r.Context())
	if err != nil {
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if users == nil {
		users = []auth.User{}
	}
	jsonOK(w, users)
}

func (s *Server) handleAdminCreateUser(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username string `json:"username"`
		Email    string `json:"email"`
		Password string `json:"password"`
		Role     string `json:"role"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, http.StatusBadRequest, "invalid body")
		return
	}
	if req.Username == "" || req.Email == "" || req.Password == "" {
		jsonError(w, http.StatusBadRequest, "username, email, and password are required")
		return
	}
	role := req.Role
	if role == "" {
		role = auth.RoleUser
	}
	u, err := s.auth.CreateUser(r.Context(), req.Username, req.Email, req.Password, role)
	if err != nil {
		if err == auth.ErrUserExists {
			jsonError(w, http.StatusConflict, err.Error())
			return
		}
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	jsonOK(w, u)
}

func (s *Server) handleDeleteUser(w http.ResponseWriter, r *http.Request) {
	targetID, err := uuid.Parse(chi.URLParam(r, "userID"))
	if err != nil {
		jsonError(w, http.StatusBadRequest, "invalid user id")
		return
	}
	if err := s.auth.DeleteUser(r.Context(), userIDFrom(r.Context()), targetID); err != nil {
		switch err {
		case auth.ErrForbidden:
			jsonError(w, http.StatusForbidden, err.Error())
		case auth.ErrUnauthorized:
			jsonError(w, http.StatusNotFound, "user not found")
		default:
			jsonError(w, http.StatusInternalServerError, err.Error())
		}
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
