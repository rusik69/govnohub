package api

import (
	"context"
	"net/http"

	"github.com/rusik69/govnohub/internal/aireview"
)

func (s *Server) handleAIReviewConfig(w http.ResponseWriter, r *http.Request) {
	if s.aiReview == nil || !s.aiReview.Client().Enabled() {
		jsonOK(w, map[string]any{"enabled": false})
		return
	}
	jsonOK(w, map[string]any{
		"enabled": true,
		"model":   s.aiReview.Client().DefaultModel(),
		"auto":    s.aiReview.Client().Auto(),
	})
}

func (s *Server) handleListAIReviews(w http.ResponseWriter, r *http.Request) {
	if s.aiReview == nil {
		jsonOK(w, []any{})
		return
	}
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
	reviews, err := s.aiReview.List(r.Context(), pr.ID)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if reviews == nil {
		jsonOK(w, []any{})
		return
	}
	jsonOK(w, reviews)
}

func (s *Server) handleCreateAIReview(w http.ResponseWriter, r *http.Request) {
	if s.aiReview == nil || !s.aiReview.Client().Enabled() {
		jsonError(w, http.StatusServiceUnavailable, "ai review is not configured")
		return
	}
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

	baseSHA, err := s.git.UpdateHead(repository.OwnerName, repository.Name, pr.BaseBranch)
	if err != nil {
		jsonError(w, http.StatusBadRequest, "invalid base branch")
		return
	}
	diff, err := s.git.Diff(repository.OwnerName, repository.Name, baseSHA, pr.HeadSHA)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}

	model := s.aiReview.Client().DefaultModel()
	body, err := s.aiReview.Client().Review(r.Context(), aireview.ReviewInput{
		Title: pr.Title,
		Body:  pr.Body,
		Diff:  diff,
		Model: model,
	})
	if err != nil {
		jsonError(w, http.StatusBadGateway, err.Error())
		return
	}
	review, err := s.aiReview.Save(r.Context(), pr.ID, model, body)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	jsonOK(w, review)
}

func (s *Server) runAutoAIReview(owner, repo string, prNumber int) {
	if s.aiReview == nil || !s.aiReview.Client().Enabled() || !s.aiReview.Client().Auto() {
		return
	}
	go func() {
		ctx := context.Background()
		repository, err := s.repos.GetByFullName(ctx, owner, repo)
		if err != nil {
			return
		}
		pr, err := s.pulls.Get(ctx, repository.ID, prNumber)
		if err != nil {
			return
		}
		baseSHA, err := s.git.UpdateHead(repository.OwnerName, repository.Name, pr.BaseBranch)
		if err != nil {
			return
		}
		diff, err := s.git.Diff(repository.OwnerName, repository.Name, baseSHA, pr.HeadSHA)
		if err != nil {
			return
		}
		model := s.aiReview.Client().DefaultModel()
		body, err := s.aiReview.Client().Review(ctx, aireview.ReviewInput{
			Title: pr.Title,
			Body:  pr.Body,
			Diff:  diff,
			Model: model,
		})
		if err != nil {
			return
		}
		_, _ = s.aiReview.Save(ctx, pr.ID, model, body)
	}()
}
