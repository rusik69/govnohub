package api

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/rusik69/govnohub/internal/notification"
	"github.com/rusik69/govnohub/internal/pull"
	"github.com/rusik69/govnohub/internal/wiki"
)

func (s *Server) handleListWikiPages(w http.ResponseWriter, r *http.Request) {
	repository, ok := s.getRepo(w, r)
	if !ok {
		return
	}
	pages, err := s.wiki.List(r.Context(), repository.ID)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if pages == nil {
		pages = []wiki.Page{}
	}
	jsonOK(w, pages)
}

func (s *Server) handleGetWikiPage(w http.ResponseWriter, r *http.Request) {
	repository, ok := s.getRepo(w, r)
	if !ok {
		return
	}
	page, err := s.wiki.Get(r.Context(), repository.ID, chi.URLParam(r, "slug"))
	if err != nil {
		jsonError(w, http.StatusNotFound, err.Error())
		return
	}
	jsonOK(w, page)
}

func (s *Server) handleUpsertWikiPage(w http.ResponseWriter, r *http.Request) {
	repository, ok := s.getRepoWrite(w, r)
	if !ok {
		return
	}
	var req struct {
		Slug    string `json:"slug"`
		Title   string `json:"title"`
		Content string `json:"content"`
	}
	json.NewDecoder(r.Body).Decode(&req)
	slug := chi.URLParam(r, "slug")
	if slug == "" || slug == "new" {
		slug = req.Slug
	}
	page, err := s.wiki.Upsert(r.Context(), repository.ID, userIDFrom(r.Context()), slug, req.Title, req.Content)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	jsonOK(w, page)
}

func (s *Server) handleDeleteWikiPage(w http.ResponseWriter, r *http.Request) {
	repository, ok := s.getRepoWrite(w, r)
	if !ok {
		return
	}
	if err := s.wiki.Delete(r.Context(), repository.ID, chi.URLParam(r, "slug")); err != nil {
		jsonError(w, http.StatusNotFound, err.Error())
		return
	}
	jsonOK(w, map[string]string{"status": "deleted"})
}

func (s *Server) handleListPRComments(w http.ResponseWriter, r *http.Request) {
	repository, ok := s.getRepo(w, r)
	if !ok {
		return
	}
	num, ok := parseNumber(w, r, "number")
	if !ok {
		return
	}
	pr, err := s.pulls.Get(r.Context(), repository.ID, num)
	if err != nil {
		jsonError(w, http.StatusNotFound, err.Error())
		return
	}
	comments, err := s.pulls.ListComments(r.Context(), pr.ID)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if comments == nil {
		comments = []pull.LineComment{}
	}
	jsonOK(w, comments)
}

func (s *Server) handleAddPRComment(w http.ResponseWriter, r *http.Request) {
	repository, ok := s.getRepoWrite(w, r)
	if !ok {
		return
	}
	num, ok := parseNumber(w, r, "number")
	if !ok {
		return
	}
	pr, err := s.pulls.Get(r.Context(), repository.ID, num)
	if err != nil {
		jsonError(w, http.StatusNotFound, err.Error())
		return
	}
	var req struct {
		Body string `json:"body"`
		Path string `json:"path"`
		Line int    `json:"line"`
	}
	json.NewDecoder(r.Body).Decode(&req)
	c, err := s.pulls.AddComment(r.Context(), pr.ID, userIDFrom(r.Context()), req.Body, req.Path, req.Line)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if pr.AuthorID != userIDFrom(r.Context()) {
		s.notify.NotifyAsync(pr.AuthorID, "PR comment",
			"New comment on PR #"+strconv.Itoa(num)+" in "+repository.FullName,
			"/"+repository.OwnerName+"/"+repository.Name+"/pulls/"+strconv.Itoa(num))
	}
	jsonOK(w, c)
}

func (s *Server) handleListNotifications(w http.ResponseWriter, r *http.Request) {
	items, err := s.notify.List(r.Context(), userIDFrom(r.Context()), 30)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if items == nil {
		items = []notification.Notification{}
	}
	jsonOK(w, items)
}

func (s *Server) handleMarkNotificationRead(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		jsonError(w, http.StatusBadRequest, "invalid id")
		return
	}
	if err := s.notify.MarkRead(r.Context(), id, userIDFrom(r.Context())); err != nil {
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	jsonOK(w, map[string]string{"status": "read"})
}

func (s *Server) handleSetIssueAssignees(w http.ResponseWriter, r *http.Request) {
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
		Assignees []string `json:"assignees"`
	}
	json.NewDecoder(r.Body).Decode(&req)

	var userIDs []uuid.UUID
	for _, username := range req.Assignees {
		u, err := s.auth.GetUserByUsername(r.Context(), username)
		if err != nil {
			jsonError(w, http.StatusBadRequest, "assignee not found: "+username)
			return
		}
		userIDs = append(userIDs, u.ID)
	}

	if err := s.issues.SetAssignees(r.Context(), i.ID, userIDs); err != nil {
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Record events
	for _, username := range req.Assignees {
		s.issues.RecordEvent(r.Context(), i.ID, userIDFrom(r.Context()), "assigned", map[string]interface{}{
			"assignee": username,
		})
	}
	if len(req.Assignees) == 0 && i.AssigneeID != nil {
		s.issues.RecordEvent(r.Context(), i.ID, userIDFrom(r.Context()), "unassigned", nil)
	}

	// Notify new assignees
	for _, username := range req.Assignees {
		u, err := s.auth.GetUserByUsername(r.Context(), username)
		if err == nil && u.ID != userIDFrom(r.Context()) {
			s.notify.NotifyAsync(u.ID, "Issue assigned",
				"You were assigned to #"+strconv.Itoa(num)+" in "+repository.FullName,
				"/"+repository.OwnerName+"/"+repository.Name+"/issues/"+strconv.Itoa(num))
		}
	}

	updated, _ := s.issues.Get(r.Context(), repository.ID, num)
	jsonOK(w, updated)
}
