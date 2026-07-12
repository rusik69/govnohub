package repo

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/rusik69/govnohub/internal/auth"
	"github.com/rusik69/govnohub/internal/testutil"
)

func TestRepoCreate(t *testing.T) {
	pg := testutil.NewPostgres(t)
	defer pg.Cleanup()
	ctx := context.Background()
	authSvc := auth.NewService(pg.Pool, "secret")
	repoSvc := NewService(pg.Pool)

	u, err := authSvc.Register(ctx, "testuser", "test@test.local", "pass")
	if err != nil {
		t.Fatal(err)
	}

	t.Run("create basic repo", func(t *testing.T) {
		r, err := repoSvc.Create(ctx, "user", u.ID, u.Username, "my-repo", "a test repo", false)
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		if r.FullName != "testuser/my-repo" {
			t.Errorf("FullName = %q, want %q", r.FullName, "testuser/my-repo")
		}
		if r.DefaultBranch != "main" {
			t.Errorf("DefaultBranch = %q, want %q", r.DefaultBranch, "main")
		}
	})

	t.Run("create with empty description", func(t *testing.T) {
		r, err := repoSvc.Create(ctx, "user", u.ID, u.Username, "empty-desc", "", false)
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		if r.Name != "empty-desc" {
			t.Errorf("Name = %q, want %q", r.Name, "empty-desc")
		}
	})

	t.Run("create private repo", func(t *testing.T) {
		r, err := repoSvc.Create(ctx, "user", u.ID, u.Username, "my-private-repo", "private", true)
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		if !r.IsPrivate {
			t.Error("expected private repo")
		}
	})

	t.Run("duplicate name returns error", func(t *testing.T) {
		_, err := repoSvc.Create(ctx, "user", u.ID, u.Username, "my-repo", "dup", false)
		if err == nil {
			t.Fatal("expected error for duplicate name")
		}
	})
}

func TestRepoGetByFullName(t *testing.T) {
	pg := testutil.NewPostgres(t)
	defer pg.Cleanup()
	ctx := context.Background()
	authSvc := auth.NewService(pg.Pool, "secret")
	repoSvc := NewService(pg.Pool)

	u, err := authSvc.Register(ctx, "getuser", "get@test.local", "pass")
	if err != nil {
		t.Fatal(err)
	}

	_, err = repoSvc.Create(ctx, "user", u.ID, u.Username, "gettest", "find me", false)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("existing repo by full name", func(t *testing.T) {
		got, err := repoSvc.GetByFullName(ctx, "getuser", "gettest")
		if err != nil {
			t.Fatalf("GetByFullName: %v", err)
		}
		if got.FullName != "getuser/gettest" {
			t.Errorf("FullName = %q, want %q", got.FullName, "getuser/gettest")
		}
		if got.Description != "find me" {
			t.Errorf("Description = %q, want %q", got.Description, "find me")
		}
	})

	t.Run("non-existent repo returns ErrNotFound", func(t *testing.T) {
		_, err := repoSvc.GetByFullName(ctx, "getuser", "nope")
		if err != ErrNotFound {
			t.Fatalf("expected ErrNotFound, got %v", err)
		}
	})

	t.Run("non-existent owner returns ErrNotFound", func(t *testing.T) {
		_, err := repoSvc.GetByFullName(ctx, "nobody", "gettest")
		if err != ErrNotFound {
			t.Fatalf("expected ErrNotFound, got %v", err)
		}
	})
}

func TestRepoListForUser(t *testing.T) {
	pg := testutil.NewPostgres(t)
	defer pg.Cleanup()
	ctx := context.Background()
	authSvc := auth.NewService(pg.Pool, "secret")
	repoSvc := NewService(pg.Pool)

	owner, _ := authSvc.Register(ctx, "listowner", "list@test.local", "pass")
	collab, _ := authSvc.Register(ctx, "collab", "collab@test.local", "pass")

	// Owner creates repos
	r1, err := repoSvc.Create(ctx, "user", owner.ID, owner.Username, "repo1", "", false)
	if err != nil {
		t.Fatal(err)
	}
	_, err = repoSvc.Create(ctx, "user", owner.ID, owner.Username, "repo2", "", false)
	if err != nil {
		t.Fatal(err)
	}

	// Add collab as collaborator on repo1
	_, err = pg.Pool.Exec(ctx, `INSERT INTO repo_collaborators (repo_id, user_id, permission) VALUES ($1, $2, 'read')`, r1.ID, collab.ID)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("owner lists own repos", func(t *testing.T) {
		repos, err := repoSvc.ListForUser(ctx, owner.ID)
		if err != nil {
			t.Fatalf("ListForUser: %v", err)
		}
		if len(repos) != 2 {
			t.Fatalf("expected 2 repos, got %d", len(repos))
		}
		names := map[string]bool{}
		for _, r := range repos {
			names[r.Name] = true
		}
		if !names["repo1"] || !names["repo2"] {
			t.Errorf("repos = %v, want [repo1 repo2]", repos)
		}
	})

	t.Run("collaborator sees shared repo", func(t *testing.T) {
		repos, err := repoSvc.ListForUser(ctx, collab.ID)
		if err != nil {
			t.Fatalf("ListForUser: %v", err)
		}
		if len(repos) != 1 {
			t.Fatalf("expected 1 repo, got %d", len(repos))
		}
		if repos[0].Name != "repo1" {
			t.Errorf("repo = %q, want %q", repos[0].Name, "repo1")
		}
	})

	t.Run("user with no repos returns empty list", func(t *testing.T) {
		noRepos, _ := authSvc.Register(ctx, "norepos", "norepos@test.local", "pass")
		repos, err := repoSvc.ListForUser(ctx, noRepos.ID)
		if err != nil {
			t.Fatalf("ListForUser: %v", err)
		}
		if len(repos) != 0 {
			t.Errorf("expected 0 repos, got %d", len(repos))
		}
	})
}

func TestRepoListForOrg(t *testing.T) {
	pg := testutil.NewPostgres(t)
	defer pg.Cleanup()
	ctx := context.Background()
	authSvc := auth.NewService(pg.Pool, "secret")
	repoSvc := NewService(pg.Pool)

	owner, _ := authSvc.Register(ctx, "orgowner", "org@test.local", "pass")

	// Create an org
	var orgID uuid.UUID
	err := pg.Pool.QueryRow(ctx, `INSERT INTO orgs (name, display_name) VALUES ('myorg', 'My Org') RETURNING id`).Scan(&orgID)
	if err != nil {
		t.Fatal(err)
	}
	_, err = pg.Pool.Exec(ctx, `INSERT INTO org_members (org_id, user_id, role) VALUES ($1, $2, 'admin')`, orgID, owner.ID)
	if err != nil {
		t.Fatal(err)
	}

	// Create repos owned by the org
	_, err = repoSvc.Create(ctx, "org", orgID, "myorg", "org-repo1", "", false)
	if err != nil {
		t.Fatal(err)
	}
	_, err = repoSvc.Create(ctx, "org", orgID, "myorg", "org-repo2", "", false)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("list org repos", func(t *testing.T) {
		repos, err := repoSvc.ListForOrg(ctx, orgID)
		if err != nil {
			t.Fatalf("ListForOrg: %v", err)
		}
		if len(repos) != 2 {
			t.Fatalf("expected 2 repos, got %d", len(repos))
		}
	})

	t.Run("org with no repos returns empty", func(t *testing.T) {
		emptyOrgID := uuid.New()
		_, err := pg.Pool.Exec(ctx, `INSERT INTO orgs (id, name) VALUES ($1, 'emptyorg')`, emptyOrgID)
		if err != nil {
			t.Fatal(err)
		}
		repos, err := repoSvc.ListForOrg(ctx, emptyOrgID)
		if err != nil {
			t.Fatalf("ListForOrg: %v", err)
		}
		if len(repos) != 0 {
			t.Errorf("expected 0 repos, got %d", len(repos))
		}
	})
}

func TestRepoFork(t *testing.T) {
	pg := testutil.NewPostgres(t)
	defer pg.Cleanup()
	ctx := context.Background()
	authSvc := auth.NewService(pg.Pool, "secret")
	repoSvc := NewService(pg.Pool)

	owner, _ := authSvc.Register(ctx, "forkowner", "fork@test.local", "pass")
	forker, _ := authSvc.Register(ctx, "forker", "forker@test.local", "pass")

	r, err := repoSvc.Create(ctx, "user", owner.ID, owner.Username, "source", "original", false)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("fork creates repo with -fork suffix", func(t *testing.T) {
		fork, err := repoSvc.Fork(ctx, r, forker.ID, forker.Username)
		if err != nil {
			t.Fatalf("Fork: %v", err)
		}
		if fork.Name != "source-fork" {
			t.Errorf("fork name = %q, want %q", fork.Name, "source-fork")
		}
		if fork.IsFork != true {
			t.Error("expected fork to have IsFork=true")
		}
		if fork.FullName != "forker/source-fork" {
			t.Errorf("FullName = %q, want %q", fork.FullName, "forker/source-fork")
		}
	})

	t.Run("fork of private repo is private", func(t *testing.T) {
		privateRepo, err := repoSvc.Create(ctx, "user", owner.ID, owner.Username, "private-source", "private", true)
		if err != nil {
			t.Fatal(err)
		}
		fork, err := repoSvc.Fork(ctx, privateRepo, forker.ID, forker.Username)
		if err != nil {
			t.Fatalf("Fork: %v", err)
		}
		if !fork.IsPrivate {
			t.Error("expected fork of private repo to be private")
		}
	})
}

func TestRepoStarUnstar(t *testing.T) {
	pg := testutil.NewPostgres(t)
	defer pg.Cleanup()
	ctx := context.Background()
	authSvc := auth.NewService(pg.Pool, "secret")
	repoSvc := NewService(pg.Pool)

	owner, _ := authSvc.Register(ctx, "starowner", "star@test.local", "pass")
	user, _ := authSvc.Register(ctx, "staruser", "staruser@test.local", "pass")

	r, _ := repoSvc.Create(ctx, "user", owner.ID, owner.Username, "star-test", "", false)

	t.Run("star increases count", func(t *testing.T) {
		if err := repoSvc.Star(ctx, r.ID, user.ID); err != nil {
			t.Fatalf("Star: %v", err)
		}
		starred, err := repoSvc.IsStarred(ctx, r.ID, user.ID)
		if err != nil {
			t.Fatalf("IsStarred: %v", err)
		}
		if !starred {
			t.Error("expected starred")
		}
		// Re-read repo to check count
		var starCount int
		err = pg.Pool.QueryRow(ctx, `SELECT star_count FROM repos WHERE id=$1`, r.ID).Scan(&starCount)
		if err != nil {
			t.Fatal(err)
		}
		if starCount != 1 {
			t.Errorf("star_count = %d, want 1", starCount)
		}
	})

	t.Run("idempotent star does not double count", func(t *testing.T) {
		if err := repoSvc.Star(ctx, r.ID, user.ID); err != nil {
			t.Fatalf("Star: %v", err)
		}
		var starCount int
		err := pg.Pool.QueryRow(ctx, `SELECT star_count FROM repos WHERE id=$1`, r.ID).Scan(&starCount)
		if err != nil {
			t.Fatal(err)
		}
		if starCount != 1 {
			t.Errorf("star_count = %d, want 1 (idempotent)", starCount)
		}
	})

	t.Run("unstar decreases count", func(t *testing.T) {
		if err := repoSvc.Unstar(ctx, r.ID, user.ID); err != nil {
			t.Fatalf("Unstar: %v", err)
		}
		starred, _ := repoSvc.IsStarred(ctx, r.ID, user.ID)
		if starred {
			t.Error("expected unstarred")
		}
		var starCount int
		err := pg.Pool.QueryRow(ctx, `SELECT star_count FROM repos WHERE id=$1`, r.ID).Scan(&starCount)
		if err != nil {
			t.Fatal(err)
		}
		if starCount != 0 {
			t.Errorf("star_count = %d, want 0", starCount)
		}
	})

	t.Run("unstar when not starred is noop", func(t *testing.T) {
		if err := repoSvc.Unstar(ctx, r.ID, user.ID); err != nil {
			t.Fatalf("Unstar: %v", err)
		}
	})
}

func TestRepoCanAccess(t *testing.T) {
	pg := testutil.NewPostgres(t)
	defer pg.Cleanup()
	ctx := context.Background()
	authSvc := auth.NewService(pg.Pool, "secret")
	repoSvc := NewService(pg.Pool)

	owner, _ := authSvc.Register(ctx, "accessowner", "access@test.local", "pass")
	collab, _ := authSvc.Register(ctx, "accesscollab", "acollab@test.local", "pass")
	stranger, _ := authSvc.Register(ctx, "stranger", "stranger@test.local", "pass")

	// Create an org and add owner as member
	var orgID uuid.UUID
	err := pg.Pool.QueryRow(ctx, `INSERT INTO orgs (name) VALUES ('accessteam') RETURNING id`).Scan(&orgID)
	if err != nil {
		t.Fatal(err)
	}
	_, err = pg.Pool.Exec(ctx, `INSERT INTO org_members (org_id, user_id, role) VALUES ($1, $2, 'admin')`, orgID, owner.ID)
	if err != nil {
		t.Fatal(err)
	}

	publicRepo, _ := repoSvc.Create(ctx, "user", owner.ID, owner.Username, "public", "", false)
	privateRepo, _ := repoSvc.Create(ctx, "user", owner.ID, owner.Username, "private", "private", true)
	orgRepo, _ := repoSvc.Create(ctx, "org", orgID, "accessteam", "org-repo", "", true)

	// Add collaborator to private repo
	_, err = pg.Pool.Exec(ctx, `INSERT INTO repo_collaborators (repo_id, user_id, permission) VALUES ($1, $2, 'write')`, privateRepo.ID, collab.ID)
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name    string
		repoID  uuid.UUID
		userID  uuid.UUID
		minPerm string
		want    bool
	}{
		{"owner can read own public repo", publicRepo.ID, owner.ID, "read", true},
		{"owner can admin own public repo", publicRepo.ID, owner.ID, "admin", true},
		{"owner can read own private repo", privateRepo.ID, owner.ID, "read", true},
		{"stranger can read public repo", publicRepo.ID, stranger.ID, "read", true},
		{"stranger can write public repo (design choice)", publicRepo.ID, stranger.ID, "write", true},
		{"stranger can admin public repo (design choice)", publicRepo.ID, stranger.ID, "admin", true},
		{"stranger cannot read private repo", privateRepo.ID, stranger.ID, "read", false},
		{"collaborator can write private repo", privateRepo.ID, collab.ID, "write", true},
		{"collaborator cannot admin", privateRepo.ID, collab.ID, "admin", false},
		{"org member can read org repo", orgRepo.ID, owner.ID, "read", true},
		{"stranger cannot read org private repo", orgRepo.ID, stranger.ID, "read", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := repoSvc.CanAccess(ctx, tc.repoID, tc.userID, tc.minPerm)
			if err != nil {
				t.Fatalf("CanAccess: %v", err)
			}
			if got != tc.want {
				t.Errorf("CanAccess = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestRepoResolveOwnerID(t *testing.T) {
	pg := testutil.NewPostgres(t)
	defer pg.Cleanup()
	ctx := context.Background()
	authSvc := auth.NewService(pg.Pool, "secret")
	repoSvc := NewService(pg.Pool)

	u, _ := authSvc.Register(ctx, "resolveuser", "resolve@test.local", "pass")

	var orgID uuid.UUID
	err := pg.Pool.QueryRow(ctx, `INSERT INTO orgs (name) VALUES ('resolveorg') RETURNING id`).Scan(&orgID)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("resolve user by name", func(t *testing.T) {
		ownerType, id, err := repoSvc.ResolveOwnerID(ctx, "resolveuser")
		if err != nil {
			t.Fatalf("ResolveOwnerID: %v", err)
		}
		if ownerType != "user" {
			t.Errorf("ownerType = %q, want %q", ownerType, "user")
		}
		if id != u.ID {
			t.Errorf("id = %v, want %v", id, u.ID)
		}
	})

	t.Run("resolve org by name", func(t *testing.T) {
		ownerType, id, err := repoSvc.ResolveOwnerID(ctx, "resolveorg")
		if err != nil {
			t.Fatalf("ResolveOwnerID: %v", err)
		}
		if ownerType != "org" {
			t.Errorf("ownerType = %q, want %q", ownerType, "org")
		}
		if id != orgID {
			t.Errorf("id = %v, want %v", id, orgID)
		}
	})

	t.Run("non-existent owner returns error", func(t *testing.T) {
		_, _, err := repoSvc.ResolveOwnerID(ctx, "nobody")
		if err == nil {
			t.Fatal("expected error for non-existent owner")
		}
	})
}

func TestRepoUpdateBranchHead(t *testing.T) {
	pg := testutil.NewPostgres(t)
	defer pg.Cleanup()
	ctx := context.Background()
	authSvc := auth.NewService(pg.Pool, "secret")
	repoSvc := NewService(pg.Pool)

	u, _ := authSvc.Register(ctx, "branchuser", "branch@test.local", "pass")
	r, _ := repoSvc.Create(ctx, "user", u.ID, u.Username, "branch-test", "", false)

	t.Run("update existing branch head", func(t *testing.T) {
		err := repoSvc.UpdateBranchHead(ctx, r.ID, "main", "abc123def456")
		if err != nil {
			t.Fatalf("UpdateBranchHead: %v", err)
		}
		var sha string
		err = pg.Pool.QueryRow(ctx, `SELECT head_sha FROM branches WHERE repo_id=$1 AND name='main'`, r.ID).Scan(&sha)
		if err != nil {
			t.Fatal(err)
		}
		if sha != "abc123def456" {
			t.Errorf("head_sha = %q, want %q", sha, "abc123def456")
		}
	})

	t.Run("create new branch", func(t *testing.T) {
		err := repoSvc.UpdateBranchHead(ctx, r.ID, "develop", "fedcba654321")
		if err != nil {
			t.Fatalf("UpdateBranchHead: %v", err)
		}
		var sha string
		err = pg.Pool.QueryRow(ctx, `SELECT head_sha FROM branches WHERE repo_id=$1 AND name='develop'`, r.ID).Scan(&sha)
		if err != nil {
			t.Fatal(err)
		}
		if sha != "fedcba654321" {
			t.Errorf("head_sha = %q, want %q", sha, "fedcba654321")
		}
	})

	t.Run("update again overwrites", func(t *testing.T) {
		err := repoSvc.UpdateBranchHead(ctx, r.ID, "main", "updatedsha")
		if err != nil {
			t.Fatalf("UpdateBranchHead: %v", err)
		}
		var sha string
		err = pg.Pool.QueryRow(ctx, `SELECT head_sha FROM branches WHERE repo_id=$1 AND name='main'`, r.ID).Scan(&sha)
		if err != nil {
			t.Fatal(err)
		}
		if sha != "updatedsha" {
			t.Errorf("head_sha = %q, want %q", sha, "updatedsha")
		}
	})
}
