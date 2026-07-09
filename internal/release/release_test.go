package release

import (
	"bytes"
	"context"
	"testing"

	"github.com/rusik69/govnohub/internal/auth"
	"github.com/rusik69/govnohub/internal/repo"
	"github.com/rusik69/govnohub/internal/testutil"
)

func TestReleaseAndAsset(t *testing.T) {
	pg := testutil.NewPostgres(t)
	defer pg.Cleanup()
	ctx := context.Background()
	authSvc := auth.NewService(pg.Pool, "s")
	repoSvc := repo.NewService(pg.Pool)
	releaseSvc := NewService(pg.Pool, t.TempDir())

	u, _ := authSvc.Register(ctx, "rel", "rel@test.local", "pass")
	r, _ := repoSvc.Create(ctx, "user", u.ID, u.Username, "app", "", false)

	rel, err := releaseSvc.Create(ctx, r.ID, u.ID, "v1.0.0", "First", "notes", false, false)
	if err != nil {
		t.Fatal(err)
	}
	asset, err := releaseSvc.UploadAsset(ctx, rel.ID, "bin", "application/octet-stream", bytes.NewReader([]byte("data")), 4)
	if err != nil || asset.Name != "bin" {
		t.Fatalf("asset: %v %+v", err, asset)
	}
	list, _ := releaseSvc.List(ctx, r.ID)
	if len(list) != 1 {
		t.Fatalf("releases=%d", len(list))
	}
}
