package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rusik69/govnohub/internal/auth"
	"github.com/rusik69/govnohub/internal/config"
	"github.com/rusik69/govnohub/internal/db"
	gitstore "github.com/rusik69/govnohub/internal/git"
	"github.com/rusik69/govnohub/internal/repo"
)

type server struct {
	git      *gitstore.Store
	auth     *auth.Service
	repos    *repo.Service
	pool     *pgxpool.Pool
	webhook  string
}

func main() {
	cfg := config.Load()
	ctx := context.Background()

	pool, err := db.Connect(ctx, cfg.DatabaseURL, cfg.DatabaseMaxConns)
	if err != nil {
		log.Fatalf("db connect: %v", err)
	}
	defer pool.Close()

	gitStore, err := gitstore.NewStore(cfg.GitRoot)
	if err != nil {
		log.Fatalf("git store: %v", err)
	}

	s := &server{
		git:     gitStore,
		auth:    auth.NewService(pool, cfg.JWTSecret),
		repos:   repo.NewService(pool),
		pool:    pool,
		webhook: cfg.WebhookURL,
	}

	// Export environment variables for git pre-receive hooks
	os.Setenv("GIT_ROOT", cfg.GitRoot)
	os.Setenv("DATABASE_URL", cfg.DatabaseURL)

	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleGit)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok"))
	})

	addr := getEnv("GIT_HTTP_ADDR", ":8081")
	httpServer := &http.Server{Addr: addr, Handler: mux}
	go func() {
		log.Printf("git-server http listening on %s", addr)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("http: %v", err)
		}
	}()

	s.startSSH(getEnv("GIT_SSH_ADDR", ":2222"), getEnv("GIT_SSH_HOST_KEY", ""))

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	httpServer.Shutdown(shutdownCtx)
}

func (s *server) handleGit(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/healthz" {
		w.Write([]byte("ok"))
		return
	}

	owner, name, service, ok := parseGitPath(r.URL.Path)
	if !ok {
		http.NotFound(w, r)
		return
	}

	userID, username, scopes, isPAT, authed := s.authenticate(r)
	if !authed {
		w.Header().Set("WWW-Authenticate", `Basic realm="govnohub"`)
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	repository, err := s.repos.GetByFullName(r.Context(), owner, name)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	okAccess, _ := s.repos.CanAccess(r.Context(), repository.ID, userID, "read")
	if !okAccess {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	if isPAT && !auth.HasScope(scopes, auth.ScopeRepo) {
		http.Error(w, "forbidden: insufficient token scope", http.StatusForbidden)
		return
	}
	if !s.git.Exists(owner, name) {
		http.NotFound(w, r)
		return
	}

	if service == "info/refs" {
		s.handleInfoRefs(w, r, owner, name)
		return
	}

	if service == "git-receive-pack" {
		canWrite, _ := s.repos.CanAccess(r.Context(), repository.ID, userID, "write")
		if !canWrite {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		if isPAT && !auth.HasScope(scopes, auth.ScopeRepoWrite) {
			http.Error(w, "forbidden: insufficient token scope", http.StatusForbidden)
			return
		}
		// Read entire body to inspect ref updates for branch protection
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "cannot read body", http.StatusInternalServerError)
			return
		}
		refs, parseErr := gitstore.ParseRefUpdates(body)
		if parseErr != nil {
			log.Printf("parse ref updates %s/%s: %v", owner, name, parseErr)
		}
		if len(refs) > 0 {
			if err := s.repos.CheckPushProtection(r.Context(), repository.ID, refs); err != nil {
				log.Printf("branch protection rejected push %s/%s: %v", owner, name, err)
				w.Header().Set("Content-Type", "text/plain")
				w.WriteHeader(http.StatusForbidden)
				w.Write([]byte(err.Error()))
				return
			}
		}
		before, err := s.git.ListBranchSHAs(owner, name)
		if err != nil {
			log.Printf("list branches before push %s/%s: %v", owner, name, err)
			return
		}
		w.Header().Set("Content-Type", gitstore.ResultContentType(service))
		if err := s.git.ReceivePack(owner, name, bytes.NewReader(body), w); err != nil {
			log.Printf("receive-pack %s/%s: %v", owner, name, err)
			return
		}
		s.afterPush(r.Context(), repository, owner, name, username, before)
		return
	}

	if service == "git-upload-pack" {
		w.Header().Set("Content-Type", gitstore.ResultContentType(service))
		if err := s.git.UploadPack(owner, name, r.Body, w); err != nil {
			log.Printf("upload-pack %s/%s: %v", owner, name, err)
		}
		return
	}

	http.NotFound(w, r)
}

func (s *server) handleInfoRefs(w http.ResponseWriter, r *http.Request, owner, name string) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	service := gitstore.ServiceFromQuery(r.URL.Query().Get("service"))
	if service == "" {
		service = "git-upload-pack"
	}
	w.Header().Set("Content-Type", gitstore.AdvertisementContentType(service))
	w.Header().Set("Cache-Control", "no-cache")
	var buf bytes.Buffer
	if err := gitstore.WriteServicePacket(&buf, service); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := gitstore.AdvertiseRefs(s.git.RepoPath(owner, name), service, &buf); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	io.Copy(w, &buf)
}

func (s *server) afterPush(ctx context.Context, repository *repo.Repository, owner, name, pusher string, before map[string]string) {
	after, err := s.git.ListBranchSHAs(owner, name)
	if err != nil {
		return
	}
	for branch, sha := range after {
		if before[branch] == sha {
			continue
		}
		if err := s.repos.UpdateBranchHead(ctx, repository.ID, branch, sha); err != nil {
			log.Printf("update branch %s/%s %s: %v", owner, name, branch, err)
		}
		s.notifyPush(ctx, repository, owner, name, branch, sha, pusher)
	}
}

func (s *server) notifyPush(ctx context.Context, repository *repo.Repository, owner, name, branch, sha, pusher string) {
	if s.webhook == "" {
		return
	}
	body, _ := json.Marshal(map[string]interface{}{
		"repo_id": repository.ID,
		"owner":   owner,
		"name":    name,
		"branch":  branch,
		"sha":     sha,
		"pusher":  pusher,
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(s.webhook, "/")+"/events/push", bytes.NewReader(body))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Printf("webhook push: %v", err)
		return
	}
	resp.Body.Close()
}

func parseGitPath(path string) (owner, name, service string, ok bool) {
	path = strings.Trim(path, "/")
	parts := strings.Split(path, "/")
	if len(parts) < 2 {
		return
	}
	owner = parts[0]
	repoPart := parts[1]
	name = strings.TrimSuffix(repoPart, ".git")
	if len(parts) == 2 {
		return owner, name, "info/refs", true
	}
	if len(parts) >= 4 && parts[2] == "info" && parts[3] == "refs" {
		return owner, name, "info/refs", true
	}
	if len(parts) >= 3 {
		return owner, name, gitstore.NormalizeService(parts[2]), true
	}
	return
}

func (s *server) authenticate(r *http.Request) (uuid.UUID, string, []string, bool, bool) {
	if u, p, ok := r.BasicAuth(); ok {
		if _, user, err := s.auth.Login(r.Context(), u, p); err == nil {
			return user.ID, user.Username, nil, false, true
		}
		token := p
		if token == "" {
			token = u
		}
		if strings.HasPrefix(token, "ghp_") {
			if id, scopes, err := s.auth.ValidatePATWithScopes(r.Context(), token); err == nil {
				user, _ := s.auth.GetUser(r.Context(), id)
				name := u
				if user != nil {
					name = user.Username
				}
				return id, name, scopes, true, true
			}
		}
	}
	if h := r.Header.Get("Authorization"); strings.HasPrefix(h, "Bearer ") {
		token := strings.TrimPrefix(h, "Bearer ")
		if strings.HasPrefix(token, "ghp_") {
			if id, scopes, err := s.auth.ValidatePATWithScopes(r.Context(), token); err == nil {
				user, _ := s.auth.GetUser(r.Context(), id)
				if user != nil {
					return id, user.Username, scopes, true, true
				}
				return id, "", scopes, true, true
			}
		}
		if id, username, err := s.auth.ValidateToken(r.Context(), token); err == nil {
			return id, username, nil, false, true
		}
	}
	return uuid.Nil, "", nil, false, false
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
