package web

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/rusik69/govnohub/internal/auth"
	"github.com/rusik69/govnohub/internal/repo"
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
		errMsg := "Invalid credentials"
		if errors.Is(err, auth.ErrAccountLocked) {
			errMsg = err.Error()
		}
		render(w, r, LoginPage(csrfFrom(r.Context()), errMsg, r.FormValue("next")))
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
	render(w, r, DashboardPage(h.layout(r, "Dashboard"), repos, orgs, csrfFrom(r.Context()), ""))
}

func (h *Handler) handleUserProfile(w http.ResponseWriter, r *http.Request) {
	username := chi.URLParam(r, "username")
	user, err := h.deps.Auth.GetUserByUsername(r.Context(), username)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	su := userFrom(r.Context())
	isOwner := su != nil && su.ID == user.ID
	// Show public repos owned by this user; if viewing own profile, show all
	rows, err := h.deps.Pool.Query(r.Context(), `
		SELECT r.id, r.owner_type, r.owner_id, r.name, COALESCE(r.description,''),
		       r.default_branch, r.is_private, r.is_fork, r.star_count, r.created_at, r.updated_at,
		       COALESCE(u.username, o.name, '') AS owner_name
		FROM repos r
		LEFT JOIN users u ON r.owner_type='user' AND r.owner_id=u.id
		LEFT JOIN orgs o ON r.owner_type='org' AND r.owner_id=o.id
		WHERE r.owner_id=$1 AND r.owner_type='user'
		  AND (NOT r.is_private OR $2)
		ORDER BY r.updated_at DESC`, user.ID, isOwner)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()
	var repos []repo.Repository
	for rows.Next() {
		var r repo.Repository
		if err := rows.Scan(&r.ID, &r.OwnerType, &r.OwnerID, &r.Name, &r.Description,
			&r.DefaultBranch, &r.IsPrivate, &r.IsFork, &r.StarCount, &r.CreatedAt, &r.UpdatedAt, &r.OwnerName); err != nil {
			continue
		}
		r.FullName = r.OwnerName + "/" + r.Name
		repos = append(repos, r)
	}
	render(w, r, UserProfilePage(h.layout(r, user.Username), UserProfileData{
		User:    user,
		Repos:   repos,
		IsOwner: isOwner,
	}))
}

func (h *Handler) handleCreateUserRepo(w http.ResponseWriter, r *http.Request) {
	if !h.requirePOST(w, r) {
		return
	}
	su := userFrom(r.Context())
	name := strings.TrimSpace(r.FormValue("name"))
	desc := r.FormValue("description")
	_, err := h.deps.Repos.Create(r.Context(), "user", su.ID, su.Username, name, desc, false)
	repos, _ := h.deps.Repos.ListForUser(r.Context(), su.ID)
	orgs, _ := h.deps.Org.ListForUser(r.Context(), su.ID)
	errMsg := ""
	if err != nil {
		errMsg = err.Error()
	} else if name != "" {
		if err := h.deps.Git.Init(r.Context(), su.Username, name); err != nil {
			errMsg = err.Error()
		} else if _, err := h.deps.Git.SeedMainBranch(su.Username, name, "main"); err != nil {
			errMsg = err.Error()
		} else {
			http.Redirect(w, r, "/"+su.Username+"/"+name, http.StatusSeeOther)
			return
		}
	}
	render(w, r, DashboardPage(h.layout(r, "Dashboard"), repos, orgs, csrfFrom(r.Context()), errMsg))
}

func (h *Handler) handleUserSearch(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" || len(q) < 1 {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]string{})
		return
	}
	users, err := h.deps.Auth.SearchUsers(r.Context(), q, 10)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]string{})
		return
	}
	names := make([]string, 0, len(users))
	for _, u := range users {
		names = append(names, u.Username)
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(names)
}

func (h *Handler) handleSearch(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	typ := r.URL.Query().Get("type")
	sort := r.URL.Query().Get("sort")
	pageStr := r.URL.Query().Get("page")
	page, _ := strconv.Atoi(pageStr)
	if page < 1 {
		page = 1
	}
	const perPage = 20
	offset := (page - 1) * perPage

	var result search.SearchResult
	if q != "" {
		opts := search.SearchOptions{
			Limit:  perPage,
			Offset: offset,
			Type:   typ,
			Sort:   sort,
		}
		result, _ = h.deps.Search.Search(r.Context(), q, opts)
	}
	render(w, r, SearchPage(h.layout(r, "Search"), q, typ, sort, result.Hits, result.Total, page, perPage))
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
	redirectWithFlash(w, r, "/settings", "Token revoked", false)
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
	redirectWithFlash(w, r, "/settings", "SSH key removed", false)
}

func (h *Handler) handleNotifications(w http.ResponseWriter, r *http.Request) {
	su := userFrom(r.Context())
	notifs, _ := h.deps.Notify.List(r.Context(), su.ID, 20)
	render(w, r, NotificationsPartial(notifs, csrfFrom(r.Context())))
}

func (h *Handler) handleNotificationsCount(w http.ResponseWriter, r *http.Request) {
	su := userFrom(r.Context())
	count, _ := h.deps.Notify.UnreadCount(r.Context(), su.ID)
	render(w, r, NotifBadge(count))
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
		redirectWithFlash(w, r, "/orgs", err.Error(), true)
		return
	}
	su := userFrom(r.Context())
	_ = h.deps.Org.AddMember(r.Context(), o.ID, su.ID, "admin")
	redirectWithFlash(w, r, "/orgs/"+o.Name, "Organization created", false)
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
	if err != nil {
		redirectWithFlash(w, r, "/orgs/"+o.Name, "User not found", true)
		return
	}
	if err := h.deps.Org.AddMember(r.Context(), o.ID, u.ID, r.FormValue("role")); err != nil {
		redirectWithFlash(w, r, "/orgs/"+o.Name, err.Error(), true)
		return
	}
	redirectWithFlash(w, r, "/orgs/"+o.Name, "Member added", false)
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
	header, nav := h.repoPageCtx(r, repository, "code")
	render(w, r, RepoPage(RepoPageData{
		Layout: h.layout(r, repository.FullName),
		Header: header, Nav: nav,
		Repo: repository, Ref: ref, Entries: entries,
		Branches: h.repoBranches(r, repository),
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
	header, nav := h.repoPageCtx(r, repository, "code")
	render(w, r, RepoPage(RepoPageData{
		Layout: h.layout(r, repository.FullName),
		Header: header, Nav: nav,
		Repo: repository, Path: path, Ref: ref, Entries: entries,
		Branches: h.repoBranches(r, repository),
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
	isBinary := false
	if err == nil {
		isBinary = isBinaryContent(data)
		if !isBinary {
			content = string(data)
		}
	}
	header, nav := h.repoPageCtx(r, repository, "code")
	render(w, r, RepoPage(RepoPageData{
		Layout: h.layout(r, repository.FullName),
		Header: header, Nav: nav,
		Repo: repository, Path: path, Ref: ref, Content: content, IsBinary: isBinary,
		Branches: h.repoBranches(r, repository),
	}))
}

func (h *Handler) handleRepoBlame(w http.ResponseWriter, r *http.Request) {
	repository, ok := h.getRepo(w, r, "read")
	if !ok {
		return
	}
	ref := r.URL.Query().Get("ref")
	if ref == "" {
		ref = repository.DefaultBranch
	}
	path := strings.TrimPrefix(chi.URLParam(r, "*"), "/")
	blameLines, err := h.deps.Git.GetBlame(repository.OwnerName, repository.Name, ref, path)
	if err != nil {
		// If blame fails (e.g. binary file, empty repo), fall back to blob view
		h.handleRepoBlob(w, r)
		return
	}
	header, nav := h.repoPageCtx(r, repository, "code")
	render(w, r, RepoPage(RepoPageData{
		Layout:     h.layout(r, repository.FullName),
		Header:     header, Nav: nav,
		Repo:       repository, Path: path, Ref: ref,
		BlameLines: blameLines,
		Branches:   h.repoBranches(r, repository),
	}))
}

func (h *Handler) handleFileFinder(w http.ResponseWriter, r *http.Request) {
	repository, ok := h.getRepo(w, r, "read")
	if !ok {
		return
	}
	ref := r.URL.Query().Get("ref")
	if ref == "" {
		ref = repository.DefaultBranch
	}
	files, err := h.deps.Git.GetAllFiles(repository.OwnerName, repository.Name, ref)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(files)
}

func (h *Handler) handleIssues(w http.ResponseWriter, r *http.Request) {
	repository, ok := h.getRepo(w, r, "read")
	if !ok {
		return
	}

	state := r.URL.Query().Get("state")
	if state == "" {
		state = "open"
	}

	page, limit := parsePageLimit(r, 1, 25)
	offset := (page - 1) * limit

	issues, _ := h.deps.Issues.ListPaginated(r.Context(), repository.ID, limit, offset)
	openCount, closedCount, _ := h.deps.Issues.CountByRepo(r.Context(), repository.ID)

	header, nav := h.repoPageCtx(r, repository, "issues")
	render(w, r, IssuesPage(h.layout(r, "Issues"), header, nav, issues, state, csrfFrom(r.Context()), page, limit, openCount, closedCount))
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
	header, nav := h.repoPageCtx(r, repository, "issues")
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
	renderToast(w, r, "Assignee updated", "success")
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
	renderToast(w, r, "Milestone updated", "success")
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
	header, nav := h.repoPageCtx(r, repository, "pulls")
	render(w, r, PullsPage(h.layout(r, "Pull requests"), header, nav, prs, state, repository.DefaultBranch, csrfFrom(r.Context())))
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
	header, nav := h.repoPageCtx(r, repository, "pulls")
	author := "author"
	if u, err := h.deps.Auth.GetUser(r.Context(), pr.AuthorID); err == nil {
		author = u.Username
	}
	render(w, r, PRDetailPage(h.layout(r, pr.Title), PRDetailData{
		Header: header, Nav: nav,
		PR: pr, Author: author, Reviews: reviews, Comments: comments, Files: files, CSRF: csrfFrom(r.Context()),
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
	wfRows, err := h.deps.Pool.Query(r.Context(), `
		SELECT id, name, path, active FROM workflows WHERE repo_id=$1 ORDER BY name`, repository.ID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer wfRows.Close()
	var workflows []WorkflowInfo
	for wfRows.Next() {
		var wf WorkflowInfo
		var id uuid.UUID
		if err := wfRows.Scan(&id, &wf.Name, &wf.Path, &wf.Active); err != nil {
			continue
		}
		wf.ID = id.String()
		workflows = append(workflows, wf)
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
	header, nav := h.repoPageCtx(r, repository, "actions")
	render(w, r, ActionsPage(h.layout(r, "Actions"), header, nav, workflows, runs, csrfFrom(r.Context())))
}

func (h *Handler) handleBadge(w http.ResponseWriter, r *http.Request) {
	repository, ok := h.getRepo(w, r, "read")
	if !ok {
		// Return "inactive" badge for unknown repos
		w.Header().Set("Content-Type", "image/svg+xml")
		w.Header().Set("Cache-Control", "no-cache, max-age=60")
		svg := badgeSVG("CI", "inactive", "#656d76")
		w.Write([]byte(svg))
		return
	}

	var conclusion, status string
	err := h.deps.Pool.QueryRow(r.Context(), `
		SELECT COALESCE(conclusion,''), status
		FROM workflow_runs
		WHERE repo_id=$1
		ORDER BY created_at DESC
		LIMIT 1`, repository.ID).Scan(&conclusion, &status)
	if err != nil {
		// No runs yet — return "not run" badge
		w.Header().Set("Content-Type", "image/svg+xml")
		w.Header().Set("Cache-Control", "no-cache, max-age=60")
		svg := badgeSVG("CI", "not run", "#656d76")
		w.Write([]byte(svg))
		return
	}

	w.Header().Set("Content-Type", "image/svg+xml")
	w.Header().Set("Cache-Control", "no-cache, max-age=60")
	svg := badgeSVG("CI", badgeValue(conclusion, status), badgeColor(conclusion, status))
	w.Write([]byte(svg))
}

func (h *Handler) handleTriggerAction(w http.ResponseWriter, r *http.Request) {
	if !h.requirePOST(w, r) {
		return
	}
	repository, ok := h.getRepo(w, r, "write")
	if !ok {
		return
	}
	workflowID, err := uuid.Parse(r.FormValue("workflow_id"))
	if err != nil {
		http.Redirect(w, r, "/"+repository.FullName+"/actions", http.StatusSeeOther)
		return
	}
	branch := r.FormValue("branch")
	if branch == "" {
		branch = repository.DefaultBranch
	}
	sha, err := h.deps.Git.UpdateHead(repository.OwnerName, repository.Name, branch)
	if err != nil {
		http.Redirect(w, r, "/"+repository.FullName+"/actions", http.StatusSeeOther)
		return
	}
	var runNumber int
	if err := h.deps.Pool.QueryRow(r.Context(), `SELECT COALESCE(MAX(run_number),0)+1 FROM workflow_runs WHERE repo_id=$1`, repository.ID).Scan(&runNumber); err != nil {
		http.Redirect(w, r, "/"+repository.FullName+"/actions", http.StatusSeeOther)
		return
	}
	_, err = h.deps.Pool.Exec(r.Context(), `
		INSERT INTO workflow_runs (repo_id, workflow_id, run_number, event, head_sha, head_branch, status)
		VALUES ($1,$2,$3,'workflow_dispatch',$4,$5,'queued')`,
		repository.ID, workflowID, runNumber, sha, branch)
	if err != nil {
		http.Redirect(w, r, "/"+repository.FullName+"/actions", http.StatusSeeOther)
		return
	}
	redirectWithFlash(w, r, "/"+repository.FullName+"/actions", "Workflow triggered", false)
}

func (h *Handler) handleActionLogs(w http.ResponseWriter, r *http.Request) {
	repository, ok := h.getRepo(w, r, "read")
	if !ok {
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
	logURL := "/" + repository.OwnerName + "/" + repository.Name + "/actions/runs/" + runID.String() + "/logs"
	render(w, r, ActionLogsPartial(logs.String(), logURL))
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
	header, nav := h.repoPageCtx(r, repository, "releases")
	render(w, r, ReleasesPage(h.layout(r, "Releases"), header, nav, rows, csrfFrom(r.Context())))
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
	_, err := h.deps.Releases.Create(r.Context(), repository.ID, su.ID, r.FormValue("tag"), r.FormValue("name"), r.FormValue("body"), false, false)
	if err != nil {
		redirectWithFlash(w, r, "/"+repository.FullName+"/releases", err.Error(), true)
		return
	}
	redirectWithFlash(w, r, "/"+repository.FullName+"/releases", "Release created", false)
}

func (h *Handler) handleTags(w http.ResponseWriter, r *http.Request) {
	repository, ok := h.getRepo(w, r, "read")
	if !ok {
		return
	}
	tags, _ := h.deps.Git.ListTags(repository.OwnerName, repository.Name)
	header, nav := h.repoPageCtx(r, repository, "tags")
	render(w, r, TagsPage(h.layout(r, "Tags"), header, nav, tags, csrfFrom(r.Context())))
}

func (h *Handler) handleCreateTag(w http.ResponseWriter, r *http.Request) {
	if !h.requirePOST(w, r) {
		return
	}
	repository, ok := h.getRepo(w, r, "write")
	if !ok {
		return
	}
	tagName := r.FormValue("name")
	ref := r.FormValue("ref")
	if tagName == "" {
		redirectWithFlash(w, r, "/"+repository.FullName+"/tags", "Tag name is required", true)
		return
	}
	if ref == "" {
		ref = repository.DefaultBranch
	}
	if err := h.deps.Git.CreateTag(repository.OwnerName, repository.Name, tagName, ref); err != nil {
		redirectWithFlash(w, r, "/"+repository.FullName+"/tags", err.Error(), true)
		return
	}
	redirectWithFlash(w, r, "/"+repository.FullName+"/tags", "Tag created", false)
}

func (h *Handler) handlePackages(w http.ResponseWriter, r *http.Request) {
	repository, ok := h.getRepo(w, r, "read")
	if !ok {
		return
	}
	pkgs, _ := h.deps.Packages.List(r.Context(), repository.ID)
	header, nav := h.repoPageCtx(r, repository, "packages")
	render(w, r, PackagesPage(h.layout(r, "Packages"), header, nav, pkgs, csrfFrom(r.Context())))
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
	header, nav := h.repoPageCtx(r, repository, "settings")
	data := RepoSettingsData{
		Header:            header,
		Nav:               nav,
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
	header, nav := h.repoPageCtx(r, repository, "wiki")
	render(w, r, WikiPage(h.layout(r, "Wiki"), header, nav, pages))
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
	header, nav := h.repoPageCtx(r, repository, "wiki")
	render(w, r, WikiViewPage(h.layout(r, page.Title), header, nav, page, html, csrfFrom(r.Context())))
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
	header, nav := h.repoPageCtx(r, repository, "wiki")
	render(w, r, WikiEditPage(h.layout(r, "Edit wiki"), header, nav, slug, title, content, csrfFrom(r.Context())))
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
	redirectWithFlash(w, r, redirectReferer(r, "/"+repository.FullName), "Repository starred", false)
}

func (h *Handler) handleUnstar(w http.ResponseWriter, r *http.Request) {
	if !h.requirePOST(w, r) {
		return
	}
	repository, ok := h.getRepo(w, r, "read")
	if !ok {
		return
	}
	su := userFrom(r.Context())
	_ = h.deps.Repos.Unstar(r.Context(), repository.ID, su.ID)
	redirectWithFlash(w, r, redirectReferer(r, "/"+repository.FullName), "Unstarred", false)
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
	redirectWithFlash(w, r, redirectReferer(r, "/"+repository.FullName), "Watching", false)
}

func (h *Handler) handleUnwatch(w http.ResponseWriter, r *http.Request) {
	if !h.requirePOST(w, r) {
		return
	}
	repository, ok := h.getRepo(w, r, "read")
	if !ok {
		return
	}
	su := userFrom(r.Context())
	_ = h.deps.Repos.Unwatch(r.Context(), repository.ID, su.ID)
	redirectWithFlash(w, r, redirectReferer(r, "/"+repository.FullName), "Unwatched", false)
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

	// Determine target namespace from form, default to current user's personal namespace
	namespace := r.FormValue("namespace")

	ownerType := "user"
	ownerID := su.ID
	ownerName := su.Username

	if namespace != "" && namespace != su.Username {
		// Check if namespace matches an org the user is a member of
		orgs, err := h.deps.Org.ListForUser(r.Context(), su.ID)
		if err == nil {
			for _, o := range orgs {
				if o.Name == namespace {
					ownerType = "org"
					ownerID = o.ID
					ownerName = o.Name
					break
				}
			}
		}
	}

	fork, err := h.deps.Repos.Fork(r.Context(), repository, ownerType, ownerID, ownerName)
	if err != nil {
		ref := r.URL.Query().Get("ref")
		if ref == "" {
			ref = repository.DefaultBranch
		}
		entries, _ := h.deps.Git.GetTree(repository.OwnerName, repository.Name, ref, "")
		header, nav := h.repoPageCtx(r, repository, "code")
		render(w, r, RepoPage(RepoPageData{
			Layout: h.layout(r, repository.FullName),
			Header: header, Nav: nav,
			Repo: repository, Ref: ref, Entries: entries,
			Branches: h.repoBranches(r, repository),
			Flash: err.Error(), FlashErr: true,
		}))
		return
	}
	http.Redirect(w, r, "/"+fork.FullName, http.StatusSeeOther)
}

func (h *Handler) handleForkDialog(w http.ResponseWriter, r *http.Request) {
	repository, ok := h.getRepo(w, r, "read")
	if !ok {
		http.Error(w, "Repository not found", http.StatusNotFound)
		return
	}
	su := userFrom(r.Context())
	if su == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Build list of namespaces: personal + orgs
	namespaces := []ForkNamespace{
		{Kind: "user", Name: su.Username, DisplayName: su.Username},
	}

	orgs, err := h.deps.Org.ListForUser(r.Context(), su.ID)
	if err == nil {
		for _, o := range orgs {
			display := o.DisplayName
			if display == "" {
				display = o.Name
			}
			namespaces = append(namespaces, ForkNamespace{
				Kind:        "org",
				Name:        o.Name,
				DisplayName: display,
			})
		}
	}

	render(w, r, ForkDialog(ForkDialogData{
		Owner:      repository.OwnerName,
		Repo:       repository.Name,
		FullName:   repository.FullName,
		CSRF:       csrfFrom(r.Context()),
		Namespaces: namespaces,
	}))
}

func (h *Handler) handleMilestones(w http.ResponseWriter, r *http.Request) {
	repository, ok := h.getRepo(w, r, "read")
	if !ok {
		return
	}
	ms, _ := h.deps.Issues.ListMilestones(r.Context(), repository.ID)
	header, nav := h.repoPageCtx(r, repository, "milestones")
	render(w, r, MilestonesPage(h.layout(r, "Milestones"), header, nav, ms, csrfFrom(r.Context())))
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

func (h *Handler) handleDeleteRepo(w http.ResponseWriter, r *http.Request) {
	if !h.requirePOST(w, r) {
		return
	}
	repository, ok := h.getRepo(w, r, "write")
	if !ok {
		return
	}
	confirmName := r.FormValue("confirm")
	if confirmName != repository.Name {
		redirectWithFlash(w, r, "/"+repository.FullName+"/settings", "Repository name does not match", true)
		return
	}
	if err := h.deps.Git.Remove(repository.OwnerName, repository.Name); err != nil {
		redirectWithFlash(w, r, "/"+repository.FullName+"/settings", "Failed to remove git data: "+err.Error(), true)
		return
	}
	if err := h.deps.Repos.Delete(r.Context(), repository.ID); err != nil {
		redirectWithFlash(w, r, "/"+repository.FullName+"/settings", "Failed to delete repository: "+err.Error(), true)
		return
	}
	if h.deps.Audit != nil {
		_ = h.deps.Audit.Record(r.Context(), userFrom(r.Context()).ID, "repo.delete", "repo", repository.ID.String(), map[string]string{
			"full_name": repository.FullName,
		})
	}
	redirectWithFlash(w, r, "/", "Repository deleted", false)
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

// handleUploadImage handles inline image paste in issue/PR comment forms.
// It accepts a multipart file upload, saves it as a uniquely-named image,
// and returns the public URL path as plain text so the client can insert
// it as ![alt](url) into the Markdown textarea.
func (h *Handler) handleUploadImage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 10<<20) // 10 MB max
	if err := r.ParseMultipartForm(10 << 20); err != nil {
		http.Error(w, "file too large", http.StatusRequestEntityTooLarge)
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "missing file", http.StatusBadRequest)
		return
	}
	defer file.Close()

	// Validate content type is an image
	contentType := header.Header.Get("Content-Type")
	if contentType == "" {
		buf := make([]byte, 512)
		n, _ := io.ReadFull(file, buf)
		contentType = http.DetectContentType(buf[:n])
		// Reconstruct the file reader with the buffered bytes
		var rdr io.Reader = io.MultiReader(bytes.NewReader(buf[:n]), file)
		file = newMultiFile(rdr)
	}
	if !strings.HasPrefix(contentType, "image/") {
		http.Error(w, "file must be an image", http.StatusBadRequest)
		return
	}

	// Create upload directory if needed
	dir := h.deps.UploadDir
	if dir == "" {
		dir = "/data/uploads"
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}

	// Generate unique filename preserving extension
	ext := ".png"
	if idx := strings.LastIndex(header.Filename, "."); idx >= 0 {
		ext = header.Filename[idx:]
	}
	name := uuid.New().String() + ext
	dst, err := os.Create(dir + "/" + name)
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	defer dst.Close()
	if _, err := io.Copy(dst, file); err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}

	// Return the public URL path for the image
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Write([]byte("/uploads/" + name))
}

// multiFile wraps an io.Reader to satisfy multipart.File interface.
type multiFile struct {
	io.Reader
}

func newMultiFile(r io.Reader) *multiFile {
	return &multiFile{Reader: r}
}

func (m *multiFile) Close() error { return nil }
func (m *multiFile) Read(p []byte) (int, error) { return m.Reader.Read(p) }
func (m *multiFile) ReadAt(p []byte, off int64) (int, error) { return 0, nil }
func (m *multiFile) Seek(offset int64, whence int) (int64, error) { return 0, nil }

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
	if err := h.deps.Git.Init(r.Context(), repo.OwnerName, repo.Name); err != nil {
		http.Redirect(w, r, "/orgs/"+o.Name, http.StatusSeeOther)
		return
	}
	_, _ = h.deps.Git.SeedMainBranch(repo.OwnerName, repo.Name, repo.DefaultBranch)
	http.Redirect(w, r, "/"+repo.FullName, http.StatusSeeOther)
}

func (h *Handler) handleCommitDetail(w http.ResponseWriter, r *http.Request) {
	repository, ok := h.getRepo(w, r, "read")
	if !ok {
		return
	}
	sha := chi.URLParam(r, "sha")
	if sha == "" {
		http.NotFound(w, r)
		return
	}
	commit, err := h.deps.Git.GetCommitDetail(repository.OwnerName, repository.Name, sha)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	header, nav := h.repoPageCtx(r, repository, "code")
	render(w, r, CommitDetailPage(h.layout(r, commit.ShortSHA+" · "+repository.FullName), CommitDetailData{
		Header: header, Nav: nav, Commit: commit, CSRF: csrfFrom(r.Context()),
	}))
}

func (h *Handler) handleCompare(w http.ResponseWriter, r *http.Request) {
	repository, ok := h.getRepo(w, r, "read")
	if !ok {
		return
	}
	// Extract {base}...{head} from the wildcard path
	path := chi.URLParam(r, "*")
	parts := strings.SplitN(path, "...", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		http.Error(w, "expected {base}...{head} format", http.StatusBadRequest)
		return
	}
	baseRef := parts[0]
	headRef := parts[1]
	result, err := h.deps.Git.CompareCommits(repository.OwnerName, repository.Name, baseRef, headRef)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	header, nav := h.repoPageCtx(r, repository, "code")
	render(w, r, ComparePage(h.layout(r, "Comparing "+baseRef+"..."+headRef+" · "+repository.FullName), CompareData{
		Header: header, Nav: nav, Result: result, Base: baseRef, Head: headRef, CSRF: csrfFrom(r.Context()),
	}))
}

func (h *Handler) handlePreviewMarkdown(w http.ResponseWriter, r *http.Request) {
	if !h.requirePOST(w, r) {
		return
	}
	body := r.FormValue("body")
	html, err := RenderMarkdown(body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(html))
}
