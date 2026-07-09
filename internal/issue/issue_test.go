package issue

import (
	"context"
	"testing"

	"github.com/rusik69/govnohub/internal/auth"
	"github.com/rusik69/govnohub/internal/repo"
	"github.com/rusik69/govnohub/internal/testutil"
)

func TestIssueLifecycle(t *testing.T) {
	pg := testutil.NewPostgres(t)
	defer pg.Cleanup()
	ctx := context.Background()
	authSvc := auth.NewService(pg.Pool, "s")
	repoSvc := repo.NewService(pg.Pool)
	issueSvc := NewService(pg.Pool)

	u, _ := authSvc.Register(ctx, "dev", "dev@test.local", "pass")
	r, _ := repoSvc.Create(ctx, "user", u.ID, u.Username, "proj", "", false)

	i, err := issueSvc.Create(ctx, r.ID, u.ID, "bug", "details")
	if err != nil || i.Number != 1 {
		t.Fatalf("create: %v %+v", err, i)
	}
	if _, err := issueSvc.AddComment(ctx, i.ID, u.ID, "fix soon"); err != nil {
		t.Fatal(err)
	}
	if err := issueSvc.Close(ctx, i.ID); err != nil {
		t.Fatal(err)
	}
	list, _ := issueSvc.List(ctx, r.ID)
	if len(list) != 1 || list[0].State != "closed" {
		t.Fatalf("list=%+v", list)
	}
}
