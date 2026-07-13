package api

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/rusik69/govnohub/internal/issue"
)

func (s *Server) handleAddIssueReaction(w http.ResponseWriter, r *http.Request) {
	repository, ok := s.getRepoWrite(w, r)
	if !ok {
		return
	}
	num, ok := parseNumber(w, r, "number")
	if !ok {
		return
	}
	i, err := s.issues.Get(r.Context(), repository.ID, num)
	if err != nil {
		jsonError(w, http.StatusNotFound, err.Error())
		return
	}
	var req struct {
		Content   string `json:"content"`
		CommentID string `json:"comment_id,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, http.StatusBadRequest, "invalid body")
		return
	}
	if !issue.ValidReactionContent(req.Content) {
		jsonError(w, http.StatusBadRequest, "invalid reaction content; valid values: +1, -1, laugh, confused, heart, hooray, rocket, eyes")
		return
	}
	var commentID *uuid.UUID
	if req.CommentID != "" {
		id, err := uuid.Parse(req.CommentID)
		if err != nil {
			jsonError(w, http.StatusBadRequest, "invalid comment_id")
			return
		}
		commentID = &id
	}
	reaction, err := s.issues.AddReaction(r.Context(), i.ID, userIDFrom(r.Context()), commentID, req.Content)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.issues.RecordEvent(r.Context(), i.ID, userIDFrom(r.Context()), "reaction_added", map[string]interface{}{
		"reaction_id": reaction.ID.String(),
		"content":     reaction.Content,
	})
	jsonOK(w, reaction)
}

func (s *Server) handleListIssueReactions(w http.ResponseWriter, r *http.Request) {
	repository, ok := s.getRepo(w, r)
	if !ok {
		return
	}
	num, ok := parseNumber(w, r, "number")
	if !ok {
		return
	}
	i, err := s.issues.Get(r.Context(), repository.ID, num)
	if err != nil {
		jsonError(w, http.StatusNotFound, err.Error())
		return
	}
	// Parse optional comment_id query param
	var commentID *uuid.UUID
	if cid := r.URL.Query().Get("comment_id"); cid != "" {
		id, err := uuid.Parse(cid)
		if err != nil {
			jsonError(w, http.StatusBadRequest, "invalid comment_id")
			return
		}
		commentID = &id
	}
	reactions, err := s.issues.ListReactions(r.Context(), i.ID, commentID)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if reactions == nil {
		reactions = []issue.Reaction{}
	}
	jsonOK(w, reactions)
}

func (s *Server) handleDeleteIssueReaction(w http.ResponseWriter, r *http.Request) {
	repository, ok := s.getRepoWrite(w, r)
	if !ok {
		return
	}
	num, ok := parseNumber(w, r, "number")
	if !ok {
		return
	}
	_, err := s.issues.Get(r.Context(), repository.ID, num)
	if err != nil {
		jsonError(w, http.StatusNotFound, err.Error())
		return
	}
	reactionID, err := uuid.Parse(chi.URLParam(r, "reactionID"))
	if err != nil {
		jsonError(w, http.StatusBadRequest, "invalid reaction id")
		return
	}
	if err := s.issues.DeleteReaction(r.Context(), reactionID, userIDFrom(r.Context())); err != nil {
		jsonError(w, http.StatusNotFound, err.Error())
		return
	}
	jsonOK(w, map[string]string{"status": "deleted"})
}
