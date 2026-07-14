package testenv

import (
	"context"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rusik69/govnohub/internal/api"
	"github.com/rusik69/govnohub/internal/aireview"
	"github.com/rusik69/govnohub/internal/audit"
	"github.com/rusik69/govnohub/internal/auth"
	"github.com/rusik69/govnohub/internal/events"
	gitstore "github.com/rusik69/govnohub/internal/git"
	"github.com/rusik69/govnohub/internal/issue"
	"github.com/rusik69/govnohub/internal/jobqueue"
	pkg "github.com/rusik69/govnohub/internal/package"
	"github.com/rusik69/govnohub/internal/notification"
	"github.com/rusik69/govnohub/internal/org"
	"github.com/rusik69/govnohub/internal/presence"
	"github.com/rusik69/govnohub/internal/pull"
	"github.com/rusik69/govnohub/internal/release"
	"github.com/rusik69/govnohub/internal/repo"
	"github.com/rusik69/govnohub/internal/search"
	"github.com/rusik69/govnohub/internal/testutil"
	"github.com/rusik69/govnohub/internal/webhook"
	"github.com/rusik69/govnohub/internal/wiki"
)

type Env struct {
	Pool         *pgxpool.Pool
	DatabaseURL  string
	GitRoot      string
	ArtifactRoot string
	Git          *gitstore.Store
	Server       *httptest.Server
	URL          string
	Cleanup      func()
}

func New(t *testing.T) *Env {
	t.Helper()
	pg := testutil.NewPostgres(t)

	tmp := t.TempDir()
	gitRoot := filepath.Join(tmp, "git")
	artifactRoot := filepath.Join(tmp, "artifacts")
	gitStore, err := gitstore.NewStore(gitRoot)
	if err != nil {
		t.Fatalf("git store: %v", err)
	}
	os.MkdirAll(artifactRoot, 0o755)

	authSvc := auth.NewService(pg.Pool, "test-secret", auth.Options{AllowPublicRegistration: false})
	if err := authSvc.BootstrapAdmin(context.Background(), "admin", "admin@test.local", "admin"); err != nil {
		t.Fatalf("bootstrap admin: %v", err)
	}

	srv := api.NewServer(
		authSvc,
		repo.NewService(pg.Pool),
		gitStore,
		issue.NewService(pg.Pool),
		pull.NewService(pg.Pool),
		release.NewService(pg.Pool, artifactRoot),
		pkg.NewService(pg.Pool, filepath.Join(artifactRoot, "packages")),
		webhook.NewService(pg.Pool),
		search.NewService("http://127.0.0.1:1"),
		events.NewService(pg.Pool),
		pg.Pool,
		org.NewService(pg.Pool),
		aireview.NewService(pg.Pool, aireview.NewClient(aireview.Config{Enabled: false})),
		notification.NewService(pg.Pool),
		wiki.NewService(pg.Pool),
		audit.NewService(pg.Pool),
		presence.NewTracker(),
		artifactRoot+"/uploads",
		[]string{"*"},
		jobqueue.New(10, 2),
	)

	ts := httptest.NewServer(srv.Router())
	return &Env{
		Pool:         pg.Pool,
		DatabaseURL:  pg.DatabaseURL,
		GitRoot:      gitRoot,
		ArtifactRoot: artifactRoot,
		Git:          gitStore,
		Server:       ts,
		URL:          ts.URL,
		Cleanup: func() {
			ts.Close()
			pg.Cleanup()
		},
	}
}
