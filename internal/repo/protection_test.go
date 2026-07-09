package repo

import (
	"context"
	"testing"

	"github.com/rusik69/govnohub/internal/auth"
	"github.com/rusik69/govnohub/internal/testutil"
)

func TestProtectionAndCollaborators(t *testing.T) {
	pg := testutil.NewPostgres(t)
	defer pg.Cleanup()
	ctx := context.Background()
	authSvc := auth.NewService(pg.Pool, "secret")
	repoSvc := NewService(pg.Pool)

	owner, _ := authSvc.Register(ctx, "powner", "powner@test.local", "pass")
	collab, _ := authSvc.Register(ctx, "pcollab", "pcollab@test.local", "pass")
	r, _ := repoSvc.Create(ctx, "user", owner.ID, owner.Username, "prot", "", false)

	if err := repoSvc.AddCollaborator(ctx, r.ID, collab.ID, "write"); err != nil {
		t.Fatal(err)
	}
	collabs, err := repoSvc.ListCollaborators(ctx, r.ID)
	if err != nil || len(collabs) != 1 || collabs[0].Username != "pcollab" {
		t.Fatalf("collabs: %v %v", collabs, err)
	}
	ok, _ := repoSvc.CanAccess(ctx, r.ID, collab.ID, "write")
	if !ok {
		t.Fatal("collaborator should have write access")
	}

	_, err = pg.Pool.Exec(ctx, `
		INSERT INTO protected_branches (repo_id, branch_name, required_checks, require_reviews)
		VALUES ($1, 'main', '{test}', 2)`, r.ID)
	if err != nil {
		t.Fatal(err)
	}
	pb, err := repoSvc.GetProtectedBranch(ctx, r.ID, "main")
	if err != nil || pb.RequireReviews != 2 {
		t.Fatalf("protection: %v %+v", err, pb)
	}
	err = repoSvc.ValidateMergeProtection(ctx, r.ID, "main", "abc", 1)
	if err == nil {
		t.Fatal("should fail with insufficient reviews")
	}

	if err := repoSvc.RemoveCollaborator(ctx, r.ID, collab.ID); err != nil {
		t.Fatal(err)
	}
	collabs, _ = repoSvc.ListCollaborators(ctx, r.ID)
	if len(collabs) != 0 {
		t.Fatalf("expected no collabs, got %d", len(collabs))
	}
}
