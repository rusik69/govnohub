package web

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

type Handler struct {
	deps Deps
}

func NewHandler(deps Deps) *Handler {
	return &Handler{deps: deps}
}

func (h *Handler) Routes() chi.Router {
	r := chi.NewRouter()

	r.Handle("/static/*", http.StripPrefix("/static/", staticHandler()))

	r.Group(func(r chi.Router) {
		r.Use(h.optionalAuth)
		r.Get("/login", h.handleLoginGet)
		r.Post("/login", h.handleLoginPost)
		r.Post("/logout", h.handleLogout)
	})

	r.Group(func(r chi.Router) {
		r.Use(h.optionalAuth, h.requireAuth)
		r.Get("/", h.handleDashboard)
		r.Get("/search", h.handleSearch)
		r.Get("/settings", h.handleSettings)
		r.Post("/settings/pat", h.handleCreatePAT)
		r.Post("/settings/pat/{id}/revoke", h.handleRevokePAT)
		r.Post("/settings/ssh-key", h.handleAddSSHKey)
		r.Post("/settings/ssh-key/{id}/delete", h.handleDeleteSSHKey)

		r.Get("/notifications", h.handleNotifications)
		r.Post("/notifications/{id}/read", h.handleMarkNotificationRead)

		r.Get("/orgs", h.handleOrgs)
		r.Post("/orgs", h.handleCreateOrg)
		r.Get("/orgs/{org}", h.handleOrgDetail)
		r.Post("/orgs/{org}/members", h.handleAddOrgMember)
		r.Get("/orgs/{org}/teams", h.handleOrgTeams)
		r.Post("/orgs/{org}/teams", h.handleCreateTeam)
		r.Get("/orgs/{org}/teams/{team}", h.handleTeamDetail)
		r.Post("/orgs/{org}/teams/{team}/members", h.handleAddTeamMember)

		r.Route("/admin", func(r chi.Router) {
			r.Use(h.requireAdmin)
			r.Get("/users", h.handleAdminUsers)
			r.Post("/users", h.handleAdminCreateUser)
			r.Post("/users/{id}/delete", h.handleAdminDeleteUser)
		})

		r.Route("/{owner}/{repo}", func(r chi.Router) {
			r.Get("/", h.handleRepo)
			r.Get("/tree/*", h.handleRepoTree)
			r.Get("/blob/*", h.handleRepoBlob)

			r.Get("/issues", h.handleIssues)
			r.Post("/issues", h.handleCreateIssue)
			r.Get("/issues/{number}", h.handleIssueDetail)
			r.Post("/issues/{number}/comment", h.handleIssueComment)
			r.Post("/issues/{number}/close", h.handleCloseIssue)
			r.Post("/issues/{number}/assignee", h.handleIssueAssignee)
			r.Post("/issues/{number}/milestone", h.handleIssueMilestone)
			r.Post("/issues/{number}/labels/{labelID}", h.handleIssueAddLabel)
			r.Post("/issues/{number}/labels/{labelID}/remove", h.handleIssueRemoveLabel)

			r.Get("/pulls", h.handlePulls)
			r.Post("/pulls", h.handleCreatePR)
			r.Get("/pulls/{number}", h.handlePRDetail)
			r.Post("/pulls/{number}/comment", h.handlePRComment)
			r.Post("/pulls/{number}/review", h.handlePRReview)
			r.Post("/pulls/{number}/merge", h.handlePRMerge)

			r.Get("/actions", h.handleActions)
			r.Get("/actions/runs/{runID}/logs", h.handleActionLogs)

			r.Get("/releases", h.handleReleases)
			r.Post("/releases", h.handleCreateRelease)

			r.Get("/packages", h.handlePackages)

			r.Get("/settings", h.handleRepoSettings)
			r.Post("/settings/webhook", h.handleCreateWebhook)

			r.Get("/wiki", h.handleWiki)
			r.Get("/wiki/{slug}", h.handleWikiPage)
			r.Get("/wiki/{slug}/edit", h.handleWikiEdit)
			r.Post("/wiki/{slug}", h.handleWikiSave)
		})
	})

	return r
}
