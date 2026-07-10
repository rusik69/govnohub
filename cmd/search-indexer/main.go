package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rusik69/govnohub/internal/config"
	"github.com/rusik69/govnohub/internal/db"
	"github.com/rusik69/govnohub/internal/search"
)

func main() {
	cfg := config.Load()
	ctx := context.Background()
	pool, err := db.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("db: %v", err)
	}
	defer pool.Close()

	searchSvc := search.NewService(cfg.OpenSearchURL)
	if err := searchSvc.EnsureIndex(ctx); err != nil {
		log.Printf("opensearch index: %v", err)
	}

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	log.Println("search-indexer started")
	indexAll(ctx, pool, searchSvc)

	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			indexAll(ctx, pool, searchSvc)
		}
	}
}

func indexAll(ctx context.Context, pool *pgxpool.Pool, searchSvc *search.Service) {
	indexRepos(ctx, pool, searchSvc)
	indexIssues(ctx, pool, searchSvc)
	indexPRs(ctx, pool, searchSvc)
}

func indexRepos(ctx context.Context, pool *pgxpool.Pool, s *search.Service) {
	rows, err := pool.Query(ctx, `
		SELECT r.id, r.name, COALESCE(r.description,''), COALESCE(u.username, o.name)
		FROM repos r
		LEFT JOIN users u ON r.owner_type='user' AND r.owner_id=u.id
		LEFT JOIN orgs o ON r.owner_type='org' AND r.owner_id=o.id`)
	if err != nil {
		return
	}
	defer rows.Close()
	for rows.Next() {
		var id, name, desc, owner string
		rows.Scan(&id, &name, &desc, &owner)
		s.Index(ctx, search.Document{
			ID: "repo-" + id, Type: "repo", Title: owner + "/" + name,
			Body: desc, Repo: owner + "/" + name, FullName: owner + "/" + name,
		})
	}
}

func indexIssues(ctx context.Context, pool *pgxpool.Pool, s *search.Service) {
	rows, err := pool.Query(ctx, `
		SELECT i.id, i.number, i.title, COALESCE(i.body,''), COALESCE(u.username, o.name), r.name
		FROM issues i
		JOIN repos r ON r.id = i.repo_id
		LEFT JOIN users u ON r.owner_type='user' AND r.owner_id=u.id
		LEFT JOIN orgs o ON r.owner_type='org' AND r.owner_id=o.id`)
	if err != nil {
		return
	}
	defer rows.Close()
	for rows.Next() {
		var id, title, body, owner, repo string
		var number int
		rows.Scan(&id, &number, &title, &body, &owner, &repo)
		s.Index(ctx, search.Document{
			ID: "issue-" + id, Type: "issue", Title: title, Body: body,
			Repo: owner + "/" + repo, Ref: strconv.Itoa(number),
		})
	}
}

func indexPRs(ctx context.Context, pool *pgxpool.Pool, s *search.Service) {
	rows, err := pool.Query(ctx, `
		SELECT p.id, p.number, p.title, COALESCE(p.body,''), COALESCE(u.username, o.name), r.name
		FROM pull_requests p
		JOIN repos r ON r.id = p.repo_id
		LEFT JOIN users u ON r.owner_type='user' AND r.owner_id=u.id
		LEFT JOIN orgs o ON r.owner_type='org' AND r.owner_id=o.id`)
	if err != nil {
		return
	}
	defer rows.Close()
	for rows.Next() {
		var id, title, body, owner, repo string
		var number int
		rows.Scan(&id, &number, &title, &body, &owner, &repo)
		s.Index(ctx, search.Document{
			ID: "pr-" + id, Type: "pull_request", Title: title, Body: body,
			Repo: owner + "/" + repo, Ref: strconv.Itoa(number),
		})
	}
}
