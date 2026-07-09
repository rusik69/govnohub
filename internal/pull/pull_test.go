package pull

import (
	"context"
	"testing"

	"github.com/rusik69/govnohub/internal/auth"
	"github.com/rusik69/govnohub/internal/repo"
	"github.com/rusik69/govnohub/internal/testutil"
)

func TestPullRequest(t *testing.T) {
	pg := testutil.NewPostgres(t)
	defer pg.Cleanup()
	ctx := context.Background()
	authSvc := auth.NewService(pg.Pool, "s")
	repoSvc := repo.NewService(pg.Pool)
	pullSvc := NewService(pg.Pool)

	u, _ := authSvc.Register(ctx, "dev", "dev@test.local", "pass")
	r, _ := repoSvc.Create(ctx, "user", u.ID, u.Username, "app", "", false)

	pr, err := pullSvc.Create(ctx, r.ID, u.ID, "feature", "", "feature", "main", "abc123")
	if err != nil || pr.Number != 1 {
		t.Fatalf("create: %v %+v", err, pr)
	}
	if _, err := pullSvc.AddReview(ctx, pr.ID, u.ID, "approved", "lgtm"); err != nil {
		t.Fatal(err)
	}
	if err := pullSvc.Merge(ctx, pr.ID, "mergedsha"); err != nil {
		t.Fatal(err)
	}
}
