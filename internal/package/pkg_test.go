package pkg

import (
	"bytes"
	"context"
	"testing"

	"github.com/rusik69/govnohub/internal/auth"
	"github.com/rusik69/govnohub/internal/repo"
	"github.com/rusik69/govnohub/internal/testutil"
)

func TestPublishPackage(t *testing.T) {
	pg := testutil.NewPostgres(t)
	defer pg.Cleanup()
	ctx := context.Background()
	authSvc := auth.NewService(pg.Pool, "s")
	repoSvc := repo.NewService(pg.Pool)
	pkgSvc := NewService(pg.Pool, t.TempDir())

	u, _ := authSvc.Register(ctx, "pkg", "pkg@test.local", "pass")
	r, _ := repoSvc.Create(ctx, "user", u.ID, u.Username, "lib", "", false)

	p, err := pkgSvc.Publish(ctx, r.ID, "mylib", "generic", "1.0.0", bytes.NewReader([]byte("artifact")))
	if err != nil || p.Version != "1.0.0" {
		t.Fatalf("publish: %v %+v", err, p)
	}
	list, _ := pkgSvc.List(ctx, r.ID)
	if len(list) != 1 {
		t.Fatalf("list=%d", len(list))
	}
}
