package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/rusik69/govnohub/internal/actions"
	"github.com/rusik69/govnohub/internal/aireview"
	"github.com/rusik69/govnohub/internal/audit"
	"github.com/rusik69/govnohub/internal/auth"
	gitstore "github.com/rusik69/govnohub/internal/git"
	"github.com/rusik69/govnohub/internal/issue"
	pkg "github.com/rusik69/govnohub/internal/package"
	"github.com/rusik69/govnohub/internal/notification"
	"github.com/rusik69/govnohub/internal/org"
	"github.com/rusik69/govnohub/internal/pull"
	"github.com/rusik69/govnohub/internal/release"
	"github.com/rusik69/govnohub/internal/repo"
	"github.com/rusik69/govnohub/internal/search"
	"github.com/rusik69/govnohub/internal/web"
	"github.com/rusik69/govnohub/internal/webhook"
	"github.com/rusik69/govnohub/internal/wiki"
)

type Server struct {
	auth     *auth.Service
	repos    *repo.Service
	git      *gitstore.Store
	issues   *issue.Service
	pulls    *pull.Service
	releases *release.Service
	packages *pkg.Service
	webhooks *webhook.Service
	search   *search.Service
	pool     *pgxpool.Pool
	org      *org.Service
	aiReview *aireview.Service
	notify   *notification.Service
	wiki     *wiki.Service
	audit    *audit.Service
	web      *web.Handler
}

func NewServer(
	authSvc *auth.Service,
	repoSvc *repo.Service,
	gitStore *gitstore.Store,
	issueSvc *issue.Service,
	pullSvc *pull.Service,
	releaseSvc *release.Service,
	pkgSvc *pkg.Service,
	webhookSvc *webhook.Service,
	searchSvc *search.Service,
	pool *pgxpool.Pool,
	orgSvc *org.Service,
	aiReviewSvc *aireview.Service,
	notifySvc *notification.Service,
	wikiSvc *wiki.Service,
	auditSvc *audit.Service,
) *Server {
	s := &Server{
		auth: authSvc, repos: repoSvc, git: gitStore,
		issues: issueSvc, pulls: pullSvc, releases: releaseSvc,
		packages: pkgSvc, webhooks: webhookSvc, search: searchSvc,
		pool: pool, org: orgSvc, aiReview: aiReviewSvc,
		notify: notifySvc, wiki: wikiSvc, audit: auditSvc,
	}
	s.web = web.NewHandler(web.Deps{
		Auth: authSvc, Repos: repoSvc, Git: gitStore, Issues: issueSvc,
		Pulls: pullSvc, Releases: releaseSvc, Packages: pkgSvc,
		Webhooks: webhookSvc, Search: searchSvc, Pool: pool,
		Org: orgSvc, AIReview: aiReviewSvc, Notify: notifySvc, Wiki: wikiSvc,
		Audit: auditSvc,
	})
	return s
}

func (s *Server) Router() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.Logger, middleware.Recoverer)
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"*"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type"},
		AllowCredentials: false,
	}))

	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		jsonOK(w, map[string]string{"status": "ok"})
	})

	r.Post("/api/v1/users", s.handleRegister)
	r.Post("/api/v1/auth/login", s.handleLogin)

	r.Route("/api/v1", func(r chi.Router) {
		r.Use(s.authenticate)
		r.Get("/user", s.handleCurrentUser)
		r.Post("/user/tokens", s.handleCreatePAT)
		r.Get("/user/tokens", s.handleListPATs)
		r.Delete("/user/tokens/{tokenID}", s.handleRevokePAT)
		r.Post("/user/ssh-keys", s.handleCreateSSHKey)
		r.Get("/user/ssh-keys", s.handleListSSHKeys)
		r.Delete("/user/ssh-keys/{keyID}", s.handleDeleteSSHKey)
		r.Get("/user/repos", s.handleListUserRepos)
		r.Get("/search", s.handleSearch)
		r.Get("/notifications", s.handleListNotifications)
		r.Post("/notifications/{id}/read", s.handleMarkNotificationRead)
		s.registerOrgRoutes(r)
		s.registerAdminRoutes(r)

		r.Post("/orgs/{org}/repos", s.handleCreateOrgRepo)
		r.Post("/users/{user}/repos", s.handleCreateUserRepo)

		r.Route("/repos/{owner}/{repo}", func(r chi.Router) {
			r.Get("/", s.handleGetRepo)
			r.Patch("/", s.handleUpdateRepo)
			r.Delete("/", s.handleDeleteRepo)
			r.Get("/contents/*", s.handleGetContents)
			r.Get("/commits", s.handleGetCommits)
			r.Post("/star", s.handleStar)
			r.Delete("/star", s.handleUnstar)
			r.Post("/watch", s.handleWatch)
			r.Delete("/watch", s.handleUnwatch)
			r.Post("/fork", s.handleFork)

			r.Get("/branches", s.handleListBranches)
			r.Get("/tags", s.handleListTags)
			r.Post("/tags", s.handleCreateTag)

			r.Get("/compare/*", s.handleCompareCommits)

			r.Get("/archive/{ref}.tar.gz", s.handleArchive)

		r.Get("/issues", s.handleListIssues)
			r.Post("/issues", s.handleCreateIssue)
			r.Get("/issues/{number}", s.handleGetIssue)
			r.Get("/issues/{number}/comments", s.handleListIssueComments)
			r.Post("/issues/{number}/comments", s.handleAddIssueComment)
			r.Post("/issues/{number}/close", s.handleCloseIssue)
			r.Patch("/issues/{number}", s.handlePatchIssue)
			r.Get("/issues/{number}/timeline", s.handleGetIssueTimeline)
			r.Put("/issues/{number}/assignees", s.handleSetIssueAssignees)
			r.Post("/issues/{number}/reactions", s.handleAddIssueReaction)
			r.Get("/issues/{number}/reactions", s.handleListIssueReactions)
			r.Delete("/issues/{number}/reactions/{reactionID}", s.handleDeleteIssueReaction)
			r.Post("/issues/{number}/labels/{labelID}", s.handleAddIssueLabel)
			r.Delete("/issues/{number}/labels/{labelID}", s.handleRemoveIssueLabel)
			r.Get("/labels", s.handleListLabels)
			r.Post("/labels", s.handleCreateLabel)
			r.Get("/milestones", s.handleListMilestones)
			r.Post("/milestones", s.handleCreateMilestone)
			r.Post("/milestones/{milestoneID}/close", s.handleCloseMilestone)

			r.Get("/pulls", s.handleListPRs)
			r.Post("/pulls", s.handleCreatePR)
			r.Get("/pulls/{number}", s.handleGetPR)
			r.Get("/pulls/{number}/reviews", s.handleListPRReviews)
			r.Post("/pulls/{number}/reviews", s.handleAddReview)
			r.Post("/pulls/{number}/merge", s.handleMergePR)
			r.Put("/pulls/{number}/update-branch", s.handleUpdatePRBranch)
			r.Get("/pulls/{number}/diff", s.handlePRDiff)
			r.Get("/pulls/{number}/commits", s.handlePRCommits)
			r.Get("/pulls/{number}/files", s.handlePRFiles)
			r.Get("/pulls/{number}/comments", s.handleListPRComments)
			r.Post("/pulls/{number}/comments", s.handleAddPRComment)
			r.Get("/pulls/{number}/ai-reviews", s.handleListAIReviews)
			r.Post("/pulls/{number}/ai-reviews", s.handleCreateAIReview)
			r.Get("/ai-review/config", s.handleAIReviewConfig)

			r.Get("/actions/workflows", s.handleListWorkflows)
			r.Post("/actions/workflows", s.handleUpsertWorkflow)
			r.Get("/actions/runs", s.handleListRuns)
			r.Post("/actions/runs", s.handleTriggerRun)
			r.Get("/actions/runs/{runID}/logs", s.handleRunLogs)

			r.Get("/releases", s.handleListReleases)
			r.Post("/releases", s.handleCreateRelease)
			r.Post("/releases/{tag}/assets", s.handleUploadAsset)
			r.Get("/releases/{tag}/assets", s.handleListReleaseAssets)
			r.Get("/releases/{tag}/assets/{name}", s.handleDownloadReleaseAsset)

			r.Get("/packages", s.handleListPackages)
			r.Post("/packages", s.handlePublishPackage)
			r.Get("/packages/{name}/{version}", s.handleDownloadPackage)

			r.Get("/webhooks", s.handleListWebhooks)
			r.Post("/webhooks", s.handleCreateWebhook)
			r.Get("/webhooks/{webhookID}/deliveries", s.handleListWebhookDeliveries)

			r.Post("/branches", s.handleCreateBranch)
			r.Get("/protected-branches", s.handleListProtectedBranches)
			r.Post("/protected-branches", s.handleProtectBranch)

			r.Get("/collaborators", s.handleListCollaborators)
			r.Put("/collaborators/{username}", s.handleAddCollaborator)
			r.Delete("/collaborators/{username}", s.handleRemoveCollaborator)

			r.Route("/git", func(r chi.Router) {
				r.Post("/blobs", s.handleCreateGitBlob)
				r.Post("/trees", s.handleCreateGitTree)
				r.Post("/commits", s.handleCreateGitCommit)
				r.Post("/refs", s.handleCreateGitRef)
			})

			r.Get("/wiki", s.handleListWikiPages)
			r.Get("/wiki/{slug}", s.handleGetWikiPage)
			r.Put("/wiki/{slug}", s.handleUpsertWikiPage)
			r.Delete("/wiki/{slug}", s.handleDeleteWikiPage)
		})
	})
	r.Mount("/", s.web.Routes())
	return r
}

type ctxKey string

const (
	userIDKey  ctxKey = "userID"
	scopesKey  ctxKey = "scopes"
	isPATKey   ctxKey = "isPAT"
)

func (s *Server) authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := r.Header.Get("Authorization")
		if h == "" {
			jsonError(w, http.StatusUnauthorized, "missing authorization")
			return
		}
		token := strings.TrimPrefix(h, "Bearer ")
		var userID uuid.UUID
		var err error
		ctx := r.Context()
		if strings.HasPrefix(token, "ghp_") {
			var scopes []string
			userID, scopes, err = s.auth.ValidatePATWithScopes(ctx, token)
			if err != nil {
				jsonError(w, http.StatusUnauthorized, "invalid token")
				return
			}
			ctx = context.WithValue(ctx, scopesKey, scopes)
			ctx = context.WithValue(ctx, isPATKey, true)
		} else {
			userID, _, err = s.auth.ValidateToken(token)
		}
		if err != nil {
			jsonError(w, http.StatusUnauthorized, "invalid token")
			return
		}
		ctx = context.WithValue(ctx, userIDKey, userID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func isPATAuth(ctx context.Context) bool {
	v, _ := ctx.Value(isPATKey).(bool)
	return v
}

func scopesFrom(ctx context.Context) []string {
	scopes, _ := ctx.Value(scopesKey).([]string)
	return scopes
}

func (s *Server) requireScope(w http.ResponseWriter, r *http.Request, scope string) bool {
	if !isPATAuth(r.Context()) {
		return true
	}
	if !auth.HasScope(scopesFrom(r.Context()), scope) {
		jsonError(w, http.StatusForbidden, "insufficient token scope")
		return false
	}
	return true
}

func userIDFrom(ctx context.Context) uuid.UUID {
	id, _ := ctx.Value(userIDKey).(uuid.UUID)
	return id
}

func (s *Server) handleRegister(w http.ResponseWriter, r *http.Request) {
	if !s.auth.AllowPublicRegistration() {
		jsonError(w, http.StatusForbidden, "public registration is disabled")
		return
	}
	var req struct {
		Username string `json:"username"`
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, http.StatusBadRequest, "invalid body")
		return
	}
	u, err := s.auth.Register(r.Context(), req.Username, req.Email, req.Password)
	if err != nil {
		jsonError(w, http.StatusConflict, err.Error())
		return
	}
	jsonOK(w, u)
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, http.StatusBadRequest, "invalid body")
		return
	}
	token, u, err := s.auth.Login(r.Context(), req.Username, req.Password)
	if err != nil {
		jsonError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}
	jsonOK(w, map[string]interface{}{"token": token, "user": u})
}

func (s *Server) handleCurrentUser(w http.ResponseWriter, r *http.Request) {
	if isPATAuth(r.Context()) {
		scopes := scopesFrom(r.Context())
		if !auth.HasScope(scopes, auth.ScopeReadUser) && !auth.HasScope(scopes, auth.ScopeRepo) {
			jsonError(w, http.StatusForbidden, "insufficient token scope")
			return
		}
	}
	u, err := s.auth.GetUser(r.Context(), userIDFrom(r.Context()))
	if err != nil {
		jsonError(w, http.StatusNotFound, "user not found")
		return
	}
	jsonOK(w, u)
}

func (s *Server) handleCreatePAT(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name   string   `json:"name"`
		Scopes []string `json:"scopes"`
	}
	json.NewDecoder(r.Body).Decode(&req)
	token, err := s.auth.CreatePAT(r.Context(), userIDFrom(r.Context()), req.Name, req.Scopes)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	jsonOK(w, map[string]string{"token": token})
}

func (s *Server) handleListPATs(w http.ResponseWriter, r *http.Request) {
	tokens, err := s.auth.ListPATs(r.Context(), userIDFrom(r.Context()))
	if err != nil {
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if tokens == nil {
		tokens = []auth.PATInfo{}
	}
	jsonOK(w, tokens)
}

func (s *Server) handleRevokePAT(w http.ResponseWriter, r *http.Request) {
	patID, err := uuid.Parse(chi.URLParam(r, "tokenID"))
	if err != nil {
		jsonError(w, http.StatusBadRequest, "invalid token id")
		return
	}
	if err := s.auth.RevokePAT(r.Context(), userIDFrom(r.Context()), patID); err != nil {
		jsonError(w, http.StatusNotFound, err.Error())
		return
	}
	jsonOK(w, map[string]string{"status": "revoked"})
}

func (s *Server) handleListUserRepos(w http.ResponseWriter, r *http.Request) {
	repos, err := s.repos.ListForUser(r.Context(), userIDFrom(r.Context()))
	if err != nil {
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if repos == nil {
		repos = []repo.Repository{}
	}
	jsonOK(w, repos)
}

func (s *Server) handleCreateUserRepo(w http.ResponseWriter, r *http.Request) {
	if !s.requireScope(w, r, auth.ScopeRepoWrite) {
		return
	}
	username := chi.URLParam(r, "user")
	u, err := s.auth.GetUser(r.Context(), userIDFrom(r.Context()))
	if err != nil || u == nil {
		jsonError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if u.Username != username {
		jsonError(w, http.StatusForbidden, "forbidden")
		return
	}
	s.createRepo(w, r, "user", u.ID, username)
}

func (s *Server) handleCreateOrgRepo(w http.ResponseWriter, r *http.Request) {
	ownerType, ownerID, err := s.repos.ResolveOwnerID(r.Context(), chi.URLParam(r, "org"))
	if err != nil {
		jsonError(w, http.StatusNotFound, err.Error())
		return
	}
	if ownerType != "org" {
		jsonError(w, http.StatusNotFound, "org not found")
		return
	}
	member, err := s.org.IsMember(r.Context(), ownerID, userIDFrom(r.Context()))
	if err != nil || !member {
		jsonError(w, http.StatusForbidden, "forbidden")
		return
	}
	s.createRepo(w, r, ownerType, ownerID, chi.URLParam(r, "org"))
}

func (s *Server) createRepo(w http.ResponseWriter, r *http.Request, ownerType string, ownerID uuid.UUID, ownerName string) {
	var req struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		Private     bool   `json:"private"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, http.StatusBadRequest, "invalid body")
		return
	}
	repository, err := s.repos.Create(r.Context(), ownerType, ownerID, ownerName, req.Name, req.Description, req.Private)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := s.git.Init(r.Context(), ownerName, req.Name); err != nil {
		s.pool.Exec(r.Context(), `DELETE FROM repos WHERE id=$1`, repository.ID)
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	sha, err := s.git.SeedMainBranch(ownerName, req.Name, repository.DefaultBranch)
	if err != nil {
		s.pool.Exec(r.Context(), `DELETE FROM repos WHERE id=$1`, repository.ID)
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	_, _ = s.pool.Exec(r.Context(), `UPDATE branches SET head_sha=$1 WHERE repo_id=$2 AND name=$3`, sha, repository.ID, repository.DefaultBranch)
	s.search.Index(r.Context(), search.Document{
		ID: "repo-" + repository.ID.String(), Type: "repo",
		Title: repository.Name, Body: repository.Description, Repo: repository.FullName,
	})
	jsonOK(w, repository)
}

func (s *Server) getRepo(w http.ResponseWriter, r *http.Request) (*repo.Repository, bool) {
	return s.getRepoPerm(w, r, "read")
}

func (s *Server) getRepoWrite(w http.ResponseWriter, r *http.Request) (*repo.Repository, bool) {
	return s.getRepoPerm(w, r, "write")
}

func (s *Server) getRepoPerm(w http.ResponseWriter, r *http.Request, perm string) (*repo.Repository, bool) {
	repository, err := s.repos.GetByFullName(r.Context(), chi.URLParam(r, "owner"), chi.URLParam(r, "repo"))
	if err != nil {
		jsonError(w, http.StatusNotFound, "repo not found")
		return nil, false
	}
	if isPATAuth(r.Context()) {
		scope := auth.ScopeRepo
		if perm == "write" {
			scope = auth.ScopeRepoWrite
		}
		if !s.requireScope(w, r, scope) {
			return nil, false
		}
	}
	ok, _ := s.repos.CanAccess(r.Context(), repository.ID, userIDFrom(r.Context()), perm)
	if !ok {
		jsonError(w, http.StatusForbidden, "forbidden")
		return nil, false
	}
	return repository, true
}

func (s *Server) handleGetRepo(w http.ResponseWriter, r *http.Request) {
	repository, ok := s.getRepo(w, r)
	if !ok {
		return
	}
	jsonOK(w, repository)
}

func (s *Server) handleUpdateRepo(w http.ResponseWriter, r *http.Request) {
	repository, ok := s.getRepoWrite(w, r)
	if !ok {
		return
	}
	var req repo.UpdateRepoInput
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, http.StatusBadRequest, "invalid body")
		return
	}
	if !s.requireScope(w, r, auth.ScopeRepoWrite) {
		return
	}
	updated, err := s.repos.Update(r.Context(), repository.ID, req)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	jsonOK(w, updated)
}

func (s *Server) handleDeleteRepo(w http.ResponseWriter, r *http.Request) {
	repository, ok := s.getRepoWrite(w, r)
	if !ok {
		return
	}
	if !s.requireScope(w, r, auth.ScopeRepoWrite) {
		return
	}
	if err := s.git.Remove(chi.URLParam(r, "owner"), chi.URLParam(r, "repo")); err != nil {
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := s.repos.Delete(r.Context(), repository.ID); err != nil {
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if s.audit != nil {
		_ = s.audit.Record(r.Context(), userIDFrom(r.Context()), "repo.delete", "repo", repository.ID.String(), map[string]string{
			"full_name": repository.FullName,
		})
	}
	jsonOK(w, map[string]string{"status": "deleted"})
}

func (s *Server) handleGetContents(w http.ResponseWriter, r *http.Request) {
	repository, ok := s.getRepo(w, r)
	if !ok {
		return
	}
	path := strings.TrimPrefix(chi.URLParam(r, "*"), "/")
	ref := r.URL.Query().Get("ref")
	if ref == "" {
		ref = repository.DefaultBranch
	}
	if path == "" {
		entries, err := s.git.GetTree(repository.OwnerName, repository.Name, ref, "")
		if err != nil {
			jsonError(w, http.StatusNotFound, err.Error())
			return
		}
		jsonOK(w, entries)
		return
	}
	data, err := s.git.GetBlob(repository.OwnerName, repository.Name, ref, path)
	if err != nil {
		jsonError(w, http.StatusNotFound, err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Write(data)
}

func (s *Server) handleGetCommits(w http.ResponseWriter, r *http.Request) {
	repository, ok := s.getRepo(w, r)
	if !ok {
		return
	}
	ref := r.URL.Query().Get("ref")
	if ref == "" {
		ref = repository.DefaultBranch
	}
	commits, err := s.git.GetCommits(repository.OwnerName, repository.Name, ref, 50)
	if err != nil {
		jsonError(w, http.StatusNotFound, err.Error())
		return
	}
	jsonOK(w, commits)
}

func (s *Server) handleStar(w http.ResponseWriter, r *http.Request) {
	repository, ok := s.getRepoWrite(w, r)
	if !ok {
		return
	}
	if err := s.repos.Star(r.Context(), repository.ID, userIDFrom(r.Context())); err != nil {
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	jsonOK(w, map[string]string{"status": "starred"})
}

func (s *Server) handleUnstar(w http.ResponseWriter, r *http.Request) {
	repository, ok := s.getRepoWrite(w, r)
	if !ok {
		return
	}
	if err := s.repos.Unstar(r.Context(), repository.ID, userIDFrom(r.Context())); err != nil {
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	jsonOK(w, map[string]string{"status": "unstarred"})
}

func (s *Server) handleWatch(w http.ResponseWriter, r *http.Request) {
	repository, ok := s.getRepoWrite(w, r)
	if !ok {
		return
	}
	if err := s.repos.Watch(r.Context(), repository.ID, userIDFrom(r.Context())); err != nil {
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	jsonOK(w, map[string]string{"status": "watching"})
}

func (s *Server) handleUnwatch(w http.ResponseWriter, r *http.Request) {
	repository, ok := s.getRepoWrite(w, r)
	if !ok {
		return
	}
	if err := s.repos.Unwatch(r.Context(), repository.ID, userIDFrom(r.Context())); err != nil {
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	jsonOK(w, map[string]string{"status": "unwatched"})
}

func (s *Server) handleListBranches(w http.ResponseWriter, r *http.Request) {
	repository, ok := s.getRepo(w, r)
	if !ok {
		return
	}
	branches, err := s.repos.ListBranches(r.Context(), repository.ID)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	jsonOK(w, branches)
}

func (s *Server) handleListTags(w http.ResponseWriter, r *http.Request) {
	repository, ok := s.getRepo(w, r)
	if !ok {
		return
	}
	tags, err := s.git.ListTags(repository.OwnerName, repository.Name)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	jsonOK(w, tags)
}

func (s *Server) handleCreateTag(w http.ResponseWriter, r *http.Request) {
	repository, ok := s.getRepoWrite(w, r)
	if !ok {
		return
	}
	var req struct {
		Name string `json:"name"`
		Ref  string `json:"ref"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Name == "" {
		jsonError(w, http.StatusBadRequest, "tag name is required")
		return
	}
	if req.Ref == "" {
		req.Ref = repository.DefaultBranch
	}
	if err := s.git.CreateTag(repository.OwnerName, repository.Name, req.Name, req.Ref); err != nil {
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	jsonOK(w, map[string]string{"tag": req.Name})
}

func (s *Server) handleFork(w http.ResponseWriter, r *http.Request) {
	repository, ok := s.getRepoWrite(w, r)
	if !ok {
		return
	}
	u, err := s.auth.GetUser(r.Context(), userIDFrom(r.Context()))
	if err != nil || u == nil {
		jsonError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	fork, err := s.repos.Fork(r.Context(), repository, u.ID, u.Username)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	jsonOK(w, fork)
}

func (s *Server) handleListIssues(w http.ResponseWriter, r *http.Request) {
	repository, ok := s.getRepo(w, r)
	if !ok {
		return
	}
	issues, err := s.issues.List(r.Context(), repository.ID)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	jsonOK(w, issues)
}

func (s *Server) handleCreateIssue(w http.ResponseWriter, r *http.Request) {
	repository, ok := s.getRepoWrite(w, r)
	if !ok {
		return
	}
	var req struct{ Title, Body string }
	json.NewDecoder(r.Body).Decode(&req)
	i, err := s.issues.Create(r.Context(), repository.ID, userIDFrom(r.Context()), req.Title, req.Body)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.issues.RecordEvent(r.Context(), i.ID, userIDFrom(r.Context()), "opened", map[string]interface{}{
		"title": i.Title,
		"body":  i.Body,
	})
	s.search.Index(r.Context(), search.Document{
		ID: "issue-" + i.ID.String(), Type: "issue", Title: i.Title, Body: i.Body,
		Repo: repository.FullName, Ref: strconv.Itoa(i.Number),
	})
	jsonOK(w, i)
}

func (s *Server) handleGetIssue(w http.ResponseWriter, r *http.Request) {
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
	jsonOK(w, i)
}

func (s *Server) handleAddIssueComment(w http.ResponseWriter, r *http.Request) {
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
	var req struct{ Body string }
	json.NewDecoder(r.Body).Decode(&req)
	c, err := s.issues.AddComment(r.Context(), i.ID, userIDFrom(r.Context()), req.Body)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.issues.RecordEvent(r.Context(), i.ID, userIDFrom(r.Context()), "commented", map[string]interface{}{
		"comment_id": c.ID.String(),
	})
	if i.AuthorID != userIDFrom(r.Context()) {
		s.notify.NotifyAsync(i.AuthorID, "Issue comment",
			"New comment on #"+strconv.Itoa(num)+" in "+repository.FullName,
			"/"+repository.OwnerName+"/"+repository.Name+"/issues/"+strconv.Itoa(num))
	}
	jsonOK(w, c)
}

func (s *Server) handleListIssueComments(w http.ResponseWriter, r *http.Request) {
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
	comments, err := s.issues.ListComments(r.Context(), i.ID)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if comments == nil {
		comments = []issue.Comment{}
	}
	jsonOK(w, comments)
}

func (s *Server) handleCloseIssue(w http.ResponseWriter, r *http.Request) {
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
	if err := s.issues.Close(r.Context(), i.ID); err != nil {
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.issues.RecordEvent(r.Context(), i.ID, userIDFrom(r.Context()), "closed", nil)
	if i.AuthorID != userIDFrom(r.Context()) {
		s.notify.NotifyAsync(i.AuthorID, "Issue closed",
			"Issue #"+strconv.Itoa(num)+" was closed in "+repository.FullName,
			"/"+repository.OwnerName+"/"+repository.Name+"/issues/"+strconv.Itoa(num))
	}
	jsonOK(w, map[string]string{"status": "closed"})
}

func (s *Server) handleCreateLabel(w http.ResponseWriter, r *http.Request) {
	repository, ok := s.getRepoWrite(w, r)
	if !ok {
		return
	}
	var req struct{ Name, Color string }
	json.NewDecoder(r.Body).Decode(&req)
	l, err := s.issues.CreateLabel(r.Context(), repository.ID, req.Name, req.Color)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	jsonOK(w, l)
}

func (s *Server) handleListLabels(w http.ResponseWriter, r *http.Request) {
	repository, ok := s.getRepo(w, r)
	if !ok {
		return
	}
	labels, err := s.issues.ListLabels(r.Context(), repository.ID)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if labels == nil {
		labels = []issue.Label{}
	}
	jsonOK(w, labels)
}

func (s *Server) handleListPRs(w http.ResponseWriter, r *http.Request) {
	repository, ok := s.getRepo(w, r)
	if !ok {
		return
	}
	prs, err := s.pulls.List(r.Context(), repository.ID)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	jsonOK(w, prs)
}

func (s *Server) handleCreatePR(w http.ResponseWriter, r *http.Request) {
	repository, ok := s.getRepoWrite(w, r)
	if !ok {
		return
	}
	var req struct {
		Title, Body, Head, Base string
	}
	json.NewDecoder(r.Body).Decode(&req)
	headSHA, err := s.git.UpdateHead(repository.OwnerName, repository.Name, req.Head)
	if err != nil {
		jsonError(w, http.StatusBadRequest, "invalid head branch")
		return
	}
	pr, err := s.pulls.Create(r.Context(), repository.ID, userIDFrom(r.Context()), req.Title, req.Body, req.Head, req.Base, headSHA)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.runAutoAIReview(repository.OwnerName, repository.Name, pr.Number)
	jsonOK(w, pr)
}

func (s *Server) handleGetPR(w http.ResponseWriter, r *http.Request) {
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
	mergeable := true
	if pr.State == "open" {
		ok, err := s.git.CanMerge(repository.OwnerName, repository.Name, pr.BaseBranch, pr.HeadBranch)
		if err == nil {
			mergeable = ok
		}
	}
	jsonOK(w, map[string]interface{}{
		"id": pr.ID, "repo_id": pr.RepoID, "number": pr.Number,
		"title": pr.Title, "body": pr.Body, "state": pr.State,
		"author_id": pr.AuthorID, "head_branch": pr.HeadBranch,
		"base_branch": pr.BaseBranch, "head_sha": pr.HeadSHA,
		"merged_at": pr.MergedAt, "merge_sha": pr.MergeSHA,
		"created_at": pr.CreatedAt, "updated_at": pr.UpdatedAt,
		"mergeable": mergeable,
	})
}

func (s *Server) handleAddReview(w http.ResponseWriter, r *http.Request) {
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
	var req struct{ State, Body string }
	json.NewDecoder(r.Body).Decode(&req)
	review, err := s.pulls.AddReview(r.Context(), pr.ID, userIDFrom(r.Context()), req.State, req.Body)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if pr.AuthorID != userIDFrom(r.Context()) {
		s.notify.NotifyAsync(pr.AuthorID, "PR review",
			"New review on PR #"+strconv.Itoa(num)+" in "+repository.FullName,
			"/"+repository.OwnerName+"/"+repository.Name+"/pulls/"+strconv.Itoa(num))
	}
	jsonOK(w, review)
}

func (s *Server) handleListPRReviews(w http.ResponseWriter, r *http.Request) {
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
	reviews, err := s.pulls.ListReviews(r.Context(), pr.ID)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if reviews == nil {
		reviews = []pull.Review{}
	}
	jsonOK(w, reviews)
}

func (s *Server) handleMergePR(w http.ResponseWriter, r *http.Request) {
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
	approved, _ := s.pulls.ReviewCount(r.Context(), pr.ID, "approved")
	if err := s.repos.ValidateMergeProtection(r.Context(), repository.ID, pr.BaseBranch, pr.HeadSHA, approved); err != nil {
		jsonError(w, http.StatusForbidden, err.Error())
		return
	}
	var req struct{ Squash bool `json:"squash"` }
	json.NewDecoder(r.Body).Decode(&req)
	sha, err := s.git.Merge(repository.OwnerName, repository.Name, pr.BaseBranch, pr.HeadBranch, req.Squash)
	if err != nil {
		jsonError(w, http.StatusConflict, err.Error())
		return
	}
	if err := s.pulls.Merge(r.Context(), pr.ID, sha); err != nil {
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := s.repos.UpdateBranchHead(r.Context(), repository.ID, pr.BaseBranch, sha); err != nil {
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.notify.NotifyAsync(pr.AuthorID, "PR merged",
		"PR #"+strconv.Itoa(num)+" was merged in "+repository.FullName,
		"/"+repository.OwnerName+"/"+repository.Name+"/pulls/"+strconv.Itoa(num))
	jsonOK(w, map[string]string{"merge_sha": sha})
}

func (s *Server) handleUpdatePRBranch(w http.ResponseWriter, r *http.Request) {
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
		jsonError(w, http.StatusNotFound, "pull request not found")
		return
	}
	if pr.State != "open" {
		jsonError(w, http.StatusConflict, "pull request is not open")
		return
	}

	// Merge base branch into head branch to update it
	sha, err := s.git.MergeBaseIntoHead(repository.OwnerName, repository.Name, pr.HeadBranch, pr.BaseBranch)
	if err != nil {
		jsonError(w, http.StatusConflict, "failed to update branch: "+err.Error())
		return
	}

	// Update the PR's head SHA
	_, err = s.pool.Exec(r.Context(),
		`UPDATE pull_requests SET head_sha=$1, updated_at=NOW() WHERE id=$2`,
		sha, pr.ID)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}

	jsonOK(w, map[string]string{"head_sha": sha})
}

func (s *Server) handlePRDiff(w http.ResponseWriter, r *http.Request) {
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
	w.Header().Set("Content-Type", "text/plain")
	w.Write([]byte(diff))
}

func (s *Server) handleCompareCommits(w http.ResponseWriter, r *http.Request) {
	repository, ok := s.getRepo(w, r)
	if !ok {
		return
	}
	// Extract base...head from the wildcard path
	path := chi.URLParam(r, "*")
	parts := strings.SplitN(path, "...", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		jsonError(w, http.StatusBadRequest, "expected {base}...{head} format")
		return
	}
	baseRef := parts[0]
	headRef := parts[1]
	result, err := s.git.CompareCommits(repository.OwnerName, repository.Name, baseRef, headRef)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	jsonOK(w, result)
}

func (s *Server) handleArchive(w http.ResponseWriter, r *http.Request) {
	repository, ok := s.getRepo(w, r)
	if !ok {
		return
	}
	ref := chi.URLParam(r, "ref")
	if ref == "" {
		ref = repository.DefaultBranch
	}
	archive, err := s.git.Archive(repository.OwnerName, repository.Name, ref)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer archive.Close()
	w.Header().Set("Content-Type", "application/gzip")
	w.Header().Set("Content-Disposition", "attachment; filename="+repository.Name+"-"+ref+".tar.gz")
	io.Copy(w, archive)
}

func (s *Server) handlePRCommits(w http.ResponseWriter, r *http.Request) {
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
	commits, err := s.git.GetPRCommits(repository.OwnerName, repository.Name, pr.BaseBranch, pr.HeadBranch, 100)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if commits == nil {
		commits = []gitstore.CommitInfo{}
	}
	jsonOK(w, commits)
}

func (s *Server) handlePRFiles(w http.ResponseWriter, r *http.Request) {
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
	files, err := s.git.GetPRFiles(repository.OwnerName, repository.Name, pr.BaseBranch, pr.HeadBranch)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	jsonOK(w, files)
}

func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	hits, err := s.search.Search(r.Context(), q, 30)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	jsonOK(w, hits)
}

func jsonOK(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

func jsonError(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

func parseNumber(w http.ResponseWriter, r *http.Request, param string) (int, bool) {
	n, err := strconv.Atoi(chi.URLParam(r, param))
	if err != nil || n <= 0 {
		jsonError(w, http.StatusBadRequest, "invalid number")
		return 0, false
	}
	return n, true
}

// Workflow handlers are in actions_handlers.go
var _ = actions.Workflow{}
