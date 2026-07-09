package testenv

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rusik69/govnohub/internal/api"
	"github.com/rusik69/govnohub/internal/auth"
	gitstore "github.com/rusik69/govnohub/internal/git"
	"github.com/rusik69/govnohub/internal/issue"
	pkg "github.com/rusik69/govnohub/internal/package"
	"github.com/rusik69/govnohub/internal/org"
	"github.com/rusik69/govnohub/internal/pull"
	"github.com/rusik69/govnohub/internal/release"
	"github.com/rusik69/govnohub/internal/repo"
	"github.com/rusik69/govnohub/internal/search"
	"github.com/rusik69/govnohub/internal/testutil"
	"github.com/rusik69/govnohub/internal/webhook"
)

type Env struct {
	Pool    *pgxpool.Pool
	Git     *gitstore.Store
	Server  *httptest.Server
	URL     string
	Cleanup func()
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

	srv := api.NewServer(
		auth.NewService(pg.Pool, "test-secret"),
		repo.NewService(pg.Pool),
		gitStore,
		issue.NewService(pg.Pool),
		pull.NewService(pg.Pool),
		release.NewService(pg.Pool, artifactRoot),
		pkg.NewService(pg.Pool, filepath.Join(artifactRoot, "packages")),
		webhook.NewService(pg.Pool),
		search.NewService("http://127.0.0.1:1"),
		pg.Pool,
		org.NewService(pg.Pool),
	)

	ts := httptest.NewServer(srv.Router())
	return &Env{
		Pool: pg.Pool,
		Git:  gitStore,
		Server: ts,
		URL:  ts.URL,
		Cleanup: func() {
			ts.Close()
			pg.Cleanup()
		},
	}
}
