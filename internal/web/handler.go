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
		r.Get("/u/{username}", h.handleUserProfile)
		r.Post("/repos/create", h.handleCreateUserRepo)
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

		r.Group(func(r chi.Router) {
			r.Use(h.requireAdmin)
			r.Get("/admin/users", h.handleAdminUsers)
			r.Post("/admin/users", h.handleAdminCreateUser)
			r.Post("/admin/users/{id}/delete", h.handleAdminDeleteUser)
			r.Get("/admin/audit", h.handleAdminAudit)
		})

		r.Route("/{owner}/{repo}", func(r chi.Router) {
			r.Get("/", h.handleRepo)
			r.Get("/tree/*", h.handleRepoTree)
			r.Get("/blob/*", h.handleRepoBlob)
			r.Get("/commit/{sha}", h.handleCommitDetail)
			r.Get("/compare/*", h.handleCompare)
			r.Post("/star", h.handleStar)
			r.Post("/unstar", h.handleUnstar)
			r.Post("/watch", h.handleWatch)
			r.Post("/unwatch", h.handleUnwatch)
			r.Post("/fork", h.handleFork)

			r.Get("/actions", h.handleActions)
			r.Post("/actions/trigger", h.handleTriggerAction)
			r.Get("/actions/runs/{runID}/logs", h.handleActionLogs)
			r.Post("/issues", h.handleCreateIssue)
			r.Get("/issues/{number}", h.handleIssueDetail)
			r.Post("/issues/{number}/comment", h.handleIssueComment)
			r.Post("/issues/{number}/close", h.handleCloseIssue)
			r.Post("/issues/{number}/assignee", h.handleIssueAssignee)
			r.Post("/issues/{number}/milestone", h.handleIssueMilestone)
			r.Post("/issues/{number}/labels/{labelID}", h.handleIssueAddLabel)
			r.Post("/issues/{number}/labels/{labelID}/remove", h.handleIssueRemoveLabel)

			r.Get("/milestones", h.handleMilestones)
			r.Post("/milestones", h.handleCreateMilestone)
			r.Post("/milestones/{id}/close", h.handleCloseMilestone)

			r.Get("/pulls", h.handlePulls)
			r.Post("/pulls", h.handleCreatePR)
			r.Get("/pulls/{number}", h.handlePRDetail)
			r.Post("/pulls/{number}/comment", h.handlePRComment)
			r.Post("/pulls/{number}/review", h.handlePRReview)
			r.Post("/pulls/{number}/merge", h.handlePRMerge)

			r.Get("/issues", h.handleIssues)
			r.Get("/releases", h.handleReleases)
			r.Post("/releases", h.handleCreateRelease)
			r.Post("/releases/{tag}/assets", h.handleUploadReleaseAsset)

			r.Get("/packages", h.handlePackages)
			r.Post("/packages", h.handlePublishPackage)

			r.Get("/settings", h.handleRepoSettings)
			r.Post("/settings/webhook", h.handleCreateWebhook)
			r.Post("/settings/collaborator", h.handleAddCollaborator)
			r.Post("/settings/collaborator/{username}/remove", h.handleRemoveCollaborator)
			r.Post("/settings/protection", h.handleProtectBranch)
			r.Post("/settings/label", h.handleCreateLabel)

			r.Get("/wiki", h.handleWiki)
			r.Get("/wiki/{slug}", h.handleWikiPage)
			r.Get("/wiki/{slug}/edit", h.handleWikiEdit)
			r.Post("/wiki/{slug}", h.handleWikiSave)
			r.Post("/wiki/{slug}/delete", h.handleWikiDelete)
		})

		r.Post("/orgs/{org}/repos", h.handleCreateOrgRepo)
	})

	return r
}
