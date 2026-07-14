package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rusik69/govnohub/internal/actions"
	"github.com/rusik69/govnohub/internal/config"
	"github.com/rusik69/govnohub/internal/db"
	"github.com/rusik69/govnohub/internal/webhook"
)

func main() {
	cfg := config.Load()
	ctx := context.Background()
	pool, err := db.Connect(ctx, cfg.DatabaseURL, cfg.DatabaseMaxConns)
	if err != nil {
		log.Fatalf("db: %v", err)
	}
	defer pool.Close()
	wh := webhook.NewService(pool)

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("ok")) })
	mux.HandleFunc("/events/push", func(w http.ResponseWriter, r *http.Request) {
		handleEvent(w, r, pool, wh, "push")
	})
	mux.HandleFunc("/events/pull_request", func(w http.ResponseWriter, r *http.Request) {
		handleEvent(w, r, pool, wh, "pull_request")
	})

	addr := getEnv("WEBHOOK_HTTP_ADDR", ":8082")
	server := &http.Server{Addr: addr, Handler: mux}
	go func() {
		log.Printf("webhook-service on %s", addr)
		server.ListenAndServe()
	}()
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	server.Shutdown(shutdownCtx)
}

func handleEvent(w http.ResponseWriter, r *http.Request, pool *pgxpool.Pool, wh *webhook.Service, eventType string) {
	var evt struct {
		RepoID uuid.UUID `json:"repo_id"`
		Owner  string    `json:"owner"`
		Name   string    `json:"name"`
		Branch string    `json:"branch"`
		SHA    string    `json:"sha"`
		Pusher string    `json:"pusher"`
	}
	if err := json.NewDecoder(r.Body).Decode(&evt); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	if eventType == "push" {
		wh.Dispatch(r.Context(), evt.RepoID, "push", webhook.NewPushEvent(evt.Owner, evt.Name, evt.Branch, evt.SHA, evt.Pusher))
	}
	triggerWorkflows(r.Context(), pool, evt.RepoID, evt.Owner, evt.Name, evt.Branch, evt.SHA, eventType)
	w.WriteHeader(202)
}

func triggerWorkflows(ctx context.Context, pool *pgxpool.Pool, repoID uuid.UUID, owner, name, branch, sha, event string) {
	rows, err := pool.Query(ctx, `SELECT id, content FROM workflows WHERE repo_id=$1 AND active=true`, repoID)
	if err != nil {
		return
	}
	defer rows.Close()
	for rows.Next() {
		var wfID uuid.UUID
		var content string
		if err := rows.Scan(&wfID, &content); err != nil {
			continue
		}
		wf, err := actions.ParseWorkflow(content)
		if err != nil {
			continue
		}
		if !wf.MatchesTrigger(actions.TriggerEvent{Type: event, Branch: branch, SHA: sha}) {
			continue
		}
		var runNumber int
		if err := pool.QueryRow(ctx, `SELECT COALESCE(MAX(run_number),0)+1 FROM workflow_runs WHERE repo_id=$1`, repoID).Scan(&runNumber); err != nil {
			log.Printf("workflow run number: %v", err)
			continue
		}
		var runID uuid.UUID
		if err := pool.QueryRow(ctx, `
			INSERT INTO workflow_runs (repo_id, workflow_id, run_number, event, head_sha, head_branch, status)
			VALUES ($1,$2,$3,$4,$5,$6,'queued') RETURNING id`,
			repoID, wfID, runNumber, event, sha, branch).Scan(&runID); err != nil {
			log.Printf("workflow run insert: %v", err)
		}
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
