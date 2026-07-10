package web

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/rusik69/govnohub/internal/search"
	"github.com/rusik69/govnohub/internal/wiki"
)

func (h *Handler) handleLoginGet(w http.ResponseWriter, r *http.Request) {
	if userFrom(r.Context()) != nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	render(w, r, LoginPage(csrfFrom(r.Context()), "", r.URL.Query().Get("next")))
}

func (h *Handler) handleLoginPost(w http.ResponseWriter, r *http.Request) {
	if !h.requirePOST(w, r) {
		return
	}
	token, _, err := h.deps.Auth.Login(r.Context(), r.FormValue("username"), r.FormValue("password"))
	if err != nil {
		render(w, r, LoginPage(csrfFrom(r.Context()), "Invalid credentials", r.FormValue("next")))
		return
	}
	h.setSession(w, token)
	next := r.FormValue("next")
	if next == "" || !strings.HasPrefix(next, "/") || strings.HasPrefix(next, "//") {
		next = "/"
	}
	http.Redirect(w, r, next, http.StatusSeeOther)
}

func (h *Handler) handleLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		_ = parseFormCSRF(r)
	}
	h.clearSession(w)
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

func (h *Handler) handleDashboard(w http.ResponseWriter, r *http.Request) {
	su := userFrom(r.Context())
	repos, _ := h.deps.Repos.ListForUser(r.Context(), su.ID)
	orgs, _ := h.deps.Org.ListForUser(r.Context(), su.ID)
	render(w, r, DashboardPage(h.layout(r, "Dashboard"), repos, orgs))
}

func (h *Handler) handleSearch(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	var hits []search.Hit
	if q != "" {
		hits, _ = h.deps.Search.Search(r.Context(), q, 30)
	}
	render(w, r, SearchPage(h.layout(r, "Search"), q, hits))
}

func (h *Handler) handleSettings(w http.ResponseWriter, r *http.Request) {
	su := userFrom(r.Context())
	pats, _ := h.deps.Auth.ListPATs(r.Context(), su.ID)
	keys, _ := h.deps.Auth.ListSSHKeys(r.Context(), su.ID)
	render(w, r, SettingsPage(h.layout(r, "Settings"), pats, keys, "", ""))
}

func (h *Handler) handleCreatePAT(w http.ResponseWriter, r *http.Request) {
	if !h.requirePOST(w, r) {
		return
	}
	su := userFrom(r.Context())
	scopes := r.Form["scope"]
	if len(scopes) == 0 {
		scopes = nil
	}
	token, err := h.deps.Auth.CreatePAT(r.Context(), su.ID, r.FormValue("name"), scopes)
	pats, _ := h.deps.Auth.ListPATs(r.Context(), su.ID)
	keys, _ := h.deps.Auth.ListSSHKeys(r.Context(), su.ID)
	errMsg := ""
	if err != nil {
		errMsg = err.Error()
		token = ""
	}
	render(w, r, SettingsPage(h.layout(r, "Settings"), pats, keys, token, errMsg))
}

func (h *Handler) handleRevokePAT(w http.ResponseWriter, r *http.Request) {
	if !h.requirePOST(w, r) {
		return
	}
	su := userFrom(r.Context())
	id, ok := parseUUIDParam(w, r, "id")
	if !ok {
		return
	}
	_ = h.deps.Auth.RevokePAT(r.Context(), su.ID, id)
	http.Redirect(w, r, "/settings", http.StatusSeeOther)
}

func (h *Handler) handleAddSSHKey(w http.ResponseWriter, r *http.Request) {
	if !h.requirePOST(w, r) {
		return
	}
	su := userFrom(r.Context())
	_, err := h.deps.Auth.AddSSHKey(r.Context(), su.ID, r.FormValue("title"), r.FormValue("key"))
	pats, _ := h.deps.Auth.ListPATs(r.Context(), su.ID)
	keys, _ := h.deps.Auth.ListSSHKeys(r.Context(), su.ID)
	errMsg := ""
	if err != nil {
		errMsg = err.Error()
	}
	render(w, r, SettingsPage(h.layout(r, "Settings"), pats, keys, "", errMsg))
}

func (h *Handler) handleDeleteSSHKey(w http.ResponseWriter, r *http.Request) {
	if !h.requirePOST(w, r) {
		return
	}
	su := userFrom(r.Context())
	id, ok := parseUUIDParam(w, r, "id")
	if !ok {
		return
	}
	_ = h.deps.Auth.DeleteSSHKey(r.Context(), su.ID, id)
	http.Redirect(w, r, "/settings", http.StatusSeeOther)
}

func (h *Handler) handleNotifications(w http.ResponseWriter, r *http.Request) {
	su := userFrom(r.Context())
	notifs, _ := h.deps.Notify.List(r.Context(), su.ID, 20)
	render(w, r, NotificationsPartial(notifs, csrfFrom(r.Context())))
}

func (h *Handler) handleMarkNotificationRead(w http.ResponseWriter, r *http.Request) {
	if !h.requirePOST(w, r) {
		return
	}
	su := userFrom(r.Context())
	id, ok := parseUUIDParam(w, r, "id")
	if !ok {
		return
	}
	_ = h.deps.Notify.MarkRead(r.Context(), id, su.ID)
	if isHTMX(r) {
		h.handleNotifications(w, r)
		return
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (h *Handler) handleOrgs(w http.ResponseWriter, r *http.Request) {
	orgs, _ := h.deps.Org.List(r.Context())
	render(w, r, OrgsPage(h.layout(r, "Organizations"), orgs, csrfFrom(r.Context())))
}

func (h *Handler) handleCreateOrg(w http.ResponseWriter, r *http.Request) {
	if !h.requirePOST(w, r) {
		return
	}
	o, err := h.deps.Org.Create(r.Context(), r.FormValue("name"), r.FormValue("display_name"), r.FormValue("description"))
	if err != nil {
		http.Redirect(w, r, "/orgs", http.StatusSeeOther)
		return
	}
	su := userFrom(r.Context())
	_ = h.deps.Org.AddMember(r.Context(), o.ID, su.ID, "admin")
	http.Redirect(w, r, "/orgs/"+o.Name, http.StatusSeeOther)
}

func (h *Handler) handleOrgDetail(w http.ResponseWriter, r *http.Request) {
	o, err := h.deps.Org.GetByName(r.Context(), chi.URLParam(r, "org"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	members, _ := h.deps.Org.ListMembers(r.Context(), o.ID)
	repos, _ := h.deps.Repos.ListForOrg(r.Context(), o.ID)
	render(w, r, OrgDetailPage(h.layout(r, o.Name), o, members, repos, csrfFrom(r.Context())))
}

func (h *Handler) handleAddOrgMember(w http.ResponseWriter, r *http.Request) {
	if !h.requirePOST(w, r) {
		return
	}
	o, err := h.deps.Org.GetByName(r.Context(), chi.URLParam(r, "org"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	u, err := h.deps.Auth.GetUserByUsername(r.Context(), r.FormValue("username"))
	if err == nil {
		_ = h.deps.Org.AddMember(r.Context(), o.ID, u.ID, r.FormValue("role"))
	}
	http.Redirect(w, r, "/orgs/"+o.Name, http.StatusSeeOther)
}

func (h *Handler) handleOrgTeams(w http.ResponseWriter, r *http.Request) {
	o, err := h.deps.Org.GetByName(r.Context(), chi.URLParam(r, "org"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	teams, _ := h.deps.Org.ListTeams(r.Context(), o.ID)
	render(w, r, OrgTeamsPage(h.layout(r, "Teams"), o.Name, teams, csrfFrom(r.Context())))
}

func (h *Handler) handleCreateTeam(w http.ResponseWriter, r *http.Request) {
	if !h.requirePOST(w, r) {
		return
	}
	o, err := h.deps.Org.GetByName(r.Context(), chi.URLParam(r, "org"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	_, _ = h.deps.Org.CreateTeam(r.Context(), o.ID, r.FormValue("name"), r.FormValue("description"))
	http.Redirect(w, r, "/orgs/"+o.Name+"/teams", http.StatusSeeOther)
}

func (h *Handler) handleTeamDetail(w http.ResponseWriter, r *http.Request) {
	o, err := h.deps.Org.GetByName(r.Context(), chi.URLParam(r, "org"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	team, err := h.deps.Org.GetTeam(r.Context(), o.ID, chi.URLParam(r, "team"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	members, _ := h.deps.Org.ListTeamMembers(r.Context(), team.ID)
	render(w, r, TeamDetailPage(h.layout(r, team.Name), o.Name, team, members, csrfFrom(r.Context())))
}

func (h *Handler) handleAddTeamMember(w http.ResponseWriter, r *http.Request) {
	if !h.requirePOST(w, r) {
		return
	}
	o, err := h.deps.Org.GetByName(r.Context(), chi.URLParam(r, "org"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	team, err := h.deps.Org.GetTeam(r.Context(), o.ID, chi.URLParam(r, "team"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	u, err := h.deps.Auth.GetUserByUsername(r.Context(), r.FormValue("username"))
	if err == nil {
		_ = h.deps.Org.AddTeamMember(r.Context(), team.ID, u.ID)
	}
	http.Redirect(w, r, "/orgs/"+o.Name+"/teams/"+team.Name, http.StatusSeeOther)
}

func (h *Handler) handleAdminUsers(w http.ResponseWriter, r *http.Request) {
	users, _ := h.deps.Auth.ListUsers(r.Context())
	render(w, r, AdminUsersPage(h.layout(r, "Admin"), users, ""))
}

func (h *Handler) handleAdminCreateUser(w http.ResponseWriter, r *http.Request) {
	if !h.requirePOST(w, r) {
		return
	}
	_, err := h.deps.Auth.CreateUser(r.Context(), r.FormValue("username"), r.FormValue("email"), r.FormValue("password"), r.FormValue("role"))
	users, _ := h.deps.Auth.ListUsers(r.Context())
	errMsg := ""
	if err != nil {
		errMsg = err.Error()
	} else if h.deps.Audit != nil {
		su := userFrom(r.Context())
		if u, uerr := h.deps.Auth.GetUserByUsername(r.Context(), r.FormValue("username")); uerr == nil {
			_ = h.deps.Audit.Record(r.Context(), su.ID, "user.create", "user", u.ID.String(), map[string]string{"username": u.Username})
		}
	}
	render(w, r, AdminUsersPage(h.layout(r, "Admin"), users, errMsg))
}

func (h *Handler) handleAdminDeleteUser(w http.ResponseWriter, r *http.Request) {
	if !h.requirePOST(w, r) {
		return
	}
	su := userFrom(r.Context())
	id, ok := parseUUIDParam(w, r, "id")
	if !ok {
		return
	}
	_ = h.deps.Auth.DeleteUser(r.Context(), su.ID, id)
	if h.deps.Audit != nil {
		_ = h.deps.Audit.Record(r.Context(), su.ID, "user.delete", "user", id.String(), nil)
	}
	http.Redirect(w, r, "/admin/users", http.StatusSeeOther)
}

func (h *Handler) handleAdminAudit(w http.ResponseWriter, r *http.Request) {
	entries, _ := h.deps.Audit.List(r.Context(), 100)
	render(w, r, AdminAuditPage(h.layout(r, "Audit log"), entries))
}

func (h *Handler) handleRepo(w http.ResponseWriter, r *http.Request) {
	repository, ok := h.getRepo(w, r, "read")
	if !ok {
		return
	}
	ref := r.URL.Query().Get("ref")
	if ref == "" {
		ref = repository.DefaultBranch
	}
	entries, _ := h.deps.Git.GetTree(repository.OwnerName, repository.Name, ref, "")
	header, nav := h.repoPageCtx(r.Context(), repository, "code")
	header.CSRF = csrfFrom(r.Context())
	render(w, r, RepoPage(RepoPageData{
		Layout: h.layout(r, repository.FullName),
		Header: header, Nav: nav,
		Repo: repository, Ref: ref, Entries: entries,
	}))
}

func (h *Handler) handleRepoTree(w http.ResponseWriter, r *http.Request) {
	repository, ok := h.getRepo(w, r, "read")
	if !ok {
		return
	}
	ref := r.URL.Query().Get("ref")
	if ref == "" {
		ref = repository.DefaultBranch
	}
	path := strings.TrimPrefix(chi.URLParam(r, "*"), "/")
	entries, _ := h.deps.Git.GetTree(repository.OwnerName, repository.Name, ref, path)
	header, nav := h.repoPageCtx(r.Context(), repository, "code")
	header.CSRF = csrfFrom(r.Context())
	render(w, r, RepoPage(RepoPageData{
		Layout: h.layout(r, repository.FullName),
		Header: header, Nav: nav,
		Repo: repository, Path: path, Ref: ref, Entries: entries,
	}))
}

func (h *Handler) handleRepoBlob(w http.ResponseWriter, r *http.Request) {
	repository, ok := h.getRepo(w, r, "read")
	if !ok {
		return
	}
	ref := r.URL.Query().Get("ref")
	if ref == "" {
		ref = repository.DefaultBranch
	}
	path := strings.TrimPrefix(chi.URLParam(r, "*"), "/")
	data, err := h.deps.Git.GetBlob(repository.OwnerName, repository.Name, ref, path)
	content := ""
	if err == nil {
		content = string(data)
	}
	header, nav := h.repoPageCtx(r.Context(), repository, "code")
	header.CSRF = csrfFrom(r.Context())
	render(w, r, RepoPage(RepoPageData{
		Layout: h.layout(r, repository.FullName),
		Header: header, Nav: nav,
		Repo: repository, Path: path, Ref: ref, Content: content,
	}))
}

func (h *Handler) handleIssues(w http.ResponseWriter, r *http.Request) {
	repository, ok := h.getRepo(w, r, "read")
	if !ok {
		return
	}
	issues, _ := h.deps.Issues.List(r.Context(), repository.ID)
	state := r.URL.Query().Get("state")
	header, nav := h.repoPageCtx(r.Context(), repository, "issues")
	header.CSRF = csrfFrom(r.Context())
	render(w, r, IssuesPage(h.layout(r, "Issues"), header, nav, issues, state, csrfFrom(r.Context())))
}

func (h *Handler) handleCreateIssue(w http.ResponseWriter, r *http.Request) {
	if !h.requirePOST(w, r) {
		return
	}
	repository, ok := h.getRepo(w, r, "write")
	if !ok {
		return
	}
	su := userFrom(r.Context())
	i, _ := h.deps.Issues.Create(r.Context(), repository.ID, su.ID, r.FormValue("title"), r.FormValue("body"))
	http.Redirect(w, r, fmt.Sprintf("/%s/issues/%d", repository.FullName, i.Number), http.StatusSeeOther)
}

func (h *Handler) handleIssueDetail(w http.ResponseWriter, r *http.Request) {
	repository, ok := h.getRepo(w, r, "read")
	if !ok {
		return
	}
	num, ok := parseNum(w, r, "number")
	if !ok {
		return
	}
	iss, err := h.deps.Issues.Get(r.Context(), repository.ID, num)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	comments, _ := h.deps.Issues.ListComments(r.Context(), iss.ID)
	labels, _ := h.deps.Issues.ListLabels(r.Context(), repository.ID)
	milestones, _ := h.deps.Issues.ListMilestones(r.Context(), repository.ID)
	users, _ := h.deps.Issues.ListRepoUsers(r.Context(), repository.ID)
	header, nav := h.repoPageCtx(r.Context(), repository, "issues")
	header.CSRF = csrfFrom(r.Context())
	render(w, r, IssueDetailPage(h.layout(r, iss.Title), header, nav, iss, comments, labels, milestones, users, csrfFrom(r.Context())))
}

func (h *Handler) handleIssueComment(w http.ResponseWriter, r *http.Request) {
	if !h.requirePOST(w, r) {
		return
	}
	repository, ok := h.getRepo(w, r, "write")
	if !ok {
		return
	}
	num, ok := parseNum(w, r, "number")
	if !ok {
		return
	}
	iss, err := h.deps.Issues.Get(r.Context(), repository.ID, num)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	su := userFrom(r.Context())
	_, _ = h.deps.Issues.AddComment(r.Context(), iss.ID, su.ID, r.FormValue("body"))
	http.Redirect(w, r, "/"+repository.FullName+"/issues/"+strconv.Itoa(num), http.StatusSeeOther)
}

func (h *Handler) handleCloseIssue(w http.ResponseWriter, r *http.Request) {
	if !h.requirePOST(w, r) {
		return
	}
	repository, ok := h.getRepo(w, r, "write")
	if !ok {
		return
	}
	num, ok := parseNum(w, r, "number")
	if !ok {
		return
	}
	iss, err := h.deps.Issues.Get(r.Context(), repository.ID, num)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	_ = h.deps.Issues.Close(r.Context(), iss.ID)
	su := userFrom(r.Context())
	if iss.AuthorID != su.ID {
		h.deps.Notify.NotifyAsync(iss.AuthorID, "Issue closed",
			"Issue #"+strconv.Itoa(num)+" was closed in "+repository.FullName,
			"/"+repository.FullName+"/issues/"+strconv.Itoa(num))
	}
	http.Redirect(w, r, "/"+repository.FullName+"/issues/"+strconv.Itoa(num), http.StatusSeeOther)
}

func (h *Handler) handleIssueAssignee(w http.ResponseWriter, r *http.Request) {
	if !h.requirePOST(w, r) {
		return
	}
	repository, ok := h.getRepo(w, r, "write")
	if !ok {
		return
	}
	num, ok := parseNum(w, r, "number")
	if !ok {
		return
	}
	iss, err := h.deps.Issues.Get(r.Context(), repository.ID, num)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	assignee := r.FormValue("assignee")
	var id *uuid.UUID
	if assignee != "" {
		u, err := h.deps.Auth.GetUserByUsername(r.Context(), assignee)
		if err == nil {
			id = &u.ID
		}
	}
	_ = h.deps.Issues.SetAssignee(r.Context(), iss.ID, id)
	w.WriteHeader(http.StatusOK)
}

func (h *Handler) handleIssueMilestone(w http.ResponseWriter, r *http.Request) {
	if !h.requirePOST(w, r) {
		return
	}
	repository, ok := h.getRepo(w, r, "write")
	if !ok {
		return
	}
	num, ok := parseNum(w, r, "number")
	if !ok {
		return
	}
	iss, err := h.deps.Issues.Get(r.Context(), repository.ID, num)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	mid := r.FormValue("milestone_id")
	var id *uuid.UUID
	if mid != "" {
		parsed, err := uuid.Parse(mid)
		if err == nil {
			id = &parsed
		}
	}
	_ = h.deps.Issues.SetMilestone(r.Context(), iss.ID, id)
	w.WriteHeader(http.StatusOK)
}

func (h *Handler) handleIssueAddLabel(w http.ResponseWriter, r *http.Request) {
	if !h.requirePOST(w, r) {
		return
	}
	repository, ok := h.getRepo(w, r, "write")
	if !ok {
		return
	}
	num, ok := parseNum(w, r, "number")
	if !ok {
		return
	}
	labelID, ok := parseUUIDParam(w, r, "labelID")
	if !ok {
		return
	}
	iss, err := h.deps.Issues.Get(r.Context(), repository.ID, num)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	_ = h.deps.Issues.AddLabel(r.Context(), iss.ID, labelID)
	http.Redirect(w, r, "/"+repository.FullName+"/issues/"+strconv.Itoa(num), http.StatusSeeOther)
}

func (h *Handler) handleIssueRemoveLabel(w http.ResponseWriter, r *http.Request) {
	if !h.requirePOST(w, r) {
		return
	}
	repository, ok := h.getRepo(w, r, "write")
	if !ok {
		return
	}
	num, ok := parseNum(w, r, "number")
	if !ok {
		return
	}
	labelID, ok := parseUUIDParam(w, r, "labelID")
	if !ok {
		return
	}
	iss, err := h.deps.Issues.Get(r.Context(), repository.ID, num)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	_ = h.deps.Issues.RemoveLabel(r.Context(), iss.ID, labelID)
	http.Redirect(w, r, "/"+repository.FullName+"/issues/"+strconv.Itoa(num), http.StatusSeeOther)
}

func (h *Handler) handlePulls(w http.ResponseWriter, r *http.Request) {
	repository, ok := h.getRepo(w, r, "read")
	if !ok {
		return
	}
	prs, _ := h.deps.Pulls.List(r.Context(), repository.ID)
	state := r.URL.Query().Get("state")
	header, nav := h.repoPageCtx(r.Context(), repository, "pulls")
	header.CSRF = csrfFrom(r.Context())
	render(w, r, PullsPage(h.layout(r, "Pull requests"), header, nav, prs, state, csrfFrom(r.Context())))
}

func (h *Handler) handleCreatePR(w http.ResponseWriter, r *http.Request) {
	if !h.requirePOST(w, r) {
		return
	}
	repository, ok := h.getRepo(w, r, "write")
	if !ok {
		return
	}
	su := userFrom(r.Context())
	head := r.FormValue("head")
	base := r.FormValue("base")
	if base == "" {
		base = repository.DefaultBranch
	}
	headSHA, err := h.deps.Git.UpdateHead(repository.OwnerName, repository.Name, head)
	if err != nil {
		http.Redirect(w, r, "/"+repository.FullName+"/pulls", http.StatusSeeOther)
		return
	}
	pr, _ := h.deps.Pulls.Create(r.Context(), repository.ID, su.ID, r.FormValue("title"), r.FormValue("body"), head, base, headSHA)
	http.Redirect(w, r, "/"+repository.FullName+"/pulls/"+strconv.Itoa(pr.Number), http.StatusSeeOther)
}

func (h *Handler) handlePRDetail(w http.ResponseWriter, r *http.Request) {
	repository, ok := h.getRepo(w, r, "read")
	if !ok {
		return
	}
	num, ok := parseNum(w, r, "number")
	if !ok {
		return
	}
	pr, err := h.deps.Pulls.Get(r.Context(), repository.ID, num)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	reviews, _ := h.deps.Pulls.ListReviews(r.Context(), pr.ID)
	comments, _ := h.deps.Pulls.ListComments(r.Context(), pr.ID)
	baseSHA, _ := h.deps.Git.UpdateHead(repository.OwnerName, repository.Name, pr.BaseBranch)
	diff, _ := h.deps.Git.Diff(repository.OwnerName, repository.Name, baseSHA, pr.HeadSHA)
	files := ParseUnifiedDiff(diff)
	mergeable := true
	if pr.State == "open" {
		ok, err := h.deps.Git.CanMerge(repository.OwnerName, repository.Name, pr.BaseBranch, pr.HeadBranch)
		if err == nil {
			mergeable = ok
		}
	}
	header, nav := h.repoPageCtx(r.Context(), repository, "pulls")
	header.CSRF = csrfFrom(r.Context())
	render(w, r, PRDetailPage(h.layout(r, pr.Title), PRDetailData{
		Header: header, Nav: nav,
		PR: pr, Reviews: reviews, Comments: comments, Files: files, CSRF: csrfFrom(r.Context()),
		Mergeable: mergeable,
	}))
}

func (h *Handler) handlePRComment(w http.ResponseWriter, r *http.Request) {
	if !h.requirePOST(w, r) {
		return
	}
	repository, ok := h.getRepo(w, r, "write")
	if !ok {
		return
	}
	num, ok := parseNum(w, r, "number")
	if !ok {
		return
	}
	pr, err := h.deps.Pulls.Get(r.Context(), repository.ID, num)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	su := userFrom(r.Context())
	line, _ := strconv.Atoi(r.FormValue("line"))
	_, _ = h.deps.Pulls.AddComment(r.Context(), pr.ID, su.ID, r.FormValue("body"), r.FormValue("path"), line)
	http.Redirect(w, r, "/"+repository.FullName+"/pulls/"+strconv.Itoa(num), http.StatusSeeOther)
}

func (h *Handler) handlePRReview(w http.ResponseWriter, r *http.Request) {
	if !h.requirePOST(w, r) {
		return
	}
	repository, ok := h.getRepo(w, r, "write")
	if !ok {
		return
	}
	num, ok := parseNum(w, r, "number")
	if !ok {
		return
	}
	pr, err := h.deps.Pulls.Get(r.Context(), repository.ID, num)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	su := userFrom(r.Context())
	_, _ = h.deps.Pulls.AddReview(r.Context(), pr.ID, su.ID, r.FormValue("state"), r.FormValue("body"))
	http.Redirect(w, r, "/"+repository.FullName+"/pulls/"+strconv.Itoa(num), http.StatusSeeOther)
}

func (h *Handler) handlePRMerge(w http.ResponseWriter, r *http.Request) {
	if !h.requirePOST(w, r) {
		return
	}
	repository, ok := h.getRepo(w, r, "write")
	if !ok {
		return
	}
	num, ok := parseNum(w, r, "number")
	if !ok {
		return
	}
	pr, err := h.deps.Pulls.Get(r.Context(), repository.ID, num)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	approved, _ := h.deps.Pulls.ReviewCount(r.Context(), pr.ID, "approved")
	if err := h.deps.Repos.ValidateMergeProtection(r.Context(), repository.ID, pr.BaseBranch, pr.HeadSHA, approved); err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}
	sha, err := h.deps.Git.Merge(repository.OwnerName, repository.Name, pr.BaseBranch, pr.HeadBranch, false)
	if err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	_ = h.deps.Pulls.Merge(r.Context(), pr.ID, sha)
	_ = h.deps.Repos.UpdateBranchHead(r.Context(), repository.ID, pr.BaseBranch, sha)
	h.deps.Notify.NotifyAsync(pr.AuthorID, "PR merged",
		"PR #"+strconv.Itoa(num)+" was merged in "+repository.FullName,
		"/"+repository.FullName+"/pulls/"+strconv.Itoa(num))
	http.Redirect(w, r, "/"+repository.FullName+"/pulls/"+strconv.Itoa(num), http.StatusSeeOther)
}

func (h *Handler) handleActions(w http.ResponseWriter, r *http.Request) {
	repository, ok := h.getRepo(w, r, "read")
	if !ok {
		return
	}
	rows, err := h.deps.Pool.Query(r.Context(), `
		SELECT id, run_number, event, head_branch, status, COALESCE(conclusion,''), created_at
		FROM workflow_runs WHERE repo_id=$1 ORDER BY run_number DESC LIMIT 50`, repository.ID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()
	var runs []WorkflowRun
	for rows.Next() {
		var run WorkflowRun
		var id uuid.UUID
		if err := rows.Scan(&id, &run.RunNumber, &run.Event, &run.HeadBranch, &run.Status, &run.Conclusion, &run.CreatedAt); err != nil {
			continue
		}
		run.ID = id.String()
		runs = append(runs, run)
	}
	render(w, r, ActionsPage(h.layout(r, "Actions"), repoNav(repository.OwnerName, repository.Name, "actions"), runs))
}

func (h *Handler) handleActionLogs(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.getRepo(w, r, "read"); !ok {
		return
	}
	runID, err := uuid.Parse(chi.URLParam(r, "runID"))
	if err != nil {
		http.Error(w, "invalid run", http.StatusBadRequest)
		return
	}
	rows, err := h.deps.Pool.Query(r.Context(), `SELECT name, log_path FROM workflow_jobs WHERE run_id=$1`, runID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()
	var logs strings.Builder
	for rows.Next() {
		var name string
		var logPath *string
		if err := rows.Scan(&name, &logPath); err != nil {
			continue
		}
		logs.WriteString("=== " + name + " ===\n")
		if logPath != nil {
			b, _ := os.ReadFile(*logPath)
			logs.Write(b)
			logs.WriteString("\n")
		}
	}
	render(w, r, ActionLogsPartial(logs.String()))
}

func (h *Handler) handleReleases(w http.ResponseWriter, r *http.Request) {
	repository, ok := h.getRepo(w, r, "read")
	if !ok {
		return
	}
	rels, _ := h.deps.Releases.List(r.Context(), repository.ID)
	var rows []ReleaseWithAssets
	for _, rel := range rels {
		assets, _ := h.deps.Releases.ListAssets(r.Context(), rel.ID)
		rows = append(rows, ReleaseWithAssets{Release: rel, Assets: assets})
	}
	render(w, r, ReleasesPage(h.layout(r, "Releases"), repoNav(repository.OwnerName, repository.Name, "releases"), rows, csrfFrom(r.Context())))
}

func (h *Handler) handleCreateRelease(w http.ResponseWriter, r *http.Request) {
	if !h.requirePOST(w, r) {
		return
	}
	repository, ok := h.getRepo(w, r, "write")
	if !ok {
		return
	}
	su := userFrom(r.Context())
	_, _ = h.deps.Releases.Create(r.Context(), repository.ID, su.ID, r.FormValue("tag"), r.FormValue("name"), r.FormValue("body"), false, false)
	http.Redirect(w, r, "/"+repository.FullName+"/releases", http.StatusSeeOther)
}

func (h *Handler) handlePackages(w http.ResponseWriter, r *http.Request) {
	repository, ok := h.getRepo(w, r, "read")
	if !ok {
		return
	}
	pkgs, _ := h.deps.Packages.List(r.Context(), repository.ID)
	render(w, r, PackagesPage(h.layout(r, "Packages"), repoNav(repository.OwnerName, repository.Name, "packages"), pkgs, csrfFrom(r.Context())))
}

func (h *Handler) handleRepoSettings(w http.ResponseWriter, r *http.Request) {
	repository, ok := h.getRepo(w, r, "read")
	if !ok {
		return
	}
	rows, err := h.deps.Pool.Query(r.Context(), `SELECT id, url, events, active FROM webhooks WHERE repo_id=$1`, repository.ID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()
	var hooks []WebhookWithDeliveries
	for rows.Next() {
		var hook WebhookRow
		var id uuid.UUID
		var events []string
		if err := rows.Scan(&id, &hook.URL, &events, &hook.Active); err == nil {
			hook.ID = id.String()
			hook.Events = strings.Join(events, ", ")
			deliveries, _ := h.deps.Webhooks.ListDeliveries(r.Context(), id, 5)
			hooks = append(hooks, WebhookWithDeliveries{Hook: hook, Deliveries: deliveries})
		}
	}
	collabs, _ := h.deps.Repos.ListCollaborators(r.Context(), repository.ID)
	protected, _ := h.deps.Repos.ListProtectedBranches(r.Context(), repository.ID)
	labels, _ := h.deps.Issues.ListLabels(r.Context(), repository.ID)
	data := RepoSettingsData{
		Nav:               repoNav(repository.OwnerName, repository.Name, "settings"),
		CSRF:              csrfFrom(r.Context()),
		Webhooks:          hooks,
		Collaborators:     collabs,
		ProtectedBranches: protected,
		Labels:            labels,
	}
	render(w, r, RepoSettingsPage(h.layout(r, "Settings"), data))
}

func (h *Handler) handleCreateWebhook(w http.ResponseWriter, r *http.Request) {
	if !h.requirePOST(w, r) {
		return
	}
	repository, ok := h.getRepo(w, r, "write")
	if !ok {
		return
	}
	events := strings.Split(r.FormValue("events"), ",")
	for i := range events {
		events[i] = strings.TrimSpace(events[i])
	}
	_, _ = h.deps.Webhooks.Create(r.Context(), repository.ID, r.FormValue("url"), "", events)
	http.Redirect(w, r, "/"+repository.FullName+"/settings", http.StatusSeeOther)
}

func (h *Handler) handleWiki(w http.ResponseWriter, r *http.Request) {
	repository, ok := h.getRepo(w, r, "read")
	if !ok {
		return
	}
	pages, _ := h.deps.Wiki.List(r.Context(), repository.ID)
	render(w, r, WikiPage(h.layout(r, "Wiki"), repoNav(repository.OwnerName, repository.Name, "wiki"), pages))
}

func (h *Handler) handleWikiPage(w http.ResponseWriter, r *http.Request) {
	repository, ok := h.getRepo(w, r, "read")
	if !ok {
		return
	}
	slug := chi.URLParam(r, "slug")
	page, err := h.deps.Wiki.Get(r.Context(), repository.ID, slug)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	html, _ := RenderMarkdown(page.Content)
	render(w, r, WikiViewPage(h.layout(r, page.Title), repoNav(repository.OwnerName, repository.Name, "wiki"), page, html, csrfFrom(r.Context())))
}

func (h *Handler) handleWikiEdit(w http.ResponseWriter, r *http.Request) {
	repository, ok := h.getRepo(w, r, "write")
	if !ok {
		return
	}
	slug := chi.URLParam(r, "slug")
	title, content := "", ""
	if slug != "home" {
		if page, err := h.deps.Wiki.Get(r.Context(), repository.ID, slug); err == nil {
			title, content = page.Title, page.Content
		}
	}
	render(w, r, WikiEditPage(h.layout(r, "Edit wiki"), repoNav(repository.OwnerName, repository.Name, "wiki"), slug, title, content, csrfFrom(r.Context())))
}

func (h *Handler) handleWikiSave(w http.ResponseWriter, r *http.Request) {
	if !h.requirePOST(w, r) {
		return
	}
	repository, ok := h.getRepo(w, r, "write")
	if !ok {
		return
	}
	su := userFrom(r.Context())
	slug := chi.URLParam(r, "slug")
	title := r.FormValue("title")
	if slug == "home" || slug == "" {
		slug = wiki.Slugify(title)
	}
	page, _ := h.deps.Wiki.Upsert(r.Context(), repository.ID, su.ID, slug, title, r.FormValue("content"))
	http.Redirect(w, r, "/"+repository.FullName+"/wiki/"+page.Slug, http.StatusSeeOther)
}

func (h *Handler) handleStar(w http.ResponseWriter, r *http.Request) {
	if !h.requirePOST(w, r) {
		return
	}
	repository, ok := h.getRepo(w, r, "read")
	if !ok {
		return
	}
	su := userFrom(r.Context())
	_ = h.deps.Repos.Star(r.Context(), repository.ID, su.ID)
	http.Redirect(w, r, "/"+repository.FullName, http.StatusSeeOther)
}

func (h *Handler) handleWatch(w http.ResponseWriter, r *http.Request) {
	if !h.requirePOST(w, r) {
		return
	}
	repository, ok := h.getRepo(w, r, "read")
	if !ok {
		return
	}
	su := userFrom(r.Context())
	_ = h.deps.Repos.Watch(r.Context(), repository.ID, su.ID)
	http.Redirect(w, r, "/"+repository.FullName, http.StatusSeeOther)
}

func (h *Handler) handleFork(w http.ResponseWriter, r *http.Request) {
	if !h.requirePOST(w, r) {
		return
	}
	repository, ok := h.getRepo(w, r, "read")
	if !ok {
		return
	}
	su := userFrom(r.Context())
	fork, err := h.deps.Repos.Fork(r.Context(), repository, su.ID, su.Username)
	if err != nil {
		http.Redirect(w, r, "/"+repository.FullName, http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/"+fork.FullName, http.StatusSeeOther)
}

func (h *Handler) handleMilestones(w http.ResponseWriter, r *http.Request) {
	repository, ok := h.getRepo(w, r, "read")
	if !ok {
		return
	}
	ms, _ := h.deps.Issues.ListMilestones(r.Context(), repository.ID)
	render(w, r, MilestonesPage(h.layout(r, "Milestones"), repoNav(repository.OwnerName, repository.Name, "milestones"), ms, csrfFrom(r.Context())))
}

func (h *Handler) handleCreateMilestone(w http.ResponseWriter, r *http.Request) {
	if !h.requirePOST(w, r) {
		return
	}
	repository, ok := h.getRepo(w, r, "write")
	if !ok {
		return
	}
	_, _ = h.deps.Issues.CreateMilestone(r.Context(), repository.ID, r.FormValue("title"), r.FormValue("description"), nil)
	http.Redirect(w, r, "/"+repository.FullName+"/milestones", http.StatusSeeOther)
}

func (h *Handler) handleCloseMilestone(w http.ResponseWriter, r *http.Request) {
	if !h.requirePOST(w, r) {
		return
	}
	repository, ok := h.getRepo(w, r, "write")
	if !ok {
		return
	}
	id, ok := parseUUIDParam(w, r, "id")
	if !ok {
		return
	}
	_ = h.deps.Issues.CloseMilestone(r.Context(), id)
	http.Redirect(w, r, "/"+repository.FullName+"/milestones", http.StatusSeeOther)
}

func (h *Handler) handleAddCollaborator(w http.ResponseWriter, r *http.Request) {
	if !h.requirePOST(w, r) {
		return
	}
	repository, ok := h.getRepo(w, r, "write")
	if !ok {
		return
	}
	u, err := h.deps.Auth.GetUserByUsername(r.Context(), r.FormValue("username"))
	if err == nil {
		perm := r.FormValue("permission")
		if perm == "" {
			perm = "read"
		}
		_ = h.deps.Repos.AddCollaborator(r.Context(), repository.ID, u.ID, perm)
	}
	http.Redirect(w, r, "/"+repository.FullName+"/settings", http.StatusSeeOther)
}

func (h *Handler) handleRemoveCollaborator(w http.ResponseWriter, r *http.Request) {
	if !h.requirePOST(w, r) {
		return
	}
	repository, ok := h.getRepo(w, r, "write")
	if !ok {
		return
	}
	u, err := h.deps.Auth.GetUserByUsername(r.Context(), chi.URLParam(r, "username"))
	if err == nil {
		_ = h.deps.Repos.RemoveCollaborator(r.Context(), repository.ID, u.ID)
	}
	http.Redirect(w, r, "/"+repository.FullName+"/settings", http.StatusSeeOther)
}

func (h *Handler) handleProtectBranch(w http.ResponseWriter, r *http.Request) {
	if !h.requirePOST(w, r) {
		return
	}
	repository, ok := h.getRepo(w, r, "write")
	if !ok {
		return
	}
	checks := []string{}
	if v := strings.TrimSpace(r.FormValue("required_checks")); v != "" {
		for _, c := range strings.Split(v, ",") {
			if c = strings.TrimSpace(c); c != "" {
				checks = append(checks, c)
			}
		}
	}
	reviews, _ := strconv.Atoi(r.FormValue("require_reviews"))
	_, _ = h.deps.Pool.Exec(r.Context(), `
		INSERT INTO protected_branches (repo_id, branch_name, required_checks, require_reviews)
		VALUES ($1,$2,$3,$4)
		ON CONFLICT (repo_id, branch_name) DO UPDATE SET required_checks=EXCLUDED.required_checks, require_reviews=EXCLUDED.require_reviews`,
		repository.ID, r.FormValue("branch"), checks, reviews)
	http.Redirect(w, r, "/"+repository.FullName+"/settings", http.StatusSeeOther)
}

func (h *Handler) handleCreateLabel(w http.ResponseWriter, r *http.Request) {
	if !h.requirePOST(w, r) {
		return
	}
	repository, ok := h.getRepo(w, r, "write")
	if !ok {
		return
	}
	color := r.FormValue("color")
	if color == "" {
		color = "#0366d6"
	}
	_, _ = h.deps.Issues.CreateLabel(r.Context(), repository.ID, r.FormValue("name"), color)
	http.Redirect(w, r, "/"+repository.FullName+"/settings", http.StatusSeeOther)
}

func (h *Handler) handleUploadReleaseAsset(w http.ResponseWriter, r *http.Request) {
	if !h.requirePOST(w, r) {
		return
	}
	repository, ok := h.getRepo(w, r, "write")
	if !ok {
		return
	}
	tag := chi.URLParam(r, "tag")
	rel, err := h.deps.Releases.GetByTag(r.Context(), repository.ID, tag)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		http.Redirect(w, r, "/"+repository.FullName+"/releases", http.StatusSeeOther)
		return
	}
	defer file.Close()
	_, _ = h.deps.Releases.UploadAsset(r.Context(), rel.ID, header.Filename, header.Header.Get("Content-Type"), file, header.Size)
	http.Redirect(w, r, "/"+repository.FullName+"/releases", http.StatusSeeOther)
}

func (h *Handler) handlePublishPackage(w http.ResponseWriter, r *http.Request) {
	if !h.requirePOST(w, r) {
		return
	}
	repository, ok := h.getRepo(w, r, "write")
	if !ok {
		return
	}
	file, _, err := r.FormFile("file")
	if err != nil {
		http.Redirect(w, r, "/"+repository.FullName+"/packages", http.StatusSeeOther)
		return
	}
	defer file.Close()
	data, _ := io.ReadAll(file)
	pkgType := r.FormValue("type")
	if pkgType == "" {
		pkgType = "generic"
	}
	_, _ = h.deps.Packages.Publish(r.Context(), repository.ID, r.FormValue("name"), pkgType, r.FormValue("version"), bytes.NewReader(data))
	http.Redirect(w, r, "/"+repository.FullName+"/packages", http.StatusSeeOther)
}

func (h *Handler) handleWikiDelete(w http.ResponseWriter, r *http.Request) {
	if !h.requirePOST(w, r) {
		return
	}
	repository, ok := h.getRepo(w, r, "write")
	if !ok {
		return
	}
	_ = h.deps.Wiki.Delete(r.Context(), repository.ID, chi.URLParam(r, "slug"))
	http.Redirect(w, r, "/"+repository.FullName+"/wiki", http.StatusSeeOther)
}

func (h *Handler) handleCreateOrgRepo(w http.ResponseWriter, r *http.Request) {
	if !h.requirePOST(w, r) {
		return
	}
	o, err := h.deps.Org.GetByName(r.Context(), chi.URLParam(r, "org"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	repo, err := h.deps.Repos.Create(r.Context(), "org", o.ID, o.Name, r.FormValue("name"), r.FormValue("description"), false)
	if err != nil {
		http.Redirect(w, r, "/orgs/"+o.Name, http.StatusSeeOther)
		return
	}
	_ = h.deps.Git.Init(r.Context(), repo.OwnerName, repo.Name)
	http.Redirect(w, r, "/"+repo.FullName, http.StatusSeeOther)
}
