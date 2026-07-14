package repo

import (
	"context"
	"testing"

	"github.com/google/uuid"
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

func TestCheckRequiredChecks(t *testing.T) {
	pg := testutil.NewPostgres(t)
	defer pg.Cleanup()
	ctx := context.Background()
	authSvc := auth.NewService(pg.Pool, "secret")
	repoSvc := NewService(pg.Pool)

	u, _ := authSvc.Register(ctx, "checkuser", "check@test.local", "pass")
	r, _ := repoSvc.Create(ctx, "user", u.ID, u.Username, "check-repo", "", false)

	headSHA := "deadbeef1234"
	otherSHA := "cafebabe5678"

	// Create a workflow record first (workflow_runs.workflow_id has NOT NULL FK).
	var workflowID uuid.UUID
	err := pg.Pool.QueryRow(ctx, `
		INSERT INTO workflows (repo_id, name, path, content)
		VALUES ($1, 'CI', '.github/workflows/ci.yml', 'name: CI') RETURNING id`,
		r.ID).Scan(&workflowID)
	if err != nil {
		t.Fatal(err)
	}

	// Create a workflow run with jobs for this SHA.
	var runID uuid.UUID
	err = pg.Pool.QueryRow(ctx, `
		INSERT INTO workflow_runs (repo_id, workflow_id, run_number, event, head_sha, head_branch, status)
		VALUES ($1, $2, 1, 'push', $3, 'main', 'completed') RETURNING id`,
		r.ID, workflowID, headSHA).Scan(&runID)
	if err != nil {
		t.Fatal(err)
	}

	// Insert workflow jobs with various states.
	jobs := []struct {
		jobID      string
		status     string
		conclusion string
	}{
		{"build", "completed", "success"},
		{"test", "completed", "failure"},
		{"lint", "queued", ""},
		// "deploy" has no job at all
	}

	for _, j := range jobs {
		_, err := pg.Pool.Exec(ctx, `
			INSERT INTO workflow_jobs (run_id, job_id, name, status, conclusion)
			VALUES ($1, $2, $2, $3, NULLIF($4,''))`,
			runID, j.jobID, j.status, j.conclusion)
		if err != nil {
			t.Fatal(err)
		}
	}

	t.Run("all checks pass", func(t *testing.T) {
		res, err := repoSvc.CheckRequiredChecks(ctx, r.ID, headSHA, []string{"build"})
		if err != nil {
			t.Fatal(err)
		}
		if res.HasBlockers() {
			t.Fatalf("expected no blockers, got failed=%v pending=%v missing=%v", res.Failed, res.Pending, res.Missing)
		}
	})

	t.Run("detects failed check", func(t *testing.T) {
		res, err := repoSvc.CheckRequiredChecks(ctx, r.ID, headSHA, []string{"test"})
		if err != nil {
			t.Fatal(err)
		}
		if len(res.Failed) != 1 || res.Failed[0] != "test" {
			t.Fatalf("expected test failed, got failed=%v pending=%v missing=%v", res.Failed, res.Pending, res.Missing)
		}
	})

	t.Run("detects pending check", func(t *testing.T) {
		res, err := repoSvc.CheckRequiredChecks(ctx, r.ID, headSHA, []string{"lint"})
		if err != nil {
			t.Fatal(err)
		}
		if len(res.Pending) != 1 || res.Pending[0] != "lint" {
			t.Fatalf("expected lint pending, got failed=%v pending=%v missing=%v", res.Failed, res.Pending, res.Missing)
		}
	})

	t.Run("detects missing check", func(t *testing.T) {
		res, err := repoSvc.CheckRequiredChecks(ctx, r.ID, headSHA, []string{"deploy"})
		if err != nil {
			t.Fatal(err)
		}
		if len(res.Missing) != 1 || res.Missing[0] != "deploy" {
			t.Fatalf("expected deploy missing, got failed=%v pending=%v missing=%v", res.Failed, res.Pending, res.Missing)
		}
	})

	t.Run("mixed results", func(t *testing.T) {
		res, err := repoSvc.CheckRequiredChecks(ctx, r.ID, headSHA, []string{"build", "test", "lint", "deploy"})
		if err != nil {
			t.Fatal(err)
		}
		if len(res.Failed) != 1 || res.Failed[0] != "test" {
			t.Errorf("expected test failed, got %v", res.Failed)
		}
		if len(res.Pending) != 1 || res.Pending[0] != "lint" {
			t.Errorf("expected lint pending, got %v", res.Pending)
		}
		if len(res.Missing) != 1 || res.Missing[0] != "deploy" {
			t.Errorf("expected deploy missing, got %v", res.Missing)
		}
	})

	t.Run("empty checks returns no blockers", func(t *testing.T) {
		res, err := repoSvc.CheckRequiredChecks(ctx, r.ID, headSHA, nil)
		if err != nil {
			t.Fatal(err)
		}
		if res.HasBlockers() {
			t.Fatal("expected no blockers for empty checks")
		}
	})

	t.Run("no workflow runs for SHA returns all checks as missing", func(t *testing.T) {
		res, err := repoSvc.CheckRequiredChecks(ctx, r.ID, otherSHA, []string{"build", "test"})
		if err != nil {
			t.Fatal(err)
		}
		if len(res.Missing) != 2 {
			t.Fatalf("expected 2 missing, got failed=%v pending=%v missing=%v", res.Failed, res.Pending, res.Missing)
		}
	})
}

func TestValidateMergeProtection(t *testing.T) {
	pg := testutil.NewPostgres(t)
	defer pg.Cleanup()
	ctx := context.Background()
	authSvc := auth.NewService(pg.Pool, "secret")
	repoSvc := NewService(pg.Pool)

	u, _ := authSvc.Register(ctx, "vpuser", "vp@test.local", "pass")
	r, _ := repoSvc.Create(ctx, "user", u.ID, u.Username, "vp-repo", "", false)

	headSHA := "validatesha"

	// Create a workflow record first.
	var workflowID uuid.UUID
	err := pg.Pool.QueryRow(ctx, `
		INSERT INTO workflows (repo_id, name, path, content)
		VALUES ($1, 'CI', '.github/workflows/ci.yml', 'name: CI') RETURNING id`,
		r.ID).Scan(&workflowID)
	if err != nil {
		t.Fatal(err)
	}

	// Insert protected branch config: require 1 review + "build" check.
	_, err = pg.Pool.Exec(ctx, `
		INSERT INTO protected_branches (repo_id, branch_name, required_checks, require_reviews)
		VALUES ($1, 'main', '{build}', 1)`, r.ID)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("blocks merge when review count insufficient", func(t *testing.T) {
		err := repoSvc.ValidateMergeProtection(ctx, r.ID, "main", headSHA, 0)
		if err == nil {
			t.Fatal("expected error for insufficient reviews")
		}
		if !contains(err.Error(), "need 1 approvals, have 0") {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	// Insert a successful workflow run for the SHA.
	var runID uuid.UUID
	err = pg.Pool.QueryRow(ctx, `
		INSERT INTO workflow_runs (repo_id, workflow_id, run_number, event, head_sha, head_branch, status)
		VALUES ($1, $2, 100, 'push', $3, 'main', 'completed') RETURNING id`,
		r.ID, workflowID, headSHA).Scan(&runID)
	if err != nil {
		t.Fatal(err)
	}
	_, err = pg.Pool.Exec(ctx, `
		INSERT INTO workflow_jobs (run_id, job_id, name, status, conclusion)
		VALUES ($1, 'build', 'build', 'completed', 'success')`, runID)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("allows merge when reviews OK and checks pass", func(t *testing.T) {
		err := repoSvc.ValidateMergeProtection(ctx, r.ID, "main", headSHA, 1)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("blocks merge when check fails", func(t *testing.T) {
		// Add a second required check that will not pass.
		_, err := pg.Pool.Exec(ctx, `
			UPDATE protected_branches SET required_checks='{build,test}' WHERE repo_id=$1 AND branch_name='main'`, r.ID)
		if err != nil {
			t.Fatal(err)
		}
		err = repoSvc.ValidateMergeProtection(ctx, r.ID, "main", headSHA, 1)
		if err == nil {
			t.Fatal("expected error for missing test check")
		}
		if !contains(err.Error(), "missing checks") {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("no protection on unprotected branch", func(t *testing.T) {
		err := repoSvc.ValidateMergeProtection(ctx, r.ID, "develop", headSHA, 0)
		if err != nil {
			t.Fatalf("expected no error on unprotected branch, got %v", err)
		}
	})

	t.Run("protection not configured errors with clear message", func(t *testing.T) {
		r2, _ := repoSvc.Create(ctx, "user", u.ID, u.Username, "no-protection", "", false)
		err := repoSvc.ValidateMergeProtection(ctx, r2.ID, "main", headSHA, 0)
		if err != nil {
			t.Fatalf("expected no error on repo without protection, got %v", err)
		}
	})
}

// contains reports whether substr is within s.
func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
