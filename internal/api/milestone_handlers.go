package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/rusik69/govnohub/internal/issue"
)

func (s *Server) handleListMilestones(w http.ResponseWriter, r *http.Request) {
	repository, ok := s.getRepo(w, r)
	if !ok {
		return
	}
	ms, err := s.issues.ListMilestones(r.Context(), repository.ID)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if ms == nil {
		ms = []issue.Milestone{}
	}
	jsonOK(w, ms)
}

func (s *Server) handleCreateMilestone(w http.ResponseWriter, r *http.Request) {
	repository, ok := s.getRepoWrite(w, r)
	if !ok {
		return
	}
	var req struct {
		Title       string `json:"title"`
		Description string `json:"description"`
		DueOn       string `json:"due_on"`
	}
	json.NewDecoder(r.Body).Decode(&req)
	var due *time.Time
	if req.DueOn != "" {
		if t, err := time.Parse(time.RFC3339, req.DueOn); err == nil {
			due = &t
		}
	}
	m, err := s.issues.CreateMilestone(r.Context(), repository.ID, req.Title, req.Description, due)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	jsonOK(w, m)
}

func (s *Server) handleCloseMilestone(w http.ResponseWriter, r *http.Request) {
	repository, ok := s.getRepoWrite(w, r)
	if !ok {
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "milestoneID"))
	if err != nil {
		jsonError(w, http.StatusBadRequest, "invalid milestone id")
		return
	}
	if _, err := s.issues.GetMilestone(r.Context(), repository.ID, id); err != nil {
		jsonError(w, http.StatusNotFound, "milestone not found")
		return
	}
	if err := s.issues.CloseMilestone(r.Context(), id); err != nil {
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	jsonOK(w, map[string]string{"status": "closed"})
}

func (s *Server) handlePatchIssue(w http.ResponseWriter, r *http.Request) {
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
		Assignee   *string    `json:"assignee"`
		Milestone  *uuid.UUID `json:"milestone_id"`
		ClearMilestone bool   `json:"clear_milestone"`
	}
	json.NewDecoder(r.Body).Decode(&req)
	if req.Assignee != nil {
		var assigneeID *uuid.UUID
		if *req.Assignee != "" {
			u, err := s.auth.GetUserByUsername(r.Context(), *req.Assignee)
			if err != nil {
				jsonError(w, http.StatusBadRequest, "assignee not found")
				return
			}
			assigneeID = &u.ID
			if i.AssigneeID == nil || *i.AssigneeID != u.ID {
				s.notify.NotifyAsync(u.ID, "Issue assigned",
					"You were assigned to #"+strconv.Itoa(num)+" in "+repository.FullName,
					"/"+repository.OwnerName+"/"+repository.Name+"/issues/"+strconv.Itoa(num))
			}
		}
		if err := s.issues.SetAssignee(r.Context(), i.ID, assigneeID); err != nil {
			jsonError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if assigneeID != nil {
			s.issues.RecordEvent(r.Context(), i.ID, userIDFrom(r.Context()), "assigned", map[string]interface{}{
				"assignee": *req.Assignee,
			})
		} else {
			s.issues.RecordEvent(r.Context(), i.ID, userIDFrom(r.Context()), "unassigned", nil)
		}
	}
	if req.ClearMilestone {
		if err := s.issues.SetMilestone(r.Context(), i.ID, nil); err != nil {
			jsonError(w, http.StatusInternalServerError, err.Error())
			return
		}
		s.issues.RecordEvent(r.Context(), i.ID, userIDFrom(r.Context()), "demilestoned", nil)
	} else if req.Milestone != nil {
		if err := s.issues.SetMilestone(r.Context(), i.ID, req.Milestone); err != nil {
			jsonError(w, http.StatusInternalServerError, err.Error())
			return
		}
		m, _ := s.issues.GetMilestone(r.Context(), repository.ID, *req.Milestone)
		if m != nil {
			s.issues.RecordEvent(r.Context(), i.ID, userIDFrom(r.Context()), "milestoned", map[string]interface{}{
				"milestone_title": m.Title,
			})
		}
	}
	updated, _ := s.issues.Get(r.Context(), repository.ID, num)
	jsonOK(w, updated)
}

func (s *Server) handleAddIssueLabel(w http.ResponseWriter, r *http.Request) {
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
	labelID, err := uuid.Parse(chi.URLParam(r, "labelID"))
	if err != nil {
		jsonError(w, http.StatusBadRequest, "invalid label id")
		return
	}
	if err := s.issues.AddLabel(r.Context(), i.ID, labelID); err != nil {
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	label, _ := s.issues.GetLabelByID(r.Context(), labelID)
	if label != nil {
		s.issues.RecordEvent(r.Context(), i.ID, userIDFrom(r.Context()), "labeled", map[string]interface{}{
			"label_name": label.Name,
			"label_color": label.Color,
		})
	}
	updated, _ := s.issues.Get(r.Context(), repository.ID, num)
	jsonOK(w, updated)
}

func (s *Server) handleRemoveIssueLabel(w http.ResponseWriter, r *http.Request) {
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
	labelID, err := uuid.Parse(chi.URLParam(r, "labelID"))
	if err != nil {
		jsonError(w, http.StatusBadRequest, "invalid label id")
		return
	}
	label, _ := s.issues.GetLabelByID(r.Context(), labelID)
	if err := s.issues.RemoveLabel(r.Context(), i.ID, labelID); err != nil {
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if label != nil {
		s.issues.RecordEvent(r.Context(), i.ID, userIDFrom(r.Context()), "unlabeled", map[string]interface{}{
			"label_name": label.Name,
			"label_color": label.Color,
		})
	}
	updated, _ := s.issues.Get(r.Context(), repository.ID, num)
	jsonOK(w, updated)
}

func (s *Server) handleGetIssueTimeline(w http.ResponseWriter, r *http.Request) {
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
	events, err := s.issues.GetTimeline(r.Context(), i.ID)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if events == nil {
		events = []issue.TimelineEvent{}
	}
	jsonOK(w, events)
}
