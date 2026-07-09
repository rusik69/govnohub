package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/rusik69/govnohub/internal/auth"
	"github.com/rusik69/govnohub/internal/config"
	"github.com/rusik69/govnohub/internal/db"
	gitstore "github.com/rusik69/govnohub/internal/git"
)

func main() {
	cfg := config.Load()
	ctx := context.Background()

	pool, err := db.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("db connect: %v", err)
	}
	defer pool.Close()

	gitStore, err := gitstore.NewStore(cfg.GitRoot)
	if err != nil {
		log.Fatalf("git store: %v", err)
	}
	authSvc := auth.NewService(pool, cfg.JWTSecret)

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" {
			w.Write([]byte("ok"))
			return
		}
		owner, name, service, ok := parseGitPath(r.URL.Path)
		if !ok {
			http.NotFound(w, r)
			return
		}
		if !authenticate(r, authSvc) {
			w.Header().Set("WWW-Authenticate", `Basic realm="govnohub"`)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		switch service {
		case "info/refs":
			w.Header().Set("Content-Type", "application/x-"+r.Header.Get("Git-Protocol")+"-advertisement")
			w.Write([]byte("001e# service=git-" + gitService(r) + "\n0000"))
		case "git-upload-pack":
			w.Header().Set("Content-Type", "application/x-git-upload-pack-result")
			gitStore.UploadPack(owner, name, r.Body, w)
		case "git-receive-pack":
			w.Header().Set("Content-Type", "application/x-git-receive-pack-result")
			gitStore.ReceivePack(owner, name, r.Body, w)
		default:
			http.NotFound(w, r)
		}
	})

	addr := getEnv("GIT_HTTP_ADDR", ":8081")
	server := &http.Server{Addr: addr, Handler: mux}
	go func() {
		log.Printf("git-server listening on %s", addr)
		server.ListenAndServe()
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	server.Shutdown(shutdownCtx)
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
		return owner, name, parts[2], true
	}
	return
}

func gitService(r *http.Request) string {
	if strings.Contains(r.URL.Path, "receive-pack") {
		return "receive-pack"
	}
	return "upload-pack"
}

func authenticate(r *http.Request, authSvc *auth.Service) bool {
	if u, p, ok := r.BasicAuth(); ok {
		_, _, err := authSvc.Login(r.Context(), u, p)
		return err == nil
	}
	if h := r.Header.Get("Authorization"); strings.HasPrefix(h, "Bearer ") {
		token := strings.TrimPrefix(h, "Bearer ")
		if strings.HasPrefix(token, "ghp_") {
			_, err := authSvc.ValidatePAT(r.Context(), token)
			return err == nil
		}
		_, _, err := authSvc.ValidateToken(token)
		return err == nil
	}
	return false
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
