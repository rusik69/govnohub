package repo

import (
	"context"
	"testing"

	"github.com/rusik69/govnohub/internal/auth"
	"github.com/rusik69/govnohub/internal/testutil"
)

func TestCreateAndAccess(t *testing.T) {
	pg := testutil.NewPostgres(t)
	defer pg.Cleanup()
	ctx := context.Background()
	authSvc := auth.NewService(pg.Pool, "secret")
	repoSvc := NewService(pg.Pool)

	u, _ := authSvc.Register(ctx, "bob", "bob@test.local", "pass")
	r, err := repoSvc.Create(ctx, "user", u.ID, u.Username, "demo", "desc", false)
	if err != nil {
		t.Fatal(err)
	}
	got, err := repoSvc.GetByFullName(ctx, "bob", "demo")
	if err != nil || got.FullName != "bob/demo" {
		t.Fatalf("get: %v %+v", err, got)
	}
	ok, _ := repoSvc.CanAccess(ctx, r.ID, u.ID, "read")
	if !ok {
		t.Fatal("owner should access")
	}
}

func TestStarAndFork(t *testing.T) {
	pg := testutil.NewPostgres(t)
	defer pg.Cleanup()
	ctx := context.Background()
	authSvc := auth.NewService(pg.Pool, "secret")
	repoSvc := NewService(pg.Pool)

	owner, _ := authSvc.Register(ctx, "owner", "owner@test.local", "pass")
	other, _ := authSvc.Register(ctx, "other", "other@test.local", "pass")
	r, _ := repoSvc.Create(ctx, "user", owner.ID, owner.Username, "app", "", false)

	if err := repoSvc.Star(ctx, r.ID, other.ID); err != nil {
		t.Fatal(err)
	}
	starred, err := repoSvc.IsStarred(ctx, r.ID, other.ID)
	if err != nil || !starred {
		t.Fatalf("starred: %v %v", starred, err)
	}
	if err := repoSvc.Unstar(ctx, r.ID, other.ID); err != nil {
		t.Fatal(err)
	}
	starred, _ = repoSvc.IsStarred(ctx, r.ID, other.ID)
	if starred {
		t.Fatal("expected unstarred")
	}
	if err := repoSvc.Watch(ctx, r.ID, other.ID); err != nil {
		t.Fatal(err)
	}
	watched, err := repoSvc.IsWatched(ctx, r.ID, other.ID)
	if err != nil || !watched {
		t.Fatalf("watched: %v %v", watched, err)
	}
	if err := repoSvc.Unwatch(ctx, r.ID, other.ID); err != nil {
		t.Fatal(err)
	}
	watched, _ = repoSvc.IsWatched(ctx, r.ID, other.ID)
	if watched {
		t.Fatal("expected unwatched")
	}
	fork, err := repoSvc.Fork(ctx, r, other.ID, other.Username)
	if err != nil || fork.Name != "app-fork" {
		t.Fatalf("fork: %v %+v", err, fork)
	}
}
