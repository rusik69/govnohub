package pkg

import (
	"bytes"
	"context"
	"io"
	"testing"

	"github.com/google/uuid"
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

func TestDownloadPackage(t *testing.T) {
	pg := testutil.NewPostgres(t)
	defer pg.Cleanup()
	ctx := context.Background()
	authSvc := auth.NewService(pg.Pool, "s")
	repoSvc := repo.NewService(pg.Pool)
	pkgSvc := NewService(pg.Pool, t.TempDir())

	u, _ := authSvc.Register(ctx, "dl", "dl@test.local", "pass")
	r, _ := repoSvc.Create(ctx, "user", u.ID, u.Username, "lib", "", false)

	content := []byte("downloadable-artifact")
	_, err := pkgSvc.Publish(ctx, r.ID, "mylib", "generic", "2.0.0", bytes.NewReader(content))
	if err != nil {
		t.Fatalf("publish: %v", err)
	}

	rc, err := pkgSvc.Open(ctx, r.ID, "mylib", "2.0.0")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer rc.Close()

	got, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !bytes.Equal(got, content) {
		t.Fatalf("got %q, want %q", got, content)
	}
}

func TestDownloadPackageNotFound(t *testing.T) {
	pg := testutil.NewPostgres(t)
	defer pg.Cleanup()
	ctx := context.Background()
	authSvc := auth.NewService(pg.Pool, "s")
	repoSvc := repo.NewService(pg.Pool)
	pkgSvc := NewService(pg.Pool, t.TempDir())

	u, _ := authSvc.Register(ctx, "dl2", "dl2@test.local", "pass")
	r, _ := repoSvc.Create(ctx, "user", u.ID, u.Username, "lib", "", false)

	_, err := pkgSvc.Open(ctx, r.ID, "nonexistent", "0.0.0")
	if err == nil {
		t.Fatal("expected error for missing package")
	}
}

func TestListEmptyPackages(t *testing.T) {
	pg := testutil.NewPostgres(t)
	defer pg.Cleanup()
	ctx := context.Background()
	authSvc := auth.NewService(pg.Pool, "s")
	repoSvc := repo.NewService(pg.Pool)
	pkgSvc := NewService(pg.Pool, t.TempDir())

	u, _ := authSvc.Register(ctx, "le", "le@test.local", "pass")
	r, _ := repoSvc.Create(ctx, "user", u.ID, u.Username, "lib", "", false)

	list, err := pkgSvc.List(ctx, r.ID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("expected empty list, got %d", len(list))
	}
}

func TestPublishDuplicatePackage(t *testing.T) {
	pg := testutil.NewPostgres(t)
	defer pg.Cleanup()
	ctx := context.Background()
	authSvc := auth.NewService(pg.Pool, "s")
	repoSvc := repo.NewService(pg.Pool)
	pkgSvc := NewService(pg.Pool, t.TempDir())

	u, _ := authSvc.Register(ctx, "dp", "dp@test.local", "pass")
	r, _ := repoSvc.Create(ctx, "user", u.ID, u.Username, "lib", "", false)

	p1, err := pkgSvc.Publish(ctx, r.ID, "mylib", "generic", "1.0.0", bytes.NewReader([]byte("v1")))
	if err != nil {
		t.Fatalf("first publish: %v", err)
	}

	p2, err := pkgSvc.Publish(ctx, r.ID, "mylib", "generic", "1.0.0", bytes.NewReader([]byte("v2")))
	if err != nil {
		t.Fatalf("second publish: %v", err)
	}

	if p1.ID != p2.ID {
		t.Fatalf("expected same package ID on upsert, got %s != %s", p1.ID, p2.ID)
	}

	rc, _ := pkgSvc.Open(ctx, r.ID, "mylib", "1.0.0")
	defer rc.Close()
	got, _ := io.ReadAll(rc)
	if string(got) != "v2" {
		t.Fatalf("expected updated content 'v2', got %q", got)
	}
}

func TestPackageNotFoundErrorForWrongRepo(t *testing.T) {
	pg := testutil.NewPostgres(t)
	defer pg.Cleanup()
	ctx := context.Background()
	authSvc := auth.NewService(pg.Pool, "s")
	repoSvc := repo.NewService(pg.Pool)
	pkgSvc := NewService(pg.Pool, t.TempDir())

	u, _ := authSvc.Register(ctx, "wr", "wr@test.local", "pass")
	r, _ := repoSvc.Create(ctx, "user", u.ID, u.Username, "lib", "", false)

	_, err := pkgSvc.Publish(ctx, r.ID, "mylib", "generic", "1.0.0", bytes.NewReader([]byte("data")))
	if err != nil {
		t.Fatalf("publish: %v", err)
	}

	wrongID := uuid.New()
	_, err = pkgSvc.Open(ctx, wrongID, "mylib", "1.0.0")
	if err == nil {
		t.Fatal("expected error when opening from wrong repo")
	}
}
