package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/rusik69/govnohub/internal/api"
	"github.com/rusik69/govnohub/internal/aireview"
	"github.com/rusik69/govnohub/internal/audit"
	"github.com/rusik69/govnohub/internal/auth"
	"github.com/rusik69/govnohub/internal/config"
	"github.com/rusik69/govnohub/internal/db"
	"github.com/rusik69/govnohub/internal/events"
	gitstore "github.com/rusik69/govnohub/internal/git"
	"github.com/rusik69/govnohub/internal/issue"
	pkg "github.com/rusik69/govnohub/internal/package"
	"github.com/rusik69/govnohub/internal/notification"
	"github.com/rusik69/govnohub/internal/org"
	"github.com/rusik69/govnohub/internal/pull"
	"github.com/rusik69/govnohub/internal/release"
	"github.com/rusik69/govnohub/internal/repo"
	"github.com/rusik69/govnohub/internal/search"
	"github.com/rusik69/govnohub/internal/webhook"
	"github.com/rusik69/govnohub/internal/wiki"
)

func main() {
	cfg := config.Load()
	ctx := context.Background()

	pool, err := db.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("db connect: %v", err)
	}
	defer pool.Close()

	if err := db.Migrate(ctx, pool); err != nil {
		log.Fatalf("migrate: %v", err)
	}

	authSvc := auth.NewService(pool, cfg.JWTSecret, auth.Options{
		AllowPublicRegistration: cfg.AllowPublicRegistration,
	})
	if err := authSvc.BootstrapAdmin(ctx, cfg.BootstrapAdminUsername, cfg.BootstrapAdminEmail, cfg.BootstrapAdminPassword); err != nil {
		log.Fatalf("bootstrap admin: %v", err)
	}

	gitStore, err := gitstore.NewStore(cfg.GitRoot)
	if err != nil {
		log.Fatalf("git store: %v", err)
	}

	searchSvc := search.NewService(cfg.OpenSearchURL)
	_ = searchSvc.EnsureIndex(ctx)

	srv := api.NewServer(
		authSvc,
		repo.NewService(pool),
		gitStore,
		issue.NewService(pool),
		pull.NewService(pool),
		release.NewService(pool, cfg.ArtifactRoot),
		pkg.NewService(pool, cfg.ArtifactRoot+"/packages"),
		webhook.NewService(pool),
		searchSvc,
		events.NewService(pool),
		pool,
		org.NewService(pool),
		aireview.NewService(pool, aireview.NewClient(aireview.Config{
			Enabled: cfg.AIReview.Enabled,
			APIKey:  cfg.AIReview.APIKey,
			BaseURL: cfg.AIReview.BaseURL,
			Model:   cfg.AIReview.Model,
			Auto:    cfg.AIReview.Auto,
		})),
		notification.NewService(pool),
		wiki.NewService(pool),
		audit.NewService(pool),
	)

	server := &http.Server{Addr: cfg.HTTPAddr, Handler: srv.Router()}
	go func() {
		log.Printf("api-server listening on %s", cfg.HTTPAddr)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	server.Shutdown(shutdownCtx)
}
