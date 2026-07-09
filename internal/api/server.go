package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/rusik69/govnohub/internal/actions"
	"github.com/rusik69/govnohub/internal/auth"
	gitstore "github.com/rusik69/govnohub/internal/git"
	"github.com/rusik69/govnohub/internal/issue"
	pkg "github.com/rusik69/govnohub/internal/package"
	"github.com/rusik69/govnohub/internal/org"
	"github.com/rusik69/govnohub/internal/pull"
	"github.com/rusik69/govnohub/internal/release"
	"github.com/rusik69/govnohub/internal/repo"
	"github.com/rusik69/govnohub/internal/search"
	"github.com/rusik69/govnohub/internal/webhook"
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
) *Server {
	return &Server{
		auth: authSvc, repos: repoSvc, git: gitStore,
		issues: issueSvc, pulls: pullSvc, releases: releaseSvc,
		packages: pkgSvc, webhooks: webhookSvc, search: searchSvc,
		pool: pool, org: orgSvc,
	}
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
		r.Get("/user/repos", s.handleListUserRepos)
		r.Get("/search", s.handleSearch)
		s.registerOrgRoutes(r)

		r.Post("/orgs/{org}/repos", s.handleCreateOrgRepo)
		r.Post("/users/{user}/repos", s.handleCreateUserRepo)

		r.Route("/repos/{owner}/{repo}", func(r chi.Router) {
			r.Get("/", s.handleGetRepo)
			r.Get("/contents/*", s.handleGetContents)
			r.Get("/commits", s.handleGetCommits)
			r.Post("/star", s.handleStar)
			r.Post("/watch", s.handleWatch)
			r.Post("/fork", s.handleFork)

			r.Get("/issues", s.handleListIssues)
			r.Post("/issues", s.handleCreateIssue)
			r.Get("/issues/{number}", s.handleGetIssue)
			r.Post("/issues/{number}/comments", s.handleAddIssueComment)
			r.Post("/issues/{number}/close", s.handleCloseIssue)
			r.Post("/labels", s.handleCreateLabel)

			r.Get("/pulls", s.handleListPRs)
			r.Post("/pulls", s.handleCreatePR)
			r.Get("/pulls/{number}", s.handleGetPR)
			r.Post("/pulls/{number}/reviews", s.handleAddReview)
			r.Post("/pulls/{number}/merge", s.handleMergePR)
			r.Get("/pulls/{number}/diff", s.handlePRDiff)

			r.Get("/actions/workflows", s.handleListWorkflows)
			r.Post("/actions/workflows", s.handleUpsertWorkflow)
			r.Get("/actions/runs", s.handleListRuns)
			r.Post("/actions/runs", s.handleTriggerRun)
			r.Get("/actions/runs/{runID}/logs", s.handleRunLogs)

			r.Get("/releases", s.handleListReleases)
			r.Post("/releases", s.handleCreateRelease)
			r.Post("/releases/{tag}/assets", s.handleUploadAsset)

			r.Get("/packages", s.handleListPackages)
			r.Post("/packages", s.handlePublishPackage)
			r.Get("/packages/{name}/{version}", s.handleDownloadPackage)

			r.Get("/webhooks", s.handleListWebhooks)
			r.Post("/webhooks", s.handleCreateWebhook)

			r.Post("/branches", s.handleCreateBranch)
			r.Post("/protected-branches", s.handleProtectBranch)
		})
	})
	return r
}

type ctxKey string

const userIDKey ctxKey = "userID"

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
		if strings.HasPrefix(token, "ghp_") {
			userID, err = s.auth.ValidatePAT(r.Context(), token)
		} else {
			userID, _, err = s.auth.ValidateToken(token)
		}
		if err != nil {
			jsonError(w, http.StatusUnauthorized, "invalid token")
			return
		}
		ctx := context.WithValue(r.Context(), userIDKey, userID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func userIDFrom(ctx context.Context) uuid.UUID {
	id, _ := ctx.Value(userIDKey).(uuid.UUID)
	return id
}

func (s *Server) handleRegister(w http.ResponseWriter, r *http.Request) {
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

func (s *Server) handleListUserRepos(w http.ResponseWriter, r *http.Request) {
	repos, err := s.repos.ListForUser(r.Context(), userIDFrom(r.Context()))
	if err != nil {
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	jsonOK(w, repos)
}

func (s *Server) handleCreateUserRepo(w http.ResponseWriter, r *http.Request) {
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
	s.search.Index(r.Context(), search.Document{
		ID: "issue-" + i.ID.String(), Type: "issue", Title: i.Title, Body: i.Body, Repo: repository.FullName,
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
	jsonOK(w, c)
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
	jsonOK(w, pr)
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
	jsonOK(w, review)
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
	jsonOK(w, map[string]string{"merge_sha": sha})
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
