//go:build e2e

package e2e

import (
	"context"
	"net/http"
	"testing"

	"github.com/rusik69/govnohub/internal/testutil"
	"github.com/rusik69/govnohub/tests/testenv"
)

func TestForkFlow_BasicFork(t *testing.T) {
	env := testenv.New(t)
	defer env.Cleanup()
	base := env.URL
	suffix := uniqueSuffix()
	short := suffix[len(suffix)-6:]
	owner := "fko" + short
	forker := "fkf" + short

	ownerToken := testutil.RegisterAndLogin(t, base, owner)
	forkerToken := testutil.RegisterAndLogin(t, base, forker)
	createRepo(t, base, ownerToken, owner, "app")
	repo := owner + "/app"

	// ---- Fork the repo ----
	t.Run("fork repo", func(t *testing.T) {
		resp, out := testutil.DoJSON(t, http.MethodPost, base+"/api/v1/repos/"+repo+"/fork", forkerToken, nil)
		requireStatus(t, resp, http.StatusOK, "fork repo")
		if isFork, ok := out["is_fork"].(bool); !ok || !isFork {
			t.Fatalf("expected is_fork=true, got %v (type %T)", out["is_fork"], out["is_fork"])
		}
		if out["name"] != "app-fork" {
			t.Fatalf("expected name 'app-fork', got %q", out["name"])
		}
		expectedFull := forker + "/app-fork"
		if out["full_name"] != expectedFull {
			t.Fatalf("expected full_name %q, got %q", expectedFull, out["full_name"])
		}
		if out["owner_name"] != forker {
			t.Fatalf("expected owner_name %q, got %q", forker, out["owner_name"])
		}
	})

	// ---- Original repo still accessible ----
	t.Run("original repo accessible", func(t *testing.T) {
		resp, out := testutil.DoJSON(t, http.MethodGet, base+"/api/v1/repos/"+repo, ownerToken, nil)
		requireStatus(t, resp, http.StatusOK, "get original repo")
		if out["name"] != "app" {
			t.Fatalf("expected name 'app', got %q", out["name"])
		}
	})

	// ---- Fork owner can access fork ----
	t.Run("fork owner can access fork", func(t *testing.T) {
		resp, out := testutil.DoJSON(t, http.MethodGet, base+"/api/v1/repos/"+forker+"/app-fork", forkerToken, nil)
		requireStatus(t, resp, http.StatusOK, "get fork")
		if out["name"] != "app-fork" {
			t.Fatalf("expected name 'app-fork', got %q", out["name"])
		}
		if out["full_name"] != forker+"/app-fork" {
			t.Fatalf("expected full_name %q, got %q", forker+"/app-fork", out["full_name"])
		}
	})

	// ---- Original owner can access fork (public repo) ----
	t.Run("original owner can access fork", func(t *testing.T) {
		resp, out := testutil.DoJSON(t, http.MethodGet, base+"/api/v1/repos/"+forker+"/app-fork", ownerToken, nil)
		requireStatus(t, resp, http.StatusOK, "get fork as original owner")
		if out["name"] != "app-fork" {
			t.Fatalf("expected name 'app-fork', got %q", out["name"])
		}
	})

	// ---- Third party can access public fork ----
	t.Run("third party can access public fork", func(t *testing.T) {
		viewer := "fkv" + short
		viewerToken := testutil.RegisterAndLogin(t, base, viewer)
		resp, out := testutil.DoJSON(t, http.MethodGet, base+"/api/v1/repos/"+forker+"/app-fork", viewerToken, nil)
		requireStatus(t, resp, http.StatusOK, "third party get fork")
		if out["name"] != "app-fork" {
			t.Fatalf("expected name 'app-fork', got %q", out["name"])
		}
	})
}

func TestForkFlow_MultipleForks(t *testing.T) {
	// Multiple users can fork the same repo
	env := testenv.New(t)
	defer env.Cleanup()
	base := env.URL
	suffix := uniqueSuffix()
	short := suffix[len(suffix)-6:]
	owner := "fkm" + short
	forker1 := "fk1" + short
	forker2 := "fk2" + short

	ownerToken := testutil.RegisterAndLogin(t, base, owner)
	forker1Token := testutil.RegisterAndLogin(t, base, forker1)
	forker2Token := testutil.RegisterAndLogin(t, base, forker2)
	createRepo(t, base, ownerToken, owner, "shared")
	repo := owner + "/shared"

	t.Run("first user fork", func(t *testing.T) {
		resp, out := testutil.DoJSON(t, http.MethodPost, base+"/api/v1/repos/"+repo+"/fork", forker1Token, nil)
		requireStatus(t, resp, http.StatusOK, "first fork")
		if out["full_name"] != forker1+"/shared-fork" {
			t.Fatalf("expected %q, got %q", forker1+"/shared-fork", out["full_name"])
		}
		if isFork, _ := out["is_fork"].(bool); !isFork {
			t.Fatalf("expected is_fork=true")
		}
	})

	t.Run("second user fork", func(t *testing.T) {
		resp, out := testutil.DoJSON(t, http.MethodPost, base+"/api/v1/repos/"+repo+"/fork", forker2Token, nil)
		requireStatus(t, resp, http.StatusOK, "second fork")
		if out["full_name"] != forker2+"/shared-fork" {
			t.Fatalf("expected %q, got %q", forker2+"/shared-fork", out["full_name"])
		}
		if isFork, _ := out["is_fork"].(bool); !isFork {
			t.Fatalf("expected is_fork=true")
		}
	})

	t.Run("each fork has unique content", func(t *testing.T) {
		resp, out1 := testutil.DoJSON(t, http.MethodGet, base+"/api/v1/repos/"+forker1+"/shared-fork", forker1Token, nil)
		requireStatus(t, resp, http.StatusOK, "get fork1")
		resp, out2 := testutil.DoJSON(t, http.MethodGet, base+"/api/v1/repos/"+forker2+"/shared-fork", forker2Token, nil)
		requireStatus(t, resp, http.StatusOK, "get fork2")
		if out1["id"] == out2["id"] {
			t.Fatal("expected different repo IDs for different forks")
		}
	})
}

func TestForkFlow_ForkOwnRepo(t *testing.T) {
	// User can fork their own repo
	env := testenv.New(t)
	defer env.Cleanup()
	base := env.URL
	suffix := uniqueSuffix()
	short := suffix[len(suffix)-6:]
	owner := "fks" + short

	token := testutil.RegisterAndLogin(t, base, owner)
	createRepo(t, base, token, owner, "myrepo")
	repo := owner + "/myrepo"

	t.Run("fork own repo", func(t *testing.T) {
		resp, out := testutil.DoJSON(t, http.MethodPost, base+"/api/v1/repos/"+repo+"/fork", token, nil)
		requireStatus(t, resp, http.StatusOK, "fork own repo")
		if out["name"] != "myrepo-fork" {
			t.Fatalf("expected name 'myrepo-fork', got %q", out["name"])
		}
		if out["full_name"] != owner+"/myrepo-fork" {
			t.Fatalf("expected full_name %q, got %q", owner+"/myrepo-fork", out["full_name"])
		}
		if isFork, _ := out["is_fork"].(bool); !isFork {
			t.Fatalf("expected is_fork=true")
		}
	})

	t.Run("forked repo listed in user repos", func(t *testing.T) {
		resp, repos := doJSONArray(t, http.MethodGet, base+"/api/v1/user/repos", token, nil)
		requireStatus(t, resp, http.StatusOK, "list user repos")
		found := false
		for _, r := range repos {
			if name, _ := r["name"].(string); name == "myrepo-fork" {
				found = true
				if isFork, _ := r["is_fork"].(bool); !isFork {
					t.Fatalf("expected is_fork=true in user repos list")
				}
				break
			}
		}
		if !found {
			t.Fatal("forked repo not found in user repos list")
		}
	})
}

func TestForkFlow_GitCommitOnFork(t *testing.T) {
	// Test that we can make git commits on a fork and create a PR
	env := testenv.New(t)
	defer env.Cleanup()
	base := env.URL
	suffix := uniqueSuffix()
	short := suffix[len(suffix)-6:]
	owner := "fkg" + short
	forker := "fkgo" + short

	ownerToken := testutil.RegisterAndLogin(t, base, owner)
	forkerToken := testutil.RegisterAndLogin(t, base, forker)
	createRepo(t, base, ownerToken, owner, "gitrepo")
	repo := owner + "/gitrepo"

	// Fork the repo
	resp, forkOut := testutil.DoJSON(t, http.MethodPost, base+"/api/v1/repos/"+repo+"/fork", forkerToken, nil)
	requireStatus(t, resp, http.StatusOK, "fork repo for git test")
	forkRepo := forker + "/gitrepo-fork"

	// The fork endpoint creates the DB record but does NOT initialize the git store.
	// We need to init and seed the fork's git store manually.
	ctx := context.Background()
	if err := env.Git.Init(ctx, forker, "gitrepo-fork"); err != nil {
		t.Fatalf("init fork git store: %v", err)
	}
	seedSHA, err := env.Git.SeedMainBranch(forker, "gitrepo-fork", "main")
	if err != nil {
		t.Fatalf("seed fork main branch: %v", err)
	}

	// Verify the fork repo exists in the git store
	t.Run("fork git store initialized", func(t *testing.T) {
		if !env.Git.Exists(forker, "gitrepo-fork") {
			t.Fatal("expected fork git store to exist after init")
		}
	})

	// Create a branch on the fork with a commit
	t.Run("commit to feature branch on fork", func(t *testing.T) {
		// Create a branch from main
		if err := env.Git.CreateBranch(forker, "gitrepo-fork", "feature", "main"); err != nil {
			t.Fatalf("create feature branch: %v", err)
		}

		// Make a commit on the feature branch
		gitCommitOnBranch(t, env, forker, "gitrepo-fork", "feature", "feature.txt", "fork change content", "feat: add feature.txt to fork")
	})

	// Create a PR within the fork repo (same-repo, since cross-repo PR is not supported yet)
	t.Run("create PR in fork repo", func(t *testing.T) {
		resp, prOut := testutil.DoJSON(t, http.MethodPost, base+"/api/v1/repos/"+forkRepo+"/pulls", forkerToken, map[string]string{
			"title": "feat: add feature.txt",
			"body":  "PR from feature branch in fork",
			"head":  "feature",
			"base":  "main",
		})
		requireStatus(t, resp, http.StatusOK, "create PR in fork")
		if prOut["title"] != "feat: add feature.txt" {
			t.Fatalf("expected PR title %q, got %q", "feat: add feature.txt", prOut["title"])
		}
		prNum, ok := prOut["number"].(float64)
		if !ok || prNum < 1 {
			t.Fatalf("expected valid PR number, got %v", prOut["number"])
		}

		// Verify PR can be retrieved
		resp, getOut := testutil.DoJSON(t, http.MethodGet, base+"/api/v1/repos/"+forkRepo+"/pulls/"+itoa(int(prNum)), forkerToken, nil)
		requireStatus(t, resp, http.StatusOK, "get PR in fork")
		if getOut["head_branch"] != "feature" {
			t.Fatalf("expected head_branch 'feature', got %q", getOut["head_branch"])
		}
		if getOut["base_branch"] != "main" {
			t.Fatalf("expected base_branch 'main', got %q", getOut["base_branch"])
		}
	})
}
